package store

import (
	"context"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/types"
	"github.com/fsnotify/fsnotify"
)

// Store 定义了数据访问的统一接口。
// 它屏蔽了底层的文结构和平台差异。
type Store interface {
	// 消息操作
	GetMessages(ctx context.Context, query types.MessageQuery) ([]*model.Message, error)
	SearchGlobalMessages(ctx context.Context, query types.MessageQuery) ([]*model.Message, error)
	GetTextMessagesGlobal(ctx context.Context, start, end time.Time, limit int) ([]string, error)

	// 联系人操作
	GetContacts(ctx context.Context, query types.ContactQuery) ([]*model.Contact, error)
	GetChatRooms(ctx context.Context, query types.ChatRoomQuery) ([]*model.ChatRoom, error)
	GetSessions(ctx context.Context, query types.SessionQuery) ([]*model.Session, error)
	DeleteSession(ctx context.Context, username string) error

	// 媒体操作
	GetMedia(ctx context.Context, mediaType string, key string) (*model.Media, error)

	// 分析操作
	GetHourlyActivity(ctx context.Context, sessionID string) ([]*model.HourlyStat, error)
	GetDailyActivity(ctx context.Context, sessionID string) ([]*model.DailyStat, error)
	GetWeekdayActivity(ctx context.Context, sessionID string) ([]*model.WeekdayStat, error)
	GetMonthlyActivity(ctx context.Context, sessionID string) ([]*model.MonthlyStat, error)
	GetYearlyMonthlyActivity(ctx context.Context, sessionID string) ([]*model.YearMonthStat, error)
	GetTopContactsHistoricalMonthlyAvg(ctx context.Context, limit int) ([]*model.MonthlyStat, error)
	GetMessageTypeDistribution(ctx context.Context, sessionID string) ([]*model.MessageTypeStat, error)
	GetCallStats(ctx context.Context, sessionID string) (*model.CallStats, error)
	GetMemberActivity(ctx context.Context, sessionID string) ([]*model.MemberActivity, error)
	GetRepeatAnalysis(ctx context.Context, sessionID string) ([]*model.RepeatStat, error)
	GetPersonalTopContacts(ctx context.Context, limit int) ([]*model.PersonalTopContact, error)
	GetDashboardData(ctx context.Context) (*model.DashboardData, error)

	// 搜索操作
	SearchMessages(ctx context.Context, query types.MessageQuery) (*model.SearchResult, error)
	GetMessageContext(ctx context.Context, talker string, seq int64, before, after int) ([]*model.Message, error)

	// 年度报告
	GetAnnualReport(ctx context.Context, year int, defaultTzOffset, pastStartYear int, segments []types.TZSegment, excludeTalkers []string) (*model.AnnualReport, error)
	GetAnnualWordCounts(ctx context.Context, year int, defaultTzOffset int, segments []types.TZSegment, excludeTalkers []string) (*model.WordCountStat, error)
	GetAnnualReportWithProgress(ctx context.Context, year int, defaultTzOffset, pastStartYear int, segments []types.TZSegment, excludeTalkers []string, progressFn types.ProgressCallback) (*model.AnnualReport, error)
	ComputeMonthlyAvgInRange(ctx context.Context, fromYear, toYear, tzOffsetSeconds int) []*model.MonthlyStat
	ComputePastOverviewAvg(ctx context.Context, year, pastStartYear, defaultTzOffset int) *model.AnnualOverview

	// 设置全局默认时区修饰符（影响联系人侧分析查询）
	SetDefaultTzModifier(mod string)
	// 当前数据版本指纹（DB 文件 path+size+mtime 的 md5）
	GetDataVersion() string

	// 客户联系提醒
	GetNeedContactList(ctx context.Context, days int) ([]*model.NeedContactItem, error)

	// Watch 注册文件系统事件的回调函数
	Watch(group string, callback func(event fsnotify.Event) error) error

	// Reload 重新加载存储（重建索引、刷新连接等）
	Reload() error

	// 生命周期管理
	Close() error
}
