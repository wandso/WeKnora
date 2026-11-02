package types

import (
	"strings"
	"testing"
)

// The shipped catalog is data, so the only thing that can silently rot is the
// file itself: a duplicated name would make seeding overwrite one card with
// another, and an empty formula would inject a useless prompt block.
func TestBuiltinMiniToolsCatalogIsWellFormed(t *testing.T) {
	cards, err := BuiltinMiniTools()
	if err != nil {
		t.Fatalf("BuiltinMiniTools() error: %v", err)
	}
	if len(cards) < 100 {
		t.Fatalf("expected the shipped catalog to carry at least 100 cards, got %d", len(cards))
	}

	names := make(map[string]struct{}, len(cards))
	categories := make(map[string]int, 8)
	for _, card := range cards {
		if _, dup := names[card.Name]; dup {
			t.Errorf("duplicate card name %q", card.Name)
		}
		names[card.Name] = struct{}{}

		if strings.TrimSpace(card.Category) == "" {
			t.Errorf("%q has no category", card.Name)
		}
		categories[card.Category]++
		if strings.TrimSpace(card.Description) == "" {
			t.Errorf("%q has no description", card.Name)
		}
		if strings.TrimSpace(card.Formula) == "" {
			t.Errorf("%q has no formula", card.Name)
		}
		if len(card.Variables) == 0 {
			t.Errorf("%q documents no input variables", card.Name)
		}
		for _, v := range card.Variables {
			if strings.TrimSpace(v.Key) == "" || strings.TrimSpace(v.Label) == "" {
				t.Errorf("%q has an input without key or label: %+v", card.Name, v)
			}
		}
		if card.Visibility != MiniToolVisibilityWorkspace {
			t.Errorf("%q should ship as a workspace card, got %q", card.Name, card.Visibility)
		}
		if !card.Enabled {
			t.Errorf("%q shipped disabled", card.Name)
		}
	}

	// The catalog covers the tool groups the water institute asked for; losing
	// a whole group means the JSON was truncated or a category was renamed.
	for _, category := range []string{"水文分析", "工程设计", "地质勘察", "安全监测", "地图工具"} {
		if categories[category] == 0 {
			t.Errorf("category %q has no cards", category)
		}
	}
}
