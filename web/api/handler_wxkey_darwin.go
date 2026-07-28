//go:build darwin

package api

import (
	"context"
	"net/http"
	"time"

	"github.com/afumu/wetrace/internal/cl/mackey"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// GetWeChatDbKey 在 macOS 上提取微信数据库密钥。
// 通过嵌入的 chatlog 后端做进程检测 + 内存扫描（lldb/vmmap）。
// 前提：微信已登录运行、进程以 root 运行（sudo）、SIP 已关闭。
func (a *API) GetWeChatDbKey(c *gin.Context) {
	log.Info().Msg("开始在 macOS 上提取微信数据库密钥（内存扫描，需 sudo + 关闭 SIP）...")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()

	res, err := mackey.ExtractWeChatKey(ctx)
	if err != nil {
		log.Error().Err(err).Msg("提取微信数据库密钥失败")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 写入 .env，并自动把数据目录配置为解密源路径（省去手动配置）。
	updates := map[string]string{
		"WECHAT_DB_KEY":      res.DataKey,
		"WECHAT_DB_SRC_PATH": res.DataDir,
	}
	if res.ImageKey != "" {
		updates["IMAGE_KEY"] = res.ImageKey
	}
	if err := updateEnv(updates); err != nil {
		log.Error().Err(err).Msg("更新 .env 文件失败")
	}

	a.mu.Lock()
	viper.Set("WECHAT_DB_KEY", res.DataKey)
	viper.Set("WECHAT_DB_SRC_PATH", res.DataDir)
	a.Conf.WechatDbKey = res.DataKey
	a.Conf.WechatDbSrcPath = res.DataDir
	if res.ImageKey != "" {
		viper.Set("IMAGE_KEY", res.ImageKey)
		a.Conf.ImageKey = res.ImageKey
		a.Media.ImageKey = res.ImageKey
	}
	a.mu.Unlock()

	log.Info().
		Uint32("pid", res.PID).
		Int("version", res.Version).
		Str("data_dir", res.DataDir).
		Msg("密钥提取成功")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"pid":      res.PID,
			"version":  res.Version,
			"data_dir": res.DataDir,
		},
	})
}

// GetWeChatImageKey 在 macOS 上提取微信图片解密密钥。
// 与数据库密钥同源（一次内存扫描同时得到），v4 下需先在微信中打开过图片才可能命中。
func (a *API) GetWeChatImageKey(c *gin.Context) {
	log.Info().Msg("开始在 macOS 上提取微信图片密钥...")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()

	res, err := mackey.ExtractWeChatKey(ctx)
	if err != nil {
		log.Error().Err(err).Msg("提取微信图片密钥失败")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if res.ImageKey == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "未能提取到图片密钥；v4 版本需先在微信中打开至少一张图片后重试。",
		})
		return
	}

	if err := updateEnv(map[string]string{"IMAGE_KEY": res.ImageKey}); err != nil {
		log.Error().Err(err).Msg("更新 .env 文件失败")
	}

	a.mu.Lock()
	viper.Set("IMAGE_KEY", res.ImageKey)
	a.Conf.ImageKey = res.ImageKey
	a.Media.ImageKey = res.ImageKey
	a.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"pid": res.PID,
		},
	})
}
