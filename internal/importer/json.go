package importer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type chatlogKeeperJSONAdapter struct{}

func (chatlogKeeperJSONAdapter) match(name string, sample []byte) bool {
	if !strings.EqualFold(filepath.Ext(name), ".json") {
		return false
	}
	trimmed := bytes.TrimSpace(sample)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return false
	}
	// chatlog-keeper writes keys in this order, so all identity fields are
	// available near the beginning even when the first message is very large.
	return bytes.Contains(trimmed, []byte(`"ts"`)) &&
		bytes.Contains(trimmed, []byte(`"sender_wxid"`)) &&
		bytes.Contains(trimmed, []byte(`"conversation_id"`)) &&
		bytes.Contains(trimmed, []byte(`"account_id"`))
}

type chatlogKeeperMessage struct {
	TS               json.RawMessage `json:"ts"`
	Sender           string          `json:"sender"`
	SenderWXID       string          `json:"sender_wxid"`
	ChatRoom         string          `json:"chat_room"`
	ConversationID   string          `json:"conversation_id"`
	Content          string          `json:"content"`
	MsgType          int64           `json:"msg_type"`
	IsSelf           bool            `json:"is_self"`
	AccountID        string          `json:"account_id"`
	ThreadID         string          `json:"thread_id"`
	ServerID         json.RawMessage `json:"server_id"`
	SourceOffset     string          `json:"source_offset"`
	ConversationType string          `json:"conversation_type"`
	IsGroupChat      bool            `json:"is_group_chat"`
}

func (chatlogKeeperJSONAdapter) parse(ctx context.Context, name string, r io.Reader, options Options, emit emitFunc) (parseReport, error) {
	decoder := json.NewDecoder(r)
	token, err := decoder.Token()
	if err != nil {
		return parseReport{}, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return parseReport{}, errors.New("chatlog-keeper JSON 顶层必须是数组")
	}

	report := parseReport{format: FormatChatlogKeeperJSON}
	row := 0
	for decoder.More() {
		row++
		if err := ctx.Err(); err != nil {
			return report, err
		}
		var raw chatlogKeeperMessage
		if err := decoder.Decode(&raw); err != nil {
			return report, fmt.Errorf("第 %d 条消息: %w", row, err)
		}
		at, err := parseJSONTime(raw.TS)
		if err != nil {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "invalid_time", Message: err.Error()})
			continue
		}
		conversationID := strings.TrimSpace(raw.ConversationID)
		if conversationID == "" {
			conversationID = conversationIDFromThread(raw.ThreadID, raw.AccountID)
		}
		if conversationID == "" {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "missing_conversation", Message: "缺少 conversation_id，已跳过"})
			continue
		}
		talkerID := strings.TrimSpace(raw.ThreadID)
		if talkerID == "" {
			talkerID = conversationID
			if accountID := strings.TrimSpace(raw.AccountID); accountID != "" {
				talkerID = accountID + "::" + conversationID
			}
		}
		senderID := strings.TrimSpace(raw.SenderWXID)
		if raw.IsSelf && senderID == "" {
			senderID = firstNonEmpty(raw.AccountID, options.SelfID)
		}
		externalID := jsonScalarString(raw.ServerID)
		if externalID == "" {
			externalID = strings.TrimSpace(raw.SourceOffset)
		}
		message := Message{
			ExternalID: externalID,
			Time:       at,
			TalkerID:   talkerID,
			TalkerName: firstNonEmpty(raw.ChatRoom, conversationID),
			IsChatRoom: raw.IsGroupChat || strings.EqualFold(raw.ConversationType, "group") || strings.HasSuffix(conversationID, "@chatroom"),
			SenderID:   senderID,
			SenderName: firstNonEmpty(raw.Sender, senderID),
			IsSelf:     raw.IsSelf,
			Type:       raw.MsgType,
			Content:    raw.Content,
			SourceFile: name,
			SourceRow:  row,
		}
		if err := emit(message); err != nil {
			return report, err
		}
		report.messages++
	}
	_, err = decoder.Token()
	return report, err
}

