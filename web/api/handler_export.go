package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/pkg/util"
	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

// ExportChat 处理导出聊天记录的请求
func (a *API) ExportChat(c *gin.Context) {
	talker := c.Query("talker")
	talkerName := c.Query("name")
	timeRange := c.Query("time_range")

	if talker == "" {
		transport.BadRequest(c, "talker 参数是必需的")
		return
	}

	if talkerName == "" {
		talkerName = talker
	}

	start, end, ok := util.TimeRangeOf(timeRange)
	if !ok {
		// 默认导出所有 (2000-01-01 ~ Now+24h 类似之前的逻辑，或者直接用 All)
		start = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Now().Add(24 * time.Hour)
	}

	format := c.Query("format")

	var (
		data        []byte
		err         error
		fileName    string
		contentType string
	)

	ctx := c.Request.Context()

	switch format {
	case "txt":
		data, err = a.Export.ExportChatTxt(ctx, talker, talkerName, start, end)
		fileName = fmt.Sprintf("chat_export_%s_%s.txt", talkerName, talker)
		contentType = "text/plain; charset=utf-8"
	case "csv":
		data, err = a.Export.ExportChatCSV(ctx, talker, talkerName, start, end)
		fileName = fmt.Sprintf("chat_export_%s_%s.csv", talkerName, talker)
		contentType = "text/csv; charset=utf-8"
	case "xlsx":
		data, err = a.Export.ExportChatXLSX(ctx, talker, talkerName, start, end)
		fileName = fmt.Sprintf("chat_export_%s_%s.xlsx", talkerName, talker)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "docx":
		data, err = a.Export.ExportChatDOCX(ctx, talker, talkerName, start, end)
		fileName = fmt.Sprintf("chat_export_%s_%s.docx", talkerName, talker)
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "pdf":
		data, err = a.Export.ExportChatPDF(ctx, talker, talkerName, start, end)
		fileName = fmt.Sprintf("chat_export_%s_%s.pdf", talkerName, talker)
		contentType = "application/pdf"
	default:
		// 默认导出 HTML ZIP
		data, err = a.Export.ExportChat(ctx, talker, talkerName, start, end)
		fileName = fmt.Sprintf("chat_export_%s_%s.zip", talkerName, talker)
		contentType = "application/octet-stream"
	}

	if err != nil {
		transport.InternalServerError(c, fmt.Sprintf("导出失败: %v", err))
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Type", contentType)
	c.Data(http.StatusOK, contentType, data)
}

// ExportForensic 处理法律取证导出请求，返回包含水印和完整性校验的 standalone HTML 报告
func (a *API) ExportForensic(c *gin.Context) {
	talker := c.Query("talker")
	talkerName := c.Query("name")
	timeRange := c.Query("time_range")

	if talker == "" {
		transport.BadRequest(c, "talker 参数是必需的")
		return
	}

	if talkerName == "" {
		talkerName = talker
	}

	start, end, ok := util.TimeRangeOf(timeRange)
	if !ok {
		start = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Now().Add(24 * time.Hour)
	}

	ctx := c.Request.Context()
	data, err := a.Export.ExportForensic(ctx, talker, talkerName, start, end)
	if err != nil {
		transport.InternalServerError(c, fmt.Sprintf("取证导出失败: %v", err))
		return
	}

	fileName := fmt.Sprintf("forensic_export_%s_%s.zip", talkerName, talker)
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Type", "application/zip")
	c.Data(http.StatusOK, "application/zip", data)
}

// setAttachment 写下载响应头。
//
// 文件名带中文时必须同时给 RFC 5987 的 filename*：裸的 filename= 只允许
// ASCII，塞中文进去是非法头，浏览器各显神通，多半落成一串乱码。
// 保留一个 ASCII 兜底的 filename= 给不认 filename* 的老客户端。
func setAttachment(c *gin.Context, fileName, contentType string) {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return '_'
		}
		return r
	}, fileName)
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(fileName)))
	c.Header("Content-Type", contentType)
}

// ExportMonthlyStats 导出某个会话「从第一条消息起，每个月多少条」的统计表。
//
// 和 ExportChat 不同，这里不接 time_range：它的意义就是**全量**回顾，
// 掐掉一段反而看不出「什么时候加上的、哪几个月热、哪几个月冷」。
//
// fill_to_now=0 时右边界停在最后一条消息所在的月，不补那条 0 尾巴；
// 缺省（或任何非 "0"/"false" 的值）都按补到当月算。
func (a *API) ExportMonthlyStats(c *gin.Context) {
	talker := c.Query("talker")
	talkerName := c.Query("name")
	if talker == "" {
		transport.BadRequest(c, "talker 参数是必需的")
		return
	}
	if talkerName == "" {
		talkerName = talker
	}

	fillToNow := true
	if v := c.Query("fill_to_now"); v == "0" || v == "false" {
		fillToNow = false
	}

	ctx := c.Request.Context()
	var (
		data        []byte
		err         error
		ext         string
		contentType string
	)
	switch c.Query("format") {
	case "xlsx":
		data, err = a.Export.ExportMonthlyStatsXLSX(ctx, talker, talkerName, fillToNow)
		ext = "xlsx"
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		data, err = a.Export.ExportMonthlyStatsCSV(ctx, talker, fillToNow)
		ext = "csv"
		contentType = "text/csv; charset=utf-8"
	}
	if err != nil {
		transport.InternalServerError(c, fmt.Sprintf("导出失败: %v", err))
		return
	}

	setAttachment(c, fmt.Sprintf("monthly_stats_%s.%s", sanitizeFileName(talkerName), ext), contentType)
	c.Data(http.StatusOK, contentType, data)
}
