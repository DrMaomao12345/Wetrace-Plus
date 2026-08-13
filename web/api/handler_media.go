package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/repo"
	"github.com/afumu/wetrace/store/types"
	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// GetMedia 处理媒体文件（如图片、视频、语音等）的请求。
func (a *API) GetMedia(c *gin.Context) {
	mediaType := c.Param("type")
	key := c.Param("key")
	path := c.Query("path")
	isThumb := c.Query("thumb") == "1"

	if mediaType == "" || key == "" {
		transport.BadRequest(c, "媒体类型和 key 是必需的。")
		return
	}

	// 1. 从 store 获取媒体元数据
	mediaInfo, err := a.Store.GetMedia(c.Request.Context(), mediaType, key)
	if err != nil {
		// 如果提供了 path，我们可以创建一个虚拟的 mediaInfo 继续处理
		if path != "" {
			mediaInfo = &model.Media{
				Type: mediaType,
				Key:  key,
				Path: path,
			}
		} else {
			log.Warn().Err(err).Str("type", mediaType).Str("key", key).Msg("从 store 获取媒体失败")
			transport.NotFound(c, "未找到媒体文件。")
			return
		}
	} else if path != "" {
		// 如果数据库中有数据，但前端传了 path，以传参为准
		mediaInfo.Path = path
	}

	// 2. 使用媒体服务准备内容
	preparedMedia := a.Media.Prepare(mediaInfo, isThumb)

	// 3. 发送响应
	transport.SendMedia(c, preparedMedia)
}

// GetEmoji 处理表情包的下载和解密请求。
func (a *API) GetEmoji(c *gin.Context) {
	url := c.Query("url")
	key := c.Query("key")

	if url == "" || key == "" {
		transport.BadRequest(c, "url 和 key 参数是必需的。")
		return
	}

	// 调用 Media Service 进行下载和解密
	preparedMedia := a.Media.DownloadAndDecryptEmoji(url, key)

	// 发送响应
	transport.SendMedia(c, preparedMedia)
}

// HandleStartCache 启动图片缓存预加载任务
// HandleStopCache 中断正在进行的图片预加载
func (a *API) HandleStopCache(c *gin.Context) {
	if err := a.Media.StopCacheTask(); err != nil {
		transport.BadRequest(c, err.Error())
		return
	}
	transport.SendSuccess(c, gin.H{"status": "stopping"})
}

func (a *API) HandleStartCache(c *gin.Context) {
	var req struct {
		Scope  string `json:"scope"`  // "all" 或 "session"
		Talker string `json:"talker"` // 仅当 scope 为 session 时需要
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "无效的请求参数")
		return
	}

	err := a.Media.StartCacheTask(req.Scope, req.Talker)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}

	transport.SendSuccess(c, "任务已启动")
}

// GetCacheStatus 获取当前缓存任务的进度
func (a *API) GetCacheStatus(c *gin.Context) {
	status := a.Media.GetCacheStatus()
	transport.SendSuccess(c, status)
}

// imageListQuery 图片列表请求参数
type imageListQuery struct {
	Talker    string `form:"talker"`
	TimeRange string `form:"time_range"`
	Limit     int    `form:"limit,default=50"`
	Offset    int    `form:"offset,default=0"`
}

// imageListItem 图片列表响应项
type imageListItem struct {
	Key          string `json:"key"`
	Talker       string `json:"talker"`
	TalkerName   string `json:"talkerName"`
	Time         string `json:"time"`
	ThumbnailURL string `json:"thumbnailUrl"`
	FullURL      string `json:"fullUrl"`
	Seq          int64  `json:"seq"`
	// Encrypted 为真表示这张图是加密存储的，当前解不出来（macOS 上 2025-05 之后的图片）
	Encrypted bool `json:"encrypted"`
}

// resolveTalkerFilter 把用户输入解析成「精确会话 ID」或「模糊名字过滤」。
// 输入正好等于某个会话 ID 时走精确匹配，只扫那一张表；
// 否则当作名字关键词，扫全部再按名字过滤。
func resolveTalkerFilter(input string, nameOf map[string]string) (talker, nameFilter string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ""
	}
	if _, ok := nameOf[input]; ok {
		return input, ""
	}
	// 名字精确命中唯一一个会话时，也能走精确路径
	var hit string
	var n int
	lower := strings.ToLower(input)
	for id, name := range nameOf {
		if strings.EqualFold(name, input) {
			hit = id
			n++
		}
	}
	if n == 1 {
		return hit, ""
	}
	return "", lower
}