type jsonAdapter struct{}

func (jsonAdapter) match(name string, sample []byte) bool {
	if !strings.EqualFold(filepath.Ext(name), ".json") {
		return false
	}
	trimmed := bytes.TrimSpace(sample)
	return len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{')
}

type chatlogMessage struct {
	Seq        int64           `json:"seq"`
	Time       json.RawMessage `json:"time"`
	Talker     string          `json:"talker"`
	TalkerName string          `json:"talkerName"`
	IsChatRoom bool            `json:"isChatRoom"`
	Sender     string          `json:"sender"`
	SenderName string          `json:"senderName"`
	IsSelf     bool            `json:"isSelf"`
	Type       int64           `json:"type"`
	SubType    int64           `json:"subType"`
	Content    string          `json:"content"`
}

type plusArchive struct {
	Format        string             `json:"format"`
	Version       int                `json:"version"`
	Account       plusAccount        `json:"account"`
	Conversations []plusConversation `json:"conversations"`
}

type plusAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type plusConversation struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	Messages []plusMessage `json:"messages"`
}

type plusMessage struct {
	ID         string          `json:"id"`
	Seq        int64           `json:"seq"`
	Time       json.RawMessage `json:"time"`
	Timestamp  json.RawMessage `json:"timestamp"`
	SenderID   string          `json:"sender_id"`
	SenderName string          `json:"sender_name"`
	Direction  string          `json:"direction"`
	IsSelf     *bool           `json:"is_self"`
	Type       int64           `json:"type"`
	SubType    int64           `json:"subtype"`
	Content    string          `json:"content"`
}

func (jsonAdapter) parse(ctx context.Context, name string, r io.Reader, options Options, emit emitFunc) (parseReport, error) {
	reader := bufio.NewReader(r)
	first, err := firstNonSpace(reader)
	if err != nil {
		return parseReport{}, err
	}
	if err := reader.UnreadByte(); err != nil {
		return parseReport{}, err
	}

	if first == '[' {
		return parseChatlogJSONArray(ctx, name, reader, options, emit)
	}
	if first != '{' {
		return parseReport{}, errors.New("JSON 顶层必须是数组或对象")
	}

	var archive plusArchive
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&archive); err != nil {
		return parseReport{}, err
	}
	if !strings.EqualFold(archive.Format, "wetrace-plus") || archive.Version != 1 {
		return parseReport{}, errors.New("无法识别 JSON 格式：对象格式需要 format=wetrace-plus 且 version=1")
	}
	if options.SelfID == "" {
		options.SelfID = archive.Account.ID
	}
	if options.SelfName == "" {
		options.SelfName = archive.Account.Name
	}

	report := parseReport{format: FormatWetracePlusJSON}
	for _, conversation := range archive.Conversations {
		isGroup := strings.EqualFold(conversation.Type, "group") || strings.HasSuffix(conversation.ID, "@chatroom")
		for index, raw := range conversation.Messages {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			at, err := parseJSONTime(firstRaw(raw.Time, raw.Timestamp))
			if err != nil {
				report.warnings = append(report.warnings, Warning{File: name, Row: index + 1, Code: "invalid_time", Message: err.Error()})
				continue
			}
			isSelf := strings.EqualFold(raw.Direction, "outgoing") || strings.EqualFold(raw.Direction, "sent")
			if raw.IsSelf != nil {
				isSelf = *raw.IsSelf
			}
			message := Message{
				ExternalID: raw.ID,
				Seq:        raw.Seq,
				Time:       at,
				TalkerID:   conversation.ID,
				TalkerName: conversation.Name,
				IsChatRoom: isGroup,
				SenderID:   raw.SenderID,
				SenderName: raw.SenderName,
				IsSelf:     isSelf,
				Type:       raw.Type,
				SubType:    raw.SubType,
				Content:    raw.Content,
				SourceFile: name,
				SourceRow:  index + 1,
			}
			if message.IsSelf {
				if message.SenderID == "" {
					message.SenderID = options.SelfID
				}
				if message.SenderName == "" {
					message.SenderName = options.SelfName
				}
			}
			if err := emit(message); err != nil {
				return report, err
			}
			report.messages++
		}
	}
	return report, nil
}

