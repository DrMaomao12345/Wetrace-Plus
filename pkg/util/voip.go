package util

import (
	"regexp"
	"strconv"
	"strings"
)

// VoIPInfo 从通话消息内容中解析出的信息
type VoIPInfo struct {
	Duration int  // 通话时长（秒），0 表示未接通/取消
	IsVideo  bool // 是否为视频通话
}

var (
	reVoipDuration = regexp.MustCompile(`<duration>(\d+)</duration>`)
	// 备用：从中文文案 "通话时长 12:34" 或 "1:23:45" 提取
	reVoipWording   = regexp.MustCompile(`通话时长[\s:：]*(?:(\d+):)?(\d+):(\d+)`)
	reVoipMsgType   = regexp.MustCompile(`<msg_type>(\d+)</msg_type>`)
	reVoipRoomType  = regexp.MustCompile(`<room_type>(\d+)</room_type>`)
	reVoipInviteTy  = regexp.MustCompile(`<invitetype>(\d+)</invitetype>`)
	reVoipDataType  = regexp.MustCompile(`<datatype>(\d+)</datatype>`)
)

// ParseVoIP 解析通话消息内容，提取时长和通话类型
func ParseVoIP(content string) VoIPInfo {
	info := VoIPInfo{}
	if content == "" {
		return info
	}

	// 1) 直接解析 <duration> 字段（最准确，单位秒）
	if m := reVoipDuration.FindStringSubmatch(content); len(m) == 2 {
		if d, err := strconv.Atoi(m[1]); err == nil && d > 0 {
			info.Duration = d
		}
	}

	// 2) 兜底：从"通话时长 X:Y[:Z]"提取
	if info.Duration == 0 {
		if m := reVoipWording.FindStringSubmatch(content); len(m) == 4 {
			h, _ := strconv.Atoi(m[1]) // 可能为空
			min, _ := strconv.Atoi(m[2])
			sec, _ := strconv.Atoi(m[3])
			info.Duration = h*3600 + min*60 + sec
		}
	}

	// 判断视频通话
	// V4 新版 voipmsg：room_type 0=视频、1=语音（对着会话摘要逐条核对过，别按直觉反过来）
	// V4 voipinvitemsg：invitetype 0=视频、1=语音
	// 老版本：msg_type=1=语音, 4/5=视频；datatype: 1=语音, 2=视频
	if m := reVoipRoomType.FindStringSubmatch(content); len(m) == 2 {
		info.IsVideo = m[1] == "0"
	} else if m := reVoipInviteTy.FindStringSubmatch(content); len(m) == 2 {
		if v, err := strconv.Atoi(m[1]); err == nil && (v == 0 || v == 2) {
			info.IsVideo = true
		}
	} else if m := reVoipDataType.FindStringSubmatch(content); len(m) == 2 {
		if v, err := strconv.Atoi(m[1]); err == nil && v == 2 {
			info.IsVideo = true
		}
	} else if m := reVoipMsgType.FindStringSubmatch(content); len(m) == 2 {
		if v, err := strconv.Atoi(m[1]); err == nil && (v == 4 || v == 5) {
			info.IsVideo = true
		}
	}

	// 文案兜底
	if !info.IsVideo && (strings.Contains(content, "视频通话") || strings.Contains(content, "VideoCall")) {
		info.IsVideo = true
	}

	return info
}
