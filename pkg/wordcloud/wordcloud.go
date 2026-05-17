package wordcloud

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/go-ego/gse"
)

// 预处理正则：去掉 wxid、URL、@提及、邮箱、纯数字串等噪声，
// 这些不应该出现在词云里。
var (
	reWxid    = regexp.MustCompile(`wxid_[A-Za-z0-9_]+`)
	reURL     = regexp.MustCompile(`https?://[^\s]+`)
	reMention = regexp.MustCompile(`@[\p{Han}\w]+`)
	reEmail   = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reChatroomID = regexp.MustCompile(`\d+@chatroom`)
)

func preprocess(text string) string {
	text = reURL.ReplaceAllString(text, " ")
	text = reEmail.ReplaceAllString(text, " ")
	text = reChatroomID.ReplaceAllString(text, " ")
	text = reWxid.ReplaceAllString(text, " ")
	text = reMention.ReplaceAllString(text, " ")
	return text
}

var (
	seg               gse.Segmenter
	segOnce           sync.Once
	dataDir           string
	customStopwords   = make(map[string]bool)
	customStopwordsMu sync.RWMutex
)

// Init 初始化数据目录（用于查找用户词典和自定义停用词）
//
//	<dataDir>/wordcloud_dict.txt        — 自定义词典（每行一个词，可选频率/词性）
//	<dataDir>/wordcloud_dict.txt.txt    — 兼容 Windows 双扩展名误存
//	<dataDir>/wordcloud_stopwords.txt   — 自定义停用词（每行一个词，#开头为注释）
func Init(dir string) {
	dataDir = dir
}

func ensureInit() {
	segOnce.Do(func() {
		seg.LoadDict() // 加载内置中文词典
		if dataDir != "" {
			// 用户自定义词典：优先 wordcloud_dict.txt，兼容 wordcloud_dict.txt.txt
			for _, name := range []string{"wordcloud_dict.txt", "wordcloud_dict.txt.txt"} {
				p := filepath.Join(dataDir, name)
				if _, err := os.Stat(p); err == nil {
					_ = seg.LoadDict(p)
				}
			}
			// 加载自定义停用词
			loadCustomStopwords(filepath.Join(dataDir, "wordcloud_stopwords.txt"))
		}
	})
}

func loadCustomStopwords(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	customStopwordsMu.Lock()
	defer customStopwordsMu.Unlock()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		customStopwords[line] = true
	}
}

// ReloadStopwords 重新加载停用词文件（不需要重启服务）
func ReloadStopwords() {
	if dataDir == "" {
		return
	}
	customStopwordsMu.Lock()
	customStopwords = make(map[string]bool)
	customStopwordsMu.Unlock()
	loadCustomStopwords(filepath.Join(dataDir, "wordcloud_stopwords.txt"))
}

// 内置中文停用词表
var builtinStopWords = map[string]bool{
	"的": true, "了": true, "是": true, "在": true, "我": true,
	"你": true, "他": true, "她": true, "它": true, "们": true,
	"这": true, "那": true, "有": true, "和": true, "就": true,
	"不": true, "也": true, "都": true, "要": true, "会": true,
	"可以": true, "没有": true, "什么": true, "一个": true, "我们": true,
	"自己": true, "他们": true, "没": true, "很": true, "到": true,
	"说": true, "对": true, "吗": true, "啊": true, "呢": true,
	"吧": true, "嗯": true, "哦": true, "哈": true, "呀": true,
	"嘛": true, "哎": true, "唉": true, "喔": true, "噢": true,
	"把": true, "被": true, "让": true, "给": true, "从": true,
	"去": true, "来": true, "上": true, "下": true, "里": true,
	"中": true, "大": true, "小": true, "多": true, "少": true,
	"个": true, "人": true, "还": true, "能": true, "做": true,
	"看": true, "想": true, "知道": true, "时候": true, "现在": true,
	"因为": true, "所以": true, "但是": true, "如果": true, "这个": true,
	"那个": true, "已经": true, "可能": true, "应该": true, "怎么": true,
	"为什么": true, "这样": true, "那样": true, "一下": true, "一些": true,
	"然后": true, "或者": true, "而且": true, "虽然": true, "不过": true,
	"只是": true, "其实": true, "觉得": true, "比较": true, "一样": true,
	"好的": true, "好吧": true, "好了": true, "好啊": true, "嗯嗯": true,
	"哈哈": true, "哈哈哈": true, "好的好的": true, "OK": true, "ok": true,
}

