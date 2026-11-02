package types

import "time"

// Moderation direction: which half of the conversation was scanned.
const (
	ModerationDirectionInput  = "input"
	ModerationDirectionOutput = "output"
)

// Moderation action. Only "record" is produced by the current implementation;
// "block" is reserved so switching a rule to blocking later is a data change
// rather than a schema change.
const (
	ModerationActionRecord = "record"
	ModerationActionBlock  = "block"
)

// Moderation categories.
const (
	// ModerationCategoryPII covers structural detectors built into the product
	// (phone numbers, national ID numbers, bank cards). They are patterns, not
	// vocabulary, so they do not age or differ by jurisdiction.
	ModerationCategoryPII = "pii"
	// ModerationCategoryCustom covers tenant-authored keyword/regex rules.
	ModerationCategoryCustom = "custom"
)

// ModerationRule is a tenant-owned detector. The product ships no built-in word
// list on purpose: a wrong entry silently censors legitimate business content,
// and vocabulary lists date quickly. Tenants bring their own keywords; the
// product only provides the structural PII detectors.
type ModerationRule struct {
	ID       string `json:"id"       gorm:"type:varchar(36);primaryKey"`
	TenantID uint64 `json:"tenant_id" gorm:"not null;default:0;index:idx_moderation_rules_tenant"`
	Name     string `json:"name"     gorm:"type:varchar(255);not null"`
	Category string `json:"category" gorm:"type:varchar(64);not null;default:'custom'"`
	// Pattern is matched case-insensitively as a substring, or as a regular
	// expression when IsRegex is set.
	Pattern string `json:"pattern" gorm:"type:text;not null"`
	IsRegex bool   `json:"is_regex" gorm:"not null;default:false"`
	Action  string `json:"action"  gorm:"type:varchar(16);not null;default:'record'"`
	Enabled bool   `json:"enabled" gorm:"not null;default:true"`

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName pins the table name to the migration.
func (ModerationRule) TableName() string { return "moderation_rules" }

// ModerationHit records that a rule matched a message.
//
// message_id + rule_id is unique so re-scanning a conversation is idempotent:
// the observation log must not grow just because someone pressed "scan" twice.
type ModerationHit struct {
	ID        string `json:"id"         gorm:"type:varchar(36);primaryKey"`
	TenantID  uint64 `json:"tenant_id"  gorm:"not null;index:idx_moderation_hits_agent"`
	AgentID   string `json:"agent_id"   gorm:"type:varchar(36);not null;default:''"`
	SessionID string `json:"session_id" gorm:"type:varchar(36);not null;default:''"`
	MessageID string `json:"message_id" gorm:"type:varchar(36);not null;default:'';uniqueIndex:idx_moderation_hits_unique"`
	Direction string `json:"direction"  gorm:"type:varchar(16);not null;default:'output'"`
	RuleID    string `json:"rule_id"    gorm:"type:varchar(36);not null;default:'';uniqueIndex:idx_moderation_hits_unique"`
	RuleName  string `json:"rule_name"  gorm:"type:varchar(255);not null;default:''"`
	Category  string `json:"category"   gorm:"type:varchar(64);not null;default:'custom'"`
	// Snippet is the surrounding text around the match, truncated. It is stored
	// so a reviewer can judge the hit without opening the conversation, and it
	// is deliberately short so the table cannot become a copy of the corpus.
	Snippet   string    `json:"snippet"   gorm:"type:text;not null;default:''"`
	Action    string    `json:"action"    gorm:"type:varchar(16);not null;default:'record'"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName pins the table name to the migration.
func (ModerationHit) TableName() string { return "moderation_hits" }

// ModerationScanResult summarises one scan run so the UI can report something
// more useful than "done".
type ModerationScanResult struct {
	// Scanned is how many messages were inspected.
	Scanned int `json:"scanned"`
	// Matched is how many hits were written (new rows only; duplicates from a
	// previous scan are not counted).
	Matched int `json:"matched"`
	// Rules is how many enabled rules were applied.
	Rules int `json:"rules"`
}
