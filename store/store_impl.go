package store

import (
	"context"
	"fmt"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/bind"
	"github.com/afumu/wetrace/store/core"
	"github.com/afumu/wetrace/store/repo"
	"github.com/afumu/wetrace/store/strategy"
	"github.com/afumu/wetrace/store/types"
	"github.com/fsnotify/fsnotify"
)

// DefaultStore 是 Store 接口的默认实现
type DefaultStore struct {
	pool    *core.ConnectionPool
	router  *bind.TimelineRouter
	watcher *core.Watcher
	repo    *repo.Repository
}

// NewStore 初始化一个新的存储实例
func NewStore(baseDir string) (*DefaultStore, error) {
	// 1. 初始化核心组件
	pool := core.NewConnectionPool(baseDir)
	watcher, err := core.NewWatcher(baseDir)
	if err != nil {
		pool.CloseAll()
		return nil, err
	}

	// 2. 策略层
	strat := strategy.NewV4()
	router := bind.NewTimelineRouter(baseDir, pool, strat)

	// 3. 构建索引 (这一步可能比较耗时，但必须在启动时完成)
	if err := router.RebuildIndex(context.Background()); err != nil {
		pool.CloseAll()
		watcher.Stop()
		return nil, fmt.Errorf("构建时间线索引失败: %w", err)
	}

	// 4. 初始化仓储
	r := repo.New(router, pool)

	// 5. 启动文件监听
	watcher.Start()

	// 注册自动刷新逻辑：当有新文件生成时，重建索引
	watcher.AddCallback(func(event fsnotify.Event) {
		if event.Op&fsnotify.Create == fsnotify.Create {
			// 只有当新文件被策略识别为消息数据库时，才重建索引
			if meta, ok := strat.Identify(event.Name); ok && meta.Type == strategy.Message {
				_ = router.RebuildIndex(context.Background())
			}
		}
	})

	return &DefaultStore{
		pool:    pool,
		router:  router,
		watcher: watcher,
		repo:    r,
	}, nil
}

func (s *DefaultStore) Close() error {
	s.watcher.Stop()
	return s.pool.CloseAll()
}

// --- 下面是 Store 接口的代理实现 ---

func (s *DefaultStore) GetMessages(ctx context.Context, query types.MessageQuery) ([]*model.Message, error) {
	return s.repo.GetMessages(ctx, query)
}

func (s *DefaultStore) GetTextMessagesGlobal(ctx context.Context, start, end time.Time, limit int) ([]string, error) {
	return s.repo.GetTextMessagesGlobal(ctx, start, end, limit)
}

func (s *DefaultStore) SearchGlobalMessages(ctx context.Context, query types.MessageQuery) ([]*model.Message, error) {
	return s.repo.SearchGlobalMessages(ctx, query)
}

func (s *DefaultStore) GetContacts(ctx context.Context, query types.ContactQuery) ([]*model.Contact, error) {
	return s.repo.GetContacts(ctx, query)
}

func (s *DefaultStore) GetChatRooms(ctx context.Context, query types.ChatRoomQuery) ([]*model.ChatRoom, error) {
	return s.repo.GetChatRooms(ctx, query)
}

func (s *DefaultStore) GetSessions(ctx context.Context, query types.SessionQuery) ([]*model.Session, error) {
	return s.repo.GetSessions(ctx, query)
}

func (s *DefaultStore) DeleteSession(ctx context.Context, username string) error {
	return s.repo.DeleteSession(ctx, username)
}

func (s *DefaultStore) GetMedia(ctx context.Context, mediaType string, key string) (*model.Media, error) {
	return s.repo.GetMedia(ctx, mediaType, key)
}

func (s *DefaultStore) GetHourlyActivity(ctx context.Context, sessionID string) ([]*model.HourlyStat, error) {
	return s.repo.GetHourlyActivity(ctx, sessionID)
}

func (s *DefaultStore) GetDailyActivity(ctx context.Context, sessionID string) ([]*model.DailyStat, error) {
	return s.repo.GetDailyActivity(ctx, sessionID)
}

func (s *DefaultStore) GetWeekdayActivity(ctx context.Context, sessionID string) ([]*model.WeekdayStat, error) {
	return s.repo.GetWeekdayActivity(ctx, sessionID)
}

