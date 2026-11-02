package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// ErrMiniToolNotFound is returned when a card does not exist in the workspace.
// Aliased to the interfaces sentinel so the service can match it without
// importing this package.
var ErrMiniToolNotFound = interfaces.ErrMiniToolNotFound

type miniToolRepository struct {
	db *gorm.DB
}

// NewMiniToolRepository creates the catalog repository.
func NewMiniToolRepository(db *gorm.DB) interfaces.MiniToolRepository {
	return &miniToolRepository{db: db}
}

func (r *miniToolRepository) Create(ctx context.Context, tool *types.MiniTool) error {
	return r.db.WithContext(ctx).Create(tool).Error
}

func (r *miniToolRepository) GetByID(
	ctx context.Context, tenantID uint64, id string,
) (*types.MiniTool, error) {
	var tool types.MiniTool
	// A public card is readable from any workspace; everything else stays
	// tenant-scoped. Write paths use Delete/Update, which filter on tenant_id
	// alone, so reading a foreign public card never implies editing it.
	err := r.db.WithContext(ctx).
		Where("id = ? AND (tenant_id = ? OR visibility = ?)",
			id, tenantID, types.MiniToolVisibilityPublic).
		First(&tool).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMiniToolNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// List returns the cards visible to a workspace: its own rows plus every card
// published as public, wherever it was created.
//
// "public" is the one cross-tenant read in this module, and it is narrowed to
// the visibility column so a caller cannot reach a card that was not
// deliberately published.
func (r *miniToolRepository) List(
	ctx context.Context, tenantID uint64, query types.MiniToolQuery,
) ([]*types.MiniTool, error) {
	db := r.db.WithContext(ctx).
		Where("tenant_id = ? OR visibility = ?", tenantID, types.MiniToolVisibilityPublic)
	if !query.IncludeDisabled {
		db = db.Where("enabled = ?", true)
	}
	if query.Category != "" {
		db = db.Where("category = ?", query.Category)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		db = db.Where(
			"LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(formula) LIKE ?",
			like, like, like,
		)
	}
	if query.OwnerUserID != "" {
		db = db.Where("owner_user_id = ?", query.OwnerUserID)
	}
	if query.Limit > 0 {
		db = db.Limit(query.Limit)
	}
	if query.Offset > 0 {
		db = db.Offset(query.Offset)
	}
	var tools []*types.MiniTool
	if err := db.Order("updated_at DESC").Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

func (r *miniToolRepository) ListByIDs(
	ctx context.Context, tenantID uint64, ids []string,
) ([]*types.MiniTool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var tools []*types.MiniTool
	// Same audience rule as GetByID: own rows plus public ones, so a turn can
	// inject a card another workspace published.
	if err := r.db.WithContext(ctx).
		Where("id IN ? AND (tenant_id = ? OR visibility = ?)",
			ids, tenantID, types.MiniToolVisibilityPublic).
		Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

func (r *miniToolRepository) Update(ctx context.Context, tool *types.MiniTool) error {
	return r.db.WithContext(ctx).Save(tool).Error
}

func (r *miniToolRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&types.MiniTool{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMiniToolNotFound
	}
	return nil
}

// IncrementUsage is best-effort: a failed counter must not fail the chat turn
// that already produced an answer.
func (r *miniToolRepository) IncrementUsage(
	ctx context.Context, tenantID uint64, ids []string,
) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&types.MiniTool{}).
		Where("tenant_id = ? AND id IN ?", tenantID, ids).
		UpdateColumn("usage_count", gorm.Expr("usage_count + 1")).Error
}

// UpsertBuiltin keeps the shipped catalog in step with an existing workspace.
//
// The lookup deliberately scopes to owner_user_id = ” so a card the user
// duplicated (which carries their own id) is never overwritten. Content fields
// are refreshed on every boot-time seed, while usage_count and enabled are
// left alone: hiding a built-in card is a deliberate admin action and must
// survive the next catalog release.
func (r *miniToolRepository) UpsertBuiltin(
	ctx context.Context, tool *types.MiniTool,
) (bool, error) {
	var existing types.MiniTool
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND name = ? AND owner_user_id = ''", tool.TenantID, tool.Name).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, r.db.WithContext(ctx).Create(tool).Error
	}
	if err != nil {
		return false, err
	}
	existing.Icon = tool.Icon
	existing.Category = tool.Category
	existing.Description = tool.Description
	existing.Formula = tool.Formula
	existing.FormulaNote = tool.FormulaNote
	existing.Variables = tool.Variables
	existing.Knowledge = tool.Knowledge
	existing.Examples = tool.Examples
	// Visibility is intentionally not refreshed. The shipped default is
	// "workspace", but an admin may have published the card to the whole
	// deployment; copying the catalog value back on every seed would silently
	// un-publish it at the next restart.
	return false, r.db.WithContext(ctx).Save(&existing).Error
}

// SetMiniToolVisibility rewrites only the visibility column.
//
// A column-level update rather than a full Save: the seeder, a concurrent edit
// and this call all touch different fields of the same row, and rewriting the
// whole tuple from a stale in-memory copy would drop their changes.
func (r *miniToolRepository) SetMiniToolVisibility(
	ctx context.Context, tenantID uint64, id string, visibility string,
) error {
	result := r.db.WithContext(ctx).
		Model(&types.MiniTool{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("visibility", visibility)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return interfaces.ErrMiniToolNotFound
	}
	return nil
}
