package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterMiniToolRoutes wires the 小工具 catalog.
//
// Reads are open to every member: a workspace card is a preset, not somebody's
// private document. Writes need at least Contributor, and the service still
// enforces ownership, because a private card is invisible to other members even
// though they may create cards of their own.
func RegisterMiniToolRoutes(r *gin.RouterGroup, h *handler.MiniToolHandler, g *rbacGuards) {
	if h == nil {
		return
	}
	tools := g.apiKeyGroup(r.Group("/mini-tools"), apiKeyFullAccess())
	{
		// Literal paths must be registered before /:id, otherwise Gin binds
		// "categories" to the :id param.
		tools.GET("/categories", g.Viewer(), h.ListMiniToolCategories)
		tools.GET("", g.Viewer(), h.ListMiniTools)
		tools.GET("/:id", g.Viewer(), h.GetMiniTool)
		tools.POST("", g.Contributor(), h.CreateMiniTool)
		// Ownership (creator or Admin+) is checked in the service against the
		// stored row, which also covers workspace-visible cards.
		tools.PUT("/:id", g.Contributor(), h.UpdateMiniTool)
		// Visibility is a workspace-policy switch, not a content edit, so it is
		// a separate route: the shipped catalogue cards are not editable in
		// place, yet an admin must still be able to publish them.
		tools.PUT("/:id/visibility", g.Contributor(), h.SetMiniToolVisibility)
		tools.DELETE("/:id", g.Contributor(), h.DeleteMiniTool)
		tools.POST("/:id/duplicate", g.Contributor(), h.DuplicateMiniTool)
	}
}
