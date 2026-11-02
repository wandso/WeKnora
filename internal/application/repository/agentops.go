package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file holds the storage layer for the agent operations centre: version
// snapshots, answer feedback, usage aggregates, templates and public pages.
//
// Usage deserves a note. It is aggregated from `messages` joined to `sessions`
// rather than from a dedicated counter table. The message row is what the user
// actually received, it already carries the token usage and the agent id, and
// it covers every channel. A separate write path would be one more thing that
// can silently stop recording.

type agentVersionRepository struct{ db *gorm.DB }

// NewAgentVersionRepository constructs the GORM-backed version store.
func NewAgentVersionRepository(db *gorm.DB) interfaces.AgentVersionRepository {
	return &agentVersionRepository{db: db}
}

func (r *agentVersionRepository) Create(ctx context.Context, v *types.AgentVersion) error {
	return r.db.WithContext(ctx).Create(v).Error
}

func (r *agentVersionRepository) NextVersionNo(ctx context.Context, tenantID uint64, agentID string) (int, error) {
	var maxNo int
	err := r.db.WithContext(ctx).
		Model(&types.AgentVersion{}).
		Where("tenant_id = ? AND agent_id = ?", tenantID, agentID).
		Select("COALESCE(MAX(version_no), 0)").
		Scan(&maxNo).Error
	if err != nil {
		return 0, err
	}
	return maxNo + 1, nil
}

func (r *agentVersionRepository) List(
	ctx context.Context, tenantID uint64, agentID string, limit, offset int,
) ([]*types.AgentVersion, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&types.AgentVersion{}).
		Where("tenant_id = ? AND agent_id = ?", tenantID, agentID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// A fresh query for the page: reusing the counted statement lets GORM's
	// Count selection leak into the row query on some drivers.
	var list []*types.AgentVersion
	err := r.db.WithContext(ctx).Model(&types.AgentVersion{}).
		Where("tenant_id = ? AND agent_id = ?", tenantID, agentID).
		Order("version_no DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}

func (r *agentVersionRepository) Get(
	ctx context.Context, tenantID uint64, agentID string, versionNo int,
) (*types.AgentVersion, error) {
	var v types.AgentVersion
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND agent_id = ? AND version_no = ?", tenantID, agentID, versionNo).
		First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

// ---------------------------------------------------------------------------

type messageFeedbackRepository struct{ db *gorm.DB }

// NewMessageFeedbackRepository constructs the GORM-backed feedback store.
func NewMessageFeedbackRepository(db *gorm.DB) interfaces.MessageFeedbackRepository {
	return &messageFeedbackRepository{db: db}
}

func (r *messageFeedbackRepository) Upsert(ctx context.Context, f *types.MessageFeedback) error {
	// A user may change a vote, so the conflict path updates the mutable
	// fields. created_at is left alone: it records when the answer was first
	// judged, which is what a "time to first feedback" metric needs.
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"}, {Name: "message_id"}, {Name: "user_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"rating", "reason_tags", "comment", "session_id", "agent_id", "updated_at",
		}),
	}).Create(f).Error
}

func (r *messageFeedbackRepository) Get(
	ctx context.Context, tenantID uint64, messageID, userID string,
) (*types.MessageFeedback, error) {
	var f types.MessageFeedback
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND message_id = ? AND user_id = ?", tenantID, messageID, userID).
		First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (r *messageFeedbackRepository) List(
	ctx context.Context, tenantID uint64, agentID, rating string, limit, offset int,
) ([]*types.MessageFeedback, int64, error) {
	scope := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&types.MessageFeedback{}).
			Where("tenant_id = ? AND agent_id = ?", tenantID, agentID)
		if rating != "" {
			q = q.Where("rating = ?", rating)
		}
		return q
	}
	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*types.MessageFeedback
	err := scope().Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}

