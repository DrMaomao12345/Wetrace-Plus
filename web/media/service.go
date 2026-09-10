package media

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/pkg/util/dat2img"
	"github.com/DrMaomao12345/Wetrace-Plus/pkg/util/silk"
	"github.com/rs/zerolog/log"
)

// Service 处理准备用于服务的媒体文件的业务逻辑。
type Service struct {
	DataDir         string
	ImageKey        string
	XorKey          string
	WechatDbSrcPath string
}

// NewService 创建一个新的媒体服务。
func NewService(dataDir, imageKey, xorKey, wechatDbSrcPath string) *Service {
	return &Service{
		DataDir:         dataDir,
		ImageKey:        imageKey,
		XorKey:          xorKey,
		WechatDbSrcPath: wechatDbSrcPath,
	}
}

// PreparedMedia 保存媒体文件的最终内容和内容类型。
type PreparedMedia struct {
	Content     []byte
	ContentType string
	Error       error
	// Encrypted 表示文件确实存在，但内容是加密的且当前解不开。
	// 与「文件缺失」「读取出错」区分开 —— 前端据此显示占位说明而不是报错。
	Encrypted bool
	// Reason 是给用户看的原因说明
	Reason string
}

// DownloadAndDecryptEmoji 下载并解密表情包
func (s *Service) DownloadAndDecryptEmoji(url string, keyHex string) PreparedMedia {
	// 1. 下载文件
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("创建请求失败: %w", err)}
	}
	// 模拟微信 User-Agent，防止被拦截
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36 MicroMessenger/7.0.20.1781(0x6700143B)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("下载失败: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return PreparedMedia{Error: fmt.Errorf("下载返回状态码: %d", resp.StatusCode)}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("读取内容失败: %w", err)}
	}

	// 2. 检查是否已经是图片 (未加密)
	contentType := detectContentType(data)
	if contentType != "application/octet-stream" {
		return PreparedMedia{Content: data, ContentType: contentType}
	}

	// 3. AES 解密
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("密钥解码失败: %w", err)}
	}

	if len(key) < 16 {
		return PreparedMedia{Error: errors.New("密钥长度不足 16 字节")}
	}

	iv := key[:16] // 微信通常使用 Key 的前16位作为 IV

	block, err := aes.NewCipher(key)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("创建 Cipher 失败: %w", err)}
	}

	if len(data)%aes.BlockSize != 0 {
		// 数据长度不是块大小的倍数，尝试直接返回（可能下载不完整或不是加密数据）
		return PreparedMedia{Content: data, ContentType: "application/octet-stream"}
	}

	decrypted := make([]byte, len(data))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, data)

	// 4. 去除 PKCS7 填充
	unpadded, err := pkcs7Unpad(decrypted, aes.BlockSize)
	if err != nil {
		// 填充错误，尝试使用解密后的原始数据（有时尾部数据不影响显示）
		unpadded = decrypted
	}

	// 5. 再次检测类型
	contentType = detectContentType(unpadded)

	return PreparedMedia{
		Content:     unpadded,
		ContentType: contentType,
	}
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("data is empty")
	}
	if length%blockSize != 0 {
		return nil, errors.New("data length is not a multiple of block size")
	}
	paddingLen := int(data[length-1])
	if paddingLen == 0 || paddingLen > blockSize {
		return nil, errors.New("invalid padding length")
	}
	// check padding
	for i := 0; i < paddingLen; i++ {
		if data[length-1-i] != byte(paddingLen) {
			return nil, errors.New("invalid padding bytes")
		}
	}
	return data[:length-paddingLen], nil
}

func detectContentType(data []byte) string {
	if len(data) > 4 && string(data[:4]) == "GIF8" {
		return "image/gif"
	}
	if len(data) > 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(data) > 2 && string(data[:2]) == "\xff\xd8" {
		return "image/jpeg"
	}
	return "application/octet-stream"
}

