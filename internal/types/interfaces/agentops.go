package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// AgentVersionRepository is the storage contract for immutable agent
// snapshots. Version numbers are allocated per (tenant, agent) and never
// reused, including after a rollback.
type AgentVersionRepository interface {
	// Create appends a snapshot. The caller supplies VersionNo.
	Create(ctx context.Context, v *types.AgentVersion) error
	// NextVersionNo returns max(version_no)+1 for the agent, starting at 1.
	NextVersionNo(ctx context.Context, tenantID uint64, agentID string) (int, error)
	// List returns snapshots newest first plus the total count.
	List(ctx context.Context, tenantID uint64, agentID string, limit, offset int) ([]*types.AgentVersion, int64, error)
	// Get returns one snapshot by number.
	Get(ctx context.Context, tenantID uint64, agentID string, versionNo int) (*types.AgentVersion, error)
}

// MessageFeedbackRepository stores one vote per (message, user).
type MessageFeedbackRepository interface {
	// Upsert writes or replaces the caller's vote for a message.
	Upsert(ctx context.Context, f *types.MessageFeedback) error
	// Get returns the caller's vote, or nil when none exists.
	Get(ctx context.Context, tenantID uint64, messageID, userID string) (*types.MessageFeedback, error)
	// List returns an agent's feedback newest first plus the total count.
	// rating may be empty to include both.
	List(ctx context.Context, tenantID uint64, agentID, rating string, limit, offset int) ([]*types.MessageFeedback, int64, error)
	// Summary tallies votes and reason tags since the given time.
	Summary(ctx context.Context, tenantID uint64, agentID string, since time.Time) (types.FeedbackSummary, error)
}

// AgentUsageRepository aggregates token usage from persisted assistant
// messages. It is read-only by design: the source of truth is the message
// table, so there is no write path to drift out of sync.
type AgentUsageRepository interface {
	Summary(ctx context.Context, tenantID uint64, agentID string, since time.Time) (types.AgentUsageSummary, error)
	Timeseries(ctx context.Context, tenantID uint64, agentID string, since time.Time) ([]types.AgentUsagePoint, error)
}

// AgentTemplateRepository stores installable agent configurations.
type AgentTemplateRepository interface {
	List(ctx context.Context, tenantID uint64, category string) ([]*types.AgentTemplate, error)
	Get(ctx context.Context, id string, tenantID uint64) (*types.AgentTemplate, error)
	Create(ctx context.Context, t *types.AgentTemplate) error
	// UpsertBuiltin writes a product-shipped template, refreshing its content
	// while preserving the install counter tenants have accumulated.
	UpsertBuiltin(ctx context.Context, t *types.AgentTemplate) error
	Delete(ctx context.Context, id string, tenantID uint64) error
	IncrementInstalls(ctx context.Context, id string) error
}

// AgentPublicPageRepository stores the public marketing surface for an agent.
type AgentPublicPageRepository interface {
	Upsert(ctx context.Context, p *types.AgentPublicPage) error
	GetByAgent(ctx context.Context, tenantID uint64, agentID string) (*types.AgentPublicPage, error)
	GetBySlug(ctx context.Context, slug string) (*types.AgentPublicPage, error)
	Delete(ctx context.Context, tenantID uint64, agentID string) error
}

// AgentVersionService owns snapshotting, listing, diffing and rollback.
type AgentVersionService interface {
	// Snapshot records the agent's current state. Callers pass the agent as it
	// was persisted so the snapshot cannot disagree with the live row.
	Snapshot(ctx context.Context, agent *types.CustomAgent, source, changelog, userID string) (*types.AgentVersion, error)
	List(ctx context.Context, agentID string, limit, offset int) ([]*types.AgentVersion, int64, error)
	Get(ctx context.Context, agentID string, versionNo int) (*types.AgentVersion, error)
	Diff(ctx context.Context, agentID string, from, to int) (*types.AgentVersionDiff, error)
	// Rollback applies an old snapshot through the normal update path, which
	// appends a new version rather than rewriting history.
	Rollback(ctx context.Context, agentID string, versionNo int, userID string) (*types.CustomAgent, error)
}

// MessageFeedbackService validates and stores answer feedback.
type MessageFeedbackService interface {
	Submit(ctx context.Context, tenantID uint64, userID, messageID, sessionID, rating string, reasons []string, comment string) (*types.MessageFeedback, error)
	GetMine(ctx context.Context, tenantID uint64, userID, messageID string) (*types.MessageFeedback, error)
	List(ctx context.Context, tenantID uint64, agentID, rating string, limit, offset int) ([]*types.MessageFeedback, int64, error)
	Summary(ctx context.Context, tenantID uint64, agentID string, since time.Time) (types.FeedbackSummary, error)
}

// AgentUsageService reports token usage derived from messages.
type AgentUsageService interface {
	Summary(ctx context.Context, tenantID uint64, agentID string, since time.Time) (types.AgentUsageSummary, error)
	Timeseries(ctx context.Context, tenantID uint64, agentID string, since time.Time) ([]types.AgentUsagePoint, error)
	Report(ctx context.Context, tenantID uint64, agentID string, since time.Time) (*types.AgentQualityReport, error)
}

// AgentTemplateService installs and saves reusable agent configurations.
type AgentTemplateService interface {
	List(ctx context.Context, tenantID uint64, category string) ([]*types.AgentTemplate, error)
	Install(ctx context.Context, tenantID uint64, userID, templateID string) (*types.CustomAgent, error)
	SaveFromAgent(ctx context.Context, tenantID uint64, userID, agentID, name, category, icon, description string) (*types.AgentTemplate, error)
	Delete(ctx context.Context, tenantID uint64, id string) error
	// SeedBuiltins upserts the product-shipped templates. It is idempotent and
	// safe to call on every boot, so upgrades refresh template content without
	// duplicating rows or resetting install counters.
	SeedBuiltins(ctx context.Context) (int, error)
}

// AgentPublicPageService manages public agent pages and resolves them by slug
// for anonymous visitors.
type AgentPublicPageService interface {
	Upsert(ctx context.Context, page *types.AgentPublicPage, password string) (*types.AgentPublicPage, error)
	GetByAgent(ctx context.Context, tenantID uint64, agentID string) (*types.AgentPublicPage, error)
	Delete(ctx context.Context, tenantID uint64, agentID string) error
	// ResolvePublic returns the page and its agent for an anonymous visitor.
	// A password-protected page is only returned when password matches.
	ResolvePublic(ctx context.Context, slug, password string) (*types.AgentPublicPage, *types.CustomAgent, error)
}

// AgentDryRunService executes an agent with a draft configuration that has not
// been saved. The run is isolated: it writes to a throwaway session that is
// removed afterwards, so a trial never appears in history or usage totals.
type AgentDryRunService interface {
	Run(ctx context.Context, agentID string, draft types.CustomAgentConfig, query string) (*types.AgentDryRunResult, error)
}
