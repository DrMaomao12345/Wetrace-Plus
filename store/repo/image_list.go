package repo

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/pkg/util/zstd"
)

// ── 图库的图片清单 ──────────────────────────────────────────────
//
// 图库要的是「全部会话、按时间倒序」的图片。走 GetMessages 逐个会话查要 ~90 秒
// （687 个会话各解析一遍全部消息），这里直接扫消息表：
// local_type 上有索引，一张表一条 SQL，只取图片消息，再从内容里抠 md5。

// reImgMD5 从图片消息的 XML 里取 md5 属性
var reImgMD5 = regexp.MustCompile(`md5\s*=\s*"([0-9a-fA-F]{32})"`)

// ImageRef 是图库需要的最小信息
type ImageRef struct {
	Talker string
	MD5    string
	Time   time.Time
	Seq    int64
}

// ListImageMessages 列出时间区间内的图片消息，按时间倒序。
// talker 为空表示全部会话。
func (r *Repository) ListImageMessages(ctx context.Context, talker string, start, end time.Time) []*ImageRef {
	md5Map := r.cachedTalkerMD5Map(ctx)
	allow := r.TableFilter(ctx, model.ModuleDashboard)

	// 指定了会话就只扫那一张表
	var only string
	if talker != "" {
		only = v4TableName(talker)
	}

	var out []*ImageRef
	for _, shard := range r.router.GetShards() {
		db, err := r.pool.GetConnection(shard.FilePath)
		if err != nil {
			continue
		}
		if r.isTableExist(db, "MSG") {
			out = append(out, r.listImagesV3(ctx, db, talker, start, end)...)
			continue
		}
		for _, tbl := range r.listMsgTables(ctx, db) {
			if only != "" && tbl != only {
				continue
			}
			if only == "" && !allow(tbl) {
				continue
			}
			owner := md5Map[trimMsgPrefix(tbl)]
			if owner == "" {
				owner = talker
			}
			out = append(out, r.listImagesV4Table(ctx, db, tbl, owner, start, end)...)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

func (r *Repository) listImagesV4Table(ctx context.Context, db *sql.DB, tbl, owner string,
	start, end time.Time) []*ImageRef {

	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT create_time, COALESCE(sort_seq,0), message_content FROM %s
		 WHERE (local_type & 4294967295) = 3 AND create_time >= ? AND create_time <= ?`, tbl),
		start.Unix(), end.Unix())
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*ImageRef
	for rows.Next() {
		var ts, seq int64
		var raw []byte
		if rows.Scan(&ts, &seq, &raw) != nil {
			continue
		}
		content := raw
		if b, err := zstd.Decompress(raw); err == nil {
			content = b
		}
		m := reImgMD5.FindSubmatch(content)
		if m == nil {
			continue
		}
		out = append(out, &ImageRef{
			Talker: owner,
			MD5:    strings.ToLower(string(m[1])),
			Time:   time.Unix(ts, 0),
			Seq:    seq,
		})
	}
	return out
}

func (r *Repository) listImagesV3(ctx context.Context, db *sql.DB, talker string,
	start, end time.Time) []*ImageRef {

	q := `SELECT CreateTime/1000, COALESCE(Sequence,0), COALESCE(StrTalker,''), COALESCE(StrContent,'')
	      FROM MSG WHERE Type = 3 AND CreateTime >= ? AND CreateTime <= ?`
	args := []interface{}{start.Unix() * 1000, end.Unix() * 1000}
	if talker != "" {
		q += " AND StrTalker = ?"
		args = append(args, talker)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*ImageRef
	for rows.Next() {
		var ts, seq int64
		var tk, content string
		if rows.Scan(&ts, &seq, &tk, &content) != nil {
			continue
		}
		m := reImgMD5.FindStringSubmatch(content)
		if m == nil {
			continue
		}
		out = append(out, &ImageRef{
			Talker: tk,
			MD5:    strings.ToLower(m[1]),
			Time:   time.Unix(ts, 0),
			Seq:    seq,
		})
	}
	return out
}
