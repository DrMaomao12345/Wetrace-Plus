package middleware

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// 这个文件挡的是「浏览器替攻击者发请求」这一整类问题。
//
// 服务默认只监听 127.0.0.1、默认也不要求密码 —— 但「只监听本机」挡不住浏览器：
// 你打开的任何网页都能让浏览器往 127.0.0.1 发请求。两条典型路线：
//
//   - DNS 重绑定：恶意页面把自己的域名解析切到 127.0.0.1，之后的请求在浏览器看来
//     是「同源」的，CORS 完全不起作用，能读到全部聊天记录。它的特征是 Host 头
//     是攻击者的域名 —— HostGuard 只认 IP 字面量、localhost 和显式配置的主机名。
//   - 跨站写请求（CSRF）：<form enctype="text/plain"> 可以不经预检就 POST 一段
//     「看起来像 JSON」的正文，而 gin 的 ShouldBindJSON 不看 Content-Type。
//     CSRFGuard 对所有写方法检查 Origin / Sec-Fetch-Site。
//
// 非浏览器客户端（iOS App、curl）不带 Origin，也不带 Sec-Fetch-Site，不受影响。

// HostGuard 只放行 Host 为 IP 字面量、localhost，或在 allowed 里显式列出的请求。
//
// IP 字面量总是安全的：DNS 重绑定必须借助攻击者控制的域名，浏览器发出的
// Host 就是那个域名；Host 是 IP 说明用户本来就是直接访问这台机器。
func HostGuard(allowed []string) gin.HandlerFunc {
	extra := make(map[string]bool, len(allowed))
	for _, h := range allowed {
		if h = normalizeHost(h); h != "" {
			extra[h] = true
		}
	}
	return func(c *gin.Context) {
		if hostAllowed(c.Request.Host, extra) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusMisdirectedRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    http.StatusMisdirectedRequest,
				"message": "拒绝访问：未知的 Host。通过域名访问时，请把它加进 .env 的 ALLOWED_HOSTS",
			},
		})
	}
}

func hostAllowed(hostport string, extra map[string]bool) bool {
	host := normalizeHost(hostport)
	if host == "" {
		// HTTP/1.0 没有 Host 头的老客户端；浏览器永远会带，放行不影响防护
		return true
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if net.ParseIP(host) != nil {
		return true
	}
	return extra[host]
}

// normalizeHost 去掉端口、方括号和末尾的点，统一小写。
func normalizeHost(hostport string) string {
	h := strings.TrimSpace(hostport)
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

// CSRFGuard 拒绝浏览器发起的跨站写请求。
//
// 判定顺序：
//  1. 读方法（GET / HEAD / OPTIONS）直接放行
//  2. 有 Origin：必须与请求的 Host 同源，或是本机开发地址（localhost / 127.0.0.1 任意端口）
//  3. 没有 Origin 但有 Sec-Fetch-Site：只接受 same-origin 和 none
//  4. 两者都没有：非浏览器客户端，放行（它们另有 token 鉴权）
func CSRFGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		if crossSiteWrite(c.Request) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"code": http.StatusForbidden, "message": "拒绝跨站请求"},
			})
			return
		}
		c.Next()
	}
}

func crossSiteWrite(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		if origin == "null" {
			return true // 沙箱 iframe、file:// 页面
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return true
		}
		if strings.EqualFold(u.Host, r.Host) {
			return false
		}
		return !isLocalDevOrigin(u)
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return false
	default: // same-site / cross-site
		return true
	}
}

// isLocalDevOrigin 本机的前端开发服务器（vite 等）跨端口调接口时的来源。
// 恶意网站不可能以 localhost 为来源，能用这个来源的只有本机软件。
func isLocalDevOrigin(u *url.URL) bool {
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	h := normalizeHost(u.Host)
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}
