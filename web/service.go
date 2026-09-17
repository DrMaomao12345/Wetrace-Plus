package web

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/store"
	"github.com/DrMaomao12345/Wetrace-Plus/web/api"
	"github.com/DrMaomao12345/Wetrace-Plus/web/media"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Service 定义了 web 服务。
type Service struct {
	store       store.Store
	router      *gin.Engine
	server      *http.Server
	localServer *http.Server // 主地址非回环时额外监听的 127.0.0.1
	conf        *Config
	api         *api.API
	media       *media.Service
	staticFS    fs.FS
}

// Config 保存 web 服务的配置。
type Config struct {
	ListenAddr       string
	AllowedHosts     []string // 除 IP 与 localhost 外，允许通过的主机名（如 Tailscale MagicDNS 名）
	DataDir          string
	AIEnabled        bool
	AIProvider       string
	AIAPIKey         string
	AIBaseURL        string
	AIModel          string
	ChangelogContent string
}

// NewService 创建一个新的 web 服务。
func NewService(store store.Store, conf *Config, staticFS fs.FS) *Service {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// 媒体根目录用专用子目录：以前直接是数据目录本身，?path=message/message_0.db
	// 就能把整个分析库当「图片」下载走。导入包目前不带附件，这个目录多半是空的。
	mediaService := media.NewService(conf.DataDir, filepath.Join(conf.DataDir, "files"))

	// 创建共享的 API 配置指针
	apiConf := &api.Config{
		DataDir:          conf.DataDir,
		AIEnabled:        conf.AIEnabled,
		AIProvider:       conf.AIProvider,
		AIAPIKey:         conf.AIAPIKey,
		AIBaseURL:        conf.AIBaseURL,
		AIModel:          conf.AIModel,
		ChangelogContent: conf.ChangelogContent,
	}

	apiHandler := api.NewAPI(store, mediaService, apiConf, staticFS)

	s := &Service{
		store:    store,
		router:   router,
		conf:     conf,
		api:      apiHandler,
		media:    mediaService,
		staticFS: staticFS,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// Start 开始提供 web 应用服务。
func (s *Service) Start() error {
	s.server = newHTTPServer(s.conf.ListenAddr, s.router)

	log.Info().Msg(fmt.Sprintf("在 %s 上启动 web 服务", s.conf.ListenAddr))

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Web 服务启动失败")
		}
	}()

	// 主地址不是本机回环时，额外再监听一份 127.0.0.1。
	//
	// 为的是把「远程」和「本机」分开：主地址（比如 Tailscale IP）暴露给手机，
	// 受密码保护；本机这份让桌面快捷方式、书签、以及免密的本地访问照常可用
	// —— 鉴权中间件正是靠请求源是不是回环来区分这两者的。
	if extra := loopbackCompanion(s.conf.ListenAddr); extra != "" {
		s.localServer = newHTTPServer(extra, s.router)
		log.Info().Msg(fmt.Sprintf("同时在 %s 上监听（本机免密访问）", extra))
		go func() {
			if err := s.localServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Warn().Err(err).Msg("本机回环监听启动失败（不影响主地址）")
			}
		}()
	}

	return nil
}

// newHTTPServer 统一设置超时。
//
// 只设 ReadHeaderTimeout / IdleTimeout：慢速发送请求头（Slowloris）会占满连接。
// 不设 ReadTimeout / WriteTimeout —— 大文件导入要慢慢传，年度报告是流式推送，
// 设了会把正常请求截断。
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// loopbackCompanion 给一个非回环的监听地址算出配套的 127.0.0.1 地址。
// 已经是回环或监听全部网卡（0.0.0.0 已包含回环）时返回空。
func loopbackCompanion(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]", "127.0.0.1", "localhost", "::1":
		return ""
	}
	return net.JoinHostPort("127.0.0.1", port)
}

// Stop 优雅地关闭 web 服务器。
func (s *Service) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 附带的本机监听也要关，否则重启时端口还占着
	if s.localServer != nil {
		_ = s.localServer.Shutdown(ctx)
	}

	if err := s.server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("优雅关闭 web 服务器失败")
		return err
	}

	log.Info().Msg("Web 服务已停止")
	return nil
}

func (s *Service) GetRouter() *gin.Engine {
	return s.router
}