func (r *messageFeedbackRepository) Summary(
	ctx context.Context, tenantID uint64, agentID string, since time.Time,
) (types.FeedbackSummary, error) {
	out := types.FeedbackSummary{Reasons: map[string]int64{}}

	type ratingCount struct {
		Rating string
		Count  int64
	}
	var counts []ratingCount
	err := r.db.WithContext(ctx).Model(&types.MessageFeedback{}).
		Select("rating, COUNT(*) AS count").
		Where("tenant_id = ? AND agent_id = ? AND created_at >= ?", tenantID, agentID, since).
		Group("rating").
		Scan(&counts).Error
	if err != nil {
		return out, err
	}
	for _, c := range counts {
		switch c.Rating {
		case types.FeedbackRatingUp:
			out.Up = c.Count
		case types.FeedbackRatingDown:
			out.Down = c.Count
		}
	}
	out.Total = out.Up + out.Down
	if out.Total > 0 {
		out.UpRate = float64(out.Up) / float64(out.Total)
	}

	// Reason tags are counted in Go: JSON array unnesting is the one piece of
	// this query that has no portable SQL form across Postgres, SQLite and
	// MySQL. The window keeps the row count small.
	type reasonRow struct {
		ReasonTags types.JSON
	}
	var rows []reasonRow
	err = r.db.WithContext(ctx).Model(&types.MessageFeedback{}).
		Select("reason_tags").
		Where("tenant_id = ? AND agent_id = ? AND created_at >= ? AND rating = ?",
			tenantID, agentID, since, types.FeedbackRatingDown).
		Scan(&rows).Error
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		var list []string
		if len(row.ReasonTags) == 0 || json.Unmarshal(row.ReasonTags, &list) != nil {
			continue
		}
		for _, tag := range list {
			out.Reasons[tag]++
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------

type agentUsageRepository struct{ db *gorm.DB }

// NewAgentUsageRepository constructs the read-only usage aggregator.
func NewAgentUsageRepository(db *gorm.DB) interfaces.AgentUsageRepository {
	return &agentUsageRepository{db: db}
}

// usageDialect captures the few SQL fragments that differ between engines.
// Everything else in the aggregate is shared.
type usageDialect struct {
	promptTokens     string
	completionTokens string
	totalTokens      string
	avgSteps         string
	avgLatency       string
	dayBucket        string
}

func (r *agentUsageRepository) dialect() usageDialect {
	switch r.db.Dialector.Name() {
	case "sqlite":
		return usageDialect{
			promptTokens:     "COALESCE(json_extract(m.usage, '$.prompt_tokens'), 0)",
			completionTokens: "COALESCE(json_extract(m.usage, '$.completion_tokens'), 0)",
			totalTokens:      "COALESCE(json_extract(m.usage, '$.total_tokens'), 0)",
			avgSteps:         "COALESCE(AVG(json_array_length(m.agent_steps)), 0)",
			avgLatency:       "COALESCE(AVG(NULLIF(m.agent_duration_ms, 0)), 0)",
			dayBucket:        "strftime('%Y-%m-%d', m.created_at)",
		}
	case "mysql":
		return usageDialect{
			promptTokens:     "COALESCE(JSON_EXTRACT(m.usage, '$.prompt_tokens'), 0)",
			completionTokens: "COALESCE(JSON_EXTRACT(m.usage, '$.completion_tokens'), 0)",
			totalTokens:      "COALESCE(JSON_EXTRACT(m.usage, '$.total_tokens'), 0)",
			avgSteps:         "COALESCE(AVG(JSON_LENGTH(m.agent_steps)), 0)",
			avgLatency:       "COALESCE(AVG(NULLIF(m.agent_duration_ms, 0)), 0)",
			dayBucket:        "DATE_FORMAT(m.created_at, '%Y-%m-%d')",
		}
	default:
		return usageDialect{
			promptTokens:     "COALESCE((m.usage->>'prompt_tokens')::bigint, 0)",
			completionTokens: "COALESCE((m.usage->>'completion_tokens')::bigint, 0)",
			totalTokens:      "COALESCE((m.usage->>'total_tokens')::bigint, 0)",
			// Postgres AVG returns numeric; casting to float8 keeps the scan
			// into float64 identical across drivers.
			avgSteps:   "COALESCE(AVG(jsonb_array_length(m.agent_steps)), 0)::float8",
			avgLatency: "COALESCE(AVG(NULLIF(m.agent_duration_ms, 0)), 0)::float8",
			dayBucket:  "to_char(date_trunc('day', m.created_at), 'YYYY-MM-DD')",
		}
	}
}

// usageFrom is the shared FROM/WHERE clause. messages has no tenant_id of its
// own — tenancy lives on the session — so the join is what keeps one
// workspace's numbers out of another's report.
const usageFrom = `
	FROM messages m
	JOIN sessions s ON s.id = m.session_id
	WHERE s.tenant_id = ? AND m.agent_id = ? AND m.role = 'assistant'
	  AND m.deleted_at IS NULL AND m.created_at >= ?`

func (r *agentUsageRepository) Summary(
	ctx context.Context, tenantID uint64, agentID string, since time.Time,
) (types.AgentUsageSummary, error) {
	d := r.dialect()
	query := fmt.Sprintf(`
		SELECT COUNT(*) AS turns,
		       COALESCE(SUM(%s), 0) AS prompt_tokens,
		       COALESCE(SUM(%s), 0) AS completion_tokens,
		       COALESCE(SUM(%s), 0) AS total_tokens,
		       %s AS avg_steps,
		       %s AS avg_latency_ms,
		       COUNT(DISTINCT m.session_id) AS unique_sessions
		%s`, d.promptTokens, d.completionTokens, d.totalTokens, d.avgSteps, d.avgLatency, usageFrom)

	var out types.AgentUsageSummary
	err := r.db.WithContext(ctx).Raw(query, tenantID, agentID, since).Scan(&out).Error
	return out, err
}

func (r *agentUsageRepository) Timeseries(
	ctx context.Context, tenantID uint64, agentID string, since time.Time,
) ([]types.AgentUsagePoint, error) {
	d := r.dialect()
	query := fmt.Sprintf(`
		SELECT %s AS bucket, COUNT(*) AS turns, COALESCE(SUM(%s), 0) AS total_tokens
		%s
		GROUP BY 1 ORDER BY 1`, d.dayBucket, d.totalTokens, usageFrom)

	var points []types.AgentUsagePoint
	err := r.db.WithContext(ctx).Raw(query, tenantID, agentID, since).Scan(&points).Error
	return points, err
}

// ---------------------------------------------------------------------------

type agentTemplateRepository struct{ db *gorm.DB }

// NewAgentTemplateRepository constructs the GORM-backed template store.
func NewAgentTemplateRepository(db *gorm.DB) interfaces.AgentTemplateRepository {
	return &agentTemplateRepository{db: db}
}

func (r *agentTemplateRepository) List(ctx context.Context, tenantID uint64, category string) ([]*types.AgentTemplate, error) {
	q := r.db.WithContext(ctx).Model(&types.AgentTemplate{}).
		Where("tenant_id = ? OR tenant_id = 0", tenantID)
	if category != "" {
		q = q.Where("category = ?", category)
	}
	var list []*types.AgentTemplate
	// Built-in templates first, then the most-installed tenant ones.
	//
	// source ASC, not DESC: 'builtin' sorts before 'tenant', so ascending is
	// what actually puts the product-shipped templates at the top. The previous
	// DESC ordering contradicted this comment and buried them at the bottom.
	err := q.Order("source ASC").Order("installs DESC").Order("created_at DESC").Find(&list).Error
	return list, err
}

func (r *agentTemplateRepository) Get(ctx context.Context, id string, tenantID uint64) (*types.AgentTemplate, error) {
	var t types.AgentTemplate
	err := r.db.WithContext(ctx).
		Where("id = ? AND (tenant_id = ? OR tenant_id = 0)", id, tenantID).
		First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *agentTemplateRepository) Create(ctx context.Context, t *types.AgentTemplate) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// UpsertBuiltin refreshes a product-shipped template in place.
//
// `installs` is intentionally absent from the update list: it is a product
// metric accumulated across tenants, so re-seeding after an upgrade must not
// reset it. `tenant_id` and `created_at` are likewise left untouched.
func (r *agentTemplateRepository) UpsertBuiltin(ctx context.Context, t *types.AgentTemplate) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "category", "icon", "description", "config",
			"agent_name", "agent_description", "agent_avatar",
			"source", "updated_at",
		}),
	}).Create(t).Error
}

