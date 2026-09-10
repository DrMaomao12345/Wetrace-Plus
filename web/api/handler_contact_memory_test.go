package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/gin-gonic/gin"
)

func TestContactMemoryHTTPMissingAndDeleteContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	memories, err := NewContactMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("初始化联系人记忆存储失败: %v", err)
	}
	a := &API{
		ContactMemories:   memories,
		contactMemoryJobs: make(map[string]*contactMemoryJob),
	}
	router := gin.New()
	router.GET("/api/v1/ai/memories/:talker", a.GetContactMemory)
	router.DELETE("/api/v1/ai/memories/:talker", a.DeleteContactMemory)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/ai/memories/wxid_alice", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("查询缺失记忆状态码=%d，响应=%s", response.Code, response.Body.String())
	}
	var missing struct {
		Success bool                        `json:"success"`
		Data    ContactMemoryStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &missing); err != nil {
		t.Fatalf("解析缺失记忆响应失败: %v", err)
	}
	if !missing.Success || missing.Data.Exists || missing.Data.Status != "missing" || missing.Data.Memory != nil {
		t.Fatalf("缺失记忆响应契约不符: %#v", missing)
	}

	if err := memories.Put(ContactMemory{
		AccountID:  "default",
		Talker:     "wxid_alice",
		TargetName: "Alice",
		Profile:    ContactMemoryProfile{Summary: "简短直接"},
	}); err != nil {
		t.Fatalf("写入待删除记忆失败: %v", err)
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/ai/memories/wxid_alice", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("删除记忆状态码=%d，响应=%s", response.Code, response.Body.String())
	}
	var deleted struct {
		Success bool `json:"success"`
		Data    struct {
			Deleted bool `json:"deleted"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &deleted); err != nil {
		t.Fatalf("解析删除记忆响应失败: %v", err)
	}
	if !deleted.Success || !deleted.Data.Deleted {
		t.Fatalf("删除记忆响应契约不符: %#v", deleted)
	}
	if _, exists := memories.Get("default", "wxid_alice"); exists {
		t.Fatal("删除接口返回成功后联系人记忆仍然存在")
	}
}

func TestExtractFirstJSONObjectHandlesFencesAndBracesInsideStrings(t *testing.T) {
	raw := "说明文字\n```json\n{\"summary\":\"会说 } 也会说 {\",\"traits\":[\"直接\"]}\n```\n尾部"
	got, err := extractFirstJSONObject(raw)
	if err != nil {
		t.Fatalf("提取 JSON 失败: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("提取结果不是合法 JSON: %v\n%s", err, got)
	}
	if decoded["summary"] != "会说 } 也会说 {" {
		t.Fatalf("字符串中的大括号被错误处理: %#v", decoded["summary"])
	}
}

func TestValidateContactMemoryRequiresRealEvidenceAndRestoresExamples(t *testing.T) {
	samples := []contactMemorySample{
		{Seq: 1, Role: "user", Modality: "text", Text: "吃饭了吗"},
		{Seq: 2, Role: "contact", Modality: "voice", Text: "吃了，哈哈"},
		{Seq: 3, Role: "contact", Modality: "text", Text: "行"},
	}
	raw := `{
  "summary": " 回复比较简短 ",
  "traits": ["直接", "直接"],
  "speaking_style": {
    "tone": ["随意"],
    "catchphrases": [
      {"text": "哈哈", "evidence_seqs": [999]},
      {"text": "从未说过", "evidence_seqs": [3]}
    ]
  },
  "facts": [
    {"content": "有真实证据", "evidence_seqs": [2], "confidence": "high"},
    {"content": "没有证据", "evidence_seqs": [999], "confidence": "high"}
  ],
  "interaction_patterns": [
    {"context": "被问吃饭", "response": "简短回答", "evidence_seqs": [1, 2]},
    {"context": "伪造", "response": "伪造", "evidence_seqs": [999]}
  ],
  "typical_examples": [
    {"user": "模型改写的用户话", "contact": "模型改写的联系人话", "evidence_seqs": [1, 2]}
  ]
}`

	profile, err := parseAndValidateContactMemoryProfile(raw, samples)
	if err != nil {
		t.Fatalf("解析画像失败: %v", err)
	}
	if len(profile.Traits) != 1 || profile.Traits[0] != "直接" {
		t.Fatalf("特点没有去重: %#v", profile.Traits)
	}
	if len(profile.SpeakingStyle.Catchphrases) != 1 || profile.SpeakingStyle.Catchphrases[0].Text != "哈哈" {
		t.Fatalf("未核验的口头禅没有被过滤: %#v", profile.SpeakingStyle.Catchphrases)
	}
	if got := profile.SpeakingStyle.Catchphrases[0].EvidenceSeqs; len(got) != 1 || got[0] != 2 {
		t.Fatalf("口头禅没有回填真实证据: %#v", got)
	}
	if len(profile.Facts) != 1 || profile.Facts[0].Content != "有真实证据" {
		t.Fatalf("未核验事实没有被过滤: %#v", profile.Facts)
	}
	if len(profile.InteractionPatterns) != 1 {
		t.Fatalf("未核验互动规律没有被过滤: %#v", profile.InteractionPatterns)
	}
	if len(profile.TypicalExamples) != 1 {
		t.Fatalf("典型对话丢失: %#v", profile.TypicalExamples)
	}
	example := profile.TypicalExamples[0]
	if example.User != "吃饭了吗" || example.Contact != "吃了，哈哈" {
		t.Fatalf("典型对话没有恢复为真实原文: %#v", example)
	}
}

func TestBuildContactMemorySamplesIsBoundedStratifiedAndChronological(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	msgs := make([]*model.Message, 0, 250)
	for i := 0; i < 250; i++ {
		message := &model.Message{
			Seq:      int64(i + 1),
			Time:     base.Add(time.Duration(i) * time.Hour),
			Talker:   "contact",
			Sender:   "self",
			IsSelf:   true,
			Type:     model.MessageTypeText,
			Content:  "用户消息",
			Contents: map[string]interface{}{},
		}
		if i%2 == 1 {
			message.Sender = "contact"
			message.IsSelf = false
			message.Content = "联系人消息"
		}
		if i == 101 {
			message.Type = model.MessageTypeVoice
			message.Content = ""
			message.Contents["voice"] = "voice-102"
		}
		msgs = append(msgs, message)
	}

	samples := buildContactMemorySamples(msgs, "contact", func(id string) string {
		if id == "voice-102" {
			return "语音原话"
		}
		return ""
	})
	if len(samples) > contactMemorySampleLimit {
		t.Fatalf("样本超过上限: %d", len(samples))
	}
	if samples[0].Seq != 1 {
		t.Fatalf("没有保留跨时间样本，首条 seq=%d", samples[0].Seq)
	}
	if samples[len(samples)-1].Seq != 250 {
		t.Fatalf("没有保留最新样本，末条 seq=%d", samples[len(samples)-1].Seq)
	}
	foundVoice := false
	for i, sample := range samples {
		if i > 0 && sample.Seq < samples[i-1].Seq {
			t.Fatalf("样本没有恢复为时间顺序: %d 在 %d 之后", sample.Seq, samples[i-1].Seq)
		}
		if sample.Seq == 102 {
			foundVoice = sample.Modality == "voice" && sample.Text == "语音原话"
		}
	}
	if !foundVoice {
		t.Fatal("语音转写样本没有被保留")
	}
}

func TestBuildContactMemorySamplesLongUserMessagesCannotStarveContact(t *testing.T) {
	msgs := make([]*model.Message, 0, 120)
	for i := 0; i < 5; i++ {
		msgs = append(msgs, &model.Message{
			Seq:     int64(i + 1),
			Talker:  "contact",
			Sender:  "contact",
			IsSelf:  false,
			Type:    model.MessageTypeText,
			Content: "联系人原话",
		})
	}
	veryLong := strings.Repeat("很长的用户消息", 3000)
	for i := 5; i < 120; i++ {
		msgs = append(msgs, &model.Message{
			Seq:     int64(i + 1),
			Talker:  "contact",
			Sender:  "self",
			IsSelf:  true,
			Type:    model.MessageTypeText,
			Content: veryLong,
		})
	}

	samples := buildContactMemorySamples(msgs, "contact", nil)
	contactCount, totalRunes := 0, 0
	for _, sample := range samples {
		if sample.Role == "contact" {
			contactCount++
		}
		if got := len([]rune(sample.Text)); got > contactMemorySampleTextLimit {
			t.Fatalf("单条样本长度=%d，超过上限 %d", got, contactMemorySampleTextLimit)
		}
		totalRunes += len([]rune(sample.Text))
	}
	if contactCount < 3 {
		t.Fatalf("超长用户消息挤掉了联系人样本，只剩 %d 条", contactCount)
	}
	if totalRunes > contactMemorySampleRuneLimit {
		t.Fatalf("样本总长度=%d，超过预算 %d", totalRunes, contactMemorySampleRuneLimit)
	}
}

func TestContactMemoryNeedsRefreshOnlyFromRealSourceChanges(t *testing.T) {
	now := time.Now()
	_, emptyVoiceHash := contactMemoryVoiceTranscriptDigest(nil, nil)
	memory := ContactMemory{
		SchemaVersion: ContactMemorySchemaVersion,
		TargetName:    "小王",
		UpdatedAt:     now,
		Generator: ContactMemoryGenerator{
			Model:         "model-a",
			PromptVersion: contactMemoryPromptVersion,
		},
		Source: ContactMemorySource{
			MaxSeq:               10,
			LastMessageAt:        now.Add(-time.Hour),
			DataVersion:          "data-v1",
			VoiceTranscriptCount: 0,
			VoiceTranscriptHash:  emptyVoiceHash,
		},
	}
	unchanged := []*model.Message{{Seq: 10, Time: now.Add(-time.Hour)}}
	if contactMemoryNeedsRefreshFromMessages(memory, unchanged, "小王", "model-a", "data-v1", 0, emptyVoiceHash) {
		t.Fatal("未变化的真实来源不应触发刷新")
	}
	newMessage := []*model.Message{{Seq: 11, Time: now}}
	if !contactMemoryNeedsRefreshFromMessages(memory, newMessage, "小王", "model-a", "data-v1", 0, emptyVoiceHash) {
		t.Fatal("新真实消息应触发刷新")
	}
	if !contactMemoryNeedsRefreshFromMessages(memory, unchanged, "小王", "model-a", "data-v1", 1, "changed") {
		t.Fatal("语音转写变化应触发刷新")
	}
	if !contactMemoryNeedsRefreshFromMessages(memory, unchanged, "小王", "model-a", "data-v2", 0, emptyVoiceHash) {
		t.Fatal("消息数据库版本变化应触发刷新")
	}
}

func TestContactMemoryVoiceTranscriptDigestOnlyUsesThisContactsVoiceIDs(t *testing.T) {
	transcripts := map[string]string{
		"voice-a": "联系人 A 的转写",
		"voice-b": "联系人 B 的转写",
	}
	lookup := func(id string) string { return transcripts[id] }
	count, before := contactMemoryVoiceTranscriptDigest([]string{"voice-a"}, lookup)
	if count != 1 {
		t.Fatalf("联系人语音转写数量=%d，want 1", count)
	}

	transcripts["voice-b"] = "联系人 B 修改后的转写"
	_, afterOtherContactChanged := contactMemoryVoiceTranscriptDigest([]string{"voice-a"}, lookup)
	if afterOtherContactChanged != before {
		t.Fatal("其他联系人的语音转写不应使当前联系人记忆过期")
	}

	transcripts["voice-a"] = "联系人 A 修改后的转写"
	_, afterThisContactChanged := contactMemoryVoiceTranscriptDigest([]string{"voice-a"}, lookup)
	if afterThisContactChanged == before {
		t.Fatal("当前联系人的语音转写变化应改变摘要")
	}
}

func TestContactMemoryPromptIncludesUserCorrections(t *testing.T) {
	memory := ContactMemory{
		TargetName: "小王",
		Profile:    ContactMemoryProfile{Summary: "说话简短"},
		UserOverrides: ContactMemoryUserOverrides{
			StyleInstructions: []string{"不要使用敬语"},
			PinnedFacts:       []string{"这是用户确认的信息"},
		},
	}
	got := contactMemoryPromptContext(memory)
	for _, want := range []string{"说话简短", "不要使用敬语", "这是用户确认的信息"} {
		if !strings.Contains(got, want) {
			t.Fatalf("运行时记忆缺少 %q: %s", want, got)
		}
	}
}
