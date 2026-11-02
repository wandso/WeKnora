package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"

	"github.com/gin-gonic/gin"
)

// ModerationHandler exposes the content-moderation observation log.
//
// It is a separate handler from AgentOpsHandler on purpose: moderation is
// workspace-scoped (rules) as well as agent-scoped (hits), and keeping it apart
// avoids growing the agent-ops constructor signature that existing tests build
// directly.
type ModerationHandler struct {
	svc interfaces.ModerationService
}

// NewModerationHandler builds the handler.
func NewModerationHandler(svc interfaces.ModerationService) *ModerationHandler {
	return &ModerationHandler{svc: svc}
}

// UpsertModerationRuleRequest is the rule body. Pointer fields distinguish
// "not provided" from "explicitly false" on update.
type UpsertModerationRuleRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"     binding:"required"`
	Category string `json:"category"`
	Pattern  string `json:"pattern"  binding:"required"`
	IsRegex  bool   `json:"is_regex"`
	Action   string `json:"action"`
	Enabled  *bool  `json:"enabled"`
}

// ListModerationRules handles GET /moderation-rules.
func (h *ModerationHandler) ListModerationRules(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	rules, err := h.svc.ListRules(c.Request.Context(), tenantID)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rules})
}

// UpsertModerationRule handles POST /moderation-rules. A non-empty id updates.
func (h *ModerationHandler) UpsertModerationRule(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	var req UpsertModerationRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rule, err := h.svc.UpsertRule(c.Request.Context(), tenantID, &types.ModerationRule{
		ID:       req.ID,
		Name:     req.Name,
		Category: req.Category,
		Pattern:  req.Pattern,
		IsRegex:  req.IsRegex,
		Action:   req.Action,
		Enabled:  enabled,
	})
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}

// DeleteModerationRule handles DELETE /moderation-rules/:id.
func (h *ModerationHandler) DeleteModerationRule(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListModerationHits handles GET /agents/:id/moderation-hits.
func (h *ModerationHandler) ListModerationHits(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	limit, offset := opsPageParams(c)
	hits, total, err := h.svc.ListHits(c.Request.Context(), tenantID, c.Param("id"), limit, offset)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": hits, "total": total})
}

// ScanModeration handles POST /agents/:id/moderation-scan.
//
// This is the only entry point in the observation phase: it never alters a
// conversation, it only appends matches to the audit log.
func (h *ModerationHandler) ScanModeration(c *gin.Context) {
	tenantID, _, ok := opsIdentity(c)
	if !ok {
		return
	}
	var req struct {
		Limit int `json:"limit"`
	}
	// Body is optional; an empty POST means "use the default window".
	_ = c.ShouldBindJSON(&req)
	result, err := h.svc.ScanAgent(c.Request.Context(), tenantID, c.Param("id"), req.Limit)
	if err != nil {
		opsFail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
