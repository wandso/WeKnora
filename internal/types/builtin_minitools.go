package types

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// The product ships a catalog of engineering calculation cards so a fresh
// workspace does not open onto an empty toolbox. The cards are data, not code:
// each one carries the inputs a model must ask for, the formula it should
// apply and the code clause that governs it, which is exactly what the chat
// pipeline injects into a turn.
//
//go:embed data/builtin_minitools.json
var builtinMiniToolCatalog []byte

// builtinMiniToolDoc mirrors data/builtin_minitools.json. Only the fields the
// seeder needs are modelled; anything else in the file is ignored on purpose
// so the catalog can grow without touching this file.
type builtinMiniToolDoc struct {
	Version int `json:"version"`
	Tools   []struct {
		Name        string            `json:"name"`
		Icon        string            `json:"icon"`
		Category    string            `json:"category"`
		Description string            `json:"description"`
		Formula     string            `json:"formula"`
		FormulaNote string            `json:"formula_note"`
		Variables   MiniToolVariables `json:"variables"`
		Knowledge   string            `json:"knowledge"`
		Examples    MiniToolExamples  `json:"examples"`
	} `json:"tools"`
}

var (
	builtinMiniToolsOnce sync.Once
	builtinMiniToolsList []*MiniTool
	builtinMiniToolsErr  error
)

// BuiltinMiniTools returns the shipped calculation cards, validated once.
//
// The returned cards carry no tenant or ID: the caller stamps those per
// workspace so the same definition can be seeded into every tenant. Names are
// the seeding key, so a duplicate name is a catalog bug and is rejected rather
// than silently collapsed.
func BuiltinMiniTools() ([]*MiniTool, error) {
	builtinMiniToolsOnce.Do(func() {
		var doc builtinMiniToolDoc
		if err := json.Unmarshal(builtinMiniToolCatalog, &doc); err != nil {
			builtinMiniToolsErr = fmt.Errorf("parse builtin mini tool catalog: %w", err)
			return
		}
		if len(doc.Tools) == 0 {
			builtinMiniToolsErr = fmt.Errorf("builtin mini tool catalog is empty")
			return
		}
		seen := make(map[string]struct{}, len(doc.Tools))
		cards := make([]*MiniTool, 0, len(doc.Tools))
		for i, raw := range doc.Tools {
			name := strings.TrimSpace(raw.Name)
			if name == "" {
				builtinMiniToolsErr = fmt.Errorf("builtin mini tool #%d has no name", i)
				return
			}
			if _, dup := seen[name]; dup {
				builtinMiniToolsErr = fmt.Errorf("duplicate builtin mini tool name %q", name)
				return
			}
			seen[name] = struct{}{}
			if len([]rune(name)) > MaxMiniToolNameLength {
				builtinMiniToolsErr = fmt.Errorf("builtin mini tool %q name is too long", name)
				return
			}
			description := strings.TrimSpace(raw.Description)
			if len([]rune(description)) > MaxMiniToolDescriptionLength {
				builtinMiniToolsErr = fmt.Errorf("builtin mini tool %q description is too long", name)
				return
			}
			formula := strings.TrimSpace(raw.Formula)
			if len([]rune(formula)) > MaxMiniToolFormulaLength {
				builtinMiniToolsErr = fmt.Errorf("builtin mini tool %q formula is too long", name)
				return
			}
			cards = append(cards, &MiniTool{
				Name:        name,
				Icon:        strings.TrimSpace(raw.Icon),
				Category:    strings.TrimSpace(raw.Category),
				Description: description,
				Formula:     formula,
				FormulaNote: strings.TrimSpace(raw.FormulaNote),
				Variables:   raw.Variables,
				Knowledge:   strings.TrimSpace(raw.Knowledge),
				Examples:    raw.Examples,
				// Shipped cards are readable by every member of the workspace
				// and never editable: a shared preset, not a shared document.
				Visibility: MiniToolVisibilityWorkspace,
				Enabled:    true,
			})
		}
		builtinMiniToolsList = cards
	})
	return builtinMiniToolsList, builtinMiniToolsErr
}
