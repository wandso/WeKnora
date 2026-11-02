package types_test

import (
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
)

// The built-in templates are shipped data, so the risks are structural rather
// than behavioural: a config that does not decode, an ID that collides, or a
// tool name that no longer exists and silently never runs. These tests pin all
// three, using the external test package so importing the tools package does
// not create an import cycle.

func TestBuiltinAgentTemplatesAreWellFormed(t *testing.T) {
	templates, err := types.BuiltinAgentTemplates()
	if err != nil {
		t.Fatalf("BuiltinAgentTemplates() error: %v", err)
	}
	if len(templates) == 0 {
		t.Fatal("expected at least one builtin template")
	}

	seen := make(map[string]bool, len(templates))
	for _, tpl := range templates {
		if seen[tpl.ID] {
			t.Fatalf("duplicate builtin template ID %q", tpl.ID)
		}
		seen[tpl.ID] = true

		if tpl.TenantID != 0 {
			t.Fatalf("%s: builtin templates must use tenant_id 0, got %d", tpl.ID, tpl.TenantID)
		}
		if tpl.Source != types.AgentTemplateSourceBuiltin {
			t.Fatalf("%s: expected source %q, got %q", tpl.ID, types.AgentTemplateSourceBuiltin, tpl.Source)
		}
		if tpl.Name == "" || tpl.Category == "" || tpl.Description == "" {
			t.Fatalf("%s: name/category/description must all be set", tpl.ID)
		}
		if tpl.AgentName == "" {
			t.Fatalf("%s: agent_name is what install uses for the new agent", tpl.ID)
		}

		var cfg types.CustomAgentConfig
		if err := json.Unmarshal(tpl.Config, &cfg); err != nil {
			t.Fatalf("%s: config does not decode into CustomAgentConfig: %v", tpl.ID, err)
		}
		switch cfg.AgentMode {
		case types.AgentModeQuickAnswer, types.AgentModeSmartReasoning:
		default:
			t.Fatalf("%s: unexpected agent_mode %q", tpl.ID, cfg.AgentMode)
		}
		if cfg.SystemPrompt == "" {
			t.Fatalf("%s: system prompt must be set", tpl.ID)
		}
		// A template cannot know which model a workspace has configured, so it
		// must leave ModelID empty and let the installer choose.
		if cfg.ModelID != "" {
			t.Fatalf("%s: templates must not pin a model, got %q", tpl.ID, cfg.ModelID)
		}
		// Installing a template must not silently grant MCP or skill access.
		if cfg.MCPSelectionMode != "none" {
			t.Fatalf("%s: mcp_selection_mode must be \"none\", got %q", tpl.ID, cfg.MCPSelectionMode)
		}
		if cfg.SkillsSelectionMode != "none" {
			t.Fatalf("%s: skills_selection_mode must be \"none\", got %q", tpl.ID, cfg.SkillsSelectionMode)
		}
	}
}

func TestBuiltinTemplateToolNames(t *testing.T) {
	known := map[string]bool{
		tools.ToolSearchKnowledge: true,
		tools.ToolWebSearch:       true,
		tools.ToolWebFetch:        true,
		tools.ToolDataSchema:      true,
		tools.ToolDataAnalysis:    true,
	}

	templates, err := types.BuiltinAgentTemplates()
	if err != nil {
		t.Fatalf("BuiltinAgentTemplates() error: %v", err)
	}
	for _, tpl := range templates {
		var cfg types.CustomAgentConfig
		if err := json.Unmarshal(tpl.Config, &cfg); err != nil {
			t.Fatalf("%s: decode config: %v", tpl.ID, err)
		}
		for _, name := range cfg.AllowedTools {
			if !known[name] {
				t.Fatalf("%s: unknown tool %q (renaming a tool must update the template)", tpl.ID, name)
			}
		}
		// quick-answer runs no tool loop, so allowing tools there is a sign the
		// template was authored for the wrong mode.
		if cfg.AgentMode == types.AgentModeQuickAnswer && len(cfg.AllowedTools) > 0 {
			t.Fatalf("%s: quick-answer templates cannot use tools", tpl.ID)
		}
	}
}

// Seeding is upsert-by-ID, so the output must be deterministic: two calls must
// produce byte-identical configs, otherwise every boot would rewrite rows.
func TestBuiltinAgentTemplatesAreDeterministic(t *testing.T) {
	first, err := types.BuiltinAgentTemplates()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := types.BuiltinAgentTemplates()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("length differs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("index %d: ID %q vs %q", i, first[i].ID, second[i].ID)
		}
		if string(first[i].Config) != string(second[i].Config) {
			t.Fatalf("%s: config is not deterministic", first[i].ID)
		}
	}
}
