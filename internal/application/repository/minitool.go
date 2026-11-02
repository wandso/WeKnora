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
var ErrMiniToolNotFound = errors.New("mini tool not found")

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
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&tool).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMiniToolNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

func (r *miniToolRepository) List(
	ctx context.Context, tenantID uint64, query types.MiniToolQuery,
) ([]*types.MiniTool, error) {
	db := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
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
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id IN ?", tenantID, ids).
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
