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
	var cached []model.GalaxyRawFeature
	if c.Query("force") != "1" {
		readJSONFile(galaxyPath("raw_features.json"), &cached) // §30 增量:复用未变联系人的特征
	}
	graph, feats, err := a.Store.BuildGalaxyIncremental(c.Request.Context(), &p, tzSec, topN, cached, loadOverrides())
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	_ = writeJSONFile(galaxyPath("raw_features.json"), feats)
	_ = writeJSONFile(galaxyPath("relationship_graph.json"), graph)
	applyGalaxyOverrides(graph)
	transport.SendSuccess(c, graph)
}

// GetGalaxyGraph GET /api/v1/galaxy/graph —— 优先读缓存，无则按 profile 现算。
func (a *API) GetGalaxyGraph(c *gin.Context) {
	var graph model.RelationshipGraph
	if readJSONFile(galaxyPath("relationship_graph.json"), &graph) && len(graph.Nodes) > 0 {
		applyGalaxyOverrides(&graph)
		transport.SendSuccess(c, graph)
		return
	}
	var p model.UserProfile
	if !readJSONFile(galaxyPath("user_profile.json"), &p) || p.BirthYear == 0 {
		c.JSON(http.StatusConflict, gin.H{"success": false, "need_profile": true, "message": "请先设置出生年月"})
		return
	}
	tzSec := resolveTzMinutes(c) * 60
	var cached []model.GalaxyRawFeature
	readJSONFile(galaxyPath("raw_features.json"), &cached)
	g, feats, err := a.Store.BuildGalaxyIncremental(c.Request.Context(), &p, tzSec, 50, cached, loadOverrides())
	if err != nil {
		transport.InternalServerError(c, err.Error())
		return
	}
	_ = writeJSONFile(galaxyPath("raw_features.json"), feats)
	_ = writeJSONFile(galaxyPath("relationship_graph.json"), g)
	applyGalaxyOverrides(g)
	transport.SendSuccess(c, g)
}

// loadOverrides 读出用户的手动修正。
// 建图时就要拿到它 —— 关系类型和人生阶段决定节点落在星图的哪个扇区，
// 只在建完之后改标签会让位置和标签对不上。
func loadOverrides() map[string]*model.ContactOverride {
	var ov model.GalaxyOverrides
	if !readJSONFile(galaxyPath("overrides.json"), &ov) || len(ov.Contacts) == 0 {
		return map[string]*model.ContactOverride{}
	}
	return ov.Contacts
}

// applyGalaxyOverrides 把 overrides.json 里的用户修正套到图上(不改缓存文件本身,
// 因此系统原判与用户修正同时保留 §28);隐藏的联系人从节点与时间轴中剔除。
func applyGalaxyOverrides(graph *model.RelationshipGraph) {
	var ov model.GalaxyOverrides
	if !readJSONFile(galaxyPath("overrides.json"), &ov) || len(ov.Contacts) == 0 {
		return
	}
	keptN := graph.Nodes[:0]
	for _, n := range graph.Nodes {
		o := ov.Contacts[n.ContactID]
		if o == nil {
			keptN = append(keptN, n)
			continue
		}
		if o.Hidden {
			continue
		}
		if o.RelationshipType != "" {
			n.RelationshipType = o.RelationshipType
		}
		if o.RelationshipLabel != "" {
			n.RelationshipLabel = o.RelationshipLabel
		}
		if o.MainLifeStage != "" {
			n.MainLifeStage = o.MainLifeStage
		}
		if o.ManualImportant {
			n.ManualImportant = true
		}
		if o.Pinned {
			n.Pinned = true
		}
		keptN = append(keptN, n)
	}
	graph.Nodes = keptN

	if graph.Timeline != nil {
		keptC := graph.Timeline.Contacts[:0]
		for _, c := range graph.Timeline.Contacts {
			o := ov.Contacts[c.ContactID]
			if o != nil {
				if o.Hidden {
					continue
				}
				if o.RelationshipType != "" {
					c.Type = o.RelationshipType
				}
			}
			keptC = append(keptC, c)
		}
		graph.Timeline.Contacts = keptC
	}
}

// GetGalaxyOverrides GET /api/v1/galaxy/overrides
func (a *API) GetGalaxyOverrides(c *gin.Context) {
	var ov model.GalaxyOverrides
	readJSONFile(galaxyPath("overrides.json"), &ov)
	if ov.Contacts == nil {
		ov.Contacts = map[string]*model.ContactOverride{}
	}
	transport.SendSuccess(c, ov)
}

// PatchGalaxyContact PATCH /api/v1/galaxy/contact/:id —— 保存单个联系人的手动修正。
// body: {relationship_type?, relationship_label?, main_life_stage?, manual_important?, hidden?, reset?}
func (a *API) PatchGalaxyContact(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		RelationshipType  *string `json:"relationship_type"`
		RelationshipLabel *string `json:"relationship_label"`
		MainLifeStage     *string `json:"main_life_stage"`
		ManualImportant   *bool   `json:"manual_important"`
		Pinned            *bool   `json:"pinned"`
		Hidden            *bool   `json:"hidden"`
		Reset             bool    `json:"reset"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.BadRequest(c, "参数错误")
		return
	}
	var ov model.GalaxyOverrides
	readJSONFile(galaxyPath("overrides.json"), &ov)
	if ov.Contacts == nil {
		ov.Contacts = map[string]*model.ContactOverride{}
	}
	if body.Reset {
		delete(ov.Contacts, id)
	} else {
		o := ov.Contacts[id]
		if o == nil {
			o = &model.ContactOverride{}
			ov.Contacts[id] = o
		}
		if body.RelationshipType != nil {
			o.RelationshipType = *body.RelationshipType
		}
		if body.RelationshipLabel != nil {
			o.RelationshipLabel = *body.RelationshipLabel
		}
		if body.MainLifeStage != nil {
			o.MainLifeStage = *body.MainLifeStage
		}
		if body.ManualImportant != nil {
			o.ManualImportant = *body.ManualImportant
		}
		if body.Pinned != nil {
			o.Pinned = *body.Pinned
		}
		if body.Hidden != nil {
			o.Hidden = *body.Hidden
		}
	}
	if err := writeJSONFile(galaxyPath("overrides.json"), &ov); err != nil {
		transport.InternalServerError(c, "保存失败: "+err.Error())
		return
	}
	// 任何修正都可能改变布局或入选名单（关系类型/人生阶段决定扇区，pin 决定是否
	// 突破 TopN），所以一律作废缓存图，下次取图时重建（走增量，很快）。
	_ = os.Remove(galaxyPath("relationship_graph.json"))
	transport.SendSuccess(c, gin.H{"ok": true})
}
