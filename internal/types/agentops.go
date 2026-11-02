package types

import "time"

// This file holds the productisation layer models: agent versioning, answer
// feedback, per-agent usage, agent templates and public agent pages.
//
// They are deliberately separate from CustomAgent: the runtime does not read
// them, and a tenant that never opens the operations centre should not pay for
// the extra rows.

// ---------------------------------------------------------------------------
// Agent versions
// ---------------------------------------------------------------------------

// AgentVersion is an immutable snapshot of a custom agent's user-visible
// fields and full config. A snapshot is written after every successful update,
// so the sequence doubles as an edit history.
//
// Rollback never rewrites history: it applies an old snapshot through the
// normal update path, which appends a new version. That keeps the list
// append-only and makes "what did we run last Tuesday" answerable.
type AgentVersion struct {
	ID          string `json:"id"          gorm:"type:varchar(36);primaryKey"`
	TenantID    uint64 `json:"tenant_id"   gorm:"not null;uniqueIndex:idx_agent_versions_no,priority:1"`
	AgentID     string `json:"agent_id"    gorm:"type:varchar(36);not null;uniqueIndex:idx_agent_versions_no,priority:2;index:idx_agent_versions_agent"`
	VersionNo   int    `json:"version_no"  gorm:"not null;uniqueIndex:idx_agent_versions_no,priority:3"`
	Name        string `json:"name"        gorm:"type:varchar(255);not null"`
	Description string `json:"description" gorm:"type:text"`
	Avatar      string `json:"avatar"      gorm:"type:varchar(64)"`
	Config      JSON   `json:"config"      gorm:"type:json"`
	Changelog   string `json:"changelog"   gorm:"type:varchar(512)"`
	CreatedBy   string `json:"created_by"  gorm:"type:varchar(36)"`
	// Source records why the snapshot exists: "update" (normal save),
	// "publish" (explicit snapshot), "rollback" (restored from an older one)
	// or "create" (the initial state).
	Source    string    `json:"source"     gorm:"type:varchar(16);not null;default:'update'"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName pins the table name to the migration.
func (AgentVersion) TableName() string { return "agent_versions" }

// Agent version sources.
const (
	AgentVersionSourceCreate   = "create"
	AgentVersionSourceUpdate   = "update"
	AgentVersionSourcePublish  = "publish"
	AgentVersionSourceRollback = "rollback"
)

// ---------------------------------------------------------------------------
// Answer feedback
// ---------------------------------------------------------------------------

// Feedback ratings.
const (
	FeedbackRatingUp   = "up"
	FeedbackRatingDown = "down"
)

// Feedback reason tags shown for a downvote. Free-form comments are allowed in
// addition, but keeping a closed tag set is what makes the numbers comparable
// across agents and over time.
const (
	FeedbackReasonWrongAnswer = "wrong_answer"
	FeedbackReasonBadCitation = "bad_citation"
	FeedbackReasonIncomplete  = "incomplete"
	FeedbackReasonTooSlow     = "too_slow"
	FeedbackReasonUnsafe      = "unsafe"
	FeedbackReasonFormat      = "bad_format"
	FeedbackReasonToolFailure = "tool_failure"
	FeedbackReasonOther       = "other"
)

// IsValidFeedbackRating reports whether r is a rating this product stores.
func IsValidFeedbackRating(r string) bool {
	return r == FeedbackRatingUp || r == FeedbackRatingDown
}

// IsValidFeedbackReason reports whether tag belongs to the closed reason set.
func IsValidFeedbackReason(tag string) bool {
	switch tag {
	case FeedbackReasonWrongAnswer, FeedbackReasonBadCitation, FeedbackReasonIncomplete,
		FeedbackReasonTooSlow, FeedbackReasonUnsafe, FeedbackReasonFormat,
		FeedbackReasonToolFailure, FeedbackReasonOther:
		return true
	default:
		return false
	}
}

// MessageFeedback is one user's verdict on one assistant message.
//
// The unique key is (tenant_id, message_id, user_id): a user may change their
// mind (upsert), but cannot stack multiple votes on the same answer. Message
// ids are UUIDs, so no foreign key is declared — messages can be deleted while
// the aggregate feedback stays useful for quality reporting.
type MessageFeedback struct {
	ID        string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	TenantID  uint64 `json:"tenant_id"  gorm:"not null;uniqueIndex:idx_message_feedback_vote,priority:1;index:idx_message_feedback_agent"`
	MessageID string `json:"message_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_message_feedback_vote,priority:2"`
	UserID    string `json:"user_id"    gorm:"type:varchar(128);not null;uniqueIndex:idx_message_feedback_vote,priority:3"`
	SessionID string `json:"session_id" gorm:"type:varchar(36);index:idx_message_feedback_session"`
	AgentID   string `json:"agent_id"   gorm:"type:varchar(36);index:idx_message_feedback_agent"`
	Rating    string `json:"rating"     gorm:"type:varchar(8);not null"`
	// ReasonTags is a JSON array of FeedbackReason* values. Empty for upvotes.
	ReasonTags JSON      `json:"reason_tags" gorm:"type:json"`
	Comment    string    `json:"comment"     gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at"  gorm:"autoCreateTime"`
	UpdatedAt  time.Time `json:"updated_at"  gorm:"autoUpdateTime"`
}

