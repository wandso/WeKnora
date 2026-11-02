package handler

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// AgentOpsHandler exposes the agent operations centre: version history and
// rollback, answer feedback, usage/quality reporting, templates and public
// pages.
//
// Authorization is enforced by the route guards (ownership for mutating agent
// state, Viewer for reads). The handler only resolves the caller's identity
// and maps service errors to status codes.
type AgentOpsHandler struct {
	versions  interfaces.AgentVersionService
	feedback  interfaces.MessageFeedbackService
	usage     interfaces.AgentUsageService
	templates interfaces.AgentTemplateService
	pages     interfaces.AgentPublicPageService
	agents    interfaces.CustomAgentService
	dryRun    interfaces.AgentDryRunService
}

// NewAgentOpsHandler builds the operations-centre handler.
func NewAgentOpsHandler(
	versions interfaces.AgentVersionService,
	feedback interfaces.MessageFeedbackService,
	usage interfaces.AgentUsageService,
	templates interfaces.AgentTemplateService,
	pages interfaces.AgentPublicPageService,
	agents interfaces.CustomAgentService,
	dryRun interfaces.AgentDryRunService,
) *AgentOpsHandler {
	return &AgentOpsHandler{
		versions:  versions,
		feedback:  feedback,
		usage:     usage,
		templates: templates,
		pages:     pages,
		agents:    agents,
		dryRun:    dryRun,
	}
}

// opsIdentity resolves the (tenant, user) pair every endpoint scopes to.
func opsIdentity(c *gin.Context) (uint64, string, bool) {
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	userID, _ := c.Get(types.UserIDContextKey.String())
	uid, _ := userID.(string)
	if tenantID == 0 || uid == "" {
		c.Error(apperrors.NewUnauthorizedError("authentication required"))
		return 0, "", false
	}
	return tenantID, uid, true
}

func opsPageParams(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	return limit, offset
}

func opsSince(c *gin.Context) time.Time {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	return time.Now().AddDate(0, 0, -days)
}

// opsFail maps a service error to the right status. Unknown errors are logged
// and reported as 500 without leaking driver text.
func opsFail(c *gin.Context, err error) {
	ctx := c.Request.Context()
	switch {
	case stderrors.Is(err, service.ErrVersionNotFound),
		stderrors.Is(err, service.ErrTemplateNotFound),
		stderrors.Is(err, service.ErrPublicPageNotFound),
		stderrors.Is(err, service.ErrModerationRuleNotFound),
		stderrors.Is(err, service.ErrFeedbackMessageNotFound):
		c.Error(apperrors.NewNotFoundError(err.Error()))
	case stderrors.Is(err, service.ErrDryRunQueryRequired),
		stderrors.Is(err, service.ErrFeedbackInvalidRating),
		stderrors.Is(err, service.ErrFeedbackNotAssistant),
		stderrors.Is(err, service.ErrModerationRuleNameRequired),
		stderrors.Is(err, service.ErrModerationRulePatternInvalid),
		stderrors.Is(err, service.ErrPublicPageInvalidSlug),
		stderrors.Is(err, service.ErrPublicPagePasswordRequired):
		c.Error(apperrors.NewBadRequestError(err.Error()))
	case stderrors.Is(err, service.ErrPublicPageSlugTaken):
		c.Error(apperrors.NewConflictError(err.Error()))
	case stderrors.Is(err, service.ErrPublicPagePassword):
		c.Error(apperrors.NewUnauthorizedError(err.Error()))
	case stderrors.Is(err, service.ErrPublicPageWrongPassword):
		c.Error(apperrors.NewForbiddenError(err.Error()))
	case stderrors.Is(err, service.ErrAgentNotFound):
		c.Error(apperrors.NewNotFoundError("Agent not found"))
	case stderrors.Is(err, service.ErrCannotModifyBuiltin):
		c.Error(apperrors.NewForbiddenError("Cannot modify built-in agent"))
	default:
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(apperrors.NewInternalServerError("request failed"))
	}
}

// DryRunAgentRequest is the body for POST /agents/:id/dry-run. Config is the
// editor's unsaved draft; it replaces the stored config for this run only.
type DryRunAgentRequest struct {
	Query  string                  `json:"query" binding:"required"`
	Config types.CustomAgentConfig `json:"config"`
}

