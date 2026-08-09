package model

import "strings"

// TalkerType 是会话（聊天）的类型标签。
// 每个会话都会被打上其中一个标签，用于「统计范围」配置按类型剔除。
type TalkerType string

const (
	TalkerFriend       TalkerType = "friend"       // 联系人（好友）
	TalkerGroup        TalkerType = "group"        // 群聊
	TalkerSubscription TalkerType = "subscription" // 订阅号
	TalkerService      TalkerType = "service"      // 服务号
	TalkerWork         TalkerType = "work"         // 企业微信 / 外部联系人
	TalkerStranger     TalkerType = "stranger"     // 陌生人（非好友，多为群成员）
	TalkerSystem       TalkerType = "system"       // 系统账号（文件传输助手等）
	TalkerOther        TalkerType = "other"        // 其他 / 无法判定
)

// AllTalkerTypes 是全部类型，顺序即前端展示顺序。
var AllTalkerTypes = []TalkerType{
	TalkerFriend, TalkerGroup, TalkerSubscription, TalkerService,
	TalkerWork, TalkerStranger, TalkerSystem, TalkerOther,
}

var talkerTypeLabels = map[TalkerType]string{
	TalkerFriend:       "联系人",
	TalkerGroup:        "群聊",
	TalkerSubscription: "订阅号",
	TalkerService:      "服务号",
	TalkerWork:         "企业微信",
	TalkerStranger:     "陌生人",
	TalkerSystem:       "系统账号",
	TalkerOther:        "其他",
}

// Label 返回类型的中文名
func (t TalkerType) Label() string {
	if s, ok := talkerTypeLabels[t]; ok {
		return s
	}
	return string(TalkerOther)
}

// Valid 判断是否是已知类型
func (t TalkerType) Valid() bool {
	_, ok := talkerTypeLabels[t]
	return ok
}

// systemTalkers 是微信内置的系统 / 服务账号，它们不是真实联系人。
var systemTalkers = map[string]bool{
	"filehelper":                true,
	"weixin":                    true,
	"fmessage":                  true,
	"medianote":                 true,
	"floatbottle":               true,
	"qmessage":                  true,
	"tmessage":                  true,
	"qqmail":                    true,
	"newsapp":                   true,
	"notifymessage":             true,
	"exmail_tool":               true,
	"weixinreminder":            true,
	"officialaccounts":          true,
	"notification_messages":     true,
	"brandsessionholder":        true,
	"brandservicesessionholder": true,
	"opencustomerservicemsg":    true,
	"service_notification":      true,
	"weappbindmobile":           true,
	"mphelper":                  true,
}

// TalkerFacts 是分类所需的原始事实，由各版本数据库分别填充。
// 缺失的字段留零值即可 —— 分类逻辑会退化为按用户名前缀判断。
type TalkerFacts struct {
	UserName   string
	LocalType  int  // V4 contact.local_type：1 好友 / 2 群聊 / 3 非好友群成员 / 5,6 企业微信
	VerifyFlag int  // contact.verify_flag：含 0x8 位表示公众号
	BizType    int  // biz_info.type：0 订阅号 / 1 服务号
	HasBizInfo bool // 是否在 biz_info 表中出现过
	IsFriend   bool // V3 Reserved1 == 1 时为真
	KnownV3    bool // 该事实来自 V3 数据库（LocalType 不可用）
}

// ClassifyTalker 依据数据库事实 + 用户名前缀判定会话类型。
// 判定优先级：系统账号 > 群聊 > 公众号（订阅号/服务号）> 企业微信 > 好友/陌生人。
func ClassifyTalker(f TalkerFacts) TalkerType {
	name := strings.ToLower(strings.TrimSpace(f.UserName))
	if name == "" {
		return TalkerOther
	}

	if systemTalkers[name] || strings.Contains(name, "@placeholder_") {
		return TalkerSystem
	}

	if strings.HasSuffix(name, "@chatroom") || f.LocalType == 2 {
		return TalkerGroup
	}

	// 公众号：用户名以 gh_ 开头，或 verify_flag 带公众号位
	isBiz := strings.HasPrefix(name, "gh_") || f.VerifyFlag&0x8 != 0
	if isBiz {
		// biz_info.type 是唯一能可靠区分订阅号/服务号的字段
		if f.HasBizInfo && f.BizType == 1 {
			return TalkerService
		}
		return TalkerSubscription
	}

	if strings.HasSuffix(name, "@openim") || f.LocalType == 5 || f.LocalType == 6 {
		return TalkerWork
	}

	// 非好友（群成员）：V4 用 local_type=3，V3 用 Reserved1
	if f.LocalType == 3 || (f.KnownV3 && !f.IsFriend) {
		return TalkerStranger
	}

	if f.LocalType == 1 || (f.KnownV3 && f.IsFriend) {
		return TalkerFriend
	}

	// 没有联系人记录的 wxid：多半是已删除好友或临时会话
	if strings.HasPrefix(name, "wxid_") {
		return TalkerStranger
	}

	return TalkerOther
}
