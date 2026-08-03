package repo

import (
	"context"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/types"
)

func (f *rawFeatures) toDTO() model.GalaxyRawFeature {
	return model.GalaxyRawFeature{
		ID: f.contactID, Remark: f.remark, Nick: f.nickName, Avatar: f.avatar,
		Total: f.total, Sent: f.sent, Recv: f.recv,
		First: f.firstTime, Last: f.lastTime, Days: f.activeDays,
		Months: f.monthCounts, Sessions: f.sessionCount,
		R30: f.recent30, R90: f.recent90, R365: f.recent365,
	}
}

func dtoToRaw(d model.GalaxyRawFeature) *rawFeatures {
	m := d.Months
	if m == nil {
		m = map[string]int{}
	}
	return &rawFeatures{
		contactID: d.ID, remark: d.Remark, nickName: d.Nick, avatar: d.Avatar,
		total: d.Total, sent: d.Sent, recv: d.Recv,
		firstTime: d.First, lastTime: d.Last, activeDays: d.Days,
		monthCounts: m, sessionCount: d.Sessions,
		recent30: d.R30, recent90: d.R90, recent365: d.R365,
	}
}

// extractIncremental 只对「有新消息」的联系人重新扫库(用会话最后消息时间 NTime 判断),
// 其余复用缓存特征。§30 增量更新的核心。
func (r *Repository) extractIncremental(ctx context.Context, tzOffsetSec int, cached []model.GalaxyRawFeature) ([]*rawFeatures, error) {
	cachedMap := make(map[string]*rawFeatures, len(cached))
	for _, d := range cached {
		cachedMap[d.ID] = dtoToRaw(d)
	}

	loc := time.FixedZone("galaxy", tzOffsetSec)
	tzMod := tzModifier(loc)
	rangeStart := time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Now()
	now := time.Now().Unix()
	t30, t90, t365 := now-30*86400, now-90*86400, now-365*86400

	sessions, err := r.GetSessions(ctx, types.SessionQuery{Limit: 8000})
	if err != nil {
		return nil, err
	}

	out := make([]*rawFeatures, 0, len(cached)+16)
	var talkers []string
	rescanned := 0

	for _, s := range sessions {
		talker := s.UserName
		if strings.HasSuffix(talker, "@chatroom") {
			continue
		}
		if c := cachedMap[talker]; c != nil && c.total > 0 && s.NTime.Unix() <= c.lastTime {
			out = append(out, c) // 无新消息 → 复用缓存,不扫库
			talkers = append(talkers, talker)
			continue
		}
		// 新联系人或有新消息 → 重新扫
		f := &rawFeatures{contactID: talker, monthCounts: map[string]int{}}
		tbl := v4TableName(talker)
		for _, target := range r.router.Resolve(rangeStart, rangeEnd, talker) {
			db, err := r.pool.GetConnection(target.FilePath)
			if err != nil || !r.isTableExist(db, tbl) {
				continue
			}
			r.galaxyMonthly(ctx, db, tbl, talker, tzMod, f)
			r.galaxySessionsRecency(ctx, db, tbl, t30, t90, t365, f)
		}
		if f.total > 0 {
			out = append(out, f)
			talkers = append(talkers, talker)
			rescanned++
		}
	}

	profiles, _ := r.getContactProfiles(ctx, talkers)
	for _, f := range out {
		if p, ok := profiles[f.contactID]; ok {
			f.remark, f.nickName, f.avatar = p.Remark, p.NickName, p.SmallHeadURL
		}
	}
	return out, nil
}

// BuildGalaxyIncremental 增量重建:cached 非空则只重扫有新消息的人,否则全量。
// 返回图 + 全量原始特征 DTO(供上层持久化到 raw_features.json)。
func (r *Repository) BuildGalaxyIncremental(ctx context.Context, profile *model.UserProfile, tzOffsetSec, topN int, cached []model.GalaxyRawFeature) (*model.RelationshipGraph, []model.GalaxyRawFeature, error) {
	var feats []*rawFeatures
	var err error
	if len(cached) > 0 {
		feats, err = r.extractIncremental(ctx, tzOffsetSec, cached)
	} else {
		feats, err = r.ExtractGalaxyFeatures(ctx, tzOffsetSec)
	}
	if err != nil {
		return nil, nil, err
	}
	graph := assembleGalaxy(feats, profile, tzOffsetSec, topN)

	dtos := make([]model.GalaxyRawFeature, 0, len(feats))
	for _, f := range feats {
		dtos = append(dtos, f.toDTO())
	}
	return graph, dtos, nil
}
