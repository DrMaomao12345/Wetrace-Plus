package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/afumu/wetrace/web/api"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// isLoopbackRequest 判断请求是否来自本机（127.0.0.1 / ::1）。
// 用 RemoteAddr（真实 TCP 对端，不可伪造），不用 ClientIP（受 X-Forwarded-For 影响）。
func isLoopbackRequest(c *gin.Context) bool {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		host = c.Request.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// AuthMiddleware 密码保护中间件
//
// 鉴权规则：
//   - 既没密码也没移动端 token → 完全开放
//   - 有有效凭据（Web 会话 token 或 移动端 API token）→ 放行
//   - 否则：本机访问 + 没设密码 → 放行（移动端 token 只防远程；本机是可信的，
//     否则电脑端自己的 Web UI 在生成 token 后会把自己锁在门外）
//   - 其余 → 401
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
			"/health",
		}
		for _, w := range whitelist {
			if strings.HasPrefix(path, w) {
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
		if validWebSession || validMobile {
			c.Next()
			return
		}

		// 兜底：本机访问 + 没设密码 → 放行。
		// 移动端 token 的目的是防"远程"访问；电脑端自己（localhost）始终可信，
		// 否则生成移动端 token 后，本机 Web UI 没凭据会把自己锁死。
		if hash == "" && isLoopbackRequest(c) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    401,
				"message": "未授权：请先验证密码或完成移动端配对",
			},
		})
	}
}
