package importer

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type csvAdapter struct{}

func (csvAdapter) match(name string, _ []byte) bool {
	return strings.EqualFold(filepath.Ext(name), ".csv")
}

func (csvAdapter) parse(ctx context.Context, name string, r io.Reader, options Options, emit emitFunc) (parseReport, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	header, err := reader.Read()
	if err != nil {
		return parseReport{}, err
	}
	columns := indexHeader(header)

	switch {
	case hasColumns(columns, "time", "sendername", "sender", "talkername", "talker", "content"):
		return parseChatlogCSV(ctx, name, reader, columns, options, emit)
	case hasColumns(columns, "消息id", "类型", "发送人", "时间", "内容"):
		return parseMemoTraceCSV(ctx, name, reader, columns, options, emit)
	case hasColumns(columns, "localid", "talkerid", "type", "issender", "createtime", "strcontent"):
		return parseMemoTraceFullCSV(ctx, name, reader, columns, options, emit)
	default:
		return parseReport{}, fmt.Errorf("无法识别 CSV 表头：%s", strings.Join(header, ", "))
	}
}

func parseChatlogCSV(ctx context.Context, name string, reader *csv.Reader, columns map[string]int, options Options, emit emitFunc) (parseReport, error) {
	report := parseReport{format: FormatChatlogCSV}
	row := 1
	warnedGroupIdentity := false
	for {
		row++
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return report, fmt.Errorf("第 %d 行: %w", row, err)
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		at, err := parseFlexibleTime(valueAt(record, columns, "time"))
		if err != nil {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "invalid_time", Message: err.Error()})
			continue
		}
		talker := strings.TrimSpace(valueAt(record, columns, "talker"))
		if talker == "" {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "missing_talker", Message: "缺少 Talker，已跳过"})
			continue
		}
		sender := strings.TrimSpace(valueAt(record, columns, "sender"))
		senderName := strings.TrimSpace(valueAt(record, columns, "sendername"))
		isGroup := strings.HasSuffix(talker, "@chatroom")
		isSelf := sender != "" && !isGroup && sender != talker
		if identityMatches(options, sender, senderName) {
			isSelf = true
		}
		if isGroup && options.SelfID == "" && options.SelfName == "" && !warnedGroupIdentity {
			report.warnings = append(report.warnings, Warning{File: name, Code: "self_identity_missing", Message: "群聊 CSV 不含发送方向；请填写自己的微信 ID 或昵称后重新导入，以获得准确的收发统计"})
			warnedGroupIdentity = true
		}
		if err := emit(Message{
			Time:       at,
			TalkerID:   talker,
			TalkerName: valueAt(record, columns, "talkername"),
			IsChatRoom: isGroup,
			SenderID:   sender,
			SenderName: senderName,
			IsSelf:     isSelf,
			Type:       1,
			Content:    valueAt(record, columns, "content"),
			SourceFile: name,
			SourceRow:  row,
		}); err != nil {
			return report, err
		}
		report.messages++
	}
	return report, nil
}

func parseMemoTraceCSV(ctx context.Context, name string, reader *csv.Reader, columns map[string]int, options Options, emit emitFunc) (parseReport, error) {
	report := parseReport{format: FormatMemoTraceCSV}
	talkerID, talkerName := memoTraceConversationFromPath(name)
	row := 1
	warnedIdentity := false
	for {
		row++
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return report, fmt.Errorf("第 %d 行: %w", row, err)
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		at, err := parseFlexibleTime(valueAt(record, columns, "时间"))
		if err != nil {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "invalid_time", Message: err.Error()})
			continue
		}
		senderName := strings.TrimSpace(valueAt(record, columns, "发送人"))
		if talkerName == "" {
			talkerName = inferMemoTraceTalkerName(name)
		}
		isGroup := strings.HasSuffix(talkerID, "@chatroom")
		isSelf := identityMatches(options, "", senderName)
		if !isGroup && !isSelf && talkerName != "" && !strings.EqualFold(senderName, talkerName) {
			isSelf = true
		}
		if options.SelfID == "" && options.SelfName == "" && !warnedIdentity {
			report.warnings = append(report.warnings, Warning{File: name, Code: "self_identity_inferred", Message: "该 CSV 没有 IsSender 字段，收发方向按文件名和发送人推断；填写自己的昵称可提高准确度"})
			warnedIdentity = true
		}
		messageID := strings.TrimSpace(valueAt(record, columns, "消息id"))
		messageType := memoTraceType(valueAt(record, columns, "类型"))
		if err := emit(Message{
			ExternalID: messageID,
			Time:       at,
			TalkerID:   talkerID,
			TalkerName: talkerName,
			IsChatRoom: isGroup,
			SenderName: senderName,
			IsSelf:     isSelf,
			Type:       messageType,
			Content:    valueAt(record, columns, "内容"),
			SourceFile: name,
			SourceRow:  row,
		}); err != nil {
			return report, err
		}
		report.messages++
	}
	return report, nil
}

