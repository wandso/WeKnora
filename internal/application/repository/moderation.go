package repository

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Storage layer for the content-moderation observation log.

type moderationRuleRepository struct {
	db *gorm.DB
}

// NewModerationRuleRepository builds the rule store.
func NewModerationRuleRepository(db *gorm.DB) interfaces.ModerationRuleRepository {
	return &moderationRuleRepository{db: db}
}

func (r *moderationRuleRepository) List(
	ctx context.Context, tenantID uint64,
) ([]*types.ModerationRule, error) {
	var rules []*types.ModerationRule
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&rules).Error
	return rules, err
}

func (r *moderationRuleRepository) Create(ctx context.Context, rule *types.ModerationRule) error {
	return r.db.WithContext(ctx).Create(rule).Error
}

func (r *moderationRuleRepository) Update(ctx context.Context, rule *types.ModerationRule) error {
	// Tenant is part of the WHERE clause so a rule cannot be moved across
	// workspaces by a crafted ID.
	return r.db.WithContext(ctx).
		Model(&types.ModerationRule{}).
		Where("id = ? AND tenant_id = ?", rule.ID, rule.TenantID).
		Updates(map[string]any{
			"name":       rule.Name,
			"category":   rule.Category,
			"pattern":    rule.Pattern,
			"is_regex":   rule.IsRegex,
			"action":     rule.Action,
			"enabled":    rule.Enabled,
			"updated_at": time.Now(),
		}).Error
}

func (r *moderationRuleRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&types.ModerationRule{}).Error
}

type moderationHitRepository struct {
	db *gorm.DB
}

// NewModerationHitRepository builds the hit store.
func NewModerationHitRepository(db *gorm.DB) interfaces.ModerationHitRepository {
	return &moderationHitRepository{db: db}
}

func (r *moderationHitRepository) InsertIgnore(
	ctx context.Context, hits []*types.ModerationHit,
) (int, error) {
	if len(hits) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&hits)
	return int(res.RowsAffected), res.Error
}

func (r *moderationHitRepository) List(
	ctx context.Context, tenantID uint64, agentID string, limit, offset int,
) ([]*types.ModerationHit, int64, error) {
	scope := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&types.ModerationHit{}).Where("tenant_id = ?", tenantID)
		if agentID != "" {
			q = q.Where("agent_id = ?", agentID)
		}
		return q
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var hits []*types.ModerationHit
	err := scope().Order("created_at DESC").Limit(limit).Offset(offset).Find(&hits).Error
	return hits, total, err
}

func (r *moderationHitRepository) CountByCategory(
	ctx context.Context, tenantID uint64, agentID string, since time.Time,
) (map[string]int64, error) {
	// Scan into a named struct rather than a map so the column aliases are
	// explicit; scanning straight into map[string]int64 is driver-dependent.
	type categoryRow struct {
		Category string
		Total    int64
	}
	var rows []categoryRow

	q := r.db.WithContext(ctx).Model(&types.ModerationHit{}).
		Select("category, COUNT(*) AS total").
		Where("tenant_id = ? AND created_at >= ?", tenantID, since)
	if agentID != "" {
		q = q.Where("agent_id = ?", agentID)
	}
	if err := q.Group("category").Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Category] = row.Total
	}
	return out, nil
}

func (r *moderationHitRepository) ListMessages(
	ctx context.Context, tenantID uint64, agentID string, limit int,
) ([]interfaces.ModerationMessage, error) {
	// messages carries no tenant_id, so ownership is established through the
	// session row — the same join the usage aggregate uses.
	var out []interfaces.ModerationMessage
	err := r.db.WithContext(ctx).
		Table("messages AS m").
		Select("m.id AS id, m.session_id AS session_id, m.role AS role, m.content AS content").
		Joins("JOIN sessions s ON s.id = m.session_id").
		Where("s.tenant_id = ? AND m.agent_id = ?", tenantID, agentID).
		Where("m.role IN ?", []string{"user", "assistant"}).
		Where("m.deleted_at IS NULL").
		Where("m.content <> ''").
		Order("m.created_at DESC").
		Limit(limit).
		Scan(&out).Error
	return out, err
}
