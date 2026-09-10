package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

// whisperFound 是一个扫到的可执行文件或模型
type whisperFound struct {
	Path   string  `json:"path"`
	Name   string  `json:"name"`
	SizeMB float64 `json:"size_mb,omitempty"`
	Source string  `json:"source"` // 说明是从哪儿找到的，便于用户判断
}

// whisperBinaryNames 是 whisper.cpp 常见的可执行文件名。
// 新版叫 whisper-cli，Homebrew 装的叫 whisper-cpp，老版本叫 main。
var whisperBinaryNames = []string{"whisper-cli", "whisper-cpp", "whisper", "main"}

// ScanWhisperLocal GET /api/v1/system/tts_local/scan
// 在常见位置查找 whisper.cpp 可执行文件与 ggml 模型，供设置页自动填充。
func (a *API) ScanWhisperLocal(c *gin.Context) {
	home, _ := os.UserHomeDir()

	binDirs, modelDirs := whisperSearchDirs(home, a.Conf.DataDir)

	binaries := scanWhisperBinaries(binDirs)
	models := scanWhisperModels(modelDirs)

	// 汇总实际搜索过的位置，找不到时展示给用户，省得他们猜
	searched := append(append([]string{}, binDirs...), modelDirs...)
	searched = dedupStrings(searched)
	sort.Strings(searched)

	transport.SendSuccess(c, gin.H{
		"binaries": binaries,
		"models":   models,
		"searched": searched,
		"platform": runtime.GOOS,
	})
}

// whisperSearchDirs 返回 (可执行文件搜索目录, 模型搜索目录)
func whisperSearchDirs(home, dataDir string) ([]string, []string) {
	binDirs := []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"}
	modelDirs := []string{
		"/opt/homebrew/share/whisper-cpp",
		"/usr/local/share/whisper-cpp",
	}

	if home != "" {
		binDirs = append(binDirs,
			filepath.Join(home, "whisper.cpp"),
			filepath.Join(home, "whisper.cpp", "build", "bin"),
			filepath.Join(home, ".local", "bin"),
		)
		modelDirs = append(modelDirs,
			filepath.Join(home, "whisper.cpp", "models"),
			filepath.Join(home, ".cache", "whisper"),
			filepath.Join(home, "Library", "Application Support", "whisper"),
			filepath.Join(home, "Downloads"),
		)
	}
	if dataDir != "" {
		// 用户也可以把模型直接丢进 Wetrace 的数据目录。
		// 配置里常是相对路径，转成绝对路径再展示，免得用户看不懂搜的是哪儿。
		if abs, err := filepath.Abs(dataDir); err == nil {
			dataDir = abs
		}
		modelDirs = append(modelDirs, filepath.Join(dataDir, "whisper"), dataDir)
	}
	if runtime.GOOS == "windows" && home != "" {
		binDirs = append(binDirs, filepath.Join(home, "whisper.cpp", "build", "bin", "Release"))
	}

	return dedupStrings(binDirs), dedupStrings(modelDirs)
}

func scanWhisperBinaries(dirs []string) []whisperFound {
	var out []whisperFound
	seen := map[string]bool{}

	add := func(path, source string) {
		if path == "" || seen[path] {
			return
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return
		}
		// 可执行位（Windows 上这个判断没意义，靠扩展名兜底）
		if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
			return
		}
		seen[path] = true
		out = append(out, whisperFound{Path: path, Name: filepath.Base(path), Source: source})
	}

	// 先查 PATH —— 装过的人多半在 PATH 里
	for _, name := range whisperBinaryNames {
		if name == "main" {
			continue // 太通用，只在 whisper.cpp 目录里认
		}
		if p, err := exec.LookPath(name); err == nil {
			if abs, err := filepath.Abs(p); err == nil {
				add(abs, "PATH")
			}
		}
	}

	for _, dir := range dirs {
		for _, name := range whisperBinaryNames {
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			add(filepath.Join(dir, name), dir)
		}
	}
	return out
}

func scanWhisperModels(dirs []string) []whisperFound {
	var out []whisperFound
	seen := map[string]bool{}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			lower := strings.ToLower(name)
			// whisper.cpp 的模型统一是 ggml-*.bin
			if !strings.HasSuffix(lower, ".bin") || !strings.HasPrefix(lower, "ggml") {
				continue
			}
			path := filepath.Join(dir, name)
			if seen[path] {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			seen[path] = true
			out = append(out, whisperFound{
				Path:   path,
				Name:   name,
				SizeMB: float64(info.Size()) / (1024 * 1024),
				Source: dir,
			})
		}
	}

	// 大的模型通常更准，排前面
	sort.Slice(out, func(i, j int) bool { return out[i].SizeMB > out[j].SizeMB })
	return out
}

func dedupStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
