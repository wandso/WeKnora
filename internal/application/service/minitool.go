// Package service implements the 小工具 catalog: workspace-scoped preset cards
// whose formula, inputs and domain knowledge are injected into an agent turn.
//
// The module deliberately does not own conversations. A card is a prompt
// preset that any agent can carry; the existing chat pipeline renders it, so
// there is one conversation store, one history and one place where approvals
// happen.
package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type miniToolService struct {
	repo interfaces.MiniToolRepository
}

// NewMiniToolService creates the catalog service.
func NewMiniToolService(repo interfaces.MiniToolRepository) interfaces.MiniToolService {
	return &miniToolService{repo: repo}
}

func (s *miniToolService) caller(ctx context.Context) (uint64, string, types.TenantRole, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return 0, "", "", apperrors.NewForbiddenError("a workspace is required to use mini tools")
	}
	userID, _ := types.UserIDFromContext(ctx)
	return tenantID, userID, types.TenantRoleFromContext(ctx), nil
}

func canManageMiniTool(tool *types.MiniTool, userID string, role types.TenantRole) bool {
	if tool == nil {
		return false
	}
	if role == types.TenantRoleOwner || role == types.TenantRoleAdmin {
		return true
	}
	return userID != "" && tool.OwnerUserID == userID
}

func canReadMiniTool(tool *types.MiniTool, userID string, role types.TenantRole) bool {
	if tool == nil {
		return false
	}
	if tool.Visibility == types.MiniToolVisibilityWorkspace {
		return true
	}
	return canManageMiniTool(tool, userID, role)
}

// decorate fills the per-request permission flags the UI renders.
func decorateMiniTool(tool *types.MiniTool, userID string, role types.TenantRole) {
	if tool == nil {
		return
	}
	manage := canManageMiniTool(tool, userID, role)
	tool.CanEdit = manage
	tool.CanDelete = manage
}

func (s *miniToolService) List(
	ctx context.Context, query types.MiniToolQuery,
) ([]*types.MiniTool, error) {
	tenantID, userID, role, err := s.caller(ctx)
	if err != nil {
		return nil, err
	}
	if query.Scope == "mine" {
		query.OwnerUserID = userID
	}
	tools, err := s.repo.List(ctx, tenantID, query)
	if err != nil {
		return nil, err
	}
	out := make([]*types.MiniTool, 0, len(tools))
	for _, tool := range tools {
		if query.Scope == "workspace" && tool.Visibility != types.MiniToolVisibilityWorkspace {
			continue
		}
		if !canReadMiniTool(tool, userID, role) {
			continue
		}
		decorateMiniTool(tool, userID, role)
		out = append(out, tool)
	}
	return out, nil
}

func (s *miniToolService) Categories(ctx context.Context) ([]string, error) {
	tools, err := s.List(ctx, types.MiniToolQuery{})
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(tools))
	out := make([]string, 0, 8)
	for _, tool := range tools {
		category := strings.TrimSpace(tool.Category)
		if category == "" {
			continue
		}
		if _, ok := seen[category]; ok {
			continue
		}
		seen[category] = struct{}{}
		out = append(out, category)
	}
	return out, nil
}

func (s *miniToolService) Get(ctx context.Context, id string) (*types.MiniTool, error) {
	tenantID, userID, role, err := s.caller(ctx)
	if err != nil {
		return nil, err
	}
	tool, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !canReadMiniTool(tool, userID, role) {
		// A private card of another member is indistinguishable from a missing
		// one, so an id probe learns nothing.
		return nil, apperrors.NewNotFoundError("mini tool not found")
	}
	decorateMiniTool(tool, userID, role)
	return tool, nil
}

func (s *miniToolService) Create(
	ctx context.Context, tool *types.MiniTool,
) (*types.MiniTool, error) {
	tenantID, userID, _, err := s.caller(ctx)
	if err != nil {
		return nil, err
	}
	if tool == nil {
		return nil, apperrors.NewValidationError("mini tool is required")
	}
	tool.ID = uuid.NewString()
	tool.TenantID = tenantID
	tool.OwnerUserID = userID
	tool.Enabled = true
	tool.UsageCount = 0
	tool.Normalize()
	if err := tool.Validate(); err != nil {
		return nil, apperrors.NewValidationError(err.Error())
	}
	if err := s.repo.Create(ctx, tool); err != nil {
		return nil, fmt.Errorf("create mini tool: %w", err)
	}
	logger.Infof(ctx, "Mini tool created: id=%s category=%s", tool.ID, tool.Category)
	return tool, nil
}

func (s *miniToolService) Update(
	ctx context.Context, id string, patch interfaces.MiniToolPatch,
) (*types.MiniTool, error) {
	tenantID, userID, role, err := s.caller(ctx)
	if err != nil {
		return nil, err
	}
	tool, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !canManageMiniTool(tool, userID, role) {
		return nil, apperrors.NewForbiddenError("you cannot edit this mini tool")
	}
	applyMiniToolPatch(tool, patch)
	tool.Normalize()
	if err := tool.Validate(); err != nil {
		return nil, apperrors.NewValidationError(err.Error())
	}
	if err := s.repo.Update(ctx, tool); err != nil {
		return nil, fmt.Errorf("update mini tool: %w", err)
	}
	decorateMiniTool(tool, userID, role)
	return tool, nil
}