// TableName pins the table name to the migration.
func (MessageFeedback) TableName() string { return "message_feedback" }

// ---------------------------------------------------------------------------
// Per-agent usage
// ---------------------------------------------------------------------------
//
// Usage is derived from the `messages` table rather than written by a new
// runtime hook. Every channel (web chat, embed widget, IM, automation) already
// persists assistant messages with an `usage` JSON and an `agent_id`, so
// aggregating there is retroactive, cannot drift from what the user actually
// received, and adds no code to the streaming hot path.
//
// The trade-off: per-turn latency and tool-call counts are not stored as
// columns. Step counts are recovered from `agent_steps`; latency is left to
// the tracing layer, which already measures it.

// ---------------------------------------------------------------------------
// Agent templates
// ---------------------------------------------------------------------------

// Agent template scopes.
const (
	AgentTemplateSourceBuiltin = "builtin"
	AgentTemplateSourceTenant  = "tenant"
)

// AgentTemplate is a reusable agent configuration that a user can install as a
// new agent in one click. Built-in templates ship with the product (TenantID
// 0); tenant templates are saved from an existing agent and stay inside that
// workspace.
type AgentTemplate struct {
	ID          string `json:"id"          gorm:"type:varchar(36);primaryKey"`
	TenantID    uint64 `json:"tenant_id"   gorm:"not null;default:0;index:idx_agent_templates_tenant"`
	Name        string `json:"name"        gorm:"type:varchar(255);not null"`
	Category    string `json:"category"    gorm:"type:varchar(64);not null;default:'general'"`
	Icon        string `json:"icon"        gorm:"type:varchar(64)"`
	Description string `json:"description" gorm:"type:text"`
	// Config is the agent config snapshot, stored in the same shape as
	// CustomAgentConfig so install is a plain copy.
	Config JSON `json:"config" gorm:"type:json"`
	// Name/Description/Avatar of the source agent are folded into Config's
	// siblings below so installing reproduces the whole agent, not just knobs.
	AgentName        string `json:"agent_name"        gorm:"type:varchar(255)"`
	AgentDescription string `json:"agent_description" gorm:"type:text"`
	AgentAvatar      string `json:"agent_avatar"      gorm:"type:varchar(64)"`
	Source           string `json:"source"            gorm:"type:varchar(16);not null;default:'tenant'"`
	CreatedBy        string `json:"created_by"        gorm:"type:varchar(36)"`
	// Installs counts how many agents were created from this template.
	Installs  int       `json:"installs"  gorm:"not null;default:0"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName pins the table name to the migration.
func (AgentTemplate) TableName() string { return "agent_templates" }

// ---------------------------------------------------------------------------
// Public agent pages
// ---------------------------------------------------------------------------

// Public page access modes.
const (
	// PublicPageAccessOpen needs no credential at all.
	PublicPageAccessOpen = "open"
	// PublicPageAccessPassword requires the shared password before the page
	// hands out a chat session.
	PublicPageAccessPassword = "password"
)

// AgentPublicPage publishes an agent at a human-readable slug so it can be
// linked, indexed and shared outside the product. The chat itself still runs
// through the embed channel's anonymous session flow; this row owns the
// marketing surface (slug, SEO, access mode).
type AgentPublicPage struct {
	ID       string `json:"id"       gorm:"type:varchar(36);primaryKey"`
	TenantID uint64 `json:"tenant_id" gorm:"not null;index:idx_agent_public_pages_tenant"`
	AgentID  string `json:"agent_id"  gorm:"type:varchar(36);not null;index:idx_agent_public_pages_agent"`
	// EmbedChannelID is the anonymous chat surface the page renders. Required:
	// a public page without a channel has no way to answer.
	EmbedChannelID string `json:"embed_channel_id" gorm:"type:varchar(36);not null"`
	Slug           string `json:"slug"             gorm:"type:varchar(64);not null;uniqueIndex:idx_agent_public_pages_slug"`
	Title          string `json:"title"            gorm:"type:varchar(255)"`
	Description    string `json:"description"      gorm:"type:text"`
	OGImageURL     string `json:"og_image_url"     gorm:"type:varchar(512)"`
	AccessMode     string `json:"access_mode"      gorm:"type:varchar(16);not null;default:'open'"`
	// PasswordHash is bcrypt; empty means no password. Never serialised.
	PasswordHash string    `json:"-"                gorm:"type:varchar(128)"`
	Enabled      bool      `json:"enabled"          gorm:"not null;default:true"`
	CreatedAt    time.Time `json:"created_at"       gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at"       gorm:"autoUpdateTime"`
}

