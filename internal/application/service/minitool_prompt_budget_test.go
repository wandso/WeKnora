package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// The shipped catalog is large enough that "attach all cards" would otherwise
// push a hundred expanded formulas into one prompt.
func TestCapMiniToolsForTurnKeepsMostUsed(t *testing.T) {
	tools := make([]*types.MiniTool, 0, 20)
	for i := 0; i < 20; i++ {
		tools = append(tools, &types.MiniTool{
			ID:         fmt.Sprintf("card-%02d", i),
			Name:       fmt.Sprintf("card-%02d", i),
			UsageCount: int64(i),
			Enabled:    true,
		})
	}

	got := capMiniToolsForTurn(context.Background(), tools)
	if len(got) != maxMiniToolsPerTurn {
		t.Fatalf("expected %d cards, got %d", maxMiniToolsPerTurn, len(got))
	}
	// Highest usage first: card-19 .. card-08.
	if got[0].Name != "card-19" {
		t.Errorf("expected most-used card first, got %q", got[0].Name)
	}
	if got[len(got)-1].Name != "card-08" {
		t.Errorf("expected the cut to fall on card-08, got %q", got[len(got)-1].Name)
	}
	for _, card := range got {
		if card.UsageCount < int64(20-maxMiniToolsPerTurn) {
			t.Errorf("card %q with usage %d should not have survived the cut", card.Name, card.UsageCount)
		}
	}
}

func TestCapMiniToolsForTurnLeavesSmallSelectionsAlone(t *testing.T) {
	tools := []*types.MiniTool{
		{Name: "坝顶高程计算", Enabled: true},
		{Name: "单库调洪演算", Enabled: true},
	}
	got := capMiniToolsForTurn(context.Background(), tools)
	if len(got) != len(tools) {
		t.Fatalf("expected the selection to pass through, got %d cards", len(got))
	}
	if got[0].Name != "坝顶高程计算" {
		t.Errorf("order should be preserved for small selections, got %q first", got[0].Name)
	}
}
