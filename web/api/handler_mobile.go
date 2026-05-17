package api

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// 移动端 API Token 持久化在 viper（.env）里。
// 它是一个长期有效的凭据，iOS App 配对后保存在 Keychain，
// 之后所有请求带 X-Auth-Token: <token> 即可。

const mobileTokenViperKey = "MOBILE_API_TOKEN"

// genMobileToken 生成 32 字节随机 token
func genMobileToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// currentMobileToken 读当前移动端 token（可能为空）
func currentMobileToken() string {
	return viper.GetString(mobileTokenViperKey)
}

// isValidMobileToken 校验 token 是否为有效的移动端 token
func isValidMobileToken(token string) bool {
	cur := currentMobileToken()
	return cur != "" && token != "" && token == cur
}

// GetMobileToken 查询当前移动端 token 状态
// GET /api/v1/system/mobile/token
func (a *API) GetMobileToken(c *gin.Context) {
	tok := currentMobileToken()
	transport.SendSuccess(c, gin.H{
		"has_token": tok != "",
		"token":     tok, // 仅在本地 Web UI 内展示，用于生成二维码
	})
}

// CreateMobileToken 生成（或重新生成）移动端 token
// POST /api/v1/system/mobile/token
func (a *API) CreateMobileToken(c *gin.Context) {
	tok, err := genMobileToken()
	if err != nil {
		transport.InternalServerError(c, "生成 token 失败")
		return
	}
	viper.Set(mobileTokenViperKey, tok)
	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}
	transport.SendSuccess(c, gin.H{"token": tok})
}

// RevokeMobileToken 吊销移动端 token（已配对的 App 立即失效）
// DELETE /api/v1/system/mobile/token
func (a *API) RevokeMobileToken(c *gin.Context) {
	viper.Set(mobileTokenViperKey, "")
	if err := viper.WriteConfig(); err != nil {
		transport.InternalServerError(c, "保存配置失败: "+err.Error())
		return
	}
	transport.SendSuccess(c, gin.H{"status": "revoked"})
}

// MobilePing iOS App 用来验证 token 是否有效 + 拿基础信息
// GET /api/v1/system/mobile/ping
func (a *API) MobilePing(c *gin.Context) {
	transport.SendSuccess(c, gin.H{
		"ok":      true,
		"service": "wetrace-pro",
	})
}
