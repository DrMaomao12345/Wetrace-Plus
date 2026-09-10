package repo

import (
	"context"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
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

// decayRecency 修正复用缓存带来的 recency 陈旧(§30 已知问题)。
//
// 缓存里的 r30/r90/r365 是「上次扫库那一刻」往前数的窗口计数。这个联系人既然
// 没有新消息，他的消息集合就没变，窗口只会随时间往前滑、把老消息滑出去 ——
// 也就是说这些计数只可能变小，绝不会变大。最后一条消息已经落在窗口外时，
// 窗口内必然一条都没有，直接归零；否则保持原值(仍是上界)。
// 这样「久未联系但曾经很活跃」的人就不会一直顶着过期的高活跃度。
func decayRecency(f *rawFeatures, t30, t90, t365 int64) {
	if f.lastTime <= 0 {
		return
	}
	if f.lastTime < t30 {
		f.recent30 = 0
	}
	if f.lastTime < t90 {
		f.recent90 = 0
	}
	if f.lastTime < t365 {
		f.recent365 = 0
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

	allowTalker := r.TalkerFilter(ctx, model.ModuleGalaxy)
	for _, s := range sessions {
		talker := s.UserName
		if strings.HasSuffix(talker, "@chatroom") {
			continue
		}
		if !allowTalker(talker) {
			continue
		}
		if c := cachedMap[talker]; c != nil && c.total > 0 && s.NTime.Unix() <= c.lastTime {
			decayRecency(c, t30, t90, t365) // 缓存是旧的,近期窗口得随时间收敛
			out = append(out, c)            // 无新消息 → 复用缓存,不扫库
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
func (r *Repository) BuildGalaxyIncremental(ctx context.Context, profile *model.UserProfile, tzOffsetSec, topN int, cached []model.GalaxyRawFeature, overrides map[string]*model.ContactOverride) (*model.RelationshipGraph, []model.GalaxyRawFeature, error) {
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
	graph := assembleGalaxy(feats, profile, tzOffsetSec, topN, overrides)

	dtos := make([]model.GalaxyRawFeature, 0, len(feats))
	for _, f := range feats {
		dtos = append(dtos, f.toDTO())
	}
	return graph, dtos, nil
}
