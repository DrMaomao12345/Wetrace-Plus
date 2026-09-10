package repo

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/store/bind"
	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
)

// GetAnnualWordCounts 计算指定年份内每个会话发送/接收的字数（消息内容字符长度）
// 与 GetAnnualReport 共用一致的 buildSegments 逻辑，确保时区段、排除列表行为对齐。
func (r *Repository) GetAnnualWordCounts(
	ctx context.Context,
	year int,
	defaultTzOffset int,
	userSegs []types.TZSegment,
	excludeTalkers []string,
) (*model.WordCountStat, error) {
	segs := buildSegments(year, defaultTzOffset, userSegs)
	excludeSet := make(map[string]bool, len(excludeTalkers))
	for _, t := range excludeTalkers {
		excludeSet[t] = true
	}

	// 聚合：talker → {字数, 条数}
	type stats struct {
		sent      int
		recv      int
		sentCount int
		recvCount int
		// 语音转写贡献的字数，单独记一份，既并入总数也能单列展示
		voiceSent int
		voiceRecv int
	}
	agg := make(map[string]*stats)

	yearStart := segs[0].start
	yearEnd := segs[len(segs)-1].end

	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 5000})
	if err != nil {
		return nil, err
	}
	tl := r.transcriptLookup() // 没有任何转写结果时为 nil，整段跳过

	allowTalker := r.TalkerFilter(ctx, model.ModuleReport)
	for _, session := range sessions {
		talker := session.UserName
		if excludeSet[talker] || !allowTalker(talker) {
			continue
		}
		targets := r.router.Resolve(yearStart, yearEnd, talker)
		s := &stats{}
		for _, target := range targets {
			db, err := r.pool.GetConnection(target.FilePath)
			if err != nil {
				continue
			}
			hash := md5.Sum([]byte(talker))
			tableName := "Msg_" + hex.EncodeToString(hash[:])
			isGroup := strings.HasSuffix(talker, "@chatroom")

			if r.isTableExist(db, tableName) {
				wc := r.queryWordCountV4(ctx, db, tableName, talker, isGroup, yearStart.Unix(), yearEnd.Unix())
				s.sent += wc.sent
				s.recv += wc.recv
				s.sentCount += wc.sentCount
				s.recvCount += wc.recvCount

				// 语音转写出来的文字也算「说了多少字」。
				// 微信自带的转写随消息存着，永远要查；Whisper 缓存则可能为空。
				{
					vs, vr := r.queryVoiceTranscriptChars(ctx, db, tableName, talker, isGroup,
						yearStart.Unix(), yearEnd.Unix(), tl)
					s.sent += vs
					s.recv += vr
					s.voiceSent += vs
					s.voiceRecv += vr
				}
			} else {
				wc := r.queryWordCountV3(ctx, db, target, yearStart.Unix()*1000, yearEnd.Unix()*1000)
				s.sent += wc.sent
				s.recv += wc.recv
				s.sentCount += wc.sentCount
				s.recvCount += wc.recvCount
			}
		}
		if s.sent > 0 || s.recv > 0 || s.sentCount > 0 || s.recvCount > 0 {
			agg[talker] = s
		}
	}

	result := &model.WordCountStat{}
	for talker, s := range agg {
		result.SentChars += s.sent
		result.RecvChars += s.recv
		result.TotalChars += s.sent + s.recv
		result.VoiceChars += s.voiceSent + s.voiceRecv
		result.Contacts = append(result.Contacts, &model.ContactWordCountStat{
			Talker:     talker,
			SentChars:  s.sent,
			RecvChars:  s.recv,
			TotalChars: s.sent + s.recv,
			SentCount:  s.sentCount,
			RecvCount:  s.recvCount,
			TotalCount: s.sentCount + s.recvCount,
			VoiceChars: s.voiceSent + s.voiceRecv,
		})
	}
	return result, nil
}

type wcResult struct{ sent, recv, sentCount, recvCount int }