func parseChatlogJSONArray(ctx context.Context, name string, r io.Reader, options Options, emit emitFunc) (parseReport, error) {
	decoder := json.NewDecoder(r)
	token, err := decoder.Token()
	if err != nil {
		return parseReport{}, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return parseReport{}, errors.New("chatlog JSON 顶层必须是数组")
	}

	report := parseReport{format: FormatChatlogJSON}
	row := 0
	for decoder.More() {
		row++
		if err := ctx.Err(); err != nil {
			return report, err
		}
		var raw chatlogMessage
		if err := decoder.Decode(&raw); err != nil {
			return report, fmt.Errorf("第 %d 条消息: %w", row, err)
		}
		at, err := parseJSONTime(raw.Time)
		if err != nil {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "invalid_time", Message: err.Error()})
			continue
		}
		if raw.Talker == "" {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "missing_talker", Message: "缺少 talker，已跳过"})
			continue
		}
		message := Message{
			ExternalID: strconv.FormatInt(raw.Seq, 10),
			Seq:        raw.Seq,
			Time:       at,
			TalkerID:   raw.Talker,
			TalkerName: raw.TalkerName,
			IsChatRoom: raw.IsChatRoom || strings.HasSuffix(raw.Talker, "@chatroom"),
			SenderID:   raw.Sender,
			SenderName: raw.SenderName,
			IsSelf:     raw.IsSelf,
			Type:       raw.Type,
			SubType:    raw.SubType,
			Content:    raw.Content,
			SourceFile: name,
			SourceRow:  row,
		}
		if message.IsSelf {
			if message.SenderID == "" {
				message.SenderID = options.SelfID
			}
			if message.SenderName == "" {
				message.SenderName = options.SelfName
			}
		}
		if err := emit(message); err != nil {
			return report, err
		}
		report.messages++
	}
	_, err = decoder.Token()
	return report, err
}

func firstNonSpace(reader *bufio.Reader) (byte, error) {
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		if !bytes.ContainsRune([]byte(" \t\r\n"), rune(b)) {
			return b, nil
		}
	}
}

func firstRaw(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(bytes.TrimSpace(value)) > 0 && string(bytes.TrimSpace(value)) != "null" {
			return value
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func conversationIDFromThread(threadID, accountID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ""
	}
	prefix := strings.TrimSpace(accountID) + "::"
	if accountID != "" && strings.HasPrefix(threadID, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(threadID, prefix))
	}
	if index := strings.LastIndex(threadID, "::"); index >= 0 {
		return strings.TrimSpace(threadID[index+2:])
	}
	return ""
}

func jsonScalarString(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		return number.String()
	}
	return ""
}

func parseJSONTime(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 {
		return time.Time{}, errors.New("缺少消息时间")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return parseFlexibleTime(text)
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return time.Time{}, fmt.Errorf("无法解析时间 %s", string(raw))
	}
	value, err := number.Float64()
	if err != nil {
		return time.Time{}, fmt.Errorf("无法解析时间 %s", string(raw))
	}
	return unixFloatTime(value)
}

func parseFlexibleTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("消息时间为空")
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		return unixFloatTime(float64(unix))
	}
	if unix, err := strconv.ParseFloat(value, 64); err == nil {
		return unixFloatTime(unix)
	}
	formats := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
	}
	for _, format := range formats {
		if parsed, err := time.ParseInLocation(format, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法识别时间 %q", value)
}

func unixFloatTime(value float64) (time.Time, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return time.Time{}, errors.New("消息时间不是有效数字")
	}
	if math.Abs(value) > 1_000_000_000_000 {
		value /= 1000
	}
	seconds := int64(value)
	nanoseconds := int64(math.Round((value - float64(seconds)) * float64(time.Second)))
	return time.Unix(seconds, nanoseconds), nil
}
