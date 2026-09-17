package media

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
	"github.com/DrMaomao12345/Wetrace-Plus/pkg/util/ffmpegpath"
	"github.com/DrMaomao12345/Wetrace-Plus/pkg/util/silk"
	"github.com/rs/zerolog/log"
)

// Service 处理准备用于服务的媒体文件的业务逻辑。
//
// 这里**不做任何解密**：Wetrace Plus 只处理导入进来的文件，
// 微信本地那套加密图片（.dat）既拿不到也不去解。
type Service struct {
	DataDir  string
	FilesDir string // 导入包里附带的媒体文件根目录（没有就是空的）
}

// NewService 创建一个新的媒体服务。
func NewService(dataDir, filesDir string) *Service {
	return &Service{DataDir: dataDir, FilesDir: filesDir}
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

// 表情包只允许从微信自己的 CDN 下载。
//
// cdnurl 来自消息 XML —— 也就是来自导入文件、来自任何给你发过消息的人。
// 以前这里对任意地址发 GET、跟随重定向、没有超时也没有大小上限，还会把内容原样返回：
// 等于一个能读内网的代理（SSRF）。
var emojiHostSuffixes = []string{".qq.com", ".qpic.cn", ".qlogo.cn", ".wechat.com"}

const maxEmojiBytes = 10 << 20

func emojiURLAllowed(u *neturl.URL) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suf := range emojiHostSuffixes {
		if host == strings.TrimPrefix(suf, ".") || strings.HasSuffix(host, suf) {
			return true
		}
	}
	return false
}

var emojiClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("重定向次数过多")
		}
		if !emojiURLAllowed(req.URL) {
			return errors.New("重定向到了非微信 CDN 的地址")
		}
		return nil
	},
}

// DownloadAndDecryptEmoji 下载并解密表情包
func (s *Service) DownloadAndDecryptEmoji(url string, keyHex string) PreparedMedia {
	// 1. 下载文件
	parsed, err := neturl.Parse(url)
	if err != nil || !emojiURLAllowed(parsed) {
		return PreparedMedia{Error: errors.New("表情包地址不是微信 CDN，已拒绝")}
	}
	req, err := http.NewRequest("GET", parsed.String(), nil)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("创建请求失败: %w", err)}
	}
	// 模拟微信 User-Agent，防止被拦截
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36 MicroMessenger/7.0.20.1781(0x6700143B)")

	resp, err := emojiClient.Do(req)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("下载失败: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return PreparedMedia{Error: fmt.Errorf("下载返回状态码: %d", resp.StatusCode)}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEmojiBytes+1))
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("读取内容失败: %w", err)}
	}
	if len(data) > maxEmojiBytes {
		return PreparedMedia{Error: errors.New("表情包超过 10 MB，已拒绝")}
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
		return PreparedMedia{Error: errors.New("表情包数据不完整")}
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

	// 5. 再次检测类型：解出来不是图片就不返回
	contentType = detectContentType(unpadded)
	if contentType == "application/octet-stream" {
		return PreparedMedia{Error: errors.New("表情包解码后不是图片")}
	}

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
	// 导入包里可能只带了缩略图或只带了原图，两个都试一次。
	// 不再找 .dat —— 那是微信本地的加密图片，这里既没有密钥也不解密。
	base := strings.TrimSuffix(relativePath, filepath.Ext(relativePath))
	if strings.HasSuffix(strings.ToLower(base), "_t") {
		base = base[:len(base)-2]
	}
	ext := filepath.Ext(relativePath)

	candidates := []string{relativePath, base + ext, base + "_t" + ext}
	if isThumb {
		candidates = []string{base + "_t" + ext, relativePath, base + ext}
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		abs, err := SafeJoin(s.FilesDir, c)
		if err != nil {
			continue
		}
		if res := s.doPrepareFile(abs, false); res.Error == nil {
			return res
		}
	}
	return PreparedMedia{Error: fmt.Errorf("图片文件不存在（导入包里没有附件）: %s", relativePath)}
}

func (s *Service) prepareFile(relativePath string, isVideo bool) PreparedMedia {
	absolutePath, err := SafeJoin(s.FilesDir, relativePath)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("无效的文件路径: %s", relativePath)}
	}
	return s.doPrepareFile(absolutePath, isVideo)
}

func (s *Service) doPrepareFile(absolutePath string, isVideo bool) PreparedMedia {
	if _, err := os.Stat(absolutePath); os.IsNotExist(err) {
		return PreparedMedia{Error: fmt.Errorf("文件在磁盘上不存在: %s", absolutePath)}
	}

	// 视频先转码成兼容性好的 H.264/AAC MP4
	if isVideo {
		if transcodedPath, err := s.ensureVideoTranscoded(absolutePath); err == nil {
			absolutePath = transcodedPath
		}
	}

	content, err := os.ReadFile(absolutePath)
	if err != nil {
		return PreparedMedia{Error: fmt.Errorf("读取文件失败: %w", err)}
	}
	return PreparedMedia{
		Content:     content,
		ContentType: getMimeTypeByExtension(strings.ToLower(filepath.Ext(absolutePath))),
	}
}

// ensureVideoTranscoded 确保视频被转码为兼容性好的格式（H.264/AAC MP4）。
// 返回转码后文件的绝对路径。如果已存在缓存，直接返回。
func (s *Service) ensureVideoTranscoded(srcPath string) (string, error) {
	// 简单的缓存键生成策略：基于文件名或路径 hash
	// 这里简单使用文件名加后缀，保存在系统临时目录的 chatlog_video_cache 子目录下
	fileName := filepath.Base(srcPath)
	cacheDir := filepath.Join(os.TempDir(), "chatlog_video_cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
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

	cmd := exec.Command(ffmpegpath.Path,
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
