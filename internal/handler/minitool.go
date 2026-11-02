package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// MiniToolHandler exposes the 小工具 catalog.
//
// A card is a prompt preset (formula + inputs + domain knowledge) that an agent
// or a single chat turn can carry. The handler owns request shape and
// permissions only; rendering into the system prompt happens at chat time.
type MiniToolHandler struct {
	service interfaces.MiniToolService
}

// NewMiniToolHandler creates the catalog handler.
func NewMiniToolHandler(service interfaces.MiniToolService) *MiniToolHandler {
	return &MiniToolHandler{service: service}
}

type createMiniToolRequest struct {
	Name        string                  `json:"name"`
	Icon        string                  `json:"icon"`
	Category    string                  `json:"category"`
	Description string                  `json:"description"`
	Formula     string                  `json:"formula"`
	FormulaNote string                  `json:"formula_note"`
	Variables   types.MiniToolVariables `json:"variables"`
	Knowledge   string                  `json:"knowledge"`
	Examples    types.MiniToolExamples  `json:"examples"`
	Visibility  string                  `json:"visibility"`
	Enabled     *bool                   `json:"enabled,omitempty"`
}

type updateMiniToolRequest struct {
	Name        *string                  `json:"name,omitempty"`
	Icon        *string                  `json:"icon,omitempty"`
	Category    *string                  `json:"category,omitempty"`
	Description *string                  `json:"description,omitempty"`
	Formula     *string                  `json:"formula,omitempty"`
	FormulaNote *string                  `json:"formula_note,omitempty"`
	Variables   *types.MiniToolVariables `json:"variables,omitempty"`
	Knowledge   *string                  `json:"knowledge,omitempty"`
	Examples    *types.MiniToolExamples  `json:"examples,omitempty"`
	Visibility  *string                  `json:"visibility,omitempty"`
	Enabled     *bool                    `json:"enabled,omitempty"`
}

func (h *MiniToolHandler) fail(c *gin.Context, err error, message string) {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		c.Error(appErr)
		return
	}
	logger.ErrorWithFields(c.Request.Context(), err, nil)
	c.Error(apperrors.NewInternalServerError(message).WithDetails(err.Error()))
}

// ListMiniTools godoc
// @Summary      列出小工具
// @Description  返回当前空间可见的小工具卡片，支持分类、关键词、范围筛选。
// @Tags         小工具
// @Produce      json
// @Param        category  query  string  false  "分类"
// @Param        keyword   query  string  false  "关键词"
// @Param        scope     query  string  false  "范围"  Enums(all,mine,workspace)
// @Param        include_disabled query bool false "是否包含已停用"
// @Success      200  {object}  handler.DataResponse{data=[]types.MiniTool}  "小工具列表"
// @Security     Bearer
// @Router       /mini-tools [get]
func (h *MiniToolHandler) ListMiniTools(c *gin.Context) {
	query := types.MiniToolQuery{
		Category:        strings.TrimSpace(c.Query("category")),
		Keyword:         strings.TrimSpace(c.Query("keyword")),
		Scope:           strings.TrimSpace(c.Query("scope")),
		IncludeDisabled: c.Query("include_disabled") == "true",
	}
	items, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		h.fail(c, err, "Failed to list mini tools")
		return
	}
	if items == nil {
		items = []*types.MiniTool{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// ListMiniToolCategories godoc
// @Summary      小工具分类
// @Description  返回当前空间可见小工具的分类集合，用于筛选栏。
// @Tags         小工具
// @Produce      json
// @Success      200  {object}  handler.DataResponse{data=[]string}  "分类列表"
// @Security     Bearer
// @Router       /mini-tools/categories [get]
func (h *MiniToolHandler) ListMiniToolCategories(c *gin.Context) {
	items, err := h.service.Categories(c.Request.Context())
	if err != nil {
		h.fail(c, err, "Failed to list mini tool categories")
		return
	}
	if items == nil {
		items = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// GetMiniTool godoc
// @Summary      获取小工具
// @Tags         小工具
// @Produce      json
// @Param        id  path  string  true  "小工具 ID"
// @Success      200  {object}  handler.DataResponse{data=types.MiniTool}  "小工具"
// @Security     Bearer
// @Router       /mini-tools/{id} [get]
func (h *MiniToolHandler) GetMiniTool(c *gin.Context) {
	item, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.fail(c, err, "Failed to get mini tool")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}

// CreateMiniTool godoc
// @Summary      创建小工具
// @Description  创建一张小工具卡片：公式/方法、输入变量、领域知识与示例。
// @Tags         小工具
// @Accept       json
// @Produce      json
// @Param        request  body  handler.createMiniToolRequest  true  "小工具"
// @Success      200  {object}  handler.DataResponse{data=types.MiniTool}  "创建结果"
// @Security     Bearer
// @Router       /mini-tools [post]
func (h *MiniToolHandler) CreateMiniTool(c *gin.Context) {
	var req createMiniToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	item := &types.MiniTool{
		Name:        req.Name,
		Icon:        req.Icon,
		Category:    req.Category,
		Description: req.Description,
		Formula:     req.Formula,
		FormulaNote: req.FormulaNote,
		Variables:   req.Variables,
		Knowledge:   req.Knowledge,
		Examples:    req.Examples,
		Visibility:  req.Visibility,
	}
	created, err := h.service.Create(c.Request.Context(), item)
	if err != nil {
		h.fail(c, err, "Failed to create mini tool")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": created})
}

// UpdateMiniTool godoc
// @Summary      更新小工具
// @Description  局部更新一张小工具卡片；未提交的字段保持原值。
// @Tags         小工具
// @Accept       json
// @Produce      json
// @Param        id       path  string  true  "小工具 ID"
// @Param        request  body  handler.updateMiniToolRequest  true  "更新内容"
// @Success      200  {object}  handler.DataResponse{data=types.MiniTool}  "更新结果"
// @Security     Bearer
// @Router       /mini-tools/{id} [put]
func (h *MiniToolHandler) UpdateMiniTool(c *gin.Context) {
	var req updateMiniToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	updated, err := h.service.Update(c.Request.Context(), c.Param("id"), interfaces.MiniToolPatch{
		Name:        req.Name,
		Icon:        req.Icon,
		Category:    req.Category,
		Description: req.Description,
		Formula:     req.Formula,
		FormulaNote: req.FormulaNote,
		Variables:   req.Variables,
		Knowledge:   req.Knowledge,
		Examples:    req.Examples,
		Visibility:  req.Visibility,
		Enabled:     req.Enabled,
	})
	if err != nil {
		h.fail(c, err, "Failed to update mini tool")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}

// DeleteMiniTool godoc
// @Summary      删除小工具
// @Tags         小工具
// @Produce      json
// @Param        id  path  string  true  "小工具 ID"
// @Success      200  {object}  handler.DataResponse  "删除结果"
// @Security     Bearer
// @Router       /mini-tools/{id} [delete]
func (h *MiniToolHandler) DeleteMiniTool(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		h.fail(c, err, "Failed to delete mini tool")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DuplicateMiniTool godoc
// @Summary      复制小工具
// @Description  复制一张卡片为当前用户的私有副本。
// @Tags         小工具
// @Produce      json
// @Param        id  path  string  true  "小工具 ID"
// @Success      200  {object}  handler.DataResponse{data=types.MiniTool}  "副本"
// @Security     Bearer
// @Router       /mini-tools/{id}/duplicate [post]
func (h *MiniToolHandler) DuplicateMiniTool(c *gin.Context) {
	item, err := h.service.Duplicate(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.fail(c, err, "Failed to duplicate mini tool")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}
