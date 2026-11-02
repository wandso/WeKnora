package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Mini tool visibility. A private tool belongs to its creator; a workspace
// tool is readable by every member of the tenant. Write access is always
// limited to the creator (and workspace admins), never to the whole tenant:
// a shared card is a preset, not a shared document.
const (
	MiniToolVisibilityPrivate   = "private"
	MiniToolVisibilityWorkspace = "workspace"
)

// Mini tool selection modes, mirroring the skills / MCP vocabulary so the
// agent editor can reuse one control for all three resource types.
const (
	MiniToolSelectionAll      = "all"
	MiniToolSelectionSelected = "selected"
	MiniToolSelectionNone     = "none"
)

// Bounds keep one card's prompt injection from crowding out the conversation
// and stop a pasted document from becoming a system prompt.
const (
	MaxMiniToolNameLength        = 120
	MaxMiniToolDescriptionLength = 1000
	MaxMiniToolFormulaLength     = 8000
	MaxMiniToolKnowledgeLength   = 20000
	MaxMiniToolVariables         = 32
	MaxMiniToolExamples          = 8
)

// MiniToolVariable documents one input of a tool's formula. The chat turn
// renders these so the model can tell the user exactly which value is missing
// instead of inventing one.
type MiniToolVariable struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Unit     string   `json:"unit,omitempty"`
	Default  string   `json:"default,omitempty"`
	Remark   string   `json:"remark,omitempty"`
	Required *bool    `json:"required,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
}

// MiniToolExample is a worked example shown in the card detail and injected
// into the prompt as a formatting hint.
type MiniToolExample struct {
	Title  string `json:"title"`
	Input  string `json:"input"`
	Output string `json:"output"`
}

// MiniToolVariables is the GORM/JSON column type for a card's inputs.
type MiniToolVariables []MiniToolVariable

// Value implements driver.Valuer.
func (v MiniToolVariables) Value() (driver.Value, error) {
	if v == nil {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

// Scan implements sql.Scanner.
func (v *MiniToolVariables) Scan(value interface{}) error {
	return scanMiniToolJSON(value, v)
}

// MiniToolExamples is the GORM/JSON column type for a card's examples.
type MiniToolExamples []MiniToolExample

// Value implements driver.Valuer.
func (v MiniToolExamples) Value() (driver.Value, error) {
	if v == nil {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

// Scan implements sql.Scanner.
func (v *MiniToolExamples) Scan(value interface{}) error {
	return scanMiniToolJSON(value, v)
}

func scanMiniToolJSON(value interface{}, dest interface{}) error {
	if value == nil {
		return nil
	}
	var b []byte
	switch raw := value.(type) {
	case []byte:
		b = raw
	case string:
		b = []byte(raw)
	default:
		return fmt.Errorf("unsupported mini tool column type %T", value)
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dest)
}

// MiniTool is one card of the 小工具 module. It is simultaneously:
//   - a catalog entry (list / filter / search / share)
//   - a chat preset: its formula and domain knowledge are injected into the
//     agent's system prompt when the card is attached to a turn
//
// The WeKnora agent config references cards by ID, so a card can be edited
// without rewriting every agent that uses it.
type MiniTool struct {
	ID       string `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID uint64 `json:"tenant_id" gorm:"not null;index:idx_mini_tools_tenant"`
	// OwnerUserID is the account that created the card. Empty is treated as
	// "workspace owned" and is only produced by built-in seeds.
	OwnerUserID string `json:"owner_user_id" gorm:"type:varchar(36);index:idx_mini_tools_owner"`

	Name        string `json:"name" gorm:"type:varchar(120);not null"`
	Icon        string `json:"icon" gorm:"type:varchar(64)"`
	Category    string `json:"category" gorm:"type:varchar(64);index:idx_mini_tools_category"`
	Description string `json:"description" gorm:"type:text"`

	// Formula is the calculation or procedure the card applies, written the
	// way the model should explain it. FormulaNote carries caveats.
	Formula     string            `json:"formula" gorm:"type:text"`
	FormulaNote string            `json:"formula_note" gorm:"type:text"`
	Variables   MiniToolVariables `json:"variables" gorm:"type:text"`
	Knowledge   string            `json:"knowledge" gorm:"type:text"`
	Examples    MiniToolExamples  `json:"examples" gorm:"type:text"`

	// Visibility is private (creator only) or workspace (all members read).
	Visibility string `json:"visibility" gorm:"type:varchar(16);not null;default:'private'"`
	// Enabled hides a card from the picker without deleting it.
	Enabled bool `json:"enabled" gorm:"not null;default:true"`

	UsageCount int64 `json:"usage_count" gorm:"not null;default:0"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// OwnerName is filled by the list endpoint so the UI can show who owns a
	// workspace card. Not persisted.
	OwnerName string `json:"owner_name,omitempty" gorm:"-"`
	// CanEdit / CanDelete are computed per request from the caller's role.
	CanEdit   bool `json:"can_edit" gorm:"-"`
	CanDelete bool `json:"can_delete" gorm:"-"`
}

// TableName returns the table name for MiniTool.
func (MiniTool) TableName() string {
	return "mini_tools"
}

// Normalize fills defaults that must hold for every write path.
func (t *MiniTool) Normalize() {
	if t == nil {
		return
	}
	t.Name = strings.TrimSpace(t.Name)
	t.Icon = strings.TrimSpace(t.Icon)
	t.Category = strings.TrimSpace(t.Category)
	t.Description = strings.TrimSpace(t.Description)
	t.Formula = strings.TrimSpace(t.Formula)
	t.FormulaNote = strings.TrimSpace(t.FormulaNote)
	t.Knowledge = strings.TrimSpace(t.Knowledge)
	switch t.Visibility {
	case MiniToolVisibilityWorkspace:
	default:
		t.Visibility = MiniToolVisibilityPrivate
	}
	if t.Category == "" {
		t.Category = "其他"
	}
	if t.Variables == nil {
		t.Variables = MiniToolVariables{}
	}
	if t.Examples == nil {
		t.Examples = MiniToolExamples{}
	}
	for i := range t.Variables {
		t.Variables[i].Key = strings.TrimSpace(t.Variables[i].Key)
		t.Variables[i].Label = strings.TrimSpace(t.Variables[i].Label)
		t.Variables[i].Unit = strings.TrimSpace(t.Variables[i].Unit)
		t.Variables[i].Default = strings.TrimSpace(t.Variables[i].Default)
		t.Variables[i].Remark = strings.TrimSpace(t.Variables[i].Remark)
	}
	for i := range t.Examples {
		t.Examples[i].Title = strings.TrimSpace(t.Examples[i].Title)
		t.Examples[i].Input = strings.TrimSpace(t.Examples[i].Input)
		t.Examples[i].Output = strings.TrimSpace(t.Examples[i].Output)
	}
}

// Validate rejects a card the prompt renderer cannot use. The checks are
// deliberately about structure (a formula must exist, variables need unique
// keys) rather than about domain correctness.
func (t *MiniTool) Validate() error {
	if t == nil {
		return fmt.Errorf("mini tool is required")
	}
	if t.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len([]rune(t.Name)) > MaxMiniToolNameLength {
		return fmt.Errorf("name must not exceed %d characters", MaxMiniToolNameLength)
	}
	if len([]rune(t.Description)) > MaxMiniToolDescriptionLength {
		return fmt.Errorf("description must not exceed %d characters", MaxMiniToolDescriptionLength)
	}
	if len([]rune(t.Formula)) > MaxMiniToolFormulaLength {
		return fmt.Errorf("formula must not exceed %d characters", MaxMiniToolFormulaLength)
	}
	if len([]rune(t.Knowledge)) > MaxMiniToolKnowledgeLength {
		return fmt.Errorf("knowledge must not exceed %d characters", MaxMiniToolKnowledgeLength)
	}
	if t.Formula == "" && t.Knowledge == "" {
		return fmt.Errorf("formula or knowledge is required")
	}
	if len(t.Variables) > MaxMiniToolVariables {
		return fmt.Errorf("at most %d variables are allowed", MaxMiniToolVariables)
	}
	seen := make(map[string]struct{}, len(t.Variables))
	for i, v := range t.Variables {
		if v.Key == "" {
			return fmt.Errorf("variable %d: key is required", i+1)
		}
		if _, dup := seen[v.Key]; dup {
			return fmt.Errorf("duplicate variable key %q", v.Key)
		}
		seen[v.Key] = struct{}{}
		if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
			return fmt.Errorf("variable %q: min must not exceed max", v.Key)
		}
	}
	if len(t.Examples) > MaxMiniToolExamples {
		return fmt.Errorf("at most %d examples are allowed", MaxMiniToolExamples)
	}
	return nil
}

// RenderPrompt renders the card as the system-prompt block injected for a
// turn. Every field is escaped by the prompt builder, so this returns plain
// text sections rather than XML.
func (t *MiniTool) RenderPrompt() string {
	if t == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Mini tool: ")
	b.WriteString(t.Name)
	if t.Category != "" {
		b.WriteString(" (category: ")
		b.WriteString(t.Category)
		b.WriteString(")")
	}
	b.WriteString("\n")
	if t.Description != "" {
		b.WriteString("Purpose: ")
		b.WriteString(t.Description)
		b.WriteString("\n")
	}
	if t.Formula != "" {
		b.WriteString("Method / formula:\n")
		b.WriteString(t.Formula)
		b.WriteString("\n")
	}
	if t.FormulaNote != "" {
		b.WriteString("Caveats: ")
		b.WriteString(t.FormulaNote)
		b.WriteString("\n")
	}
	if len(t.Variables) > 0 {
		b.WriteString("Inputs:\n")
		for _, v := range t.Variables {
			b.WriteString("- ")
			b.WriteString(v.Key)
			if v.Label != "" && v.Label != v.Key {
				b.WriteString(" (")
				b.WriteString(v.Label)
				b.WriteString(")")
			}
			if v.Unit != "" {
				b.WriteString(" [")
				b.WriteString(v.Unit)
				b.WriteString("]")
			}
			if v.Default != "" {
				b.WriteString(" default=")
				b.WriteString(v.Default)
			}
			if v.Required != nil && *v.Required {
				b.WriteString(" required")
			}
			if v.Min != nil || v.Max != nil {
				b.WriteString(" range=")
				if v.Min != nil {
					b.WriteString(formatMiniToolFloat(*v.Min))
				} else {
					b.WriteString("-inf")
				}
				b.WriteString("..")
				if v.Max != nil {
					b.WriteString(formatMiniToolFloat(*v.Max))
				} else {
					b.WriteString("+inf")
				}
			}
			if v.Remark != "" {
				b.WriteString(" — ")
				b.WriteString(v.Remark)
			}
			b.WriteString("\n")
		}
	}
	if t.Knowledge != "" {
		b.WriteString("Domain knowledge:\n")
		b.WriteString(t.Knowledge)
		b.WriteString("\n")
	}
	if len(t.Examples) > 0 {
		b.WriteString("Examples:\n")
		for _, e := range t.Examples {
			b.WriteString("- ")
			if e.Title != "" {
				b.WriteString(e.Title)
				b.WriteString(": ")
			}
			b.WriteString("input ")
			b.WriteString(e.Input)
			b.WriteString(" → ")
			b.WriteString(e.Output)
			b.WriteString("\n")
		}
	}
	b.WriteString("Use this card's method when the request matches it. Ask for any missing " +
		"required input instead of inventing a value, and show the substituted numbers.")
	return strings.TrimSpace(b.String())
}

func formatMiniToolFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
}

// MiniToolQuery narrows a catalog listing. Empty fields mean "no filter".
type MiniToolQuery struct {
	Category string
	Keyword  string
	// Scope is all | mine | workspace. Empty behaves like all.
	Scope string
	// IncludeDisabled keeps switched-off cards in the result; the picker used
	// by an agent never sets it.
	IncludeDisabled bool
	// OwnerUserID limits the result to one creator (used by scope=mine).
	OwnerUserID string
	Limit       int
	Offset      int
}
