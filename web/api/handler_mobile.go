package api

import (
	"github.com/DrMaomao12345/Wetrace-Plus/web/transport"
	"github.com/gin-gonic/gin"
)

// mobileTokenViperKey 旧版单 token 的 viper 键（仅用于启动迁移）
const mobileTokenViperKey = "MOBILE_API_TOKEN"

// ListMobilePairings 列出所有配对记录（含历史）
// GET /api/v1/system/mobile/pairings
func (a *API) ListMobilePairings(c *gin.Context) {
	transport.SendSuccess(c, gin.H{
		"pairings": a.MobilePairings.List(),
	})
}

// CreateMobilePairing 新建一条配对记录
// POST /api/v1/system/mobile/pairings  body: { "label": "我的 iPhone" }
func (a *API) CreateMobilePairing(c *gin.Context) {
	var req struct {
		Label string `json:"label"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Label == "" {
		req.Label = "新配对"
	}
	rec, err := a.MobilePairings.Create(req.Label)
	if err != nil {
		transport.InternalServerError(c, "生成配对失败")
		return
	}
	transport.SendSuccess(c, rec)
}

// DeleteMobilePairing 删除一条配对记录（该设备立即失效）
// DELETE /api/v1/system/mobile/pairings/:id
func (a *API) DeleteMobilePairing(c *gin.Context) {
	id := c.Param("id")
	if !a.MobilePairings.Delete(id) {
		transport.BadRequest(c, "记录不存在")
		return
	}
	transport.SendSuccess(c, gin.H{"status": "deleted"})
}

// RenameMobilePairing 重命名一条配对记录
// PUT /api/v1/system/mobile/pairings/:id  body: { "label": "..." }
func (a *API) RenameMobilePairing(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Label string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Label == "" {
		transport.BadRequest(c, "label 不能为空")
		return
	}
	if !a.MobilePairings.Rename(id, req.Label) {
		transport.BadRequest(c, "记录不存在")
		return
	}
	transport.SendSuccess(c, gin.H{"status": "renamed"})
}

// MobilePing iOS App 验证 token 是否有效 + 探测连通性
// GET /api/v1/system/mobile/ping
func (a *API) MobilePing(c *gin.Context) {
	transport.SendSuccess(c, gin.H{
		"ok":      true,
		"service": "wetrace-plus",
	})
}

// GetMobileConfig 返回可供移动端展示的只读配置。
func (a *API) GetMobileConfig(c *gin.Context) {
	cfg := gin.H{"mode": "import_only"}
	if a.AI != nil {
		cfg["ai_model"] = a.AI.Model
		cfg["ai_base_url"] = a.AI.BaseURL
	}
	if a.BackupScheduler != nil {
		bs := a.BackupScheduler.GetStatus()
		cfg["backup_enabled"] = bs.Enabled
		cfg["backup_interval_hours"] = bs.IntervalHours
	}
	transport.SendSuccess(c, cfg)
}
