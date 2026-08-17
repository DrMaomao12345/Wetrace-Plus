//go:build windows

package api

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/afumu/wetrace/internal/cl/winkey"
	"github.com/afumu/wetrace/key/pkg/dllloader"
	"github.com/afumu/wetrace/key/pkg/imagekey"
	"github.com/afumu/wetrace/key/pkg/logger"
	"github.com/afumu/wetrace/key/pkg/options"
	"github.com/afumu/wetrace/key/pkg/process"
	"github.com/afumu/wetrace/wxkey"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// persistDbKey 把密钥写进 .env 并同步内存配置
func (a *API) persistDbKey(key string) {
	if err := updateEnv(map[string]string{"WECHAT_DB_KEY": key}); err != nil {
		log.Error().Err(err).Msg("更新 .env 文件失败")
		return
	}
	log.Info().Msg("已自动更新 .env 文件")
	a.mu.Lock()
	viper.Set("WECHAT_DB_KEY", key)
	a.Conf.WechatDbKey = key
	a.mu.Unlock()
}

// GetWeChatDbKey 获取微信数据库密钥。
//
// 两条路径，**优先内存读取**：
//
//  1. 直接读微信进程内存（internal/cl/winkey，OpenProcess + PROCESS_VM_READ）。
//     不注入、不重启微信、不依赖任何外部二进制 —— 与 macOS 侧对称。
//  2. 失败时回退到 DLL 注入（需要 wx_key.dll，且会强杀重启微信）。
//     该 DLL 不随仓库分发，多数环境没有，所以只作兜底。
func (a *API) GetWeChatDbKey(c *gin.Context) {
	log.Info().Msg("开始获取微信数据库密钥：优先直接读取进程内存...")

	res, memErr := winkey.ExtractWeChatKey(c.Request.Context())
	if memErr == nil && res != nil && res.DataKey != "" {
		log.Info().Uint32("pid", res.PID).Msg("已从微信进程内存提取到密钥（未重启微信）")
		a.persistDbKey(res.DataKey)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    gin.H{"pid": res.PID, "method": "memory"},
		})
		return
	}
	log.Warn().Err(memErr).Msg("内存读取未成功，尝试回退到 DLL 注入")

	opts := options.CliOptions{
		AutoMode:           true,
		NoRestart:          false, // 强制重启
		DllPath:            a.Conf.WxKeyDllPath,
		WechatPath:         a.Conf.WechatPath,
		KeyWaitTimeout:     120, // 参考 CLI 默认值
		StartupWaitTimeout: 30,  // 参考 CLI 默认值
	}

	if opts.DllPath == "" {
		if path, err := wxkey.GetDllPath(); err == nil {
			opts.DllPath = path
			log.Info().Str("path", path).Msg("找到 wx_key.dll")
		} else {
			log.Warn().Err(err).Msg("未找到 wx_key.dll")
		}
	}

	// ⚠️ DLL 不可用就必须在这里返回，**不能继续往下走**。
	// 原实现只打一条 warn 就回退到 "wxkey/wx_key.dll"（一个同样不存在的路径），
	// 然后照样把用户正在运行的微信杀掉重启 —— 用户点一次「获取密钥」＝
	// 白丢一次微信会话，换一个必然失败的结果。
	//
	// 注意：wx_key.dll **不在仓库里**（.gitignore 排除了 *.dll，git 历史中从未有过），
	// 也没有任何代码会生成它（wxkey/embed.go 名为 embed 却没有 //go:embed，
	// WxKeyDll 声明后从未赋值）。所以全新 clone 必然走到这里。
	// 需自行提供 DLL 并用 WXKEY_DLL_PATH 指定，或改用不依赖 DLL 的内存读取方案
	// （internal/cl/wechat/key/windows 里已有实现，但目前无人调用）。
	if _, err := os.Stat(opts.DllPath); opts.DllPath == "" || err != nil {
		log.Error().Str("path", opts.DllPath).Msg("wx_key.dll 不可用，取消操作（未触碰微信进程）")
		c.JSON(http.StatusPreconditionFailed, gin.H{
			"success": false,
			"message": fmt.Sprintf(
				"取密钥失败，微信未被关闭。\n"+
					"① 直接读取进程内存：%v\n"+
					"② DLL 注入：未找到 wx_key.dll（该文件不随仓库分发）。\n"+
					"通常按①的提示处理即可（确认微信已登录）；确需走②请把 DLL 放到 "+
					"wxkey/wx_key.dll 或用 .env 的 WXKEY_DLL_PATH 指定。", memErr),
		})
		return
	}

	pm := process.NewProcessManager()
	dl := dllloader.NewDllLoader()

	// 1. Prepare WeChat Process (prepareWeChatProcess 逻辑)
	log.Info().Msg("正在准备微信进程...")

	// 如果正在运行且允许重启，则杀死进程
	if pm.IsProcessRunning("Weixin.exe") {
		log.Info().Msg("检测到微信正在运行，正在关闭...")
		if err := pm.KillProcess("Weixin.exe"); err != nil {
			log.Error().Err(err).Msg("关闭微信失败")
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("关闭微信失败: %v", err),
			})
			return
		}
		log.Info().Msg("微信已关闭")
		time.Sleep(2 * time.Second)
	}

	// 启动微信
	wechatPath := opts.WechatPath
	if wechatPath == "" {
		wechatPath = pm.FindWeChatPath()
	}
	if wechatPath == "" {
		log.Error().Msg("未找到微信安装路径")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "未找到微信安装路径，请在 .env 中指定",
		})
		return
	}

	log.Info().Str("path", wechatPath).Msg("正在启动微信...")
	if err := pm.LaunchWeChat(wechatPath); err != nil {
		log.Error().Err(err).Msg("微信启动失败")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("微信启动失败: %v", err),
		})
		return
	}
	log.Info().Msg("微信启动成功")

	// 等待窗口
	log.Info().Msg("等待微信窗口出现...")
	if !pm.WaitForWeChatWindow(opts.StartupWaitTimeout) {
		log.Warn().Msg("等待微信窗口超时或窗口未显示，尝试直接获取 PID")
	} else {
		log.Info().Msg("微信窗口已出现")
	}

	// 等待进程初始化
	log.Info().Msg("等待微信进程初始化...")
	time.Sleep(2 * time.Second)

	// 获取 PID
	pid := pm.FindMainWeChatPid()
	if pid == 0 {
		log.Warn().Msg("未找到主窗口关联 PID，切换为进程扫描模式...")
		var err error
		pid, err = pm.GetProcessId("Weixin.exe")
		if err != nil {
			log.Error().Err(err).Msg("未找到微信进程")
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("未找到微信进程: %v", err),
			})
			return
		}
	}
	log.Info().Uint32("pid", pid).Msg("微信进程准备完成")

	// 2. Load DLL (loadDll 逻辑)
	log.Info().Str("path", opts.DllPath).Msg("正在加载 DLL...")
	if err := dl.Load(opts.DllPath); err != nil {
		log.Error().Err(err).Str("path", opts.DllPath).Msg("DLL 加载失败")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("DLL 加载失败: %v", err),
		})
		return
	}
	log.Info().Msg("DLL 加载成功")
	defer dl.CleanupHook()

	// 3. Install Hook (installHook 逻辑)
	log.Info().Uint32("pid", pid).Msg("正在安装 Hook...")
	if err := dl.InitializeHook(pid); err != nil {
		log.Error().Err(err).Uint32("pid", pid).Msg("Hook 安装失败")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("安装 Hook 失败: %v", err),
		})
		return
	}
	log.Info().Msg("Hook 安装成功")

	// 4. Wait for Key (waitForKey 逻辑)
	log.Info().Int("timeout", opts.KeyWaitTimeout).Msg("正在等待密钥，请在微信中完成登录...")
	deadline := time.Now().Add(time.Duration(opts.KeyWaitTimeout) * time.Second)
	var key string
	found := false

	for time.Now().Before(deadline) {
		// 轮询密钥
		if k, ok := dl.PollKeyData(); ok {
			key = k
			found = true
			log.Info().Msg("密钥获取成功")
			break
		}

		// 处理来自 DLL 的状态消息
		for i := 0; i < 5; i++ {
			msg, level, ok := dl.GetStatusMessage()
			if !ok {
				break
			}
			switch level {
			case 0:
				log.Info().Str("source", "DLL").Msg(msg)
			case 1:
				log.Info().Str("source", "DLL").Msg("SUCCESS: " + msg)
			case 2:
				log.Error().Str("source", "DLL").Msg(msg)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	if !found {
		log.Error().Msg("密钥获取超时")
		c.JSON(http.StatusRequestTimeout, gin.H{
			"success": false,
			"message": "获取密钥超时，请确保已登录微信",
		})
		return
	}

	a.persistDbKey(key)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"pid": pid, "method": "dll"},
	})
}