func (s *DefaultStore) GetMonthlyActivity(ctx context.Context, sessionID string) ([]*model.MonthlyStat, error) {
	return s.repo.GetMonthlyActivity(ctx, sessionID)
}

func (s *DefaultStore) GetYearlyMonthlyActivity(ctx context.Context, sessionID string) ([]*model.YearMonthStat, error) {
	return s.repo.GetYearlyMonthlyActivity(ctx, sessionID)
}

func (s *DefaultStore) GetTopContactsHistoricalMonthlyAvg(ctx context.Context, limit int) ([]*model.MonthlyStat, error) {
	return s.repo.GetTopContactsHistoricalMonthlyAvg(ctx, limit)
}

func (s *DefaultStore) GetMessageTypeDistribution(ctx context.Context, sessionID string) ([]*model.MessageTypeStat, error) {
	return s.repo.GetMessageTypeDistribution(ctx, sessionID)
}

func (s *DefaultStore) GetCallStats(ctx context.Context, sessionID string) (*model.CallStats, error) {
	return s.repo.GetCallStats(ctx, sessionID)
}

func (s *DefaultStore) GetMemberActivity(ctx context.Context, sessionID string) ([]*model.MemberActivity, error) {
	return s.repo.GetMemberActivity(ctx, sessionID)
}

func (s *DefaultStore) GetRepeatAnalysis(ctx context.Context, sessionID string) ([]*model.RepeatStat, error) {
	return s.repo.GetRepeatAnalysis(ctx, sessionID)
}

func (s *DefaultStore) GetPersonalTopContacts(ctx context.Context, limit int) ([]*model.PersonalTopContact, error) {
	return s.repo.GetPersonalTopContacts(ctx, limit)
}

func (s *DefaultStore) GetDashboardData(ctx context.Context) (*model.DashboardData, error) {
	return s.repo.GetDashboardData(ctx)
}

func (s *DefaultStore) SearchMessages(ctx context.Context, query types.MessageQuery) (*model.SearchResult, error) {
	return s.repo.SearchMessages(ctx, query)
}

func (s *DefaultStore) GetMessageContext(ctx context.Context, talker string, seq int64, before, after int) ([]*model.Message, error) {
	return s.repo.GetMessageContext(ctx, talker, seq, before, after)
}

func (s *DefaultStore) GetAnnualReport(ctx context.Context, year int, defaultTzOffset, pastStartYear int, segments []types.TZSegment, excludeTalkers []string) (*model.AnnualReport, error) {
	return s.repo.GetAnnualReport(ctx, year, defaultTzOffset, pastStartYear, segments, excludeTalkers)
}

func (s *DefaultStore) GetTalkerAnnualReport(ctx context.Context, year int, talker string, defaultTzOffset int) (*model.AnnualReport, error) {
	return s.repo.GetTalkerAnnualReport(ctx, year, talker, defaultTzOffset)
}

func (s *DefaultStore) GetAnnualWordCounts(ctx context.Context, year int, defaultTzOffset int, segments []types.TZSegment, excludeTalkers []string) (*model.WordCountStat, error) {
	return s.repo.GetAnnualWordCounts(ctx, year, defaultTzOffset, segments, excludeTalkers)
}

func (s *DefaultStore) GetAnnualReportWithProgress(ctx context.Context, year int, defaultTzOffset, pastStartYear int, segments []types.TZSegment, excludeTalkers []string, progressFn types.ProgressCallback) (*model.AnnualReport, error) {
	return s.repo.GetAnnualReportWithProgress(ctx, year, defaultTzOffset, pastStartYear, segments, excludeTalkers, progressFn)
}

func (s *DefaultStore) ComputeMonthlyAvgInRange(ctx context.Context, fromYear, toYear, tzOffsetSeconds int) []*model.MonthlyStat {
	return s.repo.ComputeMonthlyAvgInRange(ctx, fromYear, toYear, tzOffsetSeconds)
}

func (s *DefaultStore) ComputePastOverviewAvg(ctx context.Context, year, pastStartYear, defaultTzOffset int) *model.AnnualOverview {
	return s.repo.ComputePastOverviewAvg(ctx, year, pastStartYear, defaultTzOffset)
}

