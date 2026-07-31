package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/afumu/wetrace/internal/model"
	"github.com/afumu/wetrace/store/repo"
	"github.com/afumu/wetrace/web/transport"
	"github.com/gin-gonic/gin"
)

// 星图缓存目录(相对 cwd，即 ~/.wetrace/galaxy，独立于 data/ 不随重解密丢失）
const galaxyDir = "galaxy"

func galaxyPath(name string) string { return filepath.Join(galaxyDir, name) }

func readJSONFile(path string, v any) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return os.WriteFile(path, b, 0644)
}

// GetGalaxyProfile GET /api/v1/galaxy/profile —— 读用户出生年月/人生阶段。
func (a *API) GetGalaxyProfile(c *gin.Context) {
	var p model.UserProfile
	if readJSONFile(galaxyPath("user_profile.json"), &p) && p.BirthYear > 0 {
		transport.SendSuccess(c, gin.H{"exists": true, "profile": p})
		return
	}
	transport.SendSuccess(c, gin.H{"exists": false})
}

// UpdateGalaxyProfile PUT /api/v1/galaxy/profile —— 保存出生年月，缺 life_stages 时自动生成。
func (a *API) UpdateGalaxyProfile(c *gin.Context) {
	var p model.UserProfile
	if err := c.ShouldBindJSON(&p); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}
	if p.BirthYear < 1950 || p.BirthYear > 2025 || p.BirthMonth < 1 || p.BirthMonth > 12 {
		transport.BadRequest(c, "出生年月无效")
		return
	}
	if len(p.LifeStages) == 0 {
		p.LifeStages = repo.DefaultLifeStages(p.BirthYear, p.BirthMonth)
	}
	if err := writeJSONFile(galaxyPath("user_profile.json"), &p); err != nil {
		transport.InternalServerError(c, "保存失败: "+err.Error())
		return
	}
	_ = os.Remove(galaxyPath("relationship_graph.json")) // 出生年月变了，旧图作废
	transport.SendSuccess(c, gin.H{"profile": p})
}

// RebuildGalaxy POST /api/v1/galaxy/rebuild?top=&tz_offset= —— 强制重算并缓存。
func (a *API) RebuildGalaxy(c *gin.Context) {
	var p model.UserProfile
	if !readJSONFile(galaxyPath("user_profile.json"), &p) || p.BirthYear == 0 {
		transport.BadRequest(c, "请先设置出生年月")
		return
	}
	tzSec := resolveTzMinutes(c) * 60
	topN, _ := strconv.Atoi(c.Query("top"))
	if topN == 0 {
		topN = 50
	}
	graph, err := a.Store.BuildGalaxy(c.Request.Context(), &p, tzSec, topN)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	_ = writeJSONFile(galaxyPath("relationship_graph.json"), graph)
	transport.SendSuccess(c, graph)
}

// GetGalaxyGraph GET /api/v1/galaxy/graph —— 优先读缓存，无则按 profile 现算。
func (a *API) GetGalaxyGraph(c *gin.Context) {
	var graph model.RelationshipGraph
	if readJSONFile(galaxyPath("relationship_graph.json"), &graph) && len(graph.Nodes) > 0 {
		transport.SendSuccess(c, graph)
		return
	}
	var p model.UserProfile
	if !readJSONFile(galaxyPath("user_profile.json"), &p) || p.BirthYear == 0 {
		c.JSON(http.StatusConflict, gin.H{"success": false, "need_profile": true, "message": "请先设置出生年月"})
		return
	}
	tzSec := resolveTzMinutes(c) * 60
	g, err := a.Store.BuildGalaxy(c.Request.Context(), &p, tzSec, 50)
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	_ = writeJSONFile(galaxyPath("relationship_graph.json"), g)
	transport.SendSuccess(c, g)
}
