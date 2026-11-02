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
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type miniToolService struct {
	repo interfaces.MiniToolRepository

	// seeded caches which tenants already received the shipped catalog. The
	// seed runs once per process per tenant instead of on every list call,
	// which keeps the picker a single query in the common case.
	seeded sync.Map
}

// NewMiniToolService creates the catalog service.
func NewMiniToolService(repo interfaces.MiniToolRepository) interfaces.MiniToolService {
	return &miniToolService{repo: repo}
}

// ensureBuiltins installs the shipped calculation cards into a workspace the
// first time it reads the catalog.
//
// Seeding lazily (rather than at boot) is what makes the cards show up for
// workspaces created after the server started: registration creates a tenant
// with no hook into the boot sequence, so a boot-time loop would miss it.
func (s *miniToolService) ensureBuiltins(ctx context.Context, tenantID uint64) {
	if _, done := s.seeded.Load(tenantID); done {
		return
	}
	cards, err := types.BuiltinMiniTools()
	if err != nil {
		logger.Warnf(ctx, "[MiniTool] builtin catalog unavailable: %v", err)
		// Cache the failure for this process: a broken embedded file will not
		// fix itself, and every list call would otherwise retry and log again.
		s.seeded.Store(tenantID, struct{}{})
		return
	}
	created := 0
	for _, card := range cards {
		tool := *card
		tool.ID = uuid.NewString()
		tool.TenantID = tenantID
		tool.OwnerUserID = ""
		isNew, err := s.repo.UpsertBuiltin(ctx, &tool)
		if err != nil {
			logger.Warnf(ctx, "[MiniTool] seed %q for tenant %d failed: %v", card.Name, tenantID, err)
			return
		}
		if isNew {
			created++
		}
	}
	s.seeded.Store(tenantID, struct{}{})
	logger.Infof(ctx, "[MiniTool] tenant %d seeded %d/%d builtin cards (%d new)",
		tenantID, len(cards), len(cards), created)
}

// miniToolLookupError turns the repository's sentinel into a 404.
//
// Without this a card that was already deleted, or that belongs to another
// workspace and is not public, surfaced as a 500: the handler only recognises
// *AppError, and a bare repository error falls through to the generic branch.
func miniToolLookupError(err error) error {
	if errors.Is(err, interfaces.ErrMiniToolNotFound) {
		return apperrors.NewNotFoundError("mini tool not found")
	}
	return err
}

func (s *miniToolService) caller(ctx context.Context) (uint64, string, types.TenantRole, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return 0, "", "", apperrors.NewForbiddenError("a workspace is required to use mini tools")
	}
	userID, _ := types.UserIDFromContext(ctx)
	return tenantID, userID, types.TenantRoleFromContext(ctx), nil
}

// canManageMiniTool reports whether the caller may edit or delete a card.
//
// tenantID is the caller's workspace and is checked first: a public card is
// readable from anywhere, so without this a reader who happens to be Owner of
// their own workspace would pass the role test below and be handed write access
// to somebody else's card. Write access follows the owning workspace, always.
func canManageMiniTool(tool *types.MiniTool, tenantID uint64, userID string, role types.TenantRole) bool {
	if tool == nil {
		return false
	}
	if tenantID == 0 || tool.TenantID != tenantID {
		return false
	}
	// A shipped card has no owner. It is a shared preset that the seeder
	// refreshes whenever the catalog ships a corrected formula, so allowing
	// in-place edits would silently lose them on the next restart. Callers who
	// want their own numbers duplicate the card instead.
	if tool.OwnerUserID == "" {
		return false
	}
	if role == types.TenantRoleOwner || role == types.TenantRoleAdmin {
		return true
	}
	return userID != "" && tool.OwnerUserID == userID
}

func canReadMiniTool(tool *types.MiniTool, tenantID uint64, userID string, role types.TenantRole) bool {
	if tool == nil {
		return false
	}
	// Public cards are readable from any workspace — the only cross-tenant read
	// in this module; see the repository's List/GetByID. Workspace cards are
	// readable by members of the owning workspace and nobody else: the
	// repository already filters for that, but encoding it here as well keeps
	// the rule true if that query ever changes.
	switch tool.Visibility {
	case types.MiniToolVisibilityPublic:
		return true
	case types.MiniToolVisibilityWorkspace:
		return tenantID != 0 && tool.TenantID == tenantID
	}
	return canManageMiniTool(tool, tenantID, userID, role)
}

