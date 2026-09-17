package media

import (
	"os"
	"runtime"
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "img"), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "img", "a.jpg"), []byte("x"), 0o600)

	ok := []string{"img/a.jpg", "img/../img/a.jpg", "/img/a.jpg", "img/none.jpg"}
	for _, rel := range ok {
		p, err := SafeJoin(root, rel)
		if err != nil {
			t.Errorf("%q 应合法: %v", rel, err)
			continue
		}
		if !within(root, p) {
			t.Errorf("%q → %q 跑出了根目录", rel, p)
		}
	}

	bad := []string{"../x", "../../../../etc/hosts", "img/../../x", ".."}
	if runtime.GOOS == "windows" {
		// 只有 Windows 把反斜杠当分隔符；其它系统上它只是文件名里的普通字符
		bad = append(bad, `..\..\windows`, `C:\Windows\win.ini`)
	}
	for _, rel := range bad {
		if _, err := SafeJoin(root, rel); err == nil {
			t.Errorf("%q 应被拒绝", rel)
		}
	}

	if _, err := SafeJoin("", "a.jpg"); err == nil {
		t.Error("空根目录应被拒绝")
	}
}

func TestSafeJoinSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("当前系统不支持符号链接:", err)
	}
	if _, err := SafeJoin(root, "link/secret"); err == nil {
		t.Error("通过符号链接跳出根目录应被拒绝")
	}
}

func TestEmojiURLAllowed(t *testing.T) {
	cases := map[string]bool{
		"http://wxapp.tc.qq.com/262/20304/stodownload?m=abc": true,
		"https://mmbiz.qpic.cn/x.gif":                        true,
		"http://emoji.qpic.cn/x":                             true,
		"http://127.0.0.1:5200/api/v1/sessions":              false,
		"http://169.254.169.254/latest/meta-data":            false,
		"http://qq.com.evil.example/x":                       false,
		"http://evilqq.com/x":                                false,
		"file:///etc/passwd":                                 false,
		"gopher://wxapp.tc.qq.com/x":                         false,
		"http://user:pw@wxapp.tc.qq.com/x":                   false,
	}
	for raw, want := range cases {
		u, err := parseURL(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got := emojiURLAllowed(u); got != want {
			t.Errorf("%s: got %v, want %v", raw, got, want)
		}
	}
}
