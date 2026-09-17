package tts

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	// 用调用方给的真实扩展名。以前这里写死 .mp3，喂 WAV 进来会被 whisper.cpp
	// 按扩展名误判 —— 而 whisper.cpp 本来就更想要 WAV。
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" || strings.ContainsAny(ext, `/\`) {
		ext = ".wav"
	}
	// 每次一个私有临时目录（0700）：文件名不可预测，别的本机用户也读不到语音内容
	workDir, err := os.MkdirTemp("", "wt_whisper_")
	if err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(workDir)
	audioFile := filepath.Join(workDir, "voice"+ext)
	outBase := filepath.Join(workDir, "out")
	outFile := outBase + ".txt"

	// 写入临时音频文件
	if err := os.WriteFile(audioFile, audioData, 0600); err != nil {
		return "", fmt.Errorf("写入临时音频失败: %w", err)
	}

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

// ValidateLocalBinary 检查设置里填的「whisper 可执行文件」是不是像 whisper。
//
// 这个路径会被原样 exec。设置接口一旦被滥用（跨站请求、泄露的凭据），
// 把它改成 /bin/sh、python3 之类就等于任意程序执行。所以只接受
// whisper.cpp 的常见文件名（whisper-cli / whisper-cpp / whisper / main），
// 并且必须是真实存在、可执行的普通文件。
func ValidateLocalBinary(path string) error {
	if path == "" {
		return fmt.Errorf("未填写 whisper 可执行文件路径")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("whisper 可执行文件需要填写绝对路径")
	}
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if !strings.Contains(name, "whisper") && name != "main" {
		return fmt.Errorf("「%s」看起来不是 whisper.cpp 的可执行文件（应为 whisper-cli / whisper-cpp / main）", filepath.Base(path))
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("找不到 whisper 可执行文件: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("whisper 路径不是普通文件")
	}
	if info.Mode().Perm()&0o111 == 0 && filepath.Ext(path) != ".exe" {
		return fmt.Errorf("whisper 文件没有可执行权限")
	}
	return nil
}
