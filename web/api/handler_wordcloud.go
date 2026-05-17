package api

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/afumu/wetrace/pkg/util"
	"github.com/afumu/wetrace/pkg/wordcloud"
	"github.com/afumu/wetrace/store/types"
	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
)

// buildNameStopwords 把所有联系人/会话的姓名（昵称、备注、群名）加入停用词表，
// 这样高频出现的人名就不会污染词云。
func (a *API) buildNameStopwords(ctx context.Context) map[string]bool {
	res := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		res[s] = true
	}
	if sessions, err := a.Store.GetSessions(ctx, types.SessionQuery{Limit: 0}); err == nil {
		for _, sess := range sessions {
			add(sess.NickName)
		}
	}
	if contacts, err := a.Store.GetContacts(ctx, types.ContactQuery{}); err == nil {
		for _, c := range contacts {
			add(c.NickName)
			add(c.Remark)
			add(c.Alias)
		}
	}
	return res
}

// parseChunkParams 从 query 中解析分块参数（chunks / min_chunks）
func parseChunkParams(c *gin.Context) (chunks, minChunks int) {
	chunks = 5     // 默认分 5 段
	minChunks = 2  // 默认要求至少出现在 2 段中
	if v := c.Query("chunks"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 50 {
			chunks = n
		}
	}
	if v := c.Query("min_chunks"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			minChunks = n
		}
	}
	if minChunks > chunks {
		minChunks = chunks
	}
	return
}

// GetWordCloud 获取指定会话的词云数据
func (a *API) GetWordCloud(c *gin.Context) {
	talker := c.Param("id")
	if talker == "" {
		transport.BadRequest(c, "会话 ID 不能为空")
		return
	}

	// 解析时间范围
	var start, end time.Time
	var ok bool
	timeRange := c.Query("time_range")
	if timeRange != "" {
		start, end, ok = util.TimeRangeOf(timeRange)
	}
	if !ok {
		end = time.Now()
		start = end.AddDate(-20, 0, 0)
	}

	// 解析 limit
	limit := 100
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	chunks, minChunks := parseChunkParams(c)

	// 构建查询
	query := types.MessageQuery{
		Talker:    talker,
		StartTime: start,
		EndTime:   end,
		Limit:     50000,
	}

	// 限定发送人
	if sender := c.Query("sender"); sender != "" {
		query.Sender = sender
	}

	msgs, err := a.Store.GetMessages(context.Background(), query)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}

	// 提取文本消息内容
	texts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.Type == 1 {
			texts = append(texts, m.Content)
		}
	}

	if len(texts) == 0 {
		transport.SendSuccess(c, &wordcloud.WordCloudResult{
			Words: []*wordcloud.WordItem{},
		})
		return
	}

	nameStopwords := a.buildNameStopwords(c.Request.Context())
	result := wordcloud.AnalyzeChunked(texts, limit, chunks, minChunks, nameStopwords)
	transport.SendSuccess(c, result)
}

// GetWordCloudGlobal 获取全局词云数据（不限定会话）
func (a *API) GetWordCloudGlobal(c *gin.Context) {
	// 解析时间范围
	var start, end time.Time
	var ok bool
	timeRange := c.Query("time_range")
	if timeRange != "" {
		start, end, ok = util.TimeRangeOf(timeRange)
	}
	if !ok {
		end = time.Now()
		start = end.AddDate(-20, 0, 0)
	}

	// 解析 limit
	limit := 100
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	chunks, minChunks := parseChunkParams(c)

	// 直接拉文本消息内容，跳过 LIKE 全表扫和 1000 条硬限制
	texts, err := a.Store.GetTextMessagesGlobal(c.Request.Context(), start, end, 50000)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}

	if len(texts) == 0 {
		transport.SendSuccess(c, &wordcloud.WordCloudResult{
			Words: []*wordcloud.WordItem{},
		})
		return
	}

	nameStopwords := a.buildNameStopwords(c.Request.Context())
	result := wordcloud.AnalyzeChunked(texts, limit, chunks, minChunks, nameStopwords)
	transport.SendSuccess(c, result)
}

// ReloadWordCloudStopwords 重新加载自定义停用词文件
func (a *API) ReloadWordCloudStopwords(c *gin.Context) {
	wordcloud.ReloadStopwords()
	transport.SendSuccess(c, gin.H{"status": "ok"})
}