func parseMemoTraceFullCSV(ctx context.Context, name string, reader *csv.Reader, columns map[string]int, options Options, emit emitFunc) (parseReport, error) {
	report := parseReport{format: FormatMemoTraceFullCSV}
	row := 1
	for {
		row++
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return report, fmt.Errorf("第 %d 行: %w", row, err)
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		at, err := parseFlexibleTime(firstValue(record, columns, "strtime", "createtime"))
		if err != nil {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "invalid_time", Message: err.Error()})
			continue
		}
		talker := strings.TrimSpace(valueAt(record, columns, "talkerid"))
		if talker == "" {
			report.warnings = append(report.warnings, Warning{File: name, Row: row, Code: "missing_talker", Message: "缺少 TalkerId，已跳过"})
			continue
		}
		isSelf := parseBool(valueAt(record, columns, "issender"))
		sender := strings.TrimSpace(valueAt(record, columns, "sender"))
		if isSelf && sender == "" {
			sender = options.SelfID
		}
		messageType, _ := strconv.ParseInt(strings.TrimSpace(valueAt(record, columns, "type")), 10, 64)
		subType, _ := strconv.ParseInt(strings.TrimSpace(valueAt(record, columns, "subtype")), 10, 64)
		seq, _ := strconv.ParseInt(strings.TrimSpace(valueAt(record, columns, "localid")), 10, 64)
		if err := emit(Message{
			ExternalID: valueAt(record, columns, "localid"),
			Seq:        seq,
			Time:       at,
			TalkerID:   talker,
			TalkerName: firstValue(record, columns, "remark", "nickname"),
			IsChatRoom: strings.HasSuffix(talker, "@chatroom"),
			SenderID:   sender,
			SenderName: valueAt(record, columns, "sender"),
			IsSelf:     isSelf,
			Type:       messageType,
			SubType:    subType,
			Content:    valueAt(record, columns, "strcontent"),
			SourceFile: name,
			SourceRow:  row,
		}); err != nil {
			return report, err
		}
		report.messages++
	}
	return report, nil
}

func indexHeader(header []string) map[string]int {
	result := make(map[string]int, len(header))
	for index, value := range header {
		key := normalizeHeader(value)
		if key != "" {
			result[key] = index
		}
	}
	return result
}

func normalizeHeader(value string) string {
	value = strings.TrimPrefix(value, "\ufeff")
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", " ", "", "-", "")
	return replacer.Replace(value)
}

func hasColumns(columns map[string]int, required ...string) bool {
	for _, key := range required {
		if _, ok := columns[normalizeHeader(key)]; !ok {
			return false
		}
	}
	return true
}

func valueAt(record []string, columns map[string]int, key string) string {
	index, ok := columns[normalizeHeader(key)]
	if !ok || index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func firstValue(record []string, columns map[string]int, keys ...string) string {
	for _, key := range keys {
		if value := valueAt(record, columns, key); value != "" {
			return value
		}
	}
	return ""
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "是", "发送":
		return true
	default:
		return false
	}
}

func identityMatches(options Options, senderID, senderName string) bool {
	return options.SelfID != "" && strings.EqualFold(strings.TrimSpace(senderID), options.SelfID) ||
		options.SelfName != "" && strings.EqualFold(strings.TrimSpace(senderName), options.SelfName)
}

func memoTraceConversationFromPath(name string) (string, string) {
	clean := filepath.ToSlash(name)
	parts := strings.Split(clean, "/")
	candidates := make([]string, 0, len(parts))
	if len(parts) > 1 {
		candidates = append(candidates, parts[len(parts)-2])
	}
	candidates = append(candidates, strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1])))
	for _, candidate := range candidates {
		open := strings.LastIndex(candidate, "(")
		close := strings.LastIndex(candidate, ")")
		if open > 0 && close > open+1 {
			return strings.TrimSpace(candidate[open+1 : close]), strings.TrimSpace(candidate[:open])
		}
	}
	nameOnly := inferMemoTraceTalkerName(name)
	return importedID("talker", nameOnly), nameOnly
}

func inferMemoTraceTalkerName(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	for _, suffix := range []string{"_utf8", "-utf8", "_聊天记录", "-聊天记录"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return strings.TrimSpace(base)
}

func memoTraceType(label string) int64 {
	label = strings.ToLower(strings.TrimSpace(label))
	if numeric, err := strconv.ParseInt(label, 10, 64); err == nil && numeric > 0 {
		return numeric
	}
	switch {
	case strings.Contains(label, "图片"):
		return 3
	case strings.Contains(label, "语音") && !strings.Contains(label, "通话"):
		return 34
	case strings.Contains(label, "视频") && !strings.Contains(label, "通话"):
		return 43
	case strings.Contains(label, "表情"):
		return 47
	case strings.Contains(label, "位置"):
		return 48
	case strings.Contains(label, "分享"), strings.Contains(label, "文件"), strings.Contains(label, "链接"):
		return 49
	case strings.Contains(label, "通话"):
		return 50
	case strings.Contains(label, "系统"):
		return 10000
	default:
		return 1
	}
}

func unixFromMaybeMilliseconds(value string) time.Time {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if n > 1_000_000_000_000 {
		n /= 1000
	}
	return time.Unix(n, 0)
}