// DryRunAgent godoc
// @Summary      用草稿配置试跑智能体
// @Description  使用未保存的配置执行一次问答，返回答案、步骤与 token 消耗；不产生会话记录
// @Tags         智能体
// @Param        id    path  string              true  "智能体ID"
// @Param        body  body  DryRunAgentRequest  true  "草稿配置与问题"
// @Success      200   {object}  handler.DataResponse{data=types.AgentDryRunResult}
// @Router       /agents/{id}/dry-run [post]
func (h *AgentOpsHandler) DryRunAgent(c *gin.Context) {
	var req DryRunAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	res, err := h.dryRun.Run(c.Request.Context(), c.Param("id"), req.Config, req.Query)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

// ---------------------------------------------------------------------------
// Versions
// ---------------------------------------------------------------------------

// PublishAgentVersionRequest is the optional body for an explicit snapshot.
type PublishAgentVersionRequest struct {
	Changelog string `json:"changelog"`
}

// PublishAgentVersion godoc
// @Summary      创建智能体版本快照
// @Description  把智能体当前配置固化为一个新版本，用于发布前留档
// @Tags         智能体
// @Param        id    path  string                     true  "智能体ID"
// @Param        body  body  PublishAgentVersionRequest false "变更说明"
// @Success      200   {object}  handler.DataResponse{data=types.AgentVersion}
// @Router       /agents/{id}/versions [post]
func (h *AgentOpsHandler) PublishAgentVersion(c *gin.Context) {
	ctx := c.Request.Context()
	_, userID, ok := opsIdentity(c)
	if !ok {
		return
	}
	var req PublishAgentVersionRequest
	_ = c.ShouldBindJSON(&req) // body is optional

	agent, err := h.agents.GetAgentByID(ctx, c.Param("id"))
	if err != nil {
		opsFail(c, err)
		return
	}
	v, err := h.versions.Snapshot(ctx, agent, types.AgentVersionSourcePublish, req.Changelog, userID)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
}

// ListAgentVersions godoc
// @Summary      列出版本历史
// @Tags         智能体
// @Param        id     path   string  true   "智能体ID"
// @Param        limit  query  int     false  "每页数量"
// @Param        offset query  int     false  "偏移"
// @Success      200    {object}  handler.ListResponse{data=[]types.AgentVersion}
// @Router       /agents/{id}/versions [get]
func (h *AgentOpsHandler) ListAgentVersions(c *gin.Context) {
	limit, offset := opsPageParams(c)
	list, total, err := h.versions.List(c.Request.Context(), c.Param("id"), limit, offset)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    list,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// GetAgentVersion godoc
// @Summary      获取单个版本快照
// @Tags         智能体
// @Router       /agents/{id}/versions/{version_no} [get]
func (h *AgentOpsHandler) GetAgentVersion(c *gin.Context) {
	no, err := strconv.Atoi(c.Param("version_no"))
	if err != nil || no <= 0 {
		c.Error(apperrors.NewBadRequestError("version_no must be a positive integer"))
		return
	}
	v, err := h.versions.Get(c.Request.Context(), c.Param("id"), no)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
}

// DiffAgentVersions godoc
// @Summary      对比两个版本
// @Tags         智能体
// @Param        from  query  int  true  "起始版本号"
// @Param        to    query  int  true  "目标版本号"
// @Router       /agents/{id}/versions/diff [get]
func (h *AgentOpsHandler) DiffAgentVersions(c *gin.Context) {
	from, err1 := strconv.Atoi(c.Query("from"))
	to, err2 := strconv.Atoi(c.Query("to"))
	if err1 != nil || err2 != nil || from <= 0 || to <= 0 {
		c.Error(apperrors.NewBadRequestError("from and to must be positive integers"))
		return
	}
	diff, err := h.versions.Diff(c.Request.Context(), c.Param("id"), from, to)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": diff})
}

// RollbackAgentVersion godoc
// @Summary      回滚到指定版本
// @Description  把历史快照应用回智能体；回滚本身会生成一个新版本，历史不被改写
// @Tags         智能体
// @Router       /agents/{id}/versions/{version_no}/rollback [post]
func (h *AgentOpsHandler) RollbackAgentVersion(c *gin.Context) {
	no, err := strconv.Atoi(c.Param("version_no"))
	if err != nil || no <= 0 {
		c.Error(apperrors.NewBadRequestError("version_no must be a positive integer"))
		return
	}
	_, userID, ok := opsIdentity(c)
	if !ok {
		return
	}
	agent, err := h.versions.Rollback(c.Request.Context(), c.Param("id"), no, userID)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": agent})
}

// ---------------------------------------------------------------------------
// Feedback
// ---------------------------------------------------------------------------

// SubmitFeedbackRequest is the body for POST /answer-feedback. Session id is
// required because messages are looked up within their session.
type SubmitFeedbackRequest struct {
	SessionID  string   `json:"session_id" binding:"required"`
	MessageID  string   `json:"message_id" binding:"required"`
	Rating     string   `json:"rating" binding:"required"`
	ReasonTags []string `json:"reason_tags"`
	Comment    string   `json:"comment"`
}

// SubmitFeedback godoc
// @Summary      提交答案反馈（点赞/点踩）
// @Tags         智能体
// @Router       /answer-feedback [post]
func (h *AgentOpsHandler) SubmitFeedback(c *gin.Context) {
	tenantID, userID, ok := opsIdentity(c)
	if !ok {
		return
	}
	var req SubmitFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	f, err := h.feedback.Submit(c.Request.Context(), tenantID, userID,
		req.MessageID, req.SessionID, req.Rating, req.ReasonTags, req.Comment)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

// GetMyFeedback godoc
// @Summary      查询我对某条答案的反馈
// @Tags         智能体
// @Router       /answer-feedback [get]
func (h *AgentOpsHandler) GetMyFeedback(c *gin.Context) {
	tenantID, userID, ok := opsIdentity(c)
	if !ok {
		return
	}
	messageID := c.Query("message_id")
	if strings.TrimSpace(messageID) == "" {
		c.Error(apperrors.NewBadRequestError("message_id is required"))
		return
	}
	f, err := h.feedback.GetMine(c.Request.Context(), tenantID, userID, messageID)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

// ListAgentFeedback godoc
// @Summary      列出智能体的反馈
// @Tags         智能体
// @Param        rating  query  string  false  "up | down"
// @Router       /agents/{id}/feedback [get]
func (h *AgentOpsHandler) ListAgentFeedback(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	limit, offset := opsPageParams(c)
	list, total, err := h.feedback.List(c.Request.Context(), tenantID,
		c.Param("id"), c.Query("rating"), limit, offset)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    list,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// ---------------------------------------------------------------------------
// Usage and quality
// ---------------------------------------------------------------------------

// GetAgentUsage godoc
// @Summary      智能体用量汇总
// @Tags         智能体
// @Param        days  query  int  false  "统计天数，默认 30"
// @Router       /agents/{id}/usage [get]
func (h *AgentOpsHandler) GetAgentUsage(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	summary, err := h.usage.Summary(c.Request.Context(), tenantID, c.Param("id"), opsSince(c))
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// GetAgentUsageTimeseries godoc
// @Summary      智能体用量按天趋势
// @Tags         智能体
// @Router       /agents/{id}/usage/timeseries [get]
func (h *AgentOpsHandler) GetAgentUsageTimeseries(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	points, err := h.usage.Timeseries(c.Request.Context(), tenantID, c.Param("id"), opsSince(c))
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": points})
}

// GetAgentQuality godoc
// @Summary      智能体质量与用量报告
// @Tags         智能体
// @Router       /agents/{id}/quality [get]
func (h *AgentOpsHandler) GetAgentQuality(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	report, err := h.usage.Report(c.Request.Context(), tenantID, c.Param("id"), opsSince(c))
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

// ListAgentTemplates godoc
// @Summary      列出可用智能体模板
// @Tags         智能体
// @Param        category  query  string  false  "分类"
// @Router       /agent-templates [get]
func (h *AgentOpsHandler) ListAgentTemplates(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	list, err := h.templates.List(c.Request.Context(), tenantID, c.Query("category"))
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// InstallAgentTemplate godoc
// @Summary      从模板安装智能体
// @Tags         智能体
// @Router       /agent-templates/{id}/install [post]
func (h *AgentOpsHandler) InstallAgentTemplate(c *gin.Context) {
	tenantID, userID, ok := opsIdentity(c)
	if !ok {
		return
	}
	agent, err := h.templates.Install(c.Request.Context(), tenantID, userID, c.Param("id"))
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": agent})
}

// SaveAgentAsTemplateRequest is the body for POST /agents/:id/templates.
type SaveAgentAsTemplateRequest struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
}

// SaveAgentAsTemplate godoc
// @Summary      把智能体另存为模板
// @Tags         智能体
// @Router       /agents/{id}/templates [post]
func (h *AgentOpsHandler) SaveAgentAsTemplate(c *gin.Context) {
	tenantID, userID, ok := opsIdentity(c)
	if !ok {
		return
	}
	var req SaveAgentAsTemplateRequest
	_ = c.ShouldBindJSON(&req)
	t, err := h.templates.SaveFromAgent(c.Request.Context(), tenantID, userID,
		c.Param("id"), req.Name, req.Category, req.Icon, req.Description)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// DeleteAgentTemplate godoc
// @Summary      删除工作区模板
// @Tags         智能体
// @Router       /agent-templates/{id} [delete]
func (h *AgentOpsHandler) DeleteAgentTemplate(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	if err := h.templates.Delete(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// Public pages
// ---------------------------------------------------------------------------

// UpsertPublicPageRequest is the body for PUT /agents/:id/public-page.
type UpsertPublicPageRequest struct {
	EmbedChannelID string `json:"embed_channel_id" binding:"required"`
	Slug           string `json:"slug" binding:"required"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	OGImageURL     string `json:"og_image_url"`
	AccessMode     string `json:"access_mode"`
	// Password is only used when access_mode is "password". Omit it to keep
	// the existing password on update.
	Password string `json:"password"`
	Enabled  *bool  `json:"enabled"`
}

// GetAgentPublicPage godoc
// @Summary      获取智能体的公开主页配置
// @Tags         智能体
// @Router       /agents/{id}/public-page [get]
func (h *AgentOpsHandler) GetAgentPublicPage(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	page, err := h.pages.GetByAgent(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": page})
}

// UpsertAgentPublicPage godoc
// @Summary      创建或更新智能体公开主页
// @Tags         智能体
// @Router       /agents/{id}/public-page [put]
func (h *AgentOpsHandler) UpsertAgentPublicPage(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	var req UpsertPublicPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	existing, err := h.pages.GetByAgent(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		opsFail(c, err)
		return
	}
	page := &types.AgentPublicPage{
		TenantID:       tenantID,
		AgentID:        c.Param("id"),
		EmbedChannelID: req.EmbedChannelID,
		Slug:           req.Slug,
		Title:          req.Title,
		Description:    req.Description,
		OGImageURL:     req.OGImageURL,
		AccessMode:     req.AccessMode,
		Enabled:        true,
	}
	if existing != nil {
		page.ID = existing.ID
		// Preserve the stored hash when the caller is not changing the
		// password: an empty Password must not silently unlock the page.
		page.PasswordHash = existing.PasswordHash
		// enabled is tri-state: an omitted field keeps the current value
		// instead of silently taking the page offline.
		if req.Enabled == nil {
			page.Enabled = existing.Enabled
		}
	}
	if req.Enabled != nil {
		page.Enabled = *req.Enabled
	}
	saved, err := h.pages.Upsert(c.Request.Context(), page, req.Password)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": saved})
}

// DeleteAgentPublicPage godoc
// @Summary      删除智能体公开主页
// @Tags         智能体
// @Router       /agents/{id}/public-page [delete]
func (h *AgentOpsHandler) DeleteAgentPublicPage(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	if err := h.pages.Delete(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ResolvePublicAgent godoc
// @Summary      按 slug 解析公开主页（匿名）
// @Description  只返回落地页所需的元数据与嵌入渠道，不返回智能体配置
// @Tags         公开
// @Param        slug      path   string  true   "主页 slug"
// @Param        password  query  string  false  "访问密码"
// @Success      200       {object}  handler.DataResponse
// @Router       /public/agents/{slug} [get]
func (h *AgentOpsHandler) ResolvePublicAgent(c *gin.Context) {
	ctx := context.WithoutCancel(c.Request.Context())
	page, agent, err := h.pages.ResolvePublic(ctx, c.Param("slug"), c.Query("password"))
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"slug":             page.Slug,
			"title":            page.Title,
			"description":      page.Description,
			"og_image_url":     page.OGImageURL,
			"access_mode":      page.AccessMode,
			"embed_channel_id": page.EmbedChannelID,
			"agent": gin.H{
				"id":          agent.ID,
				"name":        agent.Name,
				"description": agent.Description,
				"avatar":      agent.Avatar,
			},
		},
	})
}
