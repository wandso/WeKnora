package chatpipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type gateChat struct {
	reply string
	err   error
	calls int
}

func (c *gateChat) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return &types.ChatResponse{Content: c.reply}, nil
}

func (c *gateChat) ChatStream(
	context.Context, []chat.Message, *chat.ChatOptions,
) (<-chan types.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (c *gateChat) GetModelName() string { return "gate-test" }

func (c *gateChat) GetModelID() string { return "gate-test" }

type gateModelService struct {
	interfaces.ModelService
	model chat.Chat
	err   error
}

func (s *gateModelService) GetChatModel(context.Context, string) (chat.Chat, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.model, nil
}

func gateCandidates(n int) []*types.SearchResult {
	out := make([]*types.SearchResult, n)
	for i := range out {
		out[i] = &types.SearchResult{
			ID:         string(rune('a' + i)),
			Content:    "content",
			Score:      float64(n - i),
			MatchType:  types.MatchTypeEmbedding,
			ChunkIndex: i,
		}
	}
	return out
}

func gateChatManage(n int) *types.ChatManage {
	return &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			Query:            "如何配置超时",
			ChatModelID:      "chat",
			RerankTopK:       3,
			EmbeddingTopK:    n,
			PassageGate:      types.PassageGateSettings{Enabled: true},
			KnowledgeBaseIDs: []string{"kb"},
		},
		PipelineState: types.PipelineState{
			RewriteQuery: "如何配置超时",
			Intent:       types.IntentKBSearch,
			SearchResult: gateCandidates(n),
		},
	}
}

func runGate(t *testing.T, svc *gateModelService, cm *types.ChatManage) bool {
	t.Helper()
	plugin := &PluginPassageGate{modelService: svc}
	reached := false
	perr := plugin.OnEvent(context.Background(), types.CHUNK_RERANK, cm, func() *PluginError {
		reached = true
		return nil
	})
	require.Nil(t, perr)
	return reached
}

func TestPassageGateDisabledLeavesPoolUntouched(t *testing.T) {
	cm := gateChatManage(30)
	cm.PassageGate.Enabled = false
	svc := &gateModelService{model: &gateChat{reply: `{"keep":[1]}`}}
	require.True(t, runGate(t, svc, cm))
	require.Len(t, cm.SearchResult, 30)
	require.Equal(t, types.PassageGateOutcomeDisabled, cm.PassageGateDiagnostics.Outcome)
}

func TestPassageGateSkipsSmallPool(t *testing.T) {
	cm := gateChatManage(5)
	svc := &gateModelService{model: &gateChat{reply: `{"keep":[1]}`}}
	require.True(t, runGate(t, svc, cm))
	require.Len(t, cm.SearchResult, 5)
	require.Equal(t, types.PassageGateOutcomeSmallPool, cm.PassageGateDiagnostics.Outcome)
	require.Zero(t, svc.model.(*gateChat).calls)
}

func TestPassageGateKeepsJudgedSubsetInOriginalOrder(t *testing.T) {
	cm := gateChatManage(30)
	svc := &gateModelService{model: &gateChat{reply: "```json\n{\"keep\":[9,2]}\n```"}}
	require.True(t, runGate(t, svc, cm))
	// 2 and 9 are kept; the floor is RerankTopK (3), so candidate 1 tops it up.
	require.Len(t, cm.SearchResult, 3)
	require.Equal(t, []string{"a", "b", "i"}, []string{
		cm.SearchResult[0].ID, cm.SearchResult[1].ID, cm.SearchResult[2].ID,
	})
	require.Equal(t, types.PassageGateOutcomeApplied, cm.PassageGateDiagnostics.Outcome)
	require.Equal(t, 30, cm.PassageGateDiagnostics.InputCount)
	require.Equal(t, 3, cm.PassageGateDiagnostics.KeptCount)
}

