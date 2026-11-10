package service

import (
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

// passageGateFromConversation reads the deployment-level gate settings used by
// the chat pipeline.
func passageGateFromConversation(c *config.ConversationConfig) types.PassageGateSettings {
	if c == nil {
		return types.PassageGateSettings{}
	}
	return types.PassageGateSettings{
		Enabled:       c.PassageGateEnabled,
		ModelID:       c.PassageGateModelID,
		MinCandidates: c.PassageGateMinCandidates,
		Keep:          c.PassageGateKeep,
		TimeoutMS:     c.PassageGateTimeoutMS,
	}
}

// passageGateFromRetrieval reads the tenant-level gate settings used by the
// knowledge-search API. That path runs without a chat model, so an empty
// PassageGateModelID leaves the gate skipped rather than failing the search.
func passageGateFromRetrieval(rc *types.RetrievalConfig) types.PassageGateSettings {
	if rc == nil {
		return types.PassageGateSettings{}
	}
	return types.PassageGateSettings{
		Enabled:       rc.PassageGateEnabled,
		ModelID:       rc.PassageGateModelID,
		MinCandidates: rc.PassageGateMinCandidates,
		Keep:          rc.PassageGateKeep,
		TimeoutMS:     rc.PassageGateTimeoutMS,
	}
}