// talkerNameMap 会话 ID → 展示名
func (a *API) talkerNameMap(ctx context.Context) map[string]string {
	out := map[string]string{}
	sessions, err := a.Store.GetSessions(ctx, types.SessionQuery{Limit: 0})
	if err != nil {
		return out
	}
	for _, s := range sessions {
		if s.NickName != "" {
			out[s.UserName] = s.NickName
		}
	}
	return out
}

// imageListResponse 图片列表响应
type imageListResponse struct {
	Total int              `json:"total"`
	Items []*imageListItem `json:"items"`
}

// GetImageList 获取图片列表，支持按会话筛选和时间范围筛选。
func (a *API) GetImageList(c *gin.Context) {
	var q imageListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		transport.BadRequest(c, "无效的请求参数: "+err.Error())
		return
	}

	// 解析时间范围
	var startTime, endTime time.Time
	startTime, endTime = parseImageTimeRange(q.TimeRange)

	// 直接扫消息表拿图片清单（按时间倒序）。
	// 早先走 GetMessages：不传 talker 时它要求必须有 talker，于是返回空、
	// 悄悄回退到「扫缓存目录」—— 结果列表变成缓存文件、时间成了文件修改时间，
	// 时间筛选和排序全都失效。
	nameOf := a.talkerNameMap(c.Request.Context())

	// 筛选框既接受会话 ID，也接受昵称/备注 —— 没人记得住 wxid。
	// 能唯一定位到一个会话时按 ID 精确查（快），否则退化成对结果按名字过滤。
	scanTalker, nameFilter := resolveTalkerFilter(q.Talker, nameOf)

	refs := a.Store.ListImageMessages(c.Request.Context(), scanTalker, startTime, endTime)

	allItems := make([]*imageListItem, 0, len(refs))
	for _, ref := range refs {
		if nameFilter != "" {
			hay := strings.ToLower(nameOf[ref.Talker] + " " + ref.Talker)
			if !strings.Contains(hay, nameFilter) {
				continue
			}
		}
		thumbnailURL := fmt.Sprintf("/api/v1/media/image/%s?thumb=1", ref.MD5)
		fullURL := fmt.Sprintf("/api/v1/media/image/%s", ref.MD5)
		name := nameOf[ref.Talker]
		if name == "" {
			name = ref.Talker
		}
		allItems = append(allItems, &imageListItem{
			Key:          ref.MD5,
			Talker:       ref.Talker,
			TalkerName:   name,
			Time:         ref.Time.Format(time.RFC3339),
			ThumbnailURL: thumbnailURL,
			FullURL:      fullURL,
			Seq:          ref.Seq,
		})
	}

	// 当数据库查询结果为空时，扫描本地缓存目录获取图片列表
	if len(allItems) == 0 {
		cacheItems := a.scanCacheImages(q.Talker)
		allItems = append(allItems, cacheItems...)
	}

	total := len(allItems)

	// 分页
	start := q.Offset
	if start > total {
		start = total
	}
	end := start + q.Limit
	if end > total {
		end = total
	}
	pageItems := allItems[start:end]

	transport.SendSuccess(c, imageListResponse{
		Total: total,
		Items: pageItems,
	})
}

