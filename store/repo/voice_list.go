package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/afumu/wetrace/internal/model"
)

// ── 语音清单 ────────────────────────────────────────────────────
//
// 批量转写需要知道「哪些语音还没转」。早先的做法是把每个会话的全部消息
// 都用 GetMessages 载入内存再筛出语音 —— 687 个会话、上百万条消息，
// 既慢（枚举一遍就要 ~90 秒）又占内存。
//
// 这里直接扫消息表：local_type 上有索引，一张表一条 SQL，
// 只取语音消息的 server_id 与 packed_info_data，几 MB 内存就够。

// VoiceRef 是批量转写需要的最小信息
type VoiceRef struct {
	Talker   string
	VoiceID  string // server_id，转写结果以此为键
	Time     time.Time
	WeChatTx string // 微信自带的转写文本（有则无需再跑 Whisper）
}

// ListVoiceMessages 列出语音消息，按时间倒序（先转最近的，符合直觉）。
// talker 为空表示全部会话。
func (r *Repository) ListVoiceMessages(ctx context.Context, talker string) []*VoiceRef {
	md5Map := r.cachedTalkerMD5Map(ctx)

	var only string
	if talker != "" {
		only = v4TableName(talker)
	}

	var out []*VoiceRef
	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}
		if r.isTableExist(db, "MSG") {
			continue // v3 的语音走另一套存储，批量转写暂不覆盖
		}
		for _, tbl := range r.listMsgTables(ctx, db) {
			if only != "" && tbl != only {
				continue
			}
			owner := md5Map[trimMsgPrefix(tbl)]
			if owner == "" {
				owner = talker
			}
			out = append(out, r.listVoicesV4Table(ctx, db, tbl, owner)...)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

func (r *Repository) listVoicesV4Table(ctx context.Context, db *sql.DB, tbl, owner string) []*VoiceRef {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT server_id, create_time, packed_info_data FROM %s
		 WHERE (local_type & 4294967295) = 34`, tbl))
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*VoiceRef
	for rows.Next() {
		var sid, ts int64
		var packed []byte
		if rows.Scan(&sid, &ts, &packed) != nil || sid == 0 {
			continue
		}
		out = append(out, &VoiceRef{
			Talker:   owner,
			VoiceID:  fmt.Sprint(sid),
			Time:     time.Unix(ts, 0),
			WeChatTx: model.ParseVoiceTranscript(packed),
		})
	}
	return out
}
