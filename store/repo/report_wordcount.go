package repo

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/bind"
	"github.com/afumu/wetrace/store/types"
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

	// 聚合：talker → {sentChars, recvChars}
	type stats struct {
		sent int
		recv int
	}
	agg := make(map[string]*stats)

	yearStart := segs[0].start
	yearEnd := segs[len(segs)-1].end

	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 5000})
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		talker := session.UserName
		if excludeSet[talker] {
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
			} else {
				wc := r.queryWordCountV3(ctx, db, target, yearStart.Unix()*1000, yearEnd.Unix()*1000)
				s.sent += wc.sent
				s.recv += wc.recv
			}
		}
		if s.sent > 0 || s.recv > 0 {
			agg[talker] = s
		}
	}

	result := &model.WordCountStat{}
	for talker, s := range agg {
		result.SentChars += s.sent
		result.RecvChars += s.recv
		result.TotalChars += s.sent + s.recv
		result.Contacts = append(result.Contacts, &model.ContactWordCountStat{
			Talker:     talker,
			SentChars:  s.sent,
			RecvChars:  s.recv,
			TotalChars: s.sent + s.recv,
		})
	}
	return result, nil
}

type wcResult struct{ sent, recv int }

func (r *Repository) queryWordCountV4(ctx context.Context, db *sql.DB, tableName, talker string, isGroup bool, startUnix, endUnix int64) wcResult {
	// 仅统计 type=1（文本）消息。把 message_content 转成 TEXT 后用 length() 拿到字符数。
	// 群聊：CASE 用 status=2/real_sender_id=0 判定自己发的
	// 私聊：CASE 用 n.user_name != talker 判定自己发的
	var query string
	var args []interface{}
	if isGroup {
		query = fmt.Sprintf(`
			SELECT
				COALESCE(SUM(CASE WHEN (m.status = 2 OR m.real_sender_id = 0) THEN length(CAST(m.message_content AS TEXT)) ELSE 0 END), 0) AS sent_chars,
				COALESCE(SUM(CASE WHEN NOT (m.status = 2 OR m.real_sender_id = 0) THEN length(CAST(m.message_content AS TEXT)) ELSE 0 END), 0) AS recv_chars
			FROM %s m
			WHERE m.create_time >= ? AND m.create_time <= ? AND (m.local_type & 4294967295) = 1`, tableName)
		args = []interface{}{startUnix, endUnix}
	} else {
		query = fmt.Sprintf(`
			SELECT
				COALESCE(SUM(CASE WHEN (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) THEN length(CAST(m.message_content AS TEXT)) ELSE 0 END), 0) AS sent_chars,
				COALESCE(SUM(CASE WHEN NOT (m.status = 2 OR m.real_sender_id = 0 OR n.user_name != ?) THEN length(CAST(m.message_content AS TEXT)) ELSE 0 END), 0) AS recv_chars
			FROM %s m LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
			WHERE m.create_time >= ? AND m.create_time <= ? AND (m.local_type & 4294967295) = 1`, tableName)
		args = []interface{}{talker, talker, startUnix, endUnix}
	}

	var sent, recv sql.NullInt64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&sent, &recv); err == nil {
		return wcResult{sent: int(sent.Int64), recv: int(recv.Int64)}
	}
	return wcResult{}
}

func (r *Repository) queryWordCountV3(ctx context.Context, db *sql.DB, target bind.RouteResult, startMs, endMs int64) wcResult {
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN COALESCE(IsSender, 0) = 1 THEN length(CAST(StrContent AS TEXT)) ELSE 0 END), 0) AS sent_chars,
			COALESCE(SUM(CASE WHEN COALESCE(IsSender, 0) != 1 THEN length(CAST(StrContent AS TEXT)) ELSE 0 END), 0) AS recv_chars
		FROM MSG WHERE CreateTime >= ? AND CreateTime <= ? AND Type = 1`
	args := []interface{}{startMs, endMs}
	if target.TalkerID != 0 {
		query += " AND TalkerId = ?"
		args = append(args, target.TalkerID)
	} else {
		query += " AND StrTalker = ?"
		args = append(args, target.Talker)
	}
	var sent, recv sql.NullInt64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&sent, &recv); err == nil {
		return wcResult{sent: int(sent.Int64), recv: int(recv.Int64)}
	}
	return wcResult{}
}