// parseImageTimeRange 将前端传入的时间范围字符串转换为起止时间。
func parseImageTimeRange(timeRange string) (start, end time.Time) {
	now := time.Now()
	end = now.Add(24 * time.Hour)

	switch timeRange {
	case "last_week":
		start = now.AddDate(0, 0, -7)
	case "last_month":
		start = now.AddDate(0, -1, 0)
	case "last_year":
		start = now.AddDate(-1, 0, 0)
	default:
		// "all" 或空值，查询全部
		start = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return
}

// cacheImageExtensions 缓存目录中可能包含的图片文件扩展名。
// 缓存文件保留原始 .dat 扩展名，也可能是已解码的图片格式。
var cacheImageExtensions = map[string]bool{
	".dat": true,
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".bmp": true, ".webp": true,
}

// scanCacheImages 扫描本地缓存目录获取图片列表。
// 当数据库中没有图片消息记录时，作为回退方案使用。
func (a *API) scanCacheImages(talker string) []*imageListItem {
	cacheBaseDir := filepath.Join(a.Media.DataDir, "cache", "images", "msg", "attach")

	// 检查缓存目录是否存在
	if _, err := os.Stat(cacheBaseDir); os.IsNotExist(err) {
		log.Debug().Str("dir", cacheBaseDir).Msg("缓存图片目录不存在，跳过扫描")
		return nil
	}

	// 确定要扫描的目录列表
	scanDirs := a.getCacheScanDirs(cacheBaseDir, talker)
	if len(scanDirs) == 0 {
		return nil
	}

	// 遍历目录收集图片文件
	var items []*imageListItem
	for _, dir := range scanDirs {
		dirItems := a.scanSingleCacheDir(dir, cacheBaseDir)
		items = append(items, dirItems...)
	}

	log.Info().Int("count", len(items)).Msg("从缓存目录扫描到图片")
	return items
}

// getCacheScanDirs 根据 talker 参数确定需要扫描的目录列表。
func (a *API) getCacheScanDirs(cacheBaseDir, talker string) []string {
	if talker != "" {
		// 按会话筛选：计算 talker 的 md5 作为子目录名
		h := fmt.Sprintf("%x", md5Sum([]byte(talker)))
		targetDir := filepath.Join(cacheBaseDir, h)
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			return nil
		}
		return []string{targetDir}
	}

	// 全量模式：扫描 attach 下所有子目录
	entries, err := os.ReadDir(cacheBaseDir)
	if err != nil {
		log.Error().Err(err).Msg("读取缓存 attach 目录失败")
		return nil
	}

	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(cacheBaseDir, entry.Name()))
		}
	}
	return dirs
}

// scanSingleCacheDir 扫描单个缓存子目录中的图片文件。
func (a *API) scanSingleCacheDir(dir, cacheBaseDir string) []*imageListItem {
	var items []*imageListItem

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !cacheImageExtensions[ext] {
			return nil
		}

		item := a.buildCacheImageItem(path, cacheBaseDir, info)
		if item != nil {
			items = append(items, item)
		}
		return nil
	})

	return items
}

// buildCacheImageItem 根据缓存文件路径构造 imageListItem。
func (a *API) buildCacheImageItem(path, cacheBaseDir string, info os.FileInfo) *imageListItem {
	fileName := info.Name()

	// 跳过缩略图文件（以 _t.dat 结尾），避免重复
	if strings.HasSuffix(strings.ToLower(fileName), "_t.dat") {
		return nil
	}

	// 计算相对于 cacheBaseDir 的路径
	relPath, err := filepath.Rel(cacheBaseDir, path)
	if err != nil {
		return nil
	}

	// 使用文件名（不含扩展名）作为 key
	baseName := strings.TrimSuffix(fileName, filepath.Ext(fileName))

	// 从相对路径中提取 talker hash（第一级目录）
	parts := strings.SplitN(filepath.ToSlash(relPath), "/", 2)
	talkerHash := ""
	if len(parts) > 0 {
		talkerHash = parts[0]
	}

	// 构造 path 参数（相对于 WechatDbSrcPath）。
	// 缓存文件镜像源文件路径，扩展名可能是 .dat 或已解码的图片格式。
	// prepareImageWithFallback 会尝试 path+".dat"、path、path+"_t.dat"，
	// 所以这里去掉扩展名，让它自动匹配。
	datRelPath := filepath.ToSlash(filepath.Join("msg", "attach", relPath))
	datRelPath = strings.TrimSuffix(datRelPath, filepath.Ext(datRelPath))

	// 构造缩略图 URL，使用 path 参数让 GetMedia 能找到文件
	thumbnailURL := fmt.Sprintf("/api/v1/media/image/%s?thumb=1&path=%s",
		url.PathEscape(baseName),
		url.QueryEscape(datRelPath),
	)
	fullURL := fmt.Sprintf("/api/v1/media/image/%s?path=%s",
		url.PathEscape(baseName),
		url.QueryEscape(datRelPath),
	)

	return &imageListItem{
		Key:          baseName,
		Talker:       talkerHash,
		TalkerName:   talkerHash,
		Time:         info.ModTime().Format(time.RFC3339),
		ThumbnailURL: thumbnailURL,
		FullURL:      fullURL,
		Seq:          info.ModTime().UnixMilli(),
	}
}

// md5Sum 计算字节数组的 MD5 哈希值。
func md5Sum(data []byte) [16]byte {
	return md5.Sum(data)
}