func TestPassageGateRespectsConfiguredFloor(t *testing.T) {
	cm := gateChatManage(30)
	cm.RerankTopK = 20
	wantLast := cm.SearchResult[29].ID
	svc := &gateModelService{model: &gateChat{reply: `{"keep":[30]}`}}
	require.True(t, runGate(t, svc, cm))
	require.Len(t, cm.SearchResult, 20)
	// The judged candidate is kept even though the floor is filled from the top.
	require.Equal(t, wantLast, cm.SearchResult[len(cm.SearchResult)-1].ID)
}

func TestPassageGateFailsOpenOnModelError(t *testing.T) {
	cm := gateChatManage(30)
	svc := &gateModelService{model: &gateChat{err: errors.New("boom")}}
	require.True(t, runGate(t, svc, cm))
	require.Len(t, cm.SearchResult, 30)
	require.Equal(t, types.PassageGateOutcomeModelError, cm.PassageGateDiagnostics.Outcome)
	require.Contains(t, cm.PassageGateDiagnostics.Error, "boom")
}

func TestPassageGateFailsOpenOnUnparsableReply(t *testing.T) {
	cm := gateChatManage(30)
	svc := &gateModelService{model: &gateChat{reply: "I think passages 1 and 4 are relevant."}}
	require.True(t, runGate(t, svc, cm))
	require.Len(t, cm.SearchResult, 30)
	require.Equal(t, types.PassageGateOutcomeModelError, cm.PassageGateDiagnostics.Outcome)
}

func TestPassageGateKeepsPoolWhenJudgeFindsNothing(t *testing.T) {
	cm := gateChatManage(30)
	svc := &gateModelService{model: &gateChat{reply: `{"keep":[]}`}}
	require.True(t, runGate(t, svc, cm))
	require.Len(t, cm.SearchResult, 30)
	require.Equal(t, types.PassageGateOutcomeNoRelevant, cm.PassageGateDiagnostics.Outcome)
}

func TestPassageGateIgnoresOutOfRangeIndices(t *testing.T) {
	cm := gateChatManage(30)
	svc := &gateModelService{model: &gateChat{reply: `{"keep":[0,31,999,4]}`}}
	require.True(t, runGate(t, svc, cm))
	// Only position 4 survives the range check; the floor (3) tops up from 1.
	require.Len(t, cm.SearchResult, 3)
	require.Equal(t, "d", cm.SearchResult[2].ID)
}

func TestPassageGateNoModelSkips(t *testing.T) {
	cm := gateChatManage(30)
	cm.ChatModelID = ""
	cm.QueryUnderstandModelID = ""
	svc := &gateModelService{model: &gateChat{reply: `{"keep":[1]}`}}
	require.True(t, runGate(t, svc, cm))
	require.Len(t, cm.SearchResult, 30)
	require.Equal(t, types.PassageGateOutcomeNoModel, cm.PassageGateDiagnostics.Outcome)
}

func TestPassageGatePrefersQueryUnderstandModel(t *testing.T) {
	cm := gateChatManage(30)
	cm.QueryUnderstandModelID = "small"
	var gotID string
	svc := &gateModelService{model: &gateChat{reply: `{"keep":[1]}`}}
	recording := &recordingModelService{inner: svc, id: &gotID}
	plugin := &PluginPassageGate{modelService: recording}
	require.Nil(t, plugin.OnEvent(context.Background(), types.CHUNK_RERANK, cm, func() *PluginError {
		return nil
	}))
	require.Equal(t, "small", gotID)
}

func TestParseGateKeepAcceptsStringIndices(t *testing.T) {
	keep, err := parseGateKeep(`{"keep":["2","4"]}`, 10)
	require.NoError(t, err)
	require.Len(t, keep, 2)
	_, ok := keep[2]
	require.True(t, ok)
	_, ok = keep[4]
	require.True(t, ok)
}

type recordingModelService struct {
	interfaces.ModelService
	inner *gateModelService
	id    *string
}

func (r *recordingModelService) GetChatModel(ctx context.Context, id string) (chat.Chat, error) {
	*r.id = id
	return r.inner.GetChatModel(ctx, id)
}
