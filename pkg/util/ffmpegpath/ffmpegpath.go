// Package ffmpegpath 只做一件事：找到可用的 ffmpeg 可执行文件。
//
// 这段逻辑原本混在「解密微信 .dat 图片」的包里。Wetrace Plus 不解密任何东西，
// 那个包整个删掉了，但视频转码还要用 ffmpeg，所以单独留下这一小块。
package ffmpegpath

import (
	"os"
	"path/filepath"
	"runtime"
)

// EnvKey 是指定 ffmpeg 路径的环境变量名。
const EnvKey = "FFMPEG_PATH"

// Path 是实际使用的 ffmpeg 路径：优先环境变量，其次同目录下的 ffmpeg/，最后依赖 PATH。
var Path = resolve()

func resolve() string {
	if p := os.Getenv(EnvKey); p != "" {
		return p
	}
	local := filepath.Join("ffmpeg", "ffmpeg")
	if runtime.GOOS == "windows" {
		local += ".exe"
	}
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return "ffmpeg"
}
