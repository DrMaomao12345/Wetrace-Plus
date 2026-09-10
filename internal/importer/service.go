package importer

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	sampleSize                  = 64 * 1024
	maxArchiveEntries           = 10_000
	maxArchiveUncompressedBytes = uint64(2 << 30)
)

type emitFunc func(Message) error

type parseReport struct {
	format   string
	messages int
	warnings []Warning
}

type adapter interface {
	match(name string, sample []byte) bool
	parse(ctx context.Context, name string, r io.Reader, options Options, emit emitFunc) (parseReport, error)
}

type Service struct {
	dataDir  string
	adapters []adapter
}

func New(dataDir string) *Service {
	return &Service{
		dataDir: dataDir,
		adapters: []adapter{
			chatlogKeeperJSONAdapter{},
			jsonAdapter{},
			csvAdapter{},
		},
	}
}

func (s *Service) ImportFiles(ctx context.Context, uploads []Upload, options Options) (*Result, error) {
	if len(uploads) == 0 {
		return nil, errors.New("请选择至少一个 JSON、CSV 或 ZIP 文件")
	}

	options.SelfID = strings.TrimSpace(options.SelfID)
	options.SelfName = strings.TrimSpace(options.SelfName)
	started := time.Now().UTC()
	writer, err := newWriter(s.dataDir, options)
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	result := &Result{Files: len(uploads), StartedAt: started}
	conversationIDs := map[string]struct{}{}

	for _, upload := range uploads {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hash, err := hashFile(upload.Path, options)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败: %w", upload.Name, err)
		}

		sourceStart := len(result.Sources)
		beforeImported := writer.imported
		beforeDuplicates := writer.duplicates
		emit := func(message Message) error {
			message.SourceFile = upload.Name
			if err := writer.Add(ctx, message); err != nil {
				return err
			}
			conversationIDs[message.TalkerID] = struct{}{}
			return nil
		}

		if strings.EqualFold(filepath.Ext(upload.Name), ".zip") {
			reports, err := s.parseZIP(ctx, upload, options, emit)
			if err != nil {
				return nil, err
			}
			result.Sources = append(result.Sources, reports...)
		} else {
			report, err := s.parsePath(ctx, upload.Path, upload.Name, options, emit)
			if err != nil {
				return nil, err
			}
			result.Sources = append(result.Sources, SourceReport{
				File:     upload.Name,
				Format:   report.format,
				Messages: report.messages,
				Warnings: report.warnings,
			})
		}

		newReports := result.Sources[sourceStart:]
		allocatedImported := writer.imported - beforeImported
		allocatedDuplicates := writer.duplicates - beforeDuplicates
		for i := range newReports {
			// The writer owns aggregate duplicate counts. Allocate them to the
			// only report for normal files and proportionally for ZIP entries.
			if len(newReports) == 1 {
				newReports[i].Imported = allocatedImported
				newReports[i].Duplicates = allocatedDuplicates
			}
			result.Messages += newReports[i].Messages
			result.Warnings = append(result.Warnings, newReports[i].Warnings...)
		}
		if len(newReports) > 1 {
			remainingImported := allocatedImported
			remainingDuplicates := allocatedDuplicates
			for i := range newReports {
				if i == len(newReports)-1 {
					newReports[i].Imported = remainingImported
					newReports[i].Duplicates = remainingDuplicates
					break
				}
				share := newReports[i].Messages
				if share > remainingImported {
					share = remainingImported
				}
				newReports[i].Imported = share
				remainingImported -= share
				dupes := newReports[i].Messages - share
				if dupes > remainingDuplicates {
					dupes = remainingDuplicates
				}
				newReports[i].Duplicates = dupes
				remainingDuplicates -= dupes
			}
		}

		formats := make([]string, 0, len(newReports))
		warningCount := 0
		for _, report := range newReports {
			formats = append(formats, report.Format)
			warningCount += len(report.Warnings)
		}
		sort.Strings(formats)
		formats = compactStrings(formats)
		if err := writer.AddHistory(ctx, HistoryItem{
			SourceName:   upload.Name,
			SourceHash:   hash,
			Formats:      strings.Join(formats, ","),
			Messages:     sumMessages(newReports),
			Imported:     allocatedImported,
			Duplicates:   allocatedDuplicates,
			WarningCount: warningCount,
			ImportedAt:   time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
	}

	if result.Messages == 0 {
		return nil, errors.New("文件中没有可导入的聊天消息")
	}
	if err := writer.Commit(); err != nil {
		return nil, err
	}

	result.Conversations = len(conversationIDs)
	result.Imported = writer.imported
	result.Duplicates = writer.duplicates
	result.CompletedAt = time.Now().UTC()
	return result, nil
}

func (s *Service) parsePath(ctx context.Context, path, name string, options Options, emit emitFunc) (parseReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return parseReport{}, fmt.Errorf("打开 %s 失败: %w", name, err)
	}
	defer f.Close()
	return s.parseReader(ctx, name, f, options, emit)
}