func applyMiniToolPatch(tool *types.MiniTool, patch interfaces.MiniToolPatch) {
	if patch.Name != nil {
		tool.Name = *patch.Name
	}
	if patch.Icon != nil {
		tool.Icon = *patch.Icon
	}
	if patch.Category != nil {
		tool.Category = *patch.Category
	}
	if patch.Description != nil {
		tool.Description = *patch.Description
	}
	if patch.Formula != nil {
		tool.Formula = *patch.Formula
	}
	if patch.FormulaNote != nil {
		tool.FormulaNote = *patch.FormulaNote
	}
	if patch.Variables != nil {
		tool.Variables = *patch.Variables
	}
	if patch.Knowledge != nil {
		tool.Knowledge = *patch.Knowledge
	}
	if patch.Examples != nil {
		tool.Examples = *patch.Examples
	}
	if patch.Visibility != nil {
		tool.Visibility = *patch.Visibility
	}
	if patch.Enabled != nil {
		tool.Enabled = *patch.Enabled
	}
}

func (s *miniToolService) Delete(ctx context.Context, id string) error {
	tenantID, userID, role, err := s.caller(ctx)
	if err != nil {
		return err
	}
	tool, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if !canManageMiniTool(tool, userID, role) {
		return apperrors.NewForbiddenError("you cannot delete this mini tool")
	}
	return s.repo.Delete(ctx, tenantID, id)
}

func (s *miniToolService) Duplicate(
	ctx context.Context, id string,
) (*types.MiniTool, error) {
	source, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	copyTool := *source
	copyTool.OwnerName = ""
	copyTool.Name = truncateRunes(source.Name+" (副本)", types.MaxMiniToolNameLength)
	// A copy starts private: duplicating a workspace card must not silently
	// publish the copy to everyone.
	copyTool.Visibility = types.MiniToolVisibilityPrivate
	copyTool.UsageCount = 0
	return s.Create(ctx, &copyTool)
}

// ListForRun resolves the cards one turn injects. The agent's selection is the
// base; cards mentioned for this turn are added, and a mode of "none" leaves
// only the mentioned ones. Disabled and unreadable cards are dropped so a
// stale agent reference cannot leak another member's private card.
func (s *miniToolService) ListForRun(
	ctx context.Context, tenantID uint64, mode string, selectedIDs []string, requestedIDs []string,
) []*types.MiniTool {
	if tenantID == 0 {
		return nil
	}
	userID, _ := types.UserIDFromContext(ctx)
	role := types.TenantRoleFromContext(ctx)

	ids := make([]string, 0, len(selectedIDs)+len(requestedIDs))
	switch mode {
	case types.MiniToolSelectionAll:
		// "all" resolves against the whole catalog below; per-turn mentions are
		// folded into that same listing, so no ids are collected here.
	case types.MiniToolSelectionSelected:
		ids = append(ids, selectedIDs...)
	default:
		// none / empty: the agent carries no default, but a per-turn mention
		// still applies.
	}
	ids = append(ids, dedupeStrings(requestedIDs)...)

	var (
		tools []*types.MiniTool
		err   error
	)
	// Only "selected" and "none" narrow by id. Listing everything for "all" is
	// the point of the mode, and a per-turn mention must not shrink it to one
	// card.
	if mode == types.MiniToolSelectionAll {
		tools, err = s.repo.List(ctx, tenantID, types.MiniToolQuery{})
		if err == nil && len(ids) > 0 {
			tools = append(tools, s.listByIDs(ctx, tenantID, ids)...)
		}
	} else {
		tools, err = s.repo.ListByIDs(ctx, tenantID, dedupeStrings(ids))
	}
	if err != nil {
		logger.Warnf(ctx, "Failed to resolve mini tools for run: %v", err)
		return nil
	}
	out := make([]*types.MiniTool, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool == nil || !tool.Enabled {
			continue
		}
		if !canReadMiniTool(tool, userID, role) {
			continue
		}
		if _, dup := seen[tool.ID]; dup {
			continue
		}
		seen[tool.ID] = struct{}{}
		out = append(out, tool)
	}
	return out
}

// listByIDs fetches extra cards without failing the whole turn: a mention of a
// deleted or foreign id is dropped silently, exactly like an unknown one.
func (s *miniToolService) listByIDs(ctx context.Context, tenantID uint64, ids []string) []*types.MiniTool {
	ids = dedupeStrings(ids)
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.repo.ListByIDs(ctx, tenantID, ids)
	if err != nil {
		logger.Warnf(ctx, "Failed to resolve mentioned mini tools: %v", err)
		return nil
	}
	return rows
}

func (s *miniToolService) MarkUsed(ctx context.Context, tenantID uint64, ids []string) {
	if tenantID == 0 || len(ids) == 0 {
		return
	}
	if err := s.repo.IncrementUsage(ctx, tenantID, dedupeStrings(ids)); err != nil {
		logger.Warnf(ctx, "Failed to bump mini tool usage: %v", err)
	}
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