func (r *agentTemplateRepository) Delete(ctx context.Context, id string, tenantID uint64) error {
	// Built-in templates (tenant_id 0) are not deletable through this path.
	return r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND source = ?", id, tenantID, types.AgentTemplateSourceTenant).
		Delete(&types.AgentTemplate{}).Error
}

func (r *agentTemplateRepository) IncrementInstalls(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&types.AgentTemplate{}).
		Where("id = ?", id).
		UpdateColumn("installs", gorm.Expr("installs + 1")).Error
}

// ---------------------------------------------------------------------------

type agentPublicPageRepository struct{ db *gorm.DB }

// NewAgentPublicPageRepository constructs the GORM-backed public page store.
func NewAgentPublicPageRepository(db *gorm.DB) interfaces.AgentPublicPageRepository {
	return &agentPublicPageRepository{db: db}
}

func (r *agentPublicPageRepository) Upsert(ctx context.Context, p *types.AgentPublicPage) error {
	// One page per agent. Slug uniqueness is enforced by the database and
	// surfaces as a duplicate-key error the service turns into a 409.
	existing, err := r.GetByAgent(ctx, p.TenantID, p.AgentID)
	if err != nil {
		return err
	}
	if existing == nil {
		return r.db.WithContext(ctx).Create(p).Error
	}
	p.ID = existing.ID
	return r.db.WithContext(ctx).Model(&types.AgentPublicPage{}).
		Where("id = ?", existing.ID).
		Select("embed_channel_id", "slug", "title", "description", "og_image_url",
			"access_mode", "password_hash", "enabled", "updated_at").
		Updates(p).Error
}

func (r *agentPublicPageRepository) GetByAgent(
	ctx context.Context, tenantID uint64, agentID string,
) (*types.AgentPublicPage, error) {
	var p types.AgentPublicPage
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND agent_id = ?", tenantID, agentID).
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *agentPublicPageRepository) GetBySlug(ctx context.Context, slug string) (*types.AgentPublicPage, error) {
	var p types.AgentPublicPage
	// Deliberately not filtered on enabled: a disabled page still owns its
	// slug, so the upsert path must see it or the insert hits the unique index.
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *agentPublicPageRepository) Delete(ctx context.Context, tenantID uint64, agentID string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND agent_id = ?", tenantID, agentID).
		Delete(&types.AgentPublicPage{}).Error
}
