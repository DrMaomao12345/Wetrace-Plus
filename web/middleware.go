package web

import (
	"net"
	"strings"

	"github.com/DrMaomao12345/Wetrace-Plus/web/middleware"
	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// setupMiddleware 配置 Gin 引擎所需的中间件。
func (s *Service) setupMiddleware() {
	s.router.Use(
		gin.LoggerWithWriter(log.Logger, "/health"),
		recoveryMiddleware(),
		middleware.HostGuard(s.allowedHosts()),
		corsMiddleware(),
		middleware.CSRFGuard(),
	)
}

// corsMiddleware 只允许来自 localhost/127.0.0.1 的跨域请求，防止外部站点 CSRF。
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && isAllowedOrigin(origin) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token, X-Auth-Token")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func isAllowedOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:") ||
		strings.HasPrefix(origin, "https://localhost:") ||
		strings.HasPrefix(origin, "https://127.0.0.1:")
}

// recoveryMiddleware 从任何 panic 中恢复并写入一个 500 错误。
func recoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().Interface("error", err).Msg("Panic recovered")
				transport.InternalServerError(c, "服务器内部发生错误。")
			}
		}()
		c.Next()
	}
}

// allowedHosts 显式配置的主机名，加上监听地址本身是主机名时的那个名字。
func (s *Service) allowedHosts() []string {
	hosts := append([]string{}, s.conf.AllowedHosts...)
	if h, _, err := net.SplitHostPort(s.conf.ListenAddr); err == nil && h != "" && net.ParseIP(h) == nil {
		hosts = append(hosts, h)
	}
	return hosts
}
