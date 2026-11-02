package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// ModerationMessage is the slice of a message the scanner needs. Keeping it a
// narrow DTO (rather than a full types.Message) stops the scanner from
// accidentally depending on fields it does not need.
type ModerationMessage struct {
	ID        string
	SessionID string
	Role      string
	Content   string
}

// ModerationRuleRepository stores tenant-authored detectors.
type ModerationRuleRepository interface {
	List(ctx context.Context, tenantID uint64) ([]*types.ModerationRule, error)
	Create(ctx context.Context, rule *types.ModerationRule) error
	Update(ctx context.Context, rule *types.ModerationRule) error
	Delete(ctx context.Context, tenantID uint64, id string) error
}

// ModerationHitRepository stores the observation log.
type ModerationHitRepository interface {
	// InsertIgnore writes hits, skipping ones already recorded for the same
	// (tenant, message, rule). It returns how many rows were actually new, so
	// re-scanning reports 0 instead of inflating the log.
	InsertIgnore(ctx context.Context, hits []*types.ModerationHit) (int, error)
	List(
		ctx context.Context, tenantID uint64, agentID string, limit, offset int,
	) ([]*types.ModerationHit, int64, error)
	CountByCategory(
		ctx context.Context, tenantID uint64, agentID string, since time.Time,
	) (map[string]int64, error)
	// ListMessages returns recent conversation turns for an agent, newest
	// first. Messages have no tenant_id column, so the query joins sessions.
	ListMessages(
		ctx context.Context, tenantID uint64, agentID string, limit int,
	) ([]ModerationMessage, error)
}

// ModerationService scans conversations against the active rule set.
//
// The current implementation is record-only: it never changes what a user
// sees. Blocking is intentionally not implemented yet — see the migration
// comment for the reasoning.
type ModerationService interface {
	// ScanAgent scans the agent's most recent messages and records hits.
	ScanAgent(ctx context.Context, tenantID uint64, agentID string, limit int) (*types.ModerationScanResult, error)
	ListHits(
		ctx context.Context, tenantID uint64, agentID string, limit, offset int,
	) ([]*types.ModerationHit, int64, error)
	ListRules(ctx context.Context, tenantID uint64) ([]*types.ModerationRule, error)
	UpsertRule(ctx context.Context, tenantID uint64, rule *types.ModerationRule) (*types.ModerationRule, error)
	DeleteRule(ctx context.Context, tenantID uint64, id string) error
}
