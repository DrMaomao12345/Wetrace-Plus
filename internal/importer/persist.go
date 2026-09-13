package importer

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const defaultSelfID = "wetrace:self"

// A colon is safe in stored IDs and preserves chatlog-keeper's account-aware
// thread_id (account_id::conversation_id) without lossy rewrites or collisions.
var nonIDCharacters = regexp.MustCompile(`[^a-zA-Z0-9_@:.-]+`)

type writer struct {
	db             *sql.DB
	tx             *sql.Tx
	options        Options
	imported       int
	duplicates     int
	timestampOrder map[int64]int64
	talkers        map[string]struct{}
	committed      bool
}

func newWriter(dataDir string, options Options) (*writer, error) {
	messageDir := filepath.Join(dataDir, "message")
	contactDir := filepath.Join(dataDir, "contact")
	sessionDir := filepath.Join(dataDir, "session")
	for _, dir := range []string{messageDir, contactDir, sessionDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("创建导入数据目录失败: %w", err)
		}
	}

	messagePath := filepath.Join(messageDir, "message_0.db")
	contactPath := filepath.Join(contactDir, "contact.db")
	sessionPath := filepath.Join(sessionDir, "session.db")
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_busy_timeout=10000", messagePath))
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("ATTACH DATABASE ? AS contacts", contactPath); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接联系人数据失败: %w", err)
	}
	if _, err := db.Exec("ATTACH DATABASE ? AS sessions", sessionPath); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接会话数据失败: %w", err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	w := &writer{
		db:             db,
		tx:             tx,
		options:        options,
		timestampOrder: map[int64]int64{},
		talkers:        map[string]struct{}{},
	}
	if err := w.createSchema(); err != nil {
		_ = tx.Rollback()
		_ = db.Close()
		return nil, err
	}
	return w, nil
}

