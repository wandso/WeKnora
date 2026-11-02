package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterAgentOpsRoutes wires the operations-centre endpoints.
//
// Path layout note: the version diff lives at /agents/:id/version-diff rather
// than /agents/:id/versions/diff. Gin's radix tree does not reliably allow a
// static child ("diff") beside a parameter child (":version_no") under the same
// parent, and a route that panics at startup is not worth the prettier URL.
func RegisterAgentOpsRoutes(r *gin.RouterGroup, h *handler.AgentOpsHandler, g *rbacGuards) {
	if h == nil {
		return
	}

	agents := g.apiKeyGroup(r.Group("/agents"), apiKeyFullAccess())
	{
		// Trial run with an unsaved draft. It executes real model calls, so it
		// follows the same ownership rule as editing the agent.
		agents.POST("/:id/dry-run", g.OwnedAgentOrAdmin(), h.DryRunAgent)
		// Version history. Reads are Viewer+; creating a snapshot or rolling
		// back changes what production traffic runs, so it follows the same
		// ownership rule as editing the agent.
		agents.GET("/:id/versions", g.Viewer(), h.ListAgentVersions)
		agents.POST("/:id/versions", g.OwnedAgentOrAdmin(), h.PublishAgentVersion)
		agents.GET("/:id/version-diff", g.Viewer(), h.DiffAgentVersions)
		agents.GET("/:id/versions/:version_no", g.Viewer(), h.GetAgentVersion)
		agents.POST("/:id/versions/:version_no/rollback", g.OwnedAgentOrAdmin(), h.RollbackAgentVersion)

		// Quality and usage.
		agents.GET("/:id/feedback", g.Viewer(), h.ListAgentFeedback)
		agents.GET("/:id/usage", g.Viewer(), h.GetAgentUsage)
		agents.GET("/:id/usage/timeseries", g.Viewer(), h.GetAgentUsageTimeseries)
		agents.GET("/:id/quality", g.Viewer(), h.GetAgentQuality)

		// Templates and public pages.
		agents.POST("/:id/templates", g.Contributor(), h.SaveAgentAsTemplate)
		agents.GET("/:id/public-page", g.Viewer(), h.GetAgentPublicPage)
		agents.PUT("/:id/public-page", g.OwnedAgentOrAdmin(), h.UpsertAgentPublicPage)
		agents.DELETE("/:id/public-page", g.OwnedAgentOrAdmin(), h.DeleteAgentPublicPage)
	}

	// Feedback is addressed by message, not by agent, so it gets its own
	// prefix. It sits behind the chat capability: only a caller that can read
	// a conversation can judge its answers.
	feedback := g.apiKeyGroup(r.Group("/answer-feedback"), apiKeyChat(apiKeyFullAccess()))
	{
		feedback.POST("", g.Viewer(), h.SubmitFeedback)
		feedback.GET("", g.Viewer(), h.GetMyFeedback)
	}

	templates := g.apiKeyGroup(r.Group("/agent-templates"), apiKeyFullAccess())
	{
		templates.GET("", g.Viewer(), h.ListAgentTemplates)
		templates.POST("/:id/install", g.Contributor(), h.InstallAgentTemplate)
		// Tenant templates only; built-ins are product content and are not
		// deletable through the API.
		templates.DELETE("/:id", g.Admin(), h.DeleteAgentTemplate)
	}
}

// RegisterAgentPublicRoutes registers the anonymous landing-page resolver.
//
// It is mounted before the global auth middleware because a visitor following a
// shared link has no account. The response carries page metadata and the embed
// channel id only — never the agent's config, prompts or knowledge scope.
func RegisterAgentPublicRoutes(r *gin.Engine, h *handler.AgentOpsHandler) {
	if h == nil {
		return
	}
	r.GET("/api/v1/public/agents/:slug", h.ResolvePublicAgent)
}
