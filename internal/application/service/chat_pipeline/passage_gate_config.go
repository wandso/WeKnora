package chatpipeline

import (
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Passage gate defaults. They are chosen so that a deployment which only flips
// PassageGate.Enabled still behaves sensibly:
//   - a pool under 20 is not worth a judge request;
//   - 60 candidates per request matches what batched judges are trained for and
//     keeps one request's prompt bounded;
//   - 10 is rerank's usual top-k, so the floor never starves it;
//   - 3s bounds the gate's share of the first-token path.
const (
	passageGateDefaultMinCandidates = 20
	passageGateDefaultMaxCandidates = 60
	passageGateDefaultKeep          = 10
	passageGateDefaultPreviewChars  = 400
	passageGateDefaultTimeout       = 3 * time.Second
	// passageGateMinKeepAbs is an absolute floor: even when rerank's top-k is
	// configured to 1, a judge that keeps a single candidate should not leave
	// rerank with nothing to compare.
	passageGateMinKeepAbs = 3
)

// passageGateConfig is the resolved, defaulted form of
// types.PassageGateSettings for one turn.
type passageGateConfig struct {
	Enabled       bool
	ModelID       string
	MinCandidates int
	MaxCandidates int
	Keep          int
	PreviewChars  int
	Timeout       time.Duration
}

// resolvePassageGate applies defaults and picks the judge model. The gate
// prefers a purpose-set model, then the query-understanding model (the small
// one a deployment already routes rewrites to), then the main chat model.
func resolvePassageGate(chatManage *types.ChatManage) passageGateConfig {
	s := chatManage.PassageGate
	cfg := passageGateConfig{
		Enabled:       s.Enabled,
		ModelID:       s.ModelID,
		MinCandidates: s.MinCandidates,
		MaxCandidates: s.MaxCandidates,
		Keep:          s.Keep,
		PreviewChars:  passageGateDefaultPreviewChars,
		Timeout:       passageGateDefaultTimeout,
	}
	if cfg.MinCandidates <= 0 {
		cfg.MinCandidates = passageGateDefaultMinCandidates
	}
	if cfg.MaxCandidates <= 0 {
		cfg.MaxCandidates = passageGateDefaultMaxCandidates
	}
	if cfg.MaxCandidates < cfg.MinCandidates {
		cfg.MaxCandidates = cfg.MinCandidates
	}
	if cfg.ModelID == "" {
		cfg.ModelID = chatManage.QueryUnderstandModelID
	}
	if cfg.ModelID == "" {
		cfg.ModelID = chatManage.ChatModelID
	}
	if s.TimeoutMS > 0 {
		cfg.Timeout = time.Duration(s.TimeoutMS) * time.Millisecond
	}
	return cfg
}

// floor is the minimum pool size rerank may receive. A configured Keep wins,
// but rerank's own top-k is always met, and an absolute minimum stops a
// one-candidate judge from leaving rerank nothing to compare.
func (c passageGateConfig) floor(chatManage *types.ChatManage) int {
	keep := c.Keep
	if keep <= 0 {
		keep = chatManage.RerankTopK
	}
	if keep <= 0 {
		keep = passageGateDefaultKeep
	}
	if topK := chatManage.RerankTopK; topK > keep {
		keep = topK
	}
	if keep < passageGateMinKeepAbs {
		keep = passageGateMinKeepAbs
	}
	return keep
}

// skipReason reports why the gate should not run, if it should not.
func (c passageGateConfig) skipReason(pool int) (types.PassageGateOutcome, bool) {
	switch {
	case !c.Enabled:
		return types.PassageGateOutcomeDisabled, true
	case c.ModelID == "":
		return types.PassageGateOutcomeNoModel, true
	case pool == 0:
		return types.PassageGateOutcomeSmallPool, true
	case pool <= c.MinCandidates:
		return types.PassageGateOutcomeSmallPool, true
	}
	return "", false
}
