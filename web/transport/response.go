package transport

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	"github.com/afumu/wetrace/web/media"
	"github.com/gin-gonic/gin"
)

// Response 是成功请求的标准化 JSON 响应。
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

// SendSuccess 以 200 OK 状态和标准化的 JSON 成功载荷进行响应。
func SendSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
	})
}

// SendMedia 使用准备好的媒体内容或错误进行响应。
func SendMedia(c *gin.Context, pm media.PreparedMedia) {
	// 文件存在但解不开：用 415 与「找不到」「服务器错误」区分开，
	// 前端据此显示占位说明，而不是当成加载失败去重试。
	if pm.Encrypted {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"success":   false,
			"encrypted": true,
			"message":   pm.Reason,
		})
		return
	}
	if pm.Error != nil {
		if strings.Contains(pm.Error.Error(), "not found") || strings.Contains(pm.Error.Error(), "does not exist") {
			NotFound(c, pm.Error.Error())
		} else {
			InternalServerError(c, pm.Error.Error())
		}
		return
	}

	// 设置 Content-Type
	if pm.ContentType != "" {
		c.Header("Content-Type", pm.ContentType)
	}

	// 确定文件名后缀，辅助 ServeContent
	name := "file"
	switch pm.ContentType {
	case "video/mp4":
		name = "video.mp4"
	case "image/jpeg":
		name = "image.jpg"
	case "image/png":
		name = "image.png"
	case "audio/mp3":
		name = "audio.mp3"
	}

	// 使用 ServeContent 处理 Range 请求（对视频播放至关重要）
	reader := bytes.NewReader(pm.Content)
	http.ServeContent(c.Writer, c.Request, name, time.Time{}, reader)
}