// Prepare 处理获取、读取和解码媒体文件的完整生命周期。
func (s *Service) Prepare(media *model.Media, isThumb bool) PreparedMedia {
	if media.Type == "voice" {
		return s.prepareVoice(media.Data)
	}

	if media.Path == "" {
		return PreparedMedia{Error: fmt.Errorf("媒体路径为空，key 为 %s", media.Key)}
	}

	// 对于图片类型，使用启发式路径查找
	if media.Type == "image" {
		return s.prepareImageWithFallback(media.Path, isThumb)
	}

	res := s.prepareFile(media.Path, media.Type == "video")

	// 如果是视频类型，强制设置为 video/mp4，确保前端可以播放
	if media.Type == "video" {
		res.ContentType = "video/mp4"
	}

	return res
}

func (s *Service) prepareImageWithFallback(relativePath string, isThumb bool) PreparedMedia {
	var candidates []string

	ext := strings.ToLower(filepath.Ext(relativePath))
	base := relativePath
	if ext == ".dat" {
		base = strings.TrimSuffix(relativePath, ext)
	}
	// 如果本身已经是 _t 结尾，也去掉以便统一构造
	if strings.HasSuffix(strings.ToLower(base), "_t") {
		base = strings.TrimSuffix(base, base[len(base)-2:])
	}

	if isThumb {
		// 缩略图模式优先级：_t.dat -> .dat -> 原路径
		candidates = []string{
			base + "_t.dat",
			base + ".dat",
			base,
			relativePath,
		}
	} else {
		// 原图模式优先级：.dat -> 原路径 -> _t.dat (回退)
		candidates = []string{
			base + ".dat",
			base,
			relativePath,
			base + "_t.dat",
		}
	}

	// 去重并过滤空
	seen := make(map[string]bool)
	uniqueCandidates := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c != "" && !seen[c] {
			seen[c] = true
			uniqueCandidates = append(uniqueCandidates, c)
		}
	}

	var encrypted *PreparedMedia
	for _, c := range uniqueCandidates {
		abs := filepath.Join(s.WechatDbSrcPath, c)
		res := s.doPrepareFile(abs, false)
		if res.Error != nil {
			continue
		}
		if res.Encrypted {
			// 记下来但先别返回 —— 也许别的候选（比如明文的 _M.dat）能解出来
			if encrypted == nil {
				r := res
				encrypted = &r
			}
			continue
		}
		return res
	}
	if encrypted != nil {
		return *encrypted
	}

	return PreparedMedia{Error: fmt.Errorf("图片文件不存在 (磁盘及缓存均未找到): %s", relativePath)}
}

func (s *Service) prepareFile(relativePath string, isVideo bool) PreparedMedia {
	if strings.Contains(relativePath, "..") {
		return PreparedMedia{Error: fmt.Errorf("无效的文件路径: %s", relativePath)}
	}

	baseDir := s.WechatDbSrcPath
	absolutePath := filepath.Join(baseDir, relativePath)

	return s.doPrepareFile(absolutePath, isVideo)
}

func (s *Service) doPrepareFile(absolutePath string, isVideo bool) PreparedMedia {
	// 1. 检查缓存 (仅针对图片/解密类文件)
	// 计算相对于微信根目录的路径，用于建立缓存镜像
	relPath, err := filepath.Rel(s.WechatDbSrcPath, absolutePath)
	isDat := false
	if err == nil && !isVideo {
		ext := strings.ToLower(filepath.Ext(absolutePath))
		isDat = strings.HasSuffix(ext, ".dat") || strings.Contains(strings.ToLower(filepath.ToSlash(absolutePath)), "/img/")

		if isDat {
			cachePath := filepath.Join(s.DataDir, "cache", "images", relPath)
			// 只认「确实是图片」的缓存。早期版本把解密失败的乱码也写进过缓存，
			// 校验一下就能自动跳过那些坏条目，不用手动清理。
			if cacheContent, err := os.ReadFile(cachePath); err == nil && looksLikeImage(cacheContent) {
				return PreparedMedia{
					Content:     cacheContent,
					ContentType: detectContentType(cacheContent),
				}
			}
		}
	}

	// 2. 如果缓存不存在，则检查原文件是否存在
	if _, err := os.Stat(absolutePath); os.IsNotExist(err) {
		return PreparedMedia{Error: fmt.Errorf("文件在磁盘上不存在: %s", absolutePath)}
	}

	// 3. 如果是视频，尝试转码
	if isVideo {
		transcodedPath, err := s.ensureVideoTranscoded(absolutePath)
		if err == nil {
			absolutePath = transcodedPath
		}
	}

	// 3. 处理解密或直接读取
	var res PreparedMedia
	if isDat {
		res = s.prepareDatFile(absolutePath)
		// 只缓存真正解出来的图片：解不开的（Encrypted）内容是空的，缓存了反而有害
		if res.Error == nil && !res.Encrypted && len(res.Content) > 0 {
			cachePath := filepath.Join(s.DataDir, "cache", "images", relPath)
			go func(path string, content []byte) {
				os.MkdirAll(filepath.Dir(path), 0755)
				_ = os.WriteFile(path, content, 0644)
			}(cachePath, res.Content)
		}
	} else {
		ext := strings.ToLower(filepath.Ext(absolutePath))
		contentType := getMimeTypeByExtension(ext)
		content, err := os.ReadFile(absolutePath)
		if err != nil {
			return PreparedMedia{Error: fmt.Errorf("读取文件失败: %w", err)}
		}
		res = PreparedMedia{
			Content:     content,
			ContentType: contentType,
		}
	}

	return res
}

