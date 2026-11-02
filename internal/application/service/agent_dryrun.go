package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// SessionKindDryRun marks the throwaway conversation a dry run writes to. The
// session is deleted when the run finishes, so the kind exists only so that a
// crash mid-run leaves an identifiable row instead of an ordinary-looking
// conversation, and so usage aggregates can exclude it.
const SessionKindDryRun = "dry_run"

// dryRunTimeout bounds a trial run. The editor is a synchronous request, and a
// model that never answers must not hold the HTTP connection open forever.
const dryRunTimeout = 3 * time.Minute

// ErrDryRunQueryRequired is returned when the caller sends no question.
var ErrDryRunQueryRequired = errors.New("query is required")

// agentDryRunService executes an agent against an unsaved draft configuration.
//
// It deliberately reuses the production QA path (`SessionService.AgentQA`)
// rather than building an engine by hand: a trial run that behaves differently
// from the real thing is worse than no trial run at all. The price is that the
// run needs a session and two message rows; both are removed afterwards.
type agentDryRunService struct {
	agents   interfaces.CustomAgentService
	sessions interfaces.SessionService
	messages interfaces.MessageService
}

// NewAgentDryRunService wires the dry-run executor.
func NewAgentDryRunService(
	agents interfaces.CustomAgentService,
	sessions interfaces.SessionService,
	messages interfaces.MessageService,
) interfaces.AgentDryRunService {
	return &agentDryRunService{agents: agents, sessions: sessions, messages: messages}
}

func (s *agentDryRunService) Run(
	ctx context.Context, agentID string, draft types.CustomAgentConfig, query string,
) (*types.AgentDryRunResult, error) {
	if len([]rune(query)) == 0 {
		return nil, ErrDryRunQueryRequired
	}
	agent, err := s.agents.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	// The draft replaces the stored config wholesale. Anything the editor did
	// not send therefore falls back to the struct's defaults, which is exactly
	// what saving the same payload would produce.
	trial := *agent
	trial.Config = draft
	trial.EnsureDefaults()

	tenantID := types.MustTenantIDFromContext(ctx)
	userID, _ := types.UserIDFromContext(ctx)
	now := time.Now()
	session := &types.Session{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		UserID:    userID,
		Kind:      SessionKindDryRun,
		Title:     "dry-run",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := s.sessions.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("create dry-run session: %w", err)
	}
	// Best-effort cleanup: the run's rows must not show up in the user's
	// conversation list or in usage totals.
	defer func() {
		cleanupCtx := context.WithoutCancel(ctx)
		if err := s.sessions.DeleteSession(cleanupCtx, session.ID); err != nil {
			logger.Warnf(cleanupCtx, "Delete dry-run session %s: %v", session.ID, err)
		}
	}()

	userMsg, err := s.messages.CreateMessage(ctx, &types.Message{
		SessionID: session.ID, Role: "user", Content: query,
		IsCompleted: true, CreatedAt: now, Channel: types.ChannelWeb,
	})
	if err != nil {
		return nil, fmt.Errorf("create dry-run user message: %w", err)
	}
	answer, err := s.messages.CreateMessage(ctx, &types.Message{
		SessionID: session.ID, Role: "assistant", CreatedAt: now, Channel: types.ChannelWeb,
	})
	if err != nil {
		return nil, fmt.Errorf("create dry-run assistant message: %w", err)
	}

	bus := event.NewEventBus()
	var (
		answerText string
		usage      *types.TokenUsage
		steps      []types.AgentStep
		durationMS int64
		runErr     error
		completed  bool
	)
	bus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		if !ok {
			return nil
		}
		completed = true
		answerText = data.FinalAnswer
		durationMS = data.TotalDurationMs
		if u, ok := data.Usage.(*types.TokenUsage); ok {
			usage = u
		}
		if s, ok := data.AgentSteps.([]types.AgentStep); ok {
			steps = s
		}
		return nil
	})
	bus.On(event.EventError, func(_ context.Context, evt event.Event) error {
		if data, ok := evt.Data.(event.ErrorData); ok && data.Error != "" {
			runErr = errors.New(data.Error)
		}
		return nil
	})

	// Unattended: a dry run has no surface to answer an approval or an
	// ask_user prompt, so the run must not suspend waiting for one.
	req := &types.QARequest{
		Session:            session,
		Query:              query,
		UserMessageID:      userMsg.ID,
		AssistantMessageID: answer.ID,
		CustomAgent:        &trial,
		WebSearchEnabled:   trial.Config.WebSearchEnabled,
	}

	runCtx, cancel := context.WithTimeout(ctx, dryRunTimeout)
	defer cancel()

	started := time.Now()
	callErr := func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("agent panicked: %v", p)
			}
		}()
		return s.sessions.AgentQA(runCtx, req, bus)
	}()

	if callErr != nil {
		return nil, callErr
	}
	if runErr != nil {
		return nil, runErr
	}
	if !completed && answerText == "" {
		return nil, errors.New("the run produced no answer")
	}
	if durationMS == 0 {
		durationMS = time.Since(started).Milliseconds()
	}
	return &types.AgentDryRunResult{
		Answer:     answerText,
		Usage:      usage,
		Steps:      steps,
		DurationMS: durationMS,
	}, nil
}