// GetWeChatImageKey 获取微信图片解密密钥 (参考 CliController.runImageKeyMode 逻辑)
func (a *API) GetWeChatImageKey(c *gin.Context) {
	log.Info().Msg("开始获取微信图片解密密钥 (持续扫描模式，最长 2 分钟)...")

	pm := process.NewProcessManager()

	// 0. 检查微信是否运行，若未运行则尝试启动
	if !pm.IsProcessRunning("Weixin.exe") {
		log.Info().Msg("检测到微信尚未启动，尝试自动启动...")
		wechatPath := a.Conf.WechatPath
		if wechatPath == "" {
			wechatPath = pm.FindWeChatPath()
		}

		if wechatPath == "" {
			log.Warn().Msg("未找到微信安装路径，提示用户登录")
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未检测到微信运行，且未找到微信安装路径，请手动启动并登录微信。",
			})
			return
		}

		log.Info().Str("path", wechatPath).Msg("正在启动微信...")
		if err := pm.LaunchWeChat(wechatPath); err != nil {
			log.Error().Err(err).Msg("自动启动微信失败")
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("自动启动微信失败: %v", err),
			})
			return
		}
		// 给予一点启动时间
		time.Sleep(2 * time.Second)
	}

	// 创建一个静默日志对象用于内部服务
	l := logger.NewLogger(false, true, true)
	svc := imagekey.NewImageKeyService(l)

	// 设置 2 分钟截止时间
	deadline := time.Now().Add(2 * time.Minute)
	retryInterval := 3 * time.Second
	firstAttempt := true

	for {
		// 检查是否超时
		if time.Now().After(deadline) {
			log.Error().Msg("获取图片密钥超时 (2 分钟)")
			c.JSON(http.StatusRequestTimeout, gin.H{
				"success": false,
				"message": "获取图片密钥超时。请确保微信已登录并在 2 分钟内打开过至少一张图片。",
			})
			return
		}

		// 检查客户端是否已断开连接
		select {
		case <-c.Request.Context().Done():
			log.Info().Msg("客户端已断开连接，停止扫描图片密钥")
			return
		default:
		}

		// 1. Determine PID
		pid := pm.FindMainWeChatPid()
		if pid == 0 {
			pid, _ = pm.GetProcessId("Weixin.exe")
		}

		if pid == 0 {
			if firstAttempt {
				log.Warn().Msg("未找到运行中的微信进程，将持续等待...")
				firstAttempt = false
			}
			time.Sleep(retryInterval)
			continue
		}

		// 2. Get Keys
		// 优先使用 WECHAT_DB_SRC_PATH 作为数据目录起点
		dataPath := a.Conf.WechatDataPath
		if a.Conf.WechatDbSrcPath != "" {
			dataPath = a.Conf.WechatDbSrcPath
		}

		log.Debug().Uint32("pid", pid).Str("searchPath", dataPath).Msg("正在执行密钥提取扫描...")
		keyResult := svc.GetImageKeys(pid, dataPath)

		if keyResult.Success {
			xorStr := fmt.Sprintf("%02X", keyResult.XorKey)
			log.Info().Msg("成功获取图片密钥")

			// 写入 .env 文件
			updates := map[string]string{
				"IMAGE_KEY": keyResult.AesKey,
				"XOR_KEY":   xorStr,
			}
			if err := updateEnv(updates); err != nil {
				log.Error().Err(err).Msg("更新 .env 文件中的图片密钥失败")
			} else {
				log.Info().Msg("已自动更新 .env 文件中的 IMAGE_KEY 和 XOR_KEY")
				// 同步更新内存中的配置和 viper 状态，确保后续解密和状态获取使用最新密钥
				a.mu.Lock()
				viper.Set("IMAGE_KEY", keyResult.AesKey)
				viper.Set("XOR_KEY", xorStr)

				a.Conf.ImageKey = keyResult.AesKey
				a.Conf.XorKey = xorStr

				a.Media.ImageKey = keyResult.AesKey
				a.Media.XorKey = xorStr
				a.mu.Unlock()
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"pid": pid,
				},
			})
			return
		}

		if firstAttempt {
			log.Info().Msg("初次扫描未找到密钥，进入持续观察模式。请在微信中打开任意图片...")
			firstAttempt = false
		}

		// 未找到，等待后重试
		time.Sleep(retryInterval)
	}
}
