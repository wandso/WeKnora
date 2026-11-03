package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/application/access"
	"github.com/Tencent/WeKnora/internal/application/service"
	// Referenced only by the @Failure annotations (errors.AppError); swag
	// resolves annotation types through the file's imports.
	_ "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// usableSkillLister returns the installed skills a chat turn can actually
// invoke on one sandbox config. The @ picker and the agent editor both read
// this set so they cannot offer a skill the running image does not carry.
type usableSkillLister interface {
	ListUsableSkills(ctx context.Context, tenantID uint64, configID string) []*types.TenantSkillEntity
}

// SkillHandler handles skill-related HTTP requests
type SkillHandler struct {
	usableSkills usableSkillLister
	catalog      skillCatalogService
	// agents resolves a shared agent so the @ picker can list the skills that
	// agent can actually invoke, which live in ITS OWNER's workspace. Nil
	// disables the shared-agent path (and with it the @Skill picker for shared
	// agents) rather than falling back to the caller's own workspace.
	agents access.SharedAgentLookup
}

type skillCatalogService interface {
	ListCatalog(ctx context.Context, tenantID uint64) ([]service.SkillCatalogView, error)
	RegisterCatalogFromArchive(ctx context.Context, tenantID uint64, archive []byte) (*types.TenantSkillCatalogEntity, error)
	RegisterCatalogsFromArchive(ctx context.Context, tenantID uint64, archive []byte) ([]*types.TenantSkillCatalogEntity, error)
	RegisterCatalogFromSource(ctx context.Context, tenantID uint64, source string) (*types.TenantSkillCatalogEntity, error)
	InstallCatalogToConfigs(ctx context.Context, tenantID uint64, catalogID string, configIDs []string) (*service.CatalogInstallResult, error)
	DeleteCatalog(ctx context.Context, tenantID uint64, catalogID string) error
	SetCatalogVisibility(
		ctx context.Context, callerTenantID uint64, catalogID string, visibility string,
	) (*types.TenantSkillCatalogEntity, error)
	ListCatalogFiles(ctx context.Context, tenantID uint64, catalogID string) ([]service.SkillFileEntry, error)
	ReadCatalogFile(ctx context.Context, tenantID uint64, catalogID, relativePath string) (*service.SkillFileContent, error)
	// EditCatalogDescription rewrites the console description of one language;
	// lang "" or "source" targets the canonical SKILL.md text.
	EditCatalogDescription(
		ctx context.Context, callerTenantID uint64, catalogID, lang, description string,
	) (*service.SkillCatalogDescriptionView, error)
	// TranslateCatalogDescription machine-translates a description and, when
	// save is set, stores it as that language's variant.
	TranslateCatalogDescription(
		ctx context.Context, callerTenantID uint64, catalogID, lang, text string, save bool,
	) (string, *service.SkillCatalogDescriptionView, error)
}

// NewSkillHandler creates a new skill handler. catalog may be nil in tests
// that only exercise the chat picker.
func NewSkillHandler(
	usableSkills usableSkillLister,
	catalog skillCatalogService,
	agents access.SharedAgentLookup,
) *SkillHandler {
	return &SkillHandler{
		usableSkills: usableSkills,
		catalog:      catalog,
		agents:       agents,
	}
}

// SkillInfoResponse represents the skill info returned to frontend
type SkillInfoResponse struct {
	Name        string `json:"name" extensions:"x-agent"`
	Description string `json:"description" extensions:"x-agent"`
	// Governance metadata for the picker. Category is safe | read_local |
	// read_remote | write | unclassified; RiskLevel is low | medium | high |
	// critical. Both are always populated so the UI need not guess.
	Category   string   `json:"category" extensions:"x-agent"`
	RiskLevel  string   `json:"risk_level" extensions:"x-agent"`
	Keywords   []string `json:"keywords,omitempty"`
	PromptHint string   `json:"prompt_hint,omitempty"`
}

// ListSkills godoc
// @Summary      获取当前沙箱配置上可执行的 Skills
// @Description  返回指定沙箱配置镜像内、智能体实际能调用的已安装技能（ready 且启用）。
// @Description  不传 sandbox_config_id 时列表为空。
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        sandbox_config_id      query  string  false  "Sandbox config ID from agent.get; ignored for a shared agent" extensions(x-agent,x-agent-required)
// @Param        agent_id               query  string  false  "Agent ID; needs agent_source_tenant_id"
// @Param        agent_source_tenant_id query  int     false  "Shared agent source workspace"
// @Success      200  {object}  handler.ListSkillsResponse  "Skills列表"
// @Failure      403  {object}  errors.AppError         "无权使用该共享智能体"
// @Security     Bearer
// @Security     ApiKeyAuth
// @x-agent      {"id":"skill.list","risk":"read"}
// @x-agent-hint "Lists usable skills, chosen by name. Pass sandbox_config_id from agent.get; when the agent has none, pass host (WeKnora Lite runs skills locally). An empty list means there are no usable skills there; if the agent has no sandbox, tell the user to choose one in the agent editor."
// @Router       /skills [get]
func (h *SkillHandler) ListSkills(c *gin.Context) {
	// A shared agent runs its skills in its OWNER's workspace, on the sandbox
	// config that agent selected. Reading either from the caller's workspace
	// (or from the query string) would list nothing, which is why the @Skill
	// picker used to come up empty for every shared agent.
	agent, err := sharedAgentPickerScope(c, h.agents)
	if err != nil {
		_ = c.Error(err)
		return
	}

	configID := c.Query("sandbox_config_id")
	tenantID := sandboxConfigTenantID(c)
	// allowed nil means "no name filter"; anyAllowed false means the agent can
	// invoke no skill at all, so nothing is looked up.
	var allowed map[string]bool
	anyAllowed := true
	if agent != nil {
		// Take the config from the agent, never from the query: the caller
		// could otherwise name any config id in the owner's workspace and
		// enumerate skills the shared agent does not use.
		configID = agent.Config.SandboxConfigID
		tenantID = agent.TenantID
		allowed, anyAllowed = sharedAgentSkillScope(agent)
	}

	if configID == "" || tenantID == 0 || !anyAllowed || h.usableSkills == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"data":             []SkillInfoResponse{},
			"skills_available": false,
		})
		return
	}

	rows := h.usableSkills.ListUsableSkills(c.Request.Context(), tenantID, configID)
	response := make([]SkillInfoResponse, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if allowed != nil && !allowed[row.Name] {
			continue
		}
		response = append(response, SkillInfoResponse{
			Name:        row.Name,
			Description: row.Description,
			Category:    normalizeSkillCategory(row.Category),
			RiskLevel:   normalizeSkillRiskLevel(row.RiskLevel),
			Keywords:    splitSkillKeywords(row.Keywords),
			PromptHint:  row.PromptHint,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"data":             response,
		"skills_available": true,
	})
}

// normalizeSkillCategory maps a stored value onto the vocabulary the UI
// renders. Rows installed before governance metadata existed are empty and
// become unclassified, which the badge shows as "未分类" rather than as safe.
func normalizeSkillCategory(raw string) string {
	return skills.NormalizeSkillCategory(raw)
}

// normalizeSkillRiskLevel reports medium for an unlabelled skill: an operator
// can lower it deliberately, but silence must not read as harmless.
func normalizeSkillRiskLevel(raw string) string {
	return skills.NormalizeSkillRiskLevel(raw)
}

func splitSkillKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