// decorate fills the per-request permission flags the UI renders.
func decorateMiniTool(tool *types.MiniTool, tenantID uint64, userID string, role types.TenantRole) {
	if tool == nil {
		return
	}
	manage := canManageMiniTool(tool, tenantID, userID, role)
	tool.CanEdit = manage
	tool.CanDelete = manage
	tool.IsForeign = tenantID != 0 && tool.TenantID != tenantID
	// A published card owned by somebody else is readable but never editable,
	// even for a workspace admin: write access follows the owning workspace.
	if tool.IsForeign {
		tool.CanEdit = false
		tool.CanDelete = false
	}
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
	s.ensureBuiltins(ctx, tenantID)
	tools, err := s.repo.List(ctx, tenantID, query)
	if err != nil {
		return nil, err
	}
	out := make([]*types.MiniTool, 0, len(tools))
	for _, tool := range tools {
		if query.Scope == "workspace" && tool.Visibility != types.MiniToolVisibilityWorkspace {
			continue
		}
		if !canReadMiniTool(tool, tenantID, userID, role) {
			continue
		}
		decorateMiniTool(tool, tenantID, userID, role)
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
		return nil, miniToolLookupError(err)
	}
	if !canReadMiniTool(tool, tenantID, userID, role) {
		// A private card of another member is indistinguishable from a missing
		// one, so an id probe learns nothing.
		return nil, apperrors.NewNotFoundError("mini tool not found")
	}
	decorateMiniTool(tool, tenantID, userID, role)
	return tool, nil
}

func (s *miniToolService) Create(
	ctx context.Context, tool *types.MiniTool,
) (*types.MiniTool, error) {
	tenantID, userID, role, err := s.caller(ctx)
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
	// Computed per request, never stored. Cleared here so a caller that posts
	// them (or a duplicate that copied them) cannot smuggle a read-only flag or
	// a false "foreign" origin into the response for a card it now owns.
	tool.CanEdit = false
	tool.CanDelete = false
	tool.IsForeign = false
	tool.Normalize()
	if err := tool.Validate(); err != nil {
		return nil, apperrors.NewValidationError(err.Error())
	}
	if err := s.repo.Create(ctx, tool); err != nil {
		return nil, fmt.Errorf("create mini tool: %w", err)
	}
	logger.Infof(ctx, "Mini tool created: id=%s category=%s", tool.ID, tool.Category)
	// Decorate before returning: the editor re-renders straight from this
	// response, and an undecorated card would show up as read-only to its own
	// author until the next list fetch.
	decorateMiniTool(tool, tenantID, userID, role)
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
		return nil, miniToolLookupError(err)
	}
	if !canManageMiniTool(tool, tenantID, userID, role) {
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
	decorateMiniTool(tool, tenantID, userID, role)
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
		return miniToolLookupError(err)
	}
	if !canManageMiniTool(tool, tenantID, userID, role) {
		return apperrors.NewForbiddenError("you cannot delete this mini tool")
	}
	return miniToolLookupError(s.repo.Delete(ctx, tenantID, id))
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
	// The permission flags describe the SOURCE row relative to the caller.
	// Carrying them into the copy made a fork of somebody else's public card
	// come back as "foreign, read-only" even though the caller now owns it.
	copyTool.IsForeign = false
	copyTool.CanEdit = false
	copyTool.CanDelete = false
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
		if !canReadMiniTool(tool, tenantID, userID, role) {
			continue
		}
		if _, dup := seen[tool.ID]; dup {
			continue
		}
		seen[tool.ID] = struct{}{}
		out = append(out, tool)
	}
	return capMiniToolsForTurn(ctx, out)
}

// maxMiniToolsPerTurn bounds how many cards a single turn can carry.
//
// The shipped catalog is over a hundred cards, and every card expands into its
// formula, inputs, domain notes and a worked example. Selecting "all" on such a
// catalog would spend more context on presets than on the question, so the
// surplus is dropped — most-used first, then by name so the choice is stable
// across turns rather than dependent on row order.
const maxMiniToolsPerTurn = 12

func capMiniToolsForTurn(ctx context.Context, tools []*types.MiniTool) []*types.MiniTool {
	if len(tools) <= maxMiniToolsPerTurn {
		return tools
	}
	sort.SliceStable(tools, func(i, j int) bool {
		if tools[i].UsageCount != tools[j].UsageCount {
			return tools[i].UsageCount > tools[j].UsageCount
		}
		return tools[i].Name < tools[j].Name
	})
	logger.Warnf(ctx,
		"[MiniTool] %d cards attached to one turn, keeping the %d most used; ask the user to narrow the selection",
		len(tools), maxMiniToolsPerTurn)
	return tools[:maxMiniToolsPerTurn]
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