func (w *writer) createSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS main.Timestamp (timestamp INTEGER NOT NULL)`,
		`INSERT INTO main.Timestamp(timestamp) SELECT 0 WHERE NOT EXISTS (SELECT 1 FROM main.Timestamp)`,
		`CREATE TABLE IF NOT EXISTS main.Name2Id (user_name TEXT NOT NULL UNIQUE)`,
		`CREATE TABLE IF NOT EXISTS main.wetrace_import_sources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_name TEXT NOT NULL,
			source_hash TEXT NOT NULL,
			formats TEXT NOT NULL,
			messages INTEGER NOT NULL,
			imported INTEGER NOT NULL,
			duplicates INTEGER NOT NULL,
			warning_count INTEGER NOT NULL,
			imported_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS contacts.contact (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			local_type INTEGER NOT NULL DEFAULT 1,
			alias TEXT NOT NULL DEFAULT '',
			remark TEXT NOT NULL DEFAULT '',
			nick_name TEXT NOT NULL DEFAULT '',
			small_head_url TEXT NOT NULL DEFAULT '',
			big_head_url TEXT NOT NULL DEFAULT '',
			verify_flag INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS contacts.chat_room (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			owner TEXT NOT NULL DEFAULT '',
			ext_buffer BLOB
		)`,
		`CREATE TABLE IF NOT EXISTS contacts.chatroom_member (
			room_id INTEGER NOT NULL,
			member_id INTEGER NOT NULL,
			UNIQUE(room_id, member_id)
		)`,
		`CREATE TABLE IF NOT EXISTS contacts.biz_info (
			username TEXT PRIMARY KEY,
			type INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS sessions.SessionTable (
			username TEXT PRIMARY KEY,
			type INTEGER NOT NULL DEFAULT 0,
			unread_count INTEGER NOT NULL DEFAULT 0,
			unread_first_msg_srv_id INTEGER NOT NULL DEFAULT 0,
			is_hidden INTEGER NOT NULL DEFAULT 0,
			summary TEXT NOT NULL DEFAULT '',
			draft TEXT NOT NULL DEFAULT '',
			status INTEGER NOT NULL DEFAULT 0,
			last_timestamp INTEGER NOT NULL DEFAULT 0,
			sort_timestamp INTEGER NOT NULL DEFAULT 0,
			last_clear_unread_timestamp INTEGER NOT NULL DEFAULT 0,
			last_msg_locald_id INTEGER NOT NULL DEFAULT 0,
			last_msg_type INTEGER NOT NULL DEFAULT 0,
			last_msg_sub_type INTEGER NOT NULL DEFAULT 0,
			last_msg_sender TEXT NOT NULL DEFAULT '',
			last_sender_display_name TEXT NOT NULL DEFAULT '',
			last_msg_ext_type INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, statement := range statements {
		if _, err := w.tx.Exec(statement); err != nil {
			return fmt.Errorf("初始化分析数据结构失败: %w", err)
		}
	}
	return nil
}

func (w *writer) Add(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if message.Time.IsZero() {
		return fmt.Errorf("%s 第 %d 行缺少有效时间", message.SourceFile, message.SourceRow)
	}
	message.TalkerName = strings.TrimSpace(message.TalkerName)
	message.TalkerID = normalizeID(message.TalkerID)
	if message.TalkerID == "" {
		message.TalkerID = importedID("talker", message.TalkerName)
	}
	if message.IsChatRoom && !strings.HasSuffix(message.TalkerID, "@chatroom") {
		message.TalkerID += "@chatroom"
	}
	if message.TalkerName == "" {
		message.TalkerName = message.TalkerID
	}
	message.SenderID = normalizeID(message.SenderID)
	message.SenderName = strings.TrimSpace(message.SenderName)
	if message.IsSelf {
		if message.SenderID == "" {
			message.SenderID = normalizeID(w.options.SelfID)
		}
		if message.SenderID == "" {
			message.SenderID = defaultSelfID
		}
		if message.SenderName == "" {
			message.SenderName = w.options.SelfName
		}
	} else if message.SenderID == "" {
		if !message.IsChatRoom {
			message.SenderID = message.TalkerID
		} else {
			message.SenderID = importedID("member", message.SenderName)
		}
	}
	if message.SenderName == "" {
		message.SenderName = message.SenderID
	}
	if message.Type <= 0 {
		message.Type = 1
	}

	if err := w.upsertContact(ctx, message.TalkerID, message.TalkerName, contactType(message.TalkerID, message.IsChatRoom)); err != nil {
		return err
	}
	if err := w.upsertContact(ctx, message.SenderID, message.SenderName, senderContactType(message)); err != nil {
		return err
	}
	if message.IsChatRoom {
		if err := w.upsertMembership(ctx, message.TalkerID, message.SenderID); err != nil {
			return err
		}
	}

	senderRowID, err := w.senderRowID(ctx, message.SenderID)
	if err != nil {
		return err
	}
	tableName := messageTable(message.TalkerID)
	if err := w.ensureMessageTable(ctx, tableName); err != nil {
		return err
	}
	seq := message.Seq
	if seq <= 0 || seq < 1_000_000_000_000 {
		base := message.Time.Unix() * 1000
		w.timestampOrder[base]++
		seq = base + w.timestampOrder[base]
	}
	serverID := serverID(message)
	status := 4
	content := message.Content
	if message.IsSelf {
		status = 2
	} else if message.IsChatRoom && message.SenderID != "" {
		content = message.SenderID + ":\n" + content
	}
	localType := message.Type & 0xffffffff
	if message.SubType > 0 {
		localType |= message.SubType << 32
	}
	fingerprint := messageFingerprint(message)
	query := fmt.Sprintf(`INSERT OR IGNORE INTO main.%s
		(server_id, local_type, sort_seq, real_sender_id, create_time, status, message_content, compress_content, packed_info_data, import_fingerprint, source_file, source_message_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?, ?, ?)`, tableName)
	result, err := w.tx.ExecContext(ctx, query, serverID, localType, seq, senderRowID, message.Time.Unix(), status, content, fingerprint, message.SourceFile, message.ExternalID)
	if err != nil {
		return fmt.Errorf("写入消息失败: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		w.duplicates++
	} else {
		w.imported++
	}

	if err := w.upsertSession(ctx, message, seq); err != nil {
		return err
	}
	if _, ok := w.talkers[message.TalkerID]; !ok {
		w.talkers[message.TalkerID] = struct{}{}
	}
	_, err = w.tx.ExecContext(ctx, `UPDATE main.Timestamp SET timestamp = CASE WHEN timestamp = 0 OR timestamp > ? THEN ? ELSE timestamp END`, message.Time.Unix(), message.Time.Unix())
	return err
}

func (w *writer) ensureMessageTable(ctx context.Context, tableName string) error {
	statement := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS main.%s (
		local_id INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id INTEGER NOT NULL,
		local_type INTEGER NOT NULL,
		sort_seq INTEGER NOT NULL,
		real_sender_id INTEGER NOT NULL DEFAULT 0,
		create_time INTEGER NOT NULL,
		status INTEGER NOT NULL DEFAULT 4,
		upload_status INTEGER NOT NULL DEFAULT 0,
		download_status INTEGER NOT NULL DEFAULT 0,
		server_seq INTEGER NOT NULL DEFAULT 0,
		origin_source INTEGER NOT NULL DEFAULT 0,
		source TEXT NOT NULL DEFAULT '',
		message_content BLOB,
		compress_content BLOB,
		packed_info_data BLOB,
		import_fingerprint TEXT NOT NULL UNIQUE,
		source_file TEXT NOT NULL DEFAULT '',
		source_message_id TEXT NOT NULL DEFAULT ''
	)`, tableName)
	if _, err := w.tx.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("创建会话消息表失败: %w", err)
	}
	_, _ = w.tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS main.idx_%s_time ON %s(create_time)`, tableName, tableName))
	_, _ = w.tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS main.idx_%s_seq ON %s(sort_seq)`, tableName, tableName))
	return nil
}

func (w *writer) senderRowID(ctx context.Context, sender string) (int64, error) {
	if sender == "" {
		return 0, nil
	}
	if _, err := w.tx.ExecContext(ctx, `INSERT OR IGNORE INTO main.Name2Id(user_name) VALUES (?)`, sender); err != nil {
		return 0, err
	}
	var rowID int64
	if err := w.tx.QueryRowContext(ctx, `SELECT rowid FROM main.Name2Id WHERE user_name = ?`, sender).Scan(&rowID); err != nil {
		return 0, err
	}
	return rowID, nil
}

func (w *writer) upsertContact(ctx context.Context, username, displayName string, localType int) error {
	if username == "" {
		return nil
	}
	_, err := w.tx.ExecContext(ctx, `INSERT INTO contacts.contact(username, local_type, remark, nick_name)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET
			local_type = CASE WHEN excluded.local_type = 2 THEN 2 ELSE contact.local_type END,
			remark = CASE WHEN excluded.remark != '' THEN excluded.remark ELSE contact.remark END,
			nick_name = CASE WHEN excluded.nick_name != '' THEN excluded.nick_name ELSE contact.nick_name END`,
		username, localType, displayName, displayName)
	return err
}

func (w *writer) upsertMembership(ctx context.Context, room, member string) error {
	if room == "" || member == "" {
		return nil
	}
	if _, err := w.tx.ExecContext(ctx, `INSERT OR IGNORE INTO contacts.chat_room(username) VALUES (?)`, room); err != nil {
		return err
	}
	_, err := w.tx.ExecContext(ctx, `INSERT OR IGNORE INTO contacts.chatroom_member(room_id, member_id)
		SELECT room.id, member.id FROM contacts.chat_room room, contacts.contact member
		WHERE room.username = ? AND member.username = ?`, room, member)
	return err
}

func (w *writer) upsertSession(ctx context.Context, message Message, seq int64) error {
	_, err := w.tx.ExecContext(ctx, `INSERT INTO sessions.SessionTable
		(username, type, summary, last_timestamp, sort_timestamp, last_msg_locald_id, last_msg_type, last_msg_sub_type, last_msg_sender, last_sender_display_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET
			summary = CASE WHEN excluded.last_timestamp >= SessionTable.last_timestamp THEN excluded.summary ELSE SessionTable.summary END,
			last_timestamp = MAX(SessionTable.last_timestamp, excluded.last_timestamp),
			sort_timestamp = MAX(SessionTable.sort_timestamp, excluded.sort_timestamp),
			last_msg_locald_id = CASE WHEN excluded.last_timestamp >= SessionTable.last_timestamp THEN excluded.last_msg_locald_id ELSE SessionTable.last_msg_locald_id END,
			last_msg_type = CASE WHEN excluded.last_timestamp >= SessionTable.last_timestamp THEN excluded.last_msg_type ELSE SessionTable.last_msg_type END,
			last_msg_sub_type = CASE WHEN excluded.last_timestamp >= SessionTable.last_timestamp THEN excluded.last_msg_sub_type ELSE SessionTable.last_msg_sub_type END,
			last_msg_sender = CASE WHEN excluded.last_timestamp >= SessionTable.last_timestamp THEN excluded.last_msg_sender ELSE SessionTable.last_msg_sender END,
			last_sender_display_name = CASE WHEN excluded.last_timestamp >= SessionTable.last_timestamp THEN excluded.last_sender_display_name ELSE SessionTable.last_sender_display_name END`,
		message.TalkerID, boolInt(message.IsChatRoom), sessionSummary(message), message.Time.Unix(), message.Time.Unix(), seq, message.Type, message.SubType, message.SenderID, message.SenderName)
	return err
}

// sessionSummary 生成会话列表里那行预览。
// 非文本消息的 content 往往是 XML 或附件描述，原样截断会在列表里露出一串标签，
// 所以按类型换成微信自己那套占位文案。
func sessionSummary(message Message) string {
	switch message.Type {
	case 3:
		return "[图片]"
	case 34:
		return "[语音]"
	case 42:
		return "[名片]"
	case 43:
		return "[视频]"
	case 47:
		return "[动画表情]"
	case 48:
		return "[位置]"
	case 49:
		return "[链接]"
	case 50:
		return "[通话]"
	}
	if strings.HasPrefix(strings.TrimSpace(message.Content), "<") {
		return "[消息]"
	}
	return truncateRunes(message.Content, 120)
}

func (w *writer) AddHistory(ctx context.Context, item HistoryItem) error {
	_, err := w.tx.ExecContext(ctx, `INSERT INTO main.wetrace_import_sources
		(source_name, source_hash, formats, messages, imported, duplicates, warning_count, imported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.SourceName, item.SourceHash, item.Formats, item.Messages, item.Imported, item.Duplicates, item.WarningCount, item.ImportedAt.Unix())
	return err
}

func (w *writer) Commit() error {
	if w.committed {
		return nil
	}
	if err := w.tx.Commit(); err != nil {
		return fmt.Errorf("提交导入事务失败: %w", err)
	}
	w.committed = true
	return nil
}

func (w *writer) Close() {
	if !w.committed && w.tx != nil {
		_ = w.tx.Rollback()
	}
	if w.db != nil {
		_ = w.db.Close()
	}
}

func History(dataDir string, limit int) ([]HistoryItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	path := filepath.Join(dataDir, "message", "message_0.db")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return []HistoryItem{}, nil
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000", path))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='wetrace_import_sources'`).Scan(&exists); err != nil || exists == 0 {
		return []HistoryItem{}, nil
	}
	rows, err := db.Query(`SELECT id, source_name, source_hash, formats, messages, imported, duplicates, warning_count, imported_at
		FROM wetrace_import_sources ORDER BY imported_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]HistoryItem, 0)
	for rows.Next() {
		var item HistoryItem
		var unix int64
		if err := rows.Scan(&item.ID, &item.SourceName, &item.SourceHash, &item.Formats, &item.Messages, &item.Imported, &item.Duplicates, &item.WarningCount, &unix); err != nil {
			return nil, err
		}
		item.ImportedAt = time.Unix(unix, 0).UTC()
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return nonIDCharacters.ReplaceAllString(value, "_")
}

func importedID(kind, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "unknown"
	}
	sum := sha256.Sum256([]byte(kind + "\x00" + value))
	return "import:" + hex.EncodeToString(sum[:8])
}

func messageTable(talker string) string {
	sum := md5.Sum([]byte(talker))
	return "Msg_" + hex.EncodeToString(sum[:])
}

func serverID(message Message) int64 {
	if value, err := strconvParsePositive(message.ExternalID); err == nil {
		return value
	}
	fingerprint := messageFingerprint(message)
	var value int64
	for _, b := range []byte(fingerprint[:15]) {
		value = value*31 + int64(b)
	}
	if value < 0 {
		value = -value
	}
	return value
}

func strconvParsePositive(value string) (int64, error) {
	var parsed int64
	if _, err := fmt.Sscan(strings.TrimSpace(value), &parsed); err != nil || parsed <= 0 {
		return 0, errors.New("not a positive integer")
	}
	return parsed, nil
}

func messageFingerprint(message Message) string {
	payload := strings.Join([]string{
		message.TalkerID,
		fmt.Sprint(message.Time.UnixNano()),
		message.SenderID,
		fmt.Sprint(message.IsSelf),
		fmt.Sprint(message.Type),
		fmt.Sprint(message.SubType),
		message.Content,
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func contactType(username string, isGroup bool) int {
	if isGroup || strings.HasSuffix(username, "@chatroom") {
		return 2
	}
	if strings.HasSuffix(username, "@openim") {
		return 5
	}
	if username == defaultSelfID {
		return 1
	}
	return 1
}

func senderContactType(message Message) int {
	if message.IsSelf {
		return 1
	}
	if message.IsChatRoom && message.SenderID != message.TalkerID {
		return 3
	}
	return contactType(message.SenderID, false)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}
