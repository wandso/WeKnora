package types

// PassageGateOutcome describes what the pre-rerank passage gate did.
// Every non-"applied" value means the candidate pool reached rerank untouched.
type PassageGateOutcome string

const (
	// PassageGateOutcomeApplied means the judge kept a subset and rerank
	// received the smaller pool.
	PassageGateOutcomeApplied PassageGateOutcome = "applied"
	// PassageGateOutcomeDisabled means the request did not enable the gate.
	PassageGateOutcomeDisabled PassageGateOutcome = "disabled"
	// PassageGateOutcomeSmallPool means the pool was too small to be worth a
	// judge call.
	PassageGateOutcomeSmallPool PassageGateOutcome = "small_pool"
	// PassageGateOutcomeNoModel means no judge model was resolved.
	PassageGateOutcomeNoModel PassageGateOutcome = "no_model"
	// PassageGateOutcomeModelUnavailable means the configured model could not
	// be loaded.
	PassageGateOutcomeModelUnavailable PassageGateOutcome = "model_unavailable"
	// PassageGateOutcomeModelError means the judge call failed, timed out or
	// returned something unparsable.
	PassageGateOutcomeModelError PassageGateOutcome = "model_error"
	// PassageGateOutcomeNoRelevant means the judge kept nothing; the original
	// pool is returned so rerank keeps ownership of the relevance decision.
	PassageGateOutcomeNoRelevant PassageGateOutcome = "no_relevant"
)

// PassageGateSettings configures the pre-rerank relevance gate. Zero values
// mean "use the pipeline default"; Enabled alone does not turn the gate on
// unless a judge model can be resolved.
type PassageGateSettings struct {
	Enabled bool `json:"-"`
	// ModelID is the judge model. Empty falls back to QueryUnderstandModelID,
	// then ChatModelID, so a deployment can point the gate at a small model.
	ModelID string `json:"-"`
	// MinCandidates is the pool size below which the gate is skipped.
	MinCandidates int `json:"-"`
	// MaxCandidates caps how many candidates go into one judge request.
	MaxCandidates int `json:"-"`
	// Keep is the floor: rerank never receives fewer candidates than this.
	Keep int `json:"-"`
	// TimeoutMS bounds the judge call. A judge slower than this turns the gate
	// into a no-op, so it has to be tunable per deployment.
	TimeoutMS int `json:"-"`
}

// PassageGateDiagnostics reports what the gate did, so a skipped or degraded
// gate is observable instead of silent.
type PassageGateDiagnostics struct {
	Enabled    bool               `json:"enabled"`
	Outcome    PassageGateOutcome `json:"outcome"`
	ModelID    string             `json:"model_id,omitempty"`
	InputCount int                `json:"input_count"`
	// SentCount is how many candidates were shown to the judge.
	SentCount int `json:"sent_count"`
	KeptCount int `json:"kept_count"`
	// DurationMs is the judge call latency.
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}