// dictFileName 把请求中的 type 映射到具体文件名
func dictFileName(typ string) string {
	switch typ {
	case "dict":
		return "wordcloud_dict.txt"
	case "stopwords":
		return "wordcloud_stopwords.txt"
	default:
		return ""
	}
}

// readDictLines 读取词典/停用词文件内容（按行返回，去掉空行和注释）
func readDictLines(dataDir, fileName string) ([]string, error) {
	if fileName == "" {
		return nil, nil
	}
	p := filepath.Join(dataDir, fileName)
	// 兼容 Windows 双扩展名
	if _, err := os.Stat(p); err != nil {
		alt := p + ".txt"
		if _, err2 := os.Stat(alt); err2 == nil {
			p = alt
		}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	raw := strings.Split(string(b), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

// writeDictLines 把词列表写入文件（覆盖式），自动 dedup
func writeDictLines(dataDir, fileName string, lines []string, header string) error {
	if fileName == "" {
		return nil
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	seen := make(map[string]bool)
	cleaned := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || seen[l] {
			continue
		}
		seen[l] = true
		cleaned = append(cleaned, l)
	}
	var sb strings.Builder
	if header != "" {
		sb.WriteString(header)
		sb.WriteString("\n")
	}
	for _, l := range cleaned {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	p := filepath.Join(dataDir, fileName)
	return os.WriteFile(p, []byte(sb.String()), 0644)
}

// GetWordCloudDict 查看当前词典/停用词内容
// GET /api/v1/analysis/wordcloud/dict?type=dict|stopwords
func (a *API) GetWordCloudDict(c *gin.Context) {
	typ := c.DefaultQuery("type", "stopwords")
	fileName := dictFileName(typ)
	if fileName == "" {
		transport.BadRequest(c, "type 必须为 dict 或 stopwords")
		return
	}
	lines, err := readDictLines(a.Conf.DataDir, fileName)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	transport.SendSuccess(c, gin.H{
		"type":  typ,
		"file":  fileName,
		"words": lines,
	})
}

// UpdateWordCloudDict 添加或删除词典/停用词条目
// POST body: { "type": "dict|stopwords", "add": ["w1"], "remove": ["w2"] }
func (a *API) UpdateWordCloudDict(c *gin.Context) {
	var req struct {
		Type   string   `json:"type"`
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
		Words  []string `json:"words"` // 可选：直接整体替换
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.BadRequest(c, "请求体格式错误")
		return
	}
	fileName := dictFileName(req.Type)
	if fileName == "" {
		transport.BadRequest(c, "type 必须为 dict 或 stopwords")
		return
	}

	current, err := readDictLines(a.Conf.DataDir, fileName)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}

	var final []string
	if len(req.Words) > 0 {
		// 整体替换
		final = req.Words
	} else {
		// 增量
		set := make(map[string]bool, len(current))
		for _, w := range current {
			set[w] = true
		}
		for _, w := range req.Add {
			w = strings.TrimSpace(w)
			if w == "" {
				continue
			}
			set[w] = true
		}
		for _, w := range req.Remove {
			delete(set, strings.TrimSpace(w))
		}
		final = make([]string, 0, len(set))
		for w := range set {
			final = append(final, w)
		}
	}

	header := "# 自定义停用词 - 词云分析时会跳过这些词"
	if req.Type == "dict" {
		header = "# 自定义词典 - 加入词云分词器以识别专有名词"
	}
	if err := writeDictLines(a.Conf.DataDir, fileName, final, header); err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}

	// 停用词需要热重载到内存
	if req.Type == "stopwords" {
		wordcloud.ReloadStopwords()
	}
	// 词典需要重新加载分词器（gse 一旦加载就不能动态改，需要重启）
	// 这里只持久化，下次启动生效，并提示用户

	updated, _ := readDictLines(a.Conf.DataDir, fileName)
	transport.SendSuccess(c, gin.H{
		"type":  req.Type,
		"words": updated,
		"note":  func() string {
			if req.Type == "dict" {
				return "词典修改已保存，需重启应用后生效"
			}
			return "停用词已即时生效"
		}(),
	})
}
