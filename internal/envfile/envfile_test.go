package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// 用真实的 viper 流程验证：含 $ 的值经过 viper 会被破坏，覆盖之后恢复正确。
func TestLoadPreservesDollarValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	const bcryptHash = `$2a$10$wnZcAbGZx1cv18/oUL2PtepwjSPPPyopVDg4JvZGtnkcRvPEdP73K`
	const apiKey = `sk-live$abc$123`

	content := "" +
		"# 注释行\n" +
		"\n" +
		"PASSWORD_HASH=" + bcryptHash + "\n" +
		"TTS_API_KEY=" + apiKey + "\n" +
		"export EXPORTED=yes\n" +
		`QUOTED="has spaces"` + "\n" +
		"PLAIN=1\n" +
		"WITH_EQUALS=a=b=c\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	// 先确认 viper 确实会破坏它 —— 这正是本包存在的理由
	viper.Reset()
	viper.SetConfigFile(path)
	viper.SetConfigType("env")
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if viper.GetString("PASSWORD_HASH") == bcryptHash {
		t.Skip("当前 viper 版本不再做 $ 展开，本修复已无必要")
	}

	// 覆盖回去
	for k, v := range Load(path) {
		viper.Set(k, v)
	}

	cases := map[string]string{
		"PASSWORD_HASH": bcryptHash,
		"TTS_API_KEY":   apiKey,
		"EXPORTED":      "yes",
		"QUOTED":        "has spaces",
		"PLAIN":         "1",
		"WITH_EQUALS":   "a=b=c",
	}
	for k, want := range cases {
		if got := viper.GetString(k); got != want {
			t.Errorf("%s\n want %q\n got  %q", k, want, got)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if got := Load(filepath.Join(t.TempDir(), "nope.env")); len(got) != 0 {
		t.Fatalf("文件不存在应返回空 map，got %v", got)
	}
}
