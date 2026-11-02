package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterModerationRoutes wires the content-moderation observation log.
//
// Reads are Viewer+; writing a rule or triggering a scan is Contributor+,
// matching the other operations-centre endpoints. Scanning is a write in the
// API sense only — it appends audit rows and never alters a conversation.
func RegisterModerationRoutes(r *gin.RouterGroup, h *handler.ModerationHandler, g *rbacGuards) {
	if h == nil {
		return
	}

	agents := g.apiKeyGroup(r.Group("/agents"), apiKeyFullAccess())
	{
		agents.GET("/:id/moderation-hits", g.Viewer(), h.ListModerationHits)
		agents.POST("/:id/moderation-scan", g.Contributor(), h.ScanModeration)
	}

	rules := g.apiKeyGroup(r.Group("/moderation-rules"), apiKeyFullAccess())
	{
		rules.GET("", g.Viewer(), h.ListModerationRules)
		rules.POST("", g.Contributor(), h.UpsertModerationRule)
		rules.DELETE("/:id", g.Contributor(), h.DeleteModerationRule)
	}
}