// ensureVideoTranscoded 确保视频被转码为兼容性好的格式（H.264/AAC MP4）。
// 返回转码后文件的绝对路径。如果已存在缓存，直接返回。
func (s *Service) ensureVideoTranscoded(srcPath string) (string, error) {
	// 简单的缓存键生成策略：基于文件名或路径 hash
	// 这里简单使用文件名加后缀，保存在系统临时目录的 chatlog_video_cache 子目录下
	fileName := filepath.Base(srcPath)
	cacheDir := filepath.Join(os.TempDir(), "chatlog_video_cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("创建缓存目录失败: %w", err)
	}

	// 目标文件：文件名 + .transcoded.mp4
	// 注意：如果不同目录下有同名文件，这里会冲突。
	// 更严谨的做法是 hash(srcPath)。这里为了演示简单处理。
	// 改进：使用 srcPath 的 hash
	hashName := hex.EncodeToString([]byte(srcPath)) // 简单的 path hash，实际可用 md5
	// 或者是文件名 + hash 的组合以便调试
	dstPath := filepath.Join(cacheDir, fmt.Sprintf("%s_%s.mp4", fileName, hashName[:8]))

	// 1. 检查缓存是否存在
	if _, err := os.Stat(dstPath); err == nil {
		// 缓存存在，直接返回
		// log.Debug().Str("cache", dstPath).Msg("命中视频转码缓存")
		return dstPath, nil
	}

	// 2. 调用 ffmpeg 转码
	// 命令：ffmpeg -i <src> -c:v libx264 -c:a aac -strict experimental <dst>
	// -y 覆盖输出
	// -preset ultrafast 加速转码（牺牲压缩率）
	log.Info().Str("src", srcPath).Msg("开始视频转码 (HEVC -> H.264)...")

	cmd := exec.Command(dat2img.FFMpegPath,
		"-y",
		"-i", srcPath,
		"-c:v", "libx264",
		"-preset", "ultrafast", // 追求速度
		"-c:a", "aac",
		dstPath,
	)

	// 捕获输出以便调试
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg 转码失败: %w, output: %s", err, string(output))
	}

	log.Info().Str("dst", dstPath).Msg("视频转码成功")
	return dstPath, nil
}

