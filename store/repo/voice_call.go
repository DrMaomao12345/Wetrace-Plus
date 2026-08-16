package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/pkg/util"
)

// callWindow 是一次通话占据的时间区间（unix 秒）
type callWindow struct {
	Start, End int64
}

// callWindows 取出某个会话在时间范围内的通话区间。
//
// 微信在通话结束时才落这条消息，时长写在 XML 里（V4 还是 zstd 压缩的，SQL 取不到），
// 所以只能捞出来在 Go 里解析。通话消息数量很少，开销可以忽略。
// 区间取 [消息时间 - 时长, 消息时间]。
func (r *Repository) callWindows(ctx context.Context, db *sql.DB, tbl string, start, end time.Time) []callWindow {
	q := fmt.Sprintf(
		`SELECT create_time, message_content FROM %s
		 WHERE (local_type & 4294967295) = 50 AND create_time >= ? AND create_time <= ?`, tbl)
	rows, err := db.QueryContext(ctx, q, start.Unix(), end.Unix())
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []callWindow
	for rows.Next() {
		var ts int64
		var raw []byte
		if rows.Scan(&ts, &raw) != nil {
			continue
		}
		content := stripGroupSenderPrefix(decodeV4Content(raw))
		info := util.ParseVoIP(content)
		if info.Duration <= 0 {
			continue // 未接通的通话不占用时间
		}
		out = append(out, callWindow{Start: ts - int64(info.Duration), End: ts})
	}
	return out
}

// overlapsCall 判断一段回复间隔 [from, to] 是否与任何通话区间重叠
func overlapsCall(windows []callWindow, from, to int64) bool {
	for _, w := range windows {
		if to > w.Start && from < w.End {
			return true
		}
	}
	return false
}

// callsCTE 把通话区间拼成一段 SQL，供回复速度查询排除。
// 没有通话时返回空串与空参数，调用方据此跳过整个排除条件。
func callsCTE(windows []callWindow) (cte string, cond string, args []interface{}) {
	if len(windows) == 0 {
		return "", "", nil
	}
	cte = "calls(cs, ce) AS (VALUES "
	for i, w := range windows {
		if i > 0 {
			cte += ","
		}
		cte += "(?,?)"
		args = append(args, w.Start, w.End)
	}
	cte += "), "
	// 回复间隔 [pt, ct] 与通话区间有重叠就不算数
	cond = " AND NOT EXISTS (SELECT 1 FROM calls WHERE ct > cs AND pt < ce)"
	return cte, cond, args
}

// ── 语音消息统计 ────────────────────────────────────────────────

// GetVoiceStats 统计某个会话的语音消息次数与时长。
// talker 为空表示统计全部会话。
func (r *Repository) GetVoiceStats(ctx context.Context, talker string, start, end time.Time) *model.VoiceStats {
	st := &model.VoiceStats{}

	targets := r.router.Resolve(start, end, talker)
	tbl := v4TableName(talker)
	// 语音 XML 里的 fromusername 是发送者 wxid。私聊时它等于对方的 wxid，
	// 群聊时是群成员的 wxid —— 两种情况都只能跟「我自己的 wxid」比才判得准。
	// 早先拿它和 talker 比，群聊里 talker 是 xxx@chatroom，永远不相等，
	// 结果群里每条语音都被算成自己发的。
	myWxid := r.getCurrentUserWxid(ctx)
	// 转写表没启用 / 一条都没有时为 nil，据此整段跳过字数统计，不多查一列
	tl := r.transcriptLookup()

	for _, target := range targets {
		db, err := r.pool.GetConnection(target.FilePath)
		if err != nil || !r.isTableExist(db, tbl) {
			continue
		}
		// server_id 就是转写表的键（见 model.message_v4 里 Contents["voice"] 的赋值）
		q := fmt.Sprintf(`SELECT create_time, status, real_sender_id, message_content, server_id
			FROM %s WHERE (local_type & 4294967295) = 34 AND create_time >= ? AND create_time <= ?`, tbl)
		rows, err := db.QueryContext(ctx, q, start.Unix(), end.Unix())
		if err != nil {
			continue
		}
		for rows.Next() {
			var ts, senderID, serverID int64
			var status int
			var raw []byte
			if rows.Scan(&ts, &status, &senderID, &raw, &serverID) != nil {
				continue
			}
			content := stripGroupSenderPrefix(decodeV4Content(raw))
			dur := parseVoiceLengthMs(content)

			// 优先用 XML 里的 fromusername（最直接），拿不到再退回 status/sender_id
			isSelf := status == 2 || senderID == 0
			if from := parseVoiceFrom(content); from != "" && myWxid != "" {
				isSelf = from == myWxid
			}
			st.Add(isSelf, dur, ts)

			if tl != nil {
				if text, ok := tl.Get(strconv.FormatInt(serverID, 10)); ok {
					st.AddTranscript(isSelf, text)
				}
			}
		}
		rows.Close()
	}
	st.Finish()
	return st
}

// parseVoiceLengthMs 从语音消息 XML 里取 voicelength（毫秒）
func parseVoiceLengthMs(content string) int64 {
	return int64(attrInt(content, "voicelength"))
}

func parseVoiceFrom(content string) string {
	return attrString(content, "fromusername")
}