// GetVoiceTranscript 查询已缓存的语音转文字结果
func (a *API) GetVoiceTranscript(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		transport.BadRequest(c, "id 不能为空")
		return
	}
	if a.Transcripts == nil {
		transport.SendSuccess(c, gin.H{"text": nil, "cached": false})
		return
	}
	if text, ok := a.Transcripts.Get(id); ok {
		transport.SendSuccess(c, gin.H{"text": text, "cached": true})
		return
	}
	transport.SendSuccess(c, gin.H{"text": nil, "cached": false})
}

// TranscribeVoice 语音转文字（先查缓存，命中直接返回；未命中则识别并永久保存）
func (a *API) TranscribeVoice(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		transport.BadRequest(c, "参数错误")
		return
	}

	// 先查缓存
	if a.Transcripts != nil {
		if cached, ok := a.Transcripts.Get(req.ID); ok {
			transport.SendSuccess(c, gin.H{"text": cached, "cached": true})
			return
		}
	}

	if a.TTS == nil {
		transport.BadRequest(c, "语音转文字功能未启用，请先在设置中配置")
		return
	}

	// 获取语音媒体信息
	mediaInfo, err := a.Store.GetMedia(c.Request.Context(), "voice", req.ID)
	if err != nil {
		transport.NotFound(c, "未找到语音文件")
		return
	}

	// 准备语音内容
	prepared := a.Media.Prepare(mediaInfo, false)
	if prepared.Error != nil || len(prepared.Content) == 0 {
		transport.InternalServerError(c, "无法读取语音文件")
		return
	}

	// 识别
	text, err := a.TTS.Transcribe(prepared.Content, "voice.mp3")
	if err != nil {
		log.Error().Err(err).Str("id", req.ID).Msg("语音转文字失败")
		transport.InternalServerError(c, "语音转文字失败: "+err.Error())
		return
	}

	// 永久缓存
	if a.Transcripts != nil {
		_ = a.Transcripts.Set(req.ID, text)
	}

	transport.SendSuccess(c, gin.H{"text": text, "cached": false})
}

