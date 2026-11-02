package types

import (
	"strings"
	"testing"
)

func required(v bool) *bool { return &v }
func float(v float64) *float64 {
	return &v
}

func sampleMiniTool() *MiniTool {
	return &MiniTool{
		Name:        "渠道渗漏量估算",
		Category:    "水文分析",
		Description: "按达西公式估算渠道日渗漏量",
		Formula:     "Q = k * A * i",
		FormulaNote: "k 取现场试验值",
		Variables: MiniToolVariables{
			{Key: "k", Label: "渗透系数", Unit: "m/d", Required: required(true)},
			{Key: "b", Label: "渠底宽", Unit: "m", Required: required(true)},
			{Key: "i", Label: "水力坡降", Default: "0.001", Min: float(0), Max: float(1)},
		},
		Knowledge: "黏土 k 一般 0.001~0.01 m/d",
		Examples: MiniToolExamples{
			{Title: "示例一", Input: "k=0.005,b=3,i=0.001", Output: "Q=15 m³/d"},
		},
	}
}

func TestMiniToolRenderPromptCarriesEverySection(t *testing.T) {
	got := sampleMiniTool().RenderPrompt()

	for _, want := range []string{
		"渠道渗漏量估算",
		"category: 水文分析",
		"Purpose: 按达西公式估算渠道日渗漏量",
		"Q = k * A * i",
		"Caveats: k 取现场试验值",
		"k (渗透系数) [m/d] required",
		"i (水力坡降) default=0.001 range=0..1",
		"Domain knowledge:",
		"黏土 k 一般 0.001~0.01 m/d",
		"示例一: input k=0.005,b=3,i=0.001 → Q=15 m³/d",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderPrompt() missing %q\n---\n%s", want, got)
		}
	}

	// The closing instruction is what stops the model from inventing an input.
	if !strings.Contains(got, "Ask for any missing required input instead of inventing a value") {
		t.Fatalf("RenderPrompt() lost the missing-input guard:\n%s", got)
	}
}

func TestMiniToolRenderPromptHandlesEmptyCard(t *testing.T) {
	if got := (*MiniTool)(nil).RenderPrompt(); got != "" {
		t.Fatalf("nil tool should render nothing, got %q", got)
	}
	// A card with only a name must not emit dangling section headers.
	got := (&MiniTool{Name: "只读卡片"}).RenderPrompt()
	if strings.Contains(got, "Inputs:") || strings.Contains(got, "Examples:") {
		t.Fatalf("empty card emitted a section header:\n%s", got)
	}
}

func TestMiniToolValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*MiniTool)
		wantErr string
	}{
		{"valid", func(*MiniTool) {}, ""},
		{"missing name", func(t *MiniTool) { t.Name = "  " }, "name is required"},
		{
			"no formula and no knowledge",
			func(t *MiniTool) { t.Formula = ""; t.Knowledge = "" },
			"formula or knowledge is required",
		},
		{"knowledge only is enough", func(t *MiniTool) { t.Formula = "" }, ""},
		{
			"duplicate variable key",
			func(t *MiniTool) { t.Variables = append(t.Variables, MiniToolVariable{Key: "k"}) },
			`duplicate variable key "k"`,
		},
		{
			"empty variable key",
			func(t *MiniTool) { t.Variables = MiniToolVariables{{Key: ""}} },
			"variable 1: key is required",
		},
		{
			"min above max",
			func(t *MiniTool) {
				t.Variables = MiniToolVariables{{Key: "x", Min: float(5), Max: float(1)}}
			},
			`variable "x": min must not exceed max`,
		},
		{"name too long", func(t *MiniTool) { t.Name = strings.Repeat("a", MaxMiniToolNameLength+1) }, "name must not exceed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := sampleMiniTool()
			tc.mutate(tool)
			// The service normalizes before validating, so a whitespace-only
			// name is trimmed to empty and rejected there rather than here.
			tool.Normalize()
			err := tool.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestMiniToolVariablesRoundTrip(t *testing.T) {
	var variables MiniToolVariables
	if err := variables.Scan([]byte(`[{"key":"k","label":"渗透系数","required":true}]`)); err != nil {
		t.Fatalf("Scan() = %v", err)
	}
	if len(variables) != 1 || variables[0].Key != "k" {
		t.Fatalf("Scan() decoded %+v", variables)
	}
	if variables[0].Required == nil || !*variables[0].Required {
		t.Fatalf("Scan() lost the required flag: %+v", variables[0])
	}

	value, err := variables.Value()
	if err != nil {
		t.Fatalf("Value() = %v", err)
	}
	if !strings.Contains(value.(string), `"key":"k"`) {
		t.Fatalf("Value() = %v", value)
	}

	// A NULL column must decode without error into a fresh destination: rows
	// created before a column default was added still read back as valid cards.
	var nullVariables MiniToolVariables
	if err := nullVariables.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) = %v", err)
	}
	if len(nullVariables) != 0 {
		t.Fatalf("Scan(nil) = %+v, want empty", nullVariables)
	}
}
