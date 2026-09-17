package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func runGuard(h gin.HandlerFunc, method, host string, headers map[string]string) int {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(h)
	r.Any("/api/v1/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(method, "/api/v1/x", nil)
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestHostGuard(t *testing.T) {
	g := HostGuard([]string{"mac.tail1234.ts.net"})
	cases := []struct {
		host string
		want int
	}{
		{"127.0.0.1:5200", 200},
		{"localhost:5200", 200},
		{"[::1]:5200", 200},
		{"192.168.1.20:5200", 200},        // 直接用 IP 访问（手机、局域网）
		{"100.92.77.20:5200", 200},        // Tailscale IP
		{"mac.tail1234.ts.net:5200", 200}, // 显式配置的主机名
		{"MAC.TAIL1234.TS.NET.", 200},     // 大小写 + 末尾点
		{"evil.example:5200", 421},        // DNS 重绑定的典型 Host
		{"127.0.0.1.evil.example", 421},
		{"localhost.evil.example", 421},
	}
	for _, c := range cases {
		if got := runGuard(g, http.MethodGet, c.host, nil); got != c.want {
			t.Errorf("Host %q: got %d, want %d", c.host, got, c.want)
		}
	}
}

func TestCSRFGuard(t *testing.T) {
	g := CSRFGuard()
	cases := []struct {
		name    string
		method  string
		headers map[string]string
		want    int
	}{
		{"读请求不管来源", http.MethodGet, map[string]string{"Origin": "https://evil.example"}, 200},
		{"同源写", http.MethodPost, map[string]string{"Origin": "http://127.0.0.1:5200"}, 200},
		{"本机开发端口", http.MethodPost, map[string]string{"Origin": "http://localhost:5173"}, 200},
		{"跨站表单", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, 403},
		{"跨站 DELETE", http.MethodDelete, map[string]string{"Origin": "https://evil.example"}, 403},
		{"null 来源", http.MethodPost, map[string]string{"Origin": "null"}, 403},
		{"只有 Sec-Fetch-Site=cross-site", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"只有 Sec-Fetch-Site=same-site", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		{"Sec-Fetch-Site=same-origin", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin"}, 200},
		{"非浏览器客户端（无 Origin）", http.MethodPost, nil, 200},
		{"javascript 伪协议来源", http.MethodPost, map[string]string{"Origin": "javascript://localhost"}, 403},
	}
	for _, c := range cases {
		if got := runGuard(g, c.method, "127.0.0.1:5200", c.headers); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestMobileForbidden(t *testing.T) {
	allowed := [][2]string{
		{"GET", "/api/v1/sessions"},
		{"GET", "/api/v1/system/mobile/ping"},
		{"POST", "/api/v1/report/annual"},
		{"POST", "/api/v1/ai/summarize"},
		{"PUT", "/api/v1/galaxy/profile"},
		{"PATCH", "/api/v1/galaxy/contact/wxid_x"},
		{"POST", "/api/v1/galaxy/rebuild"},
		{"POST", "/api/v1/system/default_timezone"},
	}
	for _, c := range allowed {
		if mobileForbidden(c[0], c[1]) {
			t.Errorf("%s %s 应允许移动端调用", c[0], c[1])
		}
	}
	denied := [][2]string{
		{"GET", "/api/v1/system/mobile/pairings"}, // 列表里有别的设备的 token
		{"POST", "/api/v1/system/mobile/pairings"},
		{"POST", "/api/v1/system/ai_config"},
		{"POST", "/api/v1/system/tts_config"},
		{"POST", "/api/v1/system/password/set"},
		{"POST", "/api/v1/system/password/disable"},
		{"POST", "/api/v1/system/backup_config"},
		{"POST", "/api/v1/imports"},
		{"DELETE", "/api/v1/sessions/wxid_x"},
		{"DELETE", "/api/v1/ai/summary_history/1"},
		{"PUT", "/api/v1/feishu/config"},
		{"POST", "/api/v1/monitor/test"},
		{"POST", "/api/v1/analysis/wordcloud/dict"},
	}
	for _, c := range denied {
		if !mobileForbidden(c[0], c[1]) {
			t.Errorf("%s %s 不应允许移动端调用", c[0], c[1])
		}
	}
}
