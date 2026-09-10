package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/importer"
	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

const maxImportRequestBytes int64 = 2 << 30

func (a *API) GetImportFormats(c *gin.Context) {
	transport.SendSuccess(c, importer.SupportedFormats())
}

func (a *API) GetImportHistory(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := importer.History(a.Conf.DataDir, limit)
	if err != nil {
		transport.InternalServerError(c, "读取导入历史失败: "+err.Error())
		return
	}
	transport.SendSuccess(c, items)
}

func (a *API) ImportChats(c *gin.Context) {
	if a.Importer == nil {
		transport.InternalServerError(c, "导入服务未初始化")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportRequestBytes)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		transport.BadRequest(c, "读取上传文件失败: "+err.Error())
		return
	}

	fileHeaders := c.Request.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		fileHeaders = c.Request.MultipartForm.File["file"]
	}
	if len(fileHeaders) == 0 {
		transport.BadRequest(c, "请选择至少一个 JSON、CSV 或 ZIP 文件")
		return
	}

	tempDir, err := os.MkdirTemp("", "wetrace-plus-import-*")
	if err != nil {
		transport.InternalServerError(c, "创建临时导入目录失败: "+err.Error())
		return
	}
	defer os.RemoveAll(tempDir)

	uploads := make([]importer.Upload, 0, len(fileHeaders))
	for index, header := range fileHeaders {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".json" && ext != ".csv" && ext != ".zip" {
			transport.BadRequest(c, fmt.Sprintf("不支持文件 %s；请选择 JSON、CSV 或 ZIP", header.Filename))
			return
		}
		source, err := header.Open()
		if err != nil {
			transport.BadRequest(c, "打开上传文件失败: "+err.Error())
			return
		}
		targetPath := filepath.Join(tempDir, fmt.Sprintf("%03d%s", index, ext))
		target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			_ = source.Close()
			transport.InternalServerError(c, "保存临时文件失败: "+err.Error())
			return
		}
		written, copyErr := io.Copy(target, source)
		closeErr := target.Close()
		_ = source.Close()
		if copyErr != nil || closeErr != nil {
			transport.BadRequest(c, "接收上传文件失败")
			return
		}
		uploads = append(uploads, importer.Upload{Name: filepath.Base(header.Filename), Path: targetPath, Size: written})
	}

	result, err := a.Importer.ImportFiles(c.Request.Context(), uploads, importer.Options{
		SelfID:   c.PostForm("self_id"),
		SelfName: c.PostForm("self_name"),
	})
	if err != nil {
		transport.BadRequest(c, err.Error())
		return
	}
	if err := a.Store.Reload(); err != nil {
		transport.InternalServerError(c, "文件已导入，但刷新分析数据失败: "+err.Error())
		return
	}
	a.Store.InvalidateTalkerTags()
	transport.SendSuccess(c, result)
}
