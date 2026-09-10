package tts

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LocalClient 通过 whisper.cpp 二进制文件本地识别语音
// 每次识别时临时启动进程，识别完毕进程自动退出，不长驻内存
type LocalClient struct {
	BinaryPath string // whisper-cli.exe 或 main.exe 的路径
	ModelPath  string // ggml-*.bin 模型文件路径
}

// NewLocalClient 创建本地 Whisper 客户端
func NewLocalClient(binaryPath, modelPath string) *LocalClient {
	return &LocalClient{BinaryPath: binaryPath, ModelPath: modelPath}
}

// Transcribe 识别音频数据，返回文字
func (c *LocalClient) Transcribe(audioData []byte, filename string) (string, error) {
	tmpDir := os.TempDir()
	uid := fmt.Sprintf("%d_%d", time.Now().UnixNano(), rand.Int63())
	// 用调用方给的真实扩展名。以前这里写死 .mp3，喂 WAV 进来会被 whisper.cpp
	// 按扩展名误判 —— 而 whisper.cpp 本来就更想要 WAV。
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".wav"
	}
	audioFile := filepath.Join(tmpDir, "wt_voice_"+uid+ext)
	outBase := filepath.Join(tmpDir, "wt_out_"+uid)
	outFile := outBase + ".txt"

	// 写入临时音频文件
	if err := os.WriteFile(audioFile, audioData, 0600); err != nil {
		return "", fmt.Errorf("写入临时音频失败: %w", err)
	}
	defer os.Remove(audioFile)
	defer os.Remove(outFile)

	var stderr bytes.Buffer
	cmd := exec.Command(
		c.BinaryPath,
		"-m", c.ModelPath,
		"-f", audioFile,
		"-l", "zh",
		"-nt",   // 不输出时间戳
		"-otxt", // 输出为文本文件
		"-of", outBase,
	)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper 执行失败: %w\n%s", err, stderr.String())
	}

	raw, err := os.ReadFile(outFile)
	if err != nil {
		return "", fmt.Errorf("读取识别结果失败: %w", err)
	}

	return strings.TrimSpace(string(raw)), nil
}