func (s *Service) parseReader(ctx context.Context, name string, r io.Reader, options Options, emit emitFunc) (parseReport, error) {
	buffered := bufio.NewReaderSize(r, sampleSize)
	sample, _ := buffered.Peek(sampleSize)
	for _, candidate := range s.adapters {
		if candidate.match(name, sample) {
			report, err := candidate.parse(ctx, name, buffered, options, emit)
			if err != nil {
				return parseReport{}, fmt.Errorf("解析 %s 失败: %w", name, err)
			}
			return report, nil
		}
	}
	return parseReport{}, fmt.Errorf("不支持文件 %s；当前支持 chatlog-keeper JSON、chatlog JSON/CSV、MemoTrace CSV 和 Wetrace Plus JSON", name)
}

func (s *Service) parseZIP(ctx context.Context, upload Upload, options Options, emit emitFunc) ([]SourceReport, error) {
	archive, err := zip.OpenReader(upload.Path)
	if err != nil {
		return nil, fmt.Errorf("打开压缩包 %s 失败: %w", upload.Name, err)
	}
	defer archive.Close()
	if len(archive.File) > maxArchiveEntries {
		return nil, fmt.Errorf("压缩包 %s 包含超过 %d 个文件，已拒绝导入", upload.Name, maxArchiveEntries)
	}

	var reports []SourceReport
	var totalUncompressed uint64
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || strings.HasPrefix(filepath.Base(entry.Name), ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name))
		if ext != ".json" && ext != ".csv" {
			continue
		}
		if entry.UncompressedSize64 > maxArchiveUncompressedBytes {
			return nil, fmt.Errorf("压缩包内文件 %s 超过 2 GiB 安全上限", entry.Name)
		}
		if totalUncompressed > maxArchiveUncompressedBytes-entry.UncompressedSize64 {
			return nil, fmt.Errorf("压缩包 %s 解压后的 JSON/CSV 总量超过 2 GiB 安全上限", upload.Name)
		}
		totalUncompressed += entry.UncompressedSize64
		r, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("读取压缩包内的 %s 失败: %w", entry.Name, err)
		}
		report, parseErr := s.parseReader(ctx, entry.Name, r, options, emit)
		_ = r.Close()
		if parseErr != nil {
			return nil, parseErr
		}
		reports = append(reports, SourceReport{
			File:     upload.Name + "/" + entry.Name,
			Format:   report.format,
			Messages: report.messages,
			Warnings: report.warnings,
		})
	}
	if len(reports) == 0 {
		return nil, fmt.Errorf("压缩包 %s 中没有受支持的 JSON 或 CSV", upload.Name)
	}
	return reports, nil
}

func hashFile(path string, options Options) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.WriteString(h, options.SelfID+"\x00"+options.SelfName+"\x00")
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func sumMessages(reports []SourceReport) int {
	total := 0
	for _, report := range reports {
		total += report.Messages
	}
	return total
}