// TableName pins the table name to the migration.
func (AgentPublicPage) TableName() string { return "agent_public_pages" }

// ---------------------------------------------------------------------------
// Reporting shapes
// ---------------------------------------------------------------------------

// AgentUsageSummary is the aggregate of assistant turns over a time window.
type AgentUsageSummary struct {
	Turns            int64   `json:"turns"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	AvgSteps         float64 `json:"avg_steps"`
	AvgLatencyMS     float64 `json:"avg_latency_ms"`
	UniqueSessions   int64   `json:"unique_sessions"`
}

// AgentUsagePoint is one bucket of the usage timeseries.
type AgentUsagePoint struct {
	Bucket      string `json:"bucket"`
	Turns       int64  `json:"turns"`
	TotalTokens int64  `json:"total_tokens"`
}

// FeedbackSummary is the vote tally for one agent.
type FeedbackSummary struct {
	Up      int64            `json:"up"`
	Down    int64            `json:"down"`
	Total   int64            `json:"total"`
	UpRate  float64          `json:"up_rate"`
	Reasons map[string]int64 `json:"reasons"`
}

// AgentQualityReport joins usage and feedback for one agent. It is the payload
// behind the operations centre overview: cost, reliability and satisfaction
// side by side, because optimising one of them alone is how agents regress.
type AgentQualityReport struct {
	AgentID  string            `json:"agent_id"`
	Since    time.Time         `json:"since"`
	Usage    AgentUsageSummary `json:"usage"`
	Feedback FeedbackSummary   `json:"feedback"`
}

// AgentVersionDiff is a key-level comparison between two snapshots. Only the
// top-level config keys that differ are listed: that is enough for a review UI
// ("model, system_prompt, allowed_tools changed") without shipping a full
// structural diff renderer.
type AgentVersionDiff struct {
	From        int      `json:"from"`
	To          int      `json:"to"`
	ChangedKeys []string `json:"changed_keys"`
	// NameChanged / DescriptionChanged / AvatarChanged cover the fields that
	// live outside Config.
	NameChanged        bool `json:"name_changed"`
	DescriptionChanged bool `json:"description_changed"`
	AvatarChanged      bool `json:"avatar_changed"`
}

// AgentDryRunResult is the outcome of running an agent against an unsaved
// draft configuration. It mirrors what the editor needs to judge a change
// before publishing: the answer, what it cost, and which steps it took.
type AgentDryRunResult struct {
	Answer     string      `json:"answer"`
	Usage      *TokenUsage `json:"usage,omitempty"`
	Steps      []AgentStep `json:"steps,omitempty"`
	DurationMS int64       `json:"duration_ms"`
}
