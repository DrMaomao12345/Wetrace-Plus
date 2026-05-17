package middleware

import (
	"net/http"
	"strings"

	"github.com/afumu/wetrace/web/api"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// AuthMiddleware 密码保护中间件
//
// 鉴权在以下任一条件成立时生效：
//   - 设置了 PASSWORD_HASH（Web 密码保护）
//   - 设置了 MOBILE_API_TOKEN（移动端配对后，公网访问需要凭据）
//
// 通过校验的凭据有两种：
//   - 有效的 Web 会话 token（密码登录后获得）
//   - 移动端 API token（iOS App 配对后获得）
func AuthMiddleware(a *api.API) gin.HandlerFunc {
	return func(c *gin.Context) {
		hash := viper.GetString("PASSWORD_HASH")
		mobileToken := viper.GetString("MOBILE_API_TOKEN")

		// 既没密码也没移动端 token → 完全开放
		if hash == "" && mobileToken == "" {
			c.Next()
			return
		}

		// 白名单路径不需要验证
		path := c.Request.URL.Path
		whitelist := []string{
			"/api/v1/system/password/status",
			"/api/v1/system/password/verify",
			"/api/v1/system/compliance",
			"/api/v1/system/mobile/ping", // 让 App 能探测连通性（仍需带 token，见下）
			"/health",
		}
		for _, w := range whitelist {
			if strings.HasPrefix(path, w) {
				// mobile/ping 例外：它需要 token 才算"配对成功"，所以不在这里放行
				if w == "/api/v1/system/mobile/ping" {
					break
				}
				c.Next()
				return
			}
		}

		// 非 API 路径不需要验证（静态文件等）
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}

		// 从 header 或 cookie 获取 token
		token := c.GetHeader("X-Auth-Token")
		if token == "" {
			token, _ = c.Cookie("auth_token")
		}

		// 校验：Web 会话 token 或 移动端 API token
		validWebSession := hash != "" && token != "" && a.Password.IsValidSession(token)
		validMobile := mobileToken != "" && token != "" && token == mobileToken

		if !validWebSession && !validMobile {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    401,
					"message": "未授权：请先验证密码或完成移动端配对",
				},
			})
			return
		}

		c.Next()
	}
}