// TranscribeSession 启动后台任务：批量将会话中所有语音消息转文字并缓存
func (a *API) TranscribeSession(c *gin.Context) {
	// talker 留空表示「全部会话」—— 用于一次性把历史语音全部转写出来
	var req struct {
		Talker string `json:"talker"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}

	a.mu.Lock()
	if a.batchJob != nil && a.batchJob.Running {
		a.mu.Unlock()
		transport.BadRequest(c, "已有转文字任务在进行中")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.batchJob = &BatchTranscribeJob{Talker: req.Talker, Running: true, cancel: cancel}
	a.mu.Unlock()

	go a.runBatchTranscribe(ctx, req.Talker)

	transport.SendSuccess(c, gin.H{"message": "转文字任务已启动"})
}

// runBatchTranscribe 把指定会话（talker 为空表示全部会话）里尚未转写的语音
// 逐条转成文字并落盘。调用方需要先把 batchJob 标记为 Running。
//
// 内存上刻意做了约束：不再把每个会话的全部消息载入内存筛语音，
// 而是直接扫消息表只取语音的 server_id（几万条也就几 MB）。
// 每转完一条立刻落盘，中途被打断也不会丢进度。
func (a *API) runBatchTranscribe(ctx context.Context, talker string) {
	finish := func() {
		a.mu.Lock()
		if a.batchJob != nil {
			a.batchJob.Running = false
			a.batchJob.CurrentTalker = ""
			a.batchJob.CurrentName = ""
		}
		a.mu.Unlock()
	}

	if a.TTS == nil {
		log.Warn().Msg("批量语音转文字：未配置识别服务，任务取消")
		finish()
		return
	}

	// 1. 枚举语音（直接扫表，不走 GetMessages）
	refs := a.Store.ListVoiceMessages(ctx, talker)

	// 2. 过滤掉不需要跑 Whisper 的：微信自带转写、已缓存
	pending := make([]*repo.VoiceRef, 0, len(refs))
	skipped := 0
	for _, r := range refs {
		if r.WeChatTx != "" {
			skipped++
			continue
		}
		if a.Transcripts != nil {
			if t, ok := a.Transcripts.Get(r.VoiceID); ok && t != "" {
				skipped++
				continue
			}
		}
		pending = append(pending, r)
	}
	refs = nil // 尽早释放

	names := a.talkerNameMap(ctx)

	a.mu.Lock()
	if a.batchJob != nil {
		a.batchJob.Total = len(pending)
		a.batchJob.Skipped = skipped
	}
	a.mu.Unlock()

	log.Info().Int("待转写", len(pending)).Int("已跳过", skipped).Msg("批量语音转文字开始")

	for _, item := range pending {
		if ctx.Err() != nil {
			log.Info().Msg("批量语音转文字：已中断，进度已保存")
			finish()
			return
		}

		a.mu.Lock()
		if a.batchJob != nil {
			a.batchJob.CurrentTalker = item.Talker
			a.batchJob.CurrentName = names[item.Talker]
			if a.batchJob.CurrentName == "" {
				a.batchJob.CurrentName = item.Talker
			}
		}
		a.mu.Unlock()

		text, err := a.transcribeOne(ctx, item.VoiceID)
		a.mu.Lock()
		if a.batchJob != nil {
			if err != nil {
				a.batchJob.Errors++
			} else if text != "" {
				a.batchJob.LastText = text
			}
			a.batchJob.Done++
		}
		a.mu.Unlock()
	}

	log.Info().Msg("批量语音转文字完成")
	finish()
}

// transcribeOne 转写单条语音并落盘。每条都立即写文件，
// 这样任务被中断或进程退出时已完成的部分都不会丢。
func (a *API) transcribeOne(ctx context.Context, voiceID string) (string, error) {
	mediaInfo, err := a.Store.GetMedia(ctx, "voice", voiceID)
	if err != nil {
		return "", err
	}
	prepared := a.Media.Prepare(mediaInfo, false)
	if prepared.Error != nil || len(prepared.Content) == 0 {
		return "", fmt.Errorf("读取语音失败")
	}
	text, err := a.TTS.Transcribe(prepared.Content, "voice.mp3")
	if err != nil {
		log.Error().Err(err).Str("id", voiceID).Msg("批量语音转文字失败")
		return "", err
	}
	if a.Transcripts != nil {
		_ = a.Transcripts.Set(voiceID, text)
	}
	return text, nil
}

// StopTranscribeSession 中断批量转写；已完成的部分已经落盘，不会丢。
func (a *API) StopTranscribeSession(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.batchJob == nil || !a.batchJob.Running || a.batchJob.cancel == nil {
		transport.BadRequest(c, "当前没有正在进行的转写任务")
		return
	}
	a.batchJob.Canceled = true
	a.batchJob.cancel()
	transport.SendSuccess(c, gin.H{"status": "stopping"})
}

// GetTranscribeSessionStatus 查询批量转文字任务进度
func (a *API) GetTranscribeSessionStatus(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.batchJob == nil {
		transport.SendSuccess(c, gin.H{"running": false, "total": 0, "done": 0, "errors": 0, "talker": ""})
		return
	}
	transport.SendSuccess(c, gin.H{
		"running":        a.batchJob.Running,
		"total":          a.batchJob.Total,
		"done":           a.batchJob.Done,
		"errors":         a.batchJob.Errors,
		"talker":         a.batchJob.Talker,
		"skipped":        a.batchJob.Skipped,
		"canceled":       a.batchJob.Canceled,
		"current_talker": a.batchJob.CurrentTalker,
		"current_name":   a.batchJob.CurrentName,
		"last_text":      a.batchJob.LastText,
	})
}

// ExportVoicesRequest 语音导出请求体
type ExportVoicesRequest struct {
	Talker string   `json:"talker"`
	Name   string   `json:"name"`
	IDs    []string `json:"ids"`
}

// ExportVoices 一键导出会话中的所有语音消息为 ZIP 包（每条语音为独立 MP3 文件）
func (a *API) ExportVoices(c *gin.Context) {
	var req ExportVoicesRequest

	// 尝试解析 JSON (针对 POST)
	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn().Err(err).Msg("解析导出语音 JSON 请求失败")
		}
	}

	// 如果 JSON 没解析到，或者通过 GET 访问，则从 Query 获取
	if req.Talker == "" {
		req.Talker = c.Query("talker")
	}
	if req.Name == "" {
		req.Name = c.Query("name")
	}

	if req.Talker == "" {
		transport.BadRequest(c, "talker 参数不能为空")
		return
	}
	if req.Name == "" {
		req.Name = req.Talker
	}

	var messages []*model.Message

	// 如果前端传了具体的 IDs，我们直接构造虚拟消息来复用导出逻辑
	if len(req.IDs) > 0 {
		log.Info().Str("talker", req.Talker).Int("id_count", len(req.IDs)).Msg("根据前端提供的 ID 列表导出语音")
		for _, id := range req.IDs {
			messages = append(messages, &model.Message{
				Type:     model.MessageTypeVoice,
				Talker:   req.Talker,
				Contents: map[string]interface{}{"voice": id},
				Time:     time.Now(), // 占位
			})
		}
	} else {
		// 回退逻辑：查询该会话的所有消息 (兼容旧模式)
		msgQuery := types.MessageQuery{
			Talker: req.Talker,
			Limit:  200000,
			Offset: 0,
		}
		var err error
		messages, err = a.Store.GetMessages(c.Request.Context(), msgQuery)
		if err != nil {
			log.Error().Err(err).Str("talker", req.Talker).Msg("查询消息失败")
			transport.InternalServerError(c, "查询消息失败")
			return
		}
	}

	if len(messages) == 0 {
		transport.BadRequest(c, "未找到任何语音消息")
		return
	}

	// 创建 ZIP 缓冲区
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	voiceCount := 0
	for _, msg := range messages {
		voiceKey := ""
		if msg.Contents != nil {
			if v, ok := msg.Contents["voice"]; ok {
				voiceKey = fmt.Sprint(v)
			}
		}

		if voiceKey == "" {
			continue
		}

		var mediaInfo *model.Media
		var err error

		// 优先从消息自带的原始数据中获取 (针对 V3 等直接存储在消息表的情况)
		if msg.Contents != nil {
			if rawData, ok := msg.Contents["_raw_data"].([]byte); ok && len(rawData) > 0 {
				mediaInfo = &model.Media{
					Type: "voice",
					Key:  voiceKey,
					Data: rawData,
				}
			}
		}

		// 如果没有自带数据，则从存储层获取
		if mediaInfo == nil && voiceKey != "" {
			mediaInfo, err = a.Store.GetMedia(c.Request.Context(), "voice", voiceKey)
			if err != nil {
				continue
			}
		}

		if mediaInfo == nil {
			continue
		}

		prepared := a.Media.Prepare(mediaInfo, false)
		if prepared.Error != nil || len(prepared.Content) == 0 {
			continue
		}

		ext := ".mp3"
		if prepared.ContentType == "audio/silk" {
			ext = ".silk"
		}

		// 文件名构造
		fileName := fmt.Sprintf("%03d_%s%s", voiceCount+1, voiceKey, ext)
		if !msg.Time.IsZero() && msg.Time.Year() > 2000 {
			// 如果有具体的消息时间（如从数据库查到的），使用更友好的命名
			timeStr := msg.Time.Format("20060102_150405")
			sender := msg.SenderName
			if sender == "" {
				sender = msg.Sender
			}
			fileName = fmt.Sprintf("%03d_%s_%s%s", voiceCount+1, sanitizeFileName(sender), timeStr, ext)
		}

		w, err := zipWriter.Create(fileName)
		if err != nil {
			continue
		}
		w.Write(prepared.Content)
		voiceCount++
	}

	zipWriter.Close()

	if voiceCount == 0 {
		transport.BadRequest(c, "该会话共找到 "+fmt.Sprintf("%d", len(messages))+" 条疑似语音记录，但无法获取到原始数据")
		return
	}

	zipName := fmt.Sprintf("voices_%s_%s.zip", sanitizeFileName(req.Name), time.Now().Format("20060102"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipName))
	c.Header("Content-Type", "application/zip")
	c.Data(200, "application/zip", buf.Bytes())
}

// sanitizeFileName 清理文件名中的非法字符
func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	result := replacer.Replace(name)
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}

// maybeAutoTranscribe 在开启「自动转文字」且已配置识别服务时，
// 后台把尚未转写的语音补齐。已在跑的任务不会被重复触发。
func (a *API) maybeAutoTranscribe() {
	if !viper.GetBool("TTS_AUTO") {
		return
	}
	a.mu.Lock()
	if a.TTS == nil || (a.batchJob != nil && a.batchJob.Running) {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.batchJob = &BatchTranscribeJob{Running: true, cancel: cancel}
	a.mu.Unlock()

	log.Info().Msg("自动语音转文字：开始扫描未转写的语音")
	a.runBatchTranscribe(ctx, "")
}
