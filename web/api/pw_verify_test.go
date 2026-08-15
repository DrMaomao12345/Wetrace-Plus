package api

import (
	"encoding/base64"
	"testing"

	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

func TestPasswordHashSurvivesEnvRoundTrip(t *testing.T) {
	pw := "MyPass!123"
	h, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)

	viper.Reset()
	savePasswordHash(h)

	stored := viper.GetString(passwordHashKey)
	if stored == string(h) {
		t.Fatal("应当编码后再存，否则 .env 的 $ 展开会破坏它")
	}
	if _, err := base64.StdEncoding.DecodeString(stored[len("b64:"):]); err != nil {
		t.Fatalf("存的不是合法 base64: %v", err)
	}
	// 存的内容里不能有 $，否则仍会被展开
	for _, c := range stored {
		if c == '$' {
			t.Fatal("编码后仍含 $，仍会被 .env 解析器展开")
		}
	}
	if got := loadPasswordHash(); got != string(h) {
		t.Fatalf("读回来不一致\n want %q\n got  %q", string(h), got)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(loadPasswordHash()), []byte(pw)); err != nil {
		t.Fatalf("正确密码校验失败: %v", err)
	}
}
