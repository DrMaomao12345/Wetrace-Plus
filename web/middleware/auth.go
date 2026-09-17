package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/DrMaomao12345/Wetrace-Plus/web/api"
	"github.com/gin-gonic/gin"
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

// 不需要凭据的接口：解锁页本身要能打开。按完整路径 + 方法匹配 ——
// 以前用前缀匹配，`/system/compliance` 顺带把 `/system/compliance/agree` 也放行了。
var publicEndpoints = map[string]string{
	"/api/v1/system/password/status": http.MethodGet,
	"/api/v1/system/password/verify": http.MethodPost,
	"/api/v1/system/compliance":      http.MethodGet,
}

// AuthMiddleware 密码保护中间件
//
// 鉴权规则：
//   - 非 API 路径（静态文件）和解锁页需要的几个接口 → 放行
//   - 本机访问 + 没设密码 → 放行（电脑端自己始终可信）
//   - 有效的 Web 会话 token → 放行
//   - 有效的移动端 token → 只放行手机端需要的读与计算接口（见 mobileForbidden）
//   - 其余 → 401
//
// 注意「远程 + 没设密码 + 没有任何配对」**不再放行**。以前这种情况整个 API 完全开放，
// 只要把 LISTEN_ADDR 改成 0.0.0.0，同一网络里的任何人都能读全部聊天记录。
func AuthMiddleware(a *api.API) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		if m, ok := publicEndpoints[path]; ok && m == c.Request.Method {
			c.Next()
			return
		}

		hasPassword := api.PasswordConfigured()
		if !hasPassword && isLoopbackRequest(c) {
			c.Next()
			return
		}

		token := c.GetHeader("X-Auth-Token")
		if token == "" {
			token, _ = c.Cookie("auth_token")
		}

		if hasPassword && token != "" && a.Password.IsValidSession(token) {
			c.Next()
			return
		}

		if token != "" && a.MobilePairings != nil && a.MobilePairings.IsValidToken(token) {
			if mobileForbidden(c.Request.Method, path) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"success": false,
					"error": gin.H{
						"code":    http.StatusForbidden,
						"message": "移动端配对只能查看与分析，修改设置请在电脑上操作",
					},
				})
				return
			}
			a.MobilePairings.Touch(token) // 更新该配对的「最后访问」
			c.Next()
			return
		}

		msg := "未授权：请先验证密码或完成移动端配对"
		if !hasPassword && (a.MobilePairings == nil || len(a.MobilePairings.List()) == 0) {
			msg = "未授权：远程访问前，请先在电脑上设置密码或创建移动端配对"
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": http.StatusUnauthorized, "message": msg},
		})
	}
}

// 移动端配对可以调用的写接口 —— 都是「算一下」而不是「改设置」。
// 清单来自 iOS App 实际发出的请求，再加上同类的只读计算接口。
var mobileWritable = map[string]bool{
	"POST /api/v1/report/annual":           true,
	"POST /api/v1/report/annual/stream":    true,
	"POST /api/v1/report/word_count":       true,
	"POST /api/v1/ai/summarize":            true,
	"POST /api/v1/ai/summarize/cancel":     true,
	"POST /api/v1/ai/sentiment":            true,
	"POST /api/v1/ai/summary":              true,
	"POST /api/v1/ai/todos":                true,
	"POST /api/v1/ai/extract":              true,
	"POST /api/v1/ai/voice2text":           true,
	"POST /api/v1/media/voice/transcribe":  true,
	"POST /api/v1/galaxy/rebuild":          true,
	"PUT /api/v1/galaxy/profile":           true,
	"POST /api/v1/system/default_timezone": true,
}

// mobileForbidden 判断仅凭移动端 token 能不能做这件事。
//
// 设计成「读随便、写要白名单」：移动端 token 一旦泄露（手机丢了、截图了），
// 损失也只到「能看」为止 —— 改不了 AI 服务地址、改不了本地可执行文件路径、
// 设不了密码、也拿不到别的设备的 token。
func mobileForbidden(method, path string) bool {
	// 配对列表里有全部设备的 token，只能在电脑上看
	if strings.HasPrefix(path, "/api/v1/system/mobile/pairings") {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	if mobileWritable[method+" "+path] {
		return false
	}
	// 星图里单个联系人的手动修正（PATCH /galaxy/contact/:id），iOS App 会用
	if method == http.MethodPatch && strings.HasPrefix(path, "/api/v1/galaxy/contact/") {
		return false
	}
	return true
}