func (s *DefaultStore) GetCalendarHeatmap(ctx context.Context, year, tzOffsetSec int) []*model.DayHeat {
	return s.repo.GetCalendarHeatmap(ctx, year, tzOffsetSec)
}

func (s *DefaultStore) GetInteractionRatios(ctx context.Context, year, tzOffsetSec, gapSeconds, limit int) ([]*model.InteractionRatio, error) {
	return s.repo.GetInteractionRatios(ctx, year, tzOffsetSec, gapSeconds, limit)
}

func (s *DefaultStore) GetReplySpeedRanking(ctx context.Context, year, tzOffsetSec, limit int) ([]*model.ReplySpeed, error) {
	return s.repo.GetReplySpeedRanking(ctx, year, tzOffsetSec, limit)
}

func (s *DefaultStore) GetYearCompare(ctx context.Context, yearA, yearB, tzOffsetSec int) (*model.YearCompare, error) {
	return s.repo.GetYearCompare(ctx, yearA, yearB, tzOffsetSec)
}

func (s *DefaultStore) GetCommonGroups(ctx context.Context, wxid string) ([]*model.CommonGroup, error) {
	return s.repo.GetCommonGroups(ctx, wxid)
}

func (s *DefaultStore) BuildGalaxyIncremental(ctx context.Context, profile *model.UserProfile, tzOffsetSec, topN int, cached []model.GalaxyRawFeature, overrides map[string]*model.ContactOverride) (*model.RelationshipGraph, []model.GalaxyRawFeature, error) {
	return s.repo.BuildGalaxyIncremental(ctx, profile, tzOffsetSec, topN, cached, overrides)
}

func (s *DefaultStore) BuildGalaxy(ctx context.Context, profile *model.UserProfile, tzOffsetSec, topN int, overrides map[string]*model.ContactOverride) (*model.RelationshipGraph, error) {
	return s.repo.BuildGalaxy(ctx, profile, tzOffsetSec, topN, overrides)
}

func (s *DefaultStore) SetDefaultTzModifier(mod string) {
	s.repo.SetDefaultTzModifier(mod)
}

func (s *DefaultStore) GetDataVersion() string {
	return s.repo.GetDataVersion()
}

func (s *DefaultStore) Watch(group string, callback func(event fsnotify.Event) error) error {
	s.watcher.AddCallback(func(event fsnotify.Event) {
		_ = callback(event)
	})
	return nil
}

func (s *DefaultStore) GetNeedContactList(ctx context.Context, days int) ([]*model.NeedContactItem, error) {
	return s.repo.GetNeedContactList(ctx, days)
}

func (s *DefaultStore) GetTalkerExtras(ctx context.Context, talker string, year, tzOffsetSec int) *model.TalkerExtras {
	return s.repo.GetTalkerExtras(ctx, talker, year, tzOffsetSec)
}

func (s *DefaultStore) GetBizProfile(ctx context.Context, year, tzOffsetSec int, withTitles bool) *model.BizProfile {
	return s.repo.GetBizProfile(ctx, year, tzOffsetSec, withTitles)
}

func (s *DefaultStore) SetStatsScope(scope *model.StatsScope) {
	s.repo.SetStatsScope(scope)
}

func (s *DefaultStore) StatsScope() *model.StatsScope {
	return s.repo.StatsScope()
}

func (s *DefaultStore) TalkerTypes(ctx context.Context) map[string]model.TalkerType {
	return s.repo.TalkerTypes(ctx)
}

func (s *DefaultStore) TalkerTypeOf(ctx context.Context, talker string) model.TalkerType {
	return s.repo.TalkerTypeOf(ctx, talker)
}

func (s *DefaultStore) InvalidateTalkerTags() {
	s.repo.InvalidateTalkerTags()
}

// Reload 重新加载存储（重建索引、刷新连接等）
func (s *DefaultStore) Reload() error {
	// 1. 关闭所有现有连接（这将强制下次查询时重新打开连接）
	if err := s.pool.CloseAll(); err != nil {
		return fmt.Errorf("reload: close all connections failed: %w", err)
	}

	// 2. 重新构建时间线索引（扫描目录，重新发现文件）
	if err := s.router.RebuildIndex(context.Background()); err != nil {
		return fmt.Errorf("reload: rebuild index failed: %w", err)
	}

	return nil
}