// isStopword 判断是否为停用词（合并内置 + 自定义 + 额外）
func isStopword(w string, extra map[string]bool) bool {
	if builtinStopWords[w] {
		return true
	}
	customStopwordsMu.RLock()
	custom := customStopwords[w]
	customStopwordsMu.RUnlock()
	if custom {
		return true
	}
	if extra != nil && extra[w] {
		return true
	}
	return false
}

// WordCloudResult 词云结果
type WordCloudResult struct {
	TotalMessages int         `json:"total_messages"`
	TotalWords    int         `json:"total_words"`
	Words         []*WordItem `json:"words"`
}

// WordItem 词频项
type WordItem struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// Analyze 对文本列表进行词频统计，返回词云结果
func Analyze(texts []string, limit int) *WordCloudResult {
	return AnalyzeWithExtraStopwords(texts, limit, nil)
}

// AnalyzeWithExtraStopwords 在 Analyze 基础上额外加入运行时停用词（如联系人姓名）
func AnalyzeWithExtraStopwords(texts []string, limit int, extra map[string]bool) *WordCloudResult {
	return AnalyzeChunked(texts, limit, 1, 1, extra)
}

// AnalyzeChunked 把消息按时间顺序切成 numChunks 段，分别统计词频。
// 只有在至少 minChunks 段都出现过的词才算入最终结果，
// 这样可以过滤掉某段时期突然爆发的"刷屏词"（如某个梗、某次活动），
// 保留长期稳定的高频词。
//
// numChunks=1, minChunks=1 时退化为普通词频统计。
// 推荐：numChunks=5, minChunks=2 或 3。
func AnalyzeChunked(texts []string, limit int, numChunks int, minChunks int, extra map[string]bool) *WordCloudResult {
	ensureInit()

	if limit <= 0 {
		limit = 100
	}
	if numChunks < 1 {
		numChunks = 1
	}
	if minChunks < 1 {
		minChunks = 1
	}
	if minChunks > numChunks {
		minChunks = numChunks
	}

	// 计算每段大小
	chunkSize := (len(texts) + numChunks - 1) / numChunks
	if chunkSize == 0 {
		chunkSize = 1
	}

	chunkAppearance := make(map[string]int) // 每个词在多少个 chunk 出现过
	totalFreq := make(map[string]int)
	totalWords := 0
	actualChunks := 0

	for chunkIdx := 0; chunkIdx < numChunks; chunkIdx++ {
		start := chunkIdx * chunkSize
		if start >= len(texts) {
			break
		}
		end := start + chunkSize
		if end > len(texts) {
			end = len(texts)
		}
		chunkFreq := make(map[string]int)
		for _, text := range texts[start:end] {
			for _, w := range tokenize(text) {
				if isStopword(w, extra) {
					continue
				}
				chunkFreq[w]++
				totalFreq[w]++
				totalWords++
			}
		}
		for w := range chunkFreq {
			chunkAppearance[w]++
		}
		actualChunks++
	}

	// 实际段数比要求的少时，调整阈值
	effMinChunks := minChunks
	if actualChunks < effMinChunks {
		effMinChunks = actualChunks
	}

	items := make([]*WordItem, 0, len(totalFreq))
	for w, count := range totalFreq {
		if chunkAppearance[w] >= effMinChunks {
			items = append(items, &WordItem{Text: w, Count: count})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})

	if len(items) > limit {
		items = items[:limit]
	}

	return &WordCloudResult{
		TotalMessages: len(texts),
		TotalWords:    totalWords,
		Words:         items,
	}
}

// tokenize 使用 gse 进行中文分词
func tokenize(text string) []string {
	text = preprocess(text)
	words := seg.Cut(text, true)
	result := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.TrimSpace(w)
		// 过滤：长度 < 2、纯空白、纯符号/数字
		if utf8.RuneCountInString(w) < 2 {
			continue
		}
		if isNoise(w) {
			continue
		}
		result = append(result, w)
	}
	return result
}

// isNoise 过滤纯数字、纯标点、纯英文单字母组合等噪声词
func isNoise(w string) bool {
	allNoise := true
	for _, r := range w {
		if unicode.IsLetter(r) && !unicode.IsDigit(r) {
			allNoise = false
			break
		}
		if unicode.Is(unicode.Han, r) {
			allNoise = false
			break
		}
	}
	return allNoise
}