func (r *Repository) queryWordCountV4(ctx context.Context, db *sql.DB, tableName, talker string, isGroup bool, startUnix, endUnix int64) wcResult {
	// 仅统计 type=1（文本）消息。把 message_content 转成 TEXT 后用 length() 拿到字符数。
	// 群聊：CASE 用 status=2/real_sender_id=0 判定自己发的
	// 私聊：CASE 用 n.user_name != talker 判定自己发的
	// 字数只统计 type=1（文本）；条数统计所有非系统消息（local_type != 10000）。
	var query string
	var args []interface{}
	if isGroup {
		query = fmt.Sprintf(`
			SELECT
				COALESCE(SUM(CASE WHEN (m.status = 2 OR m.real_sender_id = 0) AND (m.local_type & 4294967295) = 1 THEN length(CAST(m.message_content AS TEXT)) ELSE 0 END), 0) AS sent_chars,
				COALESCE(SUM(CASE WHEN NOT (m.status = 2 OR m.real_sender_id = 0) AND (m.local_type & 4294967295) = 1 THEN length(CAST(m.message_content AS TEXT)) ELSE 0 END), 0) AS recv_chars,
				COALESCE(SUM(CASE WHEN (m.status = 2 OR m.real_sender_id = 0) AND (m.local_type & 4294967295) != 10000 THEN 1 ELSE 0 END), 0) AS sent_count,
				COALESCE(SUM(CASE WHEN NOT (m.status = 2 OR m.real_sender_id = 0) AND (m.local_type & 4294967295) != 10000 THEN 1 ELSE 0 END), 0) AS recv_count
			FROM %s m
			WHERE m.create_time >= ? AND m.create_time <= ?`, tableName)
		args = []interface{}{startUnix, endUnix}
	} else {
		query = fmt.Sprintf(`
			SELECT
				COALESCE(SUM(CASE WHEN (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) AND (m.local_type & 4294967295) = 1 THEN length(CAST(m.message_content AS TEXT)) ELSE 0 END), 0) AS sent_chars,
				COALESCE(SUM(CASE WHEN NOT (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) AND (m.local_type & 4294967295) = 1 THEN length(CAST(m.message_content AS TEXT)) ELSE 0 END), 0) AS recv_chars,
				COALESCE(SUM(CASE WHEN (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) AND (m.local_type & 4294967295) != 10000 THEN 1 ELSE 0 END), 0) AS sent_count,
				COALESCE(SUM(CASE WHEN NOT (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) AND (m.local_type & 4294967295) != 10000 THEN 1 ELSE 0 END), 0) AS recv_count
			FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
			WHERE m.create_time >= ? AND m.create_time <= ?`, tableName)
		args = []interface{}{talker, talker, talker, talker, startUnix, endUnix}
	}

	var sent, recv, sentCount, recvCount sql.NullInt64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&sent, &recv, &sentCount, &recvCount); err == nil {
		return wcResult{
			sent: int(sent.Int64), recv: int(recv.Int64),
			sentCount: int(sentCount.Int64), recvCount: int(recvCount.Int64),
		}
	}
	return wcResult{}
}

func (r *Repository) queryWordCountV3(ctx context.Context, db *sql.DB, target bind.RouteResult, startMs, endMs int64) wcResult {
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN COALESCE(IsSender, 0) = 1 AND Type = 1 THEN length(CAST(StrContent AS TEXT)) ELSE 0 END), 0) AS sent_chars,
			COALESCE(SUM(CASE WHEN COALESCE(IsSender, 0) != 1 AND Type = 1 THEN length(CAST(StrContent AS TEXT)) ELSE 0 END), 0) AS recv_chars,
			COALESCE(SUM(CASE WHEN COALESCE(IsSender, 0) = 1 AND Type != 10000 THEN 1 ELSE 0 END), 0) AS sent_count,
			COALESCE(SUM(CASE WHEN COALESCE(IsSender, 0) != 1 AND Type != 10000 THEN 1 ELSE 0 END), 0) AS recv_count
		FROM MSG WHERE CreateTime >= ? AND CreateTime <= ?`
	args := []interface{}{startMs, endMs}
	if target.TalkerID != 0 {
		query += " AND TalkerId = ?"
		args = append(args, target.TalkerID)
	} else {
		query += " AND StrTalker = ?"
		args = append(args, target.Talker)
	}
	var sent, recv, sentCount, recvCount sql.NullInt64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&sent, &recv, &sentCount, &recvCount); err == nil {
		return wcResult{
			sent: int(sent.Int64), recv: int(recv.Int64),
			sentCount: int(sentCount.Int64), recvCount: int(recvCount.Int64),
		}
	}
	return wcResult{}
}

// queryVoiceTranscriptChars 统计某个会话在时间区间内、已转写成文字的语音消息字数。
//
// 转写文本按语音消息的 server_id 存放在 transcripts 里，SQL 侧无从 join，
// 因此这里把区间内的语音消息列出来，再逐条查转写表累加字符数。
// 只在确实存在转写结果时才会被调用。
func (r *Repository) queryVoiceTranscriptChars(ctx context.Context, db *sql.DB,
	tableName, talker string, isGroup bool, startUnix, endUnix int64,
	tl TranscriptLookup) (sentChars, recvChars int) {

	var query string
	var args []interface{}
	if isGroup {
		query = fmt.Sprintf(`
			SELECT m.server_id, (m.status = 2 OR m.real_sender_id = 0) AS is_self, m.packed_info_data
			FROM %s m
			WHERE (m.local_type & 4294967295) = 34 AND m.create_time >= ? AND m.create_time <= ?`, tableName)
		args = []interface{}{startUnix, endUnix}
	} else {
		query = fmt.Sprintf(`
			SELECT m.server_id, (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) AS is_self, m.packed_info_data
			FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
			WHERE (m.local_type & 4294967295) = 34 AND m.create_time >= ? AND m.create_time <= ?`, tableName)
		args = []interface{}{talker, startUnix, endUnix}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()
	for rows.Next() {
		var serverID int64
		var isSelf int
		var packed []byte
		if rows.Scan(&serverID, &isSelf, &packed) != nil {
			continue
		}
		// 微信自己转好的优先 —— 它是用户在微信里看到的那份
		text := model.ParseVoiceTranscript(packed)
		if text == "" && tl != nil {
			text, _ = tl.Get(strconv.FormatInt(serverID, 10))
		}
		if text == "" {
			continue
		}
		n := len([]rune(text))
		if isSelf == 1 {
			sentChars += n
		} else {
			recvChars += n
		}
	}
	return sentChars, recvChars
}