func (s *Service) prepareDatFile(path string) PreparedMedia {
	b, err := os.ReadFile(path)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("读取 .dat 文件失败: %w", err)}
	}

	// 使用配置的密钥
	foundKey := hex.EncodeToString([]byte(s.ImageKey))
	dat2img.SetAesKey(foundKey)
	_ = dat2img.SetV4XorKey(s.XorKey)

	out, ext, err := dat2img.Dat2Image(b)

	// 微信 4.x 的加密图片（魔数 07085631/07085632）需要 16 字节 AES 密钥，
	// 该密钥由微信自研加密处理、不经过系统加密接口，目前提取不到。
	// 用错误的密钥解 AES-ECB 不会报错，只会产出一堆乱码 —— 所以不能只看 err，
	// 还要检查解出来的东西到底是不是图片。
	if isWeChatV4Encrypted(b) && (err != nil || !looksLikeImage(out)) {
		return PreparedMedia{
			Encrypted: true,
			Reason:    "这张图片由微信加密存储（2025 年 5 月后的新版格式），当前无法解出",
		}
	}

	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("解码 .dat 文件失败，提供原始数据。")
		return PreparedMedia{Content: b, ContentType: "application/octet-stream"}
	}

	contentType := getMimeTypeByExtension(ext)
	return PreparedMedia{Content: out, ContentType: contentType}
}

// looksLikeImage 检查一段数据是不是常见图片格式的开头。
// 用来判断「解出来的到底是图片还是乱码」。
func looksLikeImage(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	switch {
	case b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF: // JPEG
		return true
	case b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G': // PNG
		return true
	case b[0] == 'G' && b[1] == 'I' && b[2] == 'F': // GIF
		return true
	case b[0] == 'R' && b[1] == 'I' && b[2] == 'F' && b[3] == 'F': // WEBP
		return true
	case b[0] == 'B' && b[1] == 'M': // BMP
		return true
	case b[0] == 'w' && b[1] == 'x' && b[2] == 'g' && b[3] == 'f': // 微信 WXGF
		return true
	}
	return false
}

// isWeChatV4Encrypted 判断是否是微信 4.x 的加密图片容器。
// 头 4 字节为 0x07085631（V1）或 0x07085632（V2）。
func isWeChatV4Encrypted(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	return (b[0] == 0x07 && b[1] == 0x08 && b[2] == 0x56 && (b[3] == 0x31 || b[3] == 0x32))
}

func getMimeTypeByExtension(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "mp4", "mov", "m4v", "3gp", "mkv", "avi", "wmv", "flv", "webm":
		return "video/mp4"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "ico":
		return "image/x-icon"
	case "svg":
		return "image/svg+xml"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "m4a":
		return "audio/mp4"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "ogg":
		return "audio/ogg"
	case "pdf":
		return "application/pdf"
	case "doc":
		return "application/msword"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xls":
		return "application/vnd.ms-excel"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "ppt":
		return "application/vnd.ms-powerpoint"
	case "pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case "txt", "md", "log", "json", "xml":
		return "text/plain; charset=utf-8"
	case "csv":
		return "text/csv"
	case "zip":
		return "application/zip"
	case "rar":
		return "application/x-rar-compressed"
	case "7z":
		return "application/x-7z-compressed"
	case "tar":
		return "application/x-tar"
	case "gz":
		return "application/gzip"
	default:
		return "application/octet-stream"
	}
}

func (s *Service) prepareVoice(data []byte) PreparedMedia {
	if len(data) == 0 {
		return PreparedMedia{Error: fmt.Errorf("语音数据为空")}
	}

	out, err := silk.Silk2MP3(data)
	if err != nil {
		log.Warn().Err(err).Msg("解码 .silk 音频失败，提供原始数据。")
		return PreparedMedia{Content: data, ContentType: "audio/silk"} // 回退
	}

	return PreparedMedia{Content: out, ContentType: "audio/mp3"}
}

// PrepareVoiceLossless 把语音解成无损 WAV。
//
// 转写和音色克隆都该走这条：MP3 那条会把 4.4 kHz 以上削掉，而齿音和说话人
// 身份特征就在那一段（详见 pkg/util/silk 的包注释）。播放仍走 prepareVoice。
func (s *Service) PrepareVoiceLossless(data []byte) PreparedMedia {
	if len(data) == 0 {
		return PreparedMedia{Error: fmt.Errorf("语音数据为空")}
	}

	out, err := silk.Silk2WAV(data)
	if err != nil {
		log.Warn().Err(err).Msg("解码 .silk 为 WAV 失败，回退到 MP3。")
		return s.prepareVoice(data)
	}

	return PreparedMedia{Content: out, ContentType: "audio/wav"}
}
