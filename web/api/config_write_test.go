package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestSaveConfigStripsLineBreaks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigFile(path)
	viper.SetConfigType("env")

	viper.Set("TTS_MODEL", "whisper-1\nLISTEN_ADDR=0.0.0.0:5200\r\nPASSWORD_HASH=")
	viper.Set("AI_MODEL", "deepseek-chat INJECTED=1")
	if err := saveConfig(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "LISTEN_ADDR=") || strings.HasPrefix(line, "PASSWORD_HASH=") || strings.HasPrefix(line, "INJECTED=") {
			t.Fatalf("换行注入出了新配置行: %q\n完整文件:\n%s", line, out)
		}
	}
	if !strings.Contains(out, "TTS_MODEL=whisper-1LISTEN_ADDR=0.0.0.0:5200PASSWORD_HASH=") {
		t.Fatalf("值应该被压成一行，实际:\n%s", out)
	}
}

func TestHighlightKeywordAlwaysEscapes(t *testing.T) {
	evil := `<img src=x onerror=alert(1)>`
	for _, kw := range []string{"", "img", "x"} {
		got := highlightKeyword(evil, kw)
		if strings.Contains(got, "<img") {
			t.Errorf("keyword=%q 时没有转义: %s", kw, got)
		}
	}
}

func TestUnlockRateLimit(t *testing.T) {
	pm := NewPasswordManager()
	src := "192.168.1.9"
	for i := 0; i < freeUnlockAttempts; i++ {
		if pm.unlockBlocked(src) > 0 {
			t.Fatalf("第 %d 次就被锁了，免费次数应为 %d", i+1, freeUnlockAttempts)
		}
		pm.recordUnlock(src, false)
	}
	pm.recordUnlock(src, false)
	if pm.unlockBlocked(src) == 0 {
		t.Fatal("超过免费次数后应被锁定")
	}
	if pm.unlockBlocked("192.168.1.10") > 0 {
		t.Fatal("锁定只应影响同一个来源")
	}
	pm.recordUnlock(src, true)
	if pm.unlockBlocked(src) > 0 {
		t.Fatal("成功解锁后应清零")
	}
}

func TestSessionExpiry(t *testing.T) {
	pm := NewPasswordManager()
	pm.AddSession("t1")
	if !pm.IsValidSession("t1") {
		t.Fatal("新会话应有效")
	}
	pm.mu.Lock()
	pm.sessions["t1"] = pm.sessions["t1"].Add(-sessionTTL - 1)
	pm.mu.Unlock()
	if pm.IsValidSession("t1") {
		t.Fatal("过期会话应失效")
	}
	if pm.IsValidSession("") {
		t.Fatal("空 token 不应有效")
	}
}
