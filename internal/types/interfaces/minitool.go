package interfaces

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
)

// ErrMiniToolNotFound is returned by the catalog repository when a card does
// not exist, is soft-deleted, or belongs to another workspace and is not
// public. It lives here rather than in the repository package so the service
// can translate it into a 404 without importing the storage layer.
var ErrMiniToolNotFound = errors.New("mini tool not found")

// MiniToolRepository persists the 小工具 catalog.
//
// Every method takes the tenant explicitly. The catalog is workspace-scoped
// rather than user-scoped because a workspace card may be shared with the
// whole tenant; owner checks belong to the service, which also knows the
// caller's role.
type MiniToolRepository interface {
	Create(ctx context.Context, tool *types.MiniTool) error
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.MiniTool, error)
	List(ctx context.Context, tenantID uint64, query types.MiniToolQuery) ([]*types.MiniTool, error)
	ListByIDs(ctx context.Context, tenantID uint64, ids []string) ([]*types.MiniTool, error)
	Update(ctx context.Context, tool *types.MiniTool) error
	Delete(ctx context.Context, tenantID uint64, id string) error
	IncrementUsage(ctx context.Context, tenantID uint64, ids []string) error

	// UpsertBuiltin inserts or refreshes one product-shipped card for a
	// tenant. It keys on (tenant_id, name, owner_user_id=''), which is the
	// only combination the seeder writes, so a user copying a shipped card
	// keeps their own row. Returns true when a new card was created.
	//
	// Usage counters, the enabled flag and the visibility of an existing card
	// are preserved: re-seeding after a catalog update must not reset what the
	// workspace has done with a card, nor resurrect one an admin deliberately
	// hid or un-published.
	UpsertBuiltin(ctx context.Context, tool *types.MiniTool) (bool, error)

	// SetMiniToolVisibility rewrites only the visibility column of a card in
	// the given workspace. Returns ErrMiniToolNotFound when the row is gone.
	SetMiniToolVisibility(ctx context.Context, tenantID uint64, id string, visibility string) error
}

// MiniToolPatch is a partial update: nil fields keep their stored value.
type MiniToolPatch struct {
	Name        *string
	Icon        *string
	Category    *string
	Description *string
	Formula     *string
	FormulaNote *string
	Variables   *types.MiniToolVariables
	Knowledge   *string
	Examples    *types.MiniToolExamples
	Visibility  *string
	Enabled     *bool
}

// MiniToolService is the catalog + prompt-injection service.
type MiniToolService interface {
	// List returns cards visible to the caller.
	List(ctx context.Context, query types.MiniToolQuery) ([]*types.MiniTool, error)
	// Categories returns the distinct categories of visible cards.
	Categories(ctx context.Context) ([]string, error)
	Get(ctx context.Context, id string) (*types.MiniTool, error)
	Create(ctx context.Context, tool *types.MiniTool) (*types.MiniTool, error)
	Update(ctx context.Context, id string, patch MiniToolPatch) (*types.MiniTool, error)
	Delete(ctx context.Context, id string) error
	Duplicate(ctx context.Context, id string) (*types.MiniTool, error)
	// SetVisibility changes who can see a card without touching its content.
	//
	// Split from Update because the two have different permission rules: a
	// shipped card cannot be edited in place, yet a workspace admin must still
	// be able to publish it. Update therefore refuses it, while this method
	// accepts it.
	SetVisibility(ctx context.Context, id string, visibility string) (*types.MiniTool, error)
	// ListForRun resolves the cards one agent turn should inject:
	// agent selection mode first, then per-turn mentions, always filtered to
	// enabled cards the workspace can see.
	ListForRun(
		ctx context.Context, tenantID uint64, mode string, selectedIDs []string, requestedIDs []string,
	) []*types.MiniTool
	// MarkUsed bumps the usage counters after a successful turn.
	MarkUsed(ctx context.Context, tenantID uint64, ids []string)
}
