package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Errors surfaced by the operations centre. Handlers map these to status
// codes; anything else is an internal failure.
var (
	ErrFeedbackInvalidRating   = errors.New("rating must be 'up' or 'down'")
	ErrFeedbackMessageNotFound = errors.New("message not found")
	ErrFeedbackNotAssistant    = errors.New("feedback is only allowed on assistant messages")
	ErrVersionNotFound         = errors.New("agent version not found")
	ErrTemplateNotFound        = errors.New("agent template not found")
	ErrPublicPageNotFound      = errors.New("public page not found")
	ErrPublicPageInvalidSlug   = errors.New("slug must be 2-64 lowercase letters, digits or hyphens")
	ErrPublicPageSlugTaken     = errors.New("slug is already in use")
	ErrPublicPagePassword      = errors.New("password required")
	// ErrPublicPagePasswordRequired is a configuration mistake (a password
	// page with no password), not an authentication failure.
	ErrPublicPagePasswordRequired = errors.New("a password-protected page needs a password")
	ErrPublicPageWrongPassword    = errors.New("incorrect password")
)

// maxFeedbackCommentRunes bounds the free-text comment. The column is TEXT, so
// without a cap a client could store megabytes per vote.
const maxFeedbackCommentRunes = 2000

// ---------------------------------------------------------------------------
// Versions
// ---------------------------------------------------------------------------

type agentVersionService struct {
	repo   interfaces.AgentVersionRepository
	agents interfaces.CustomAgentService
}

// NewAgentVersionService wires version snapshots to the agent service so a
// rollback goes through the same validation as a normal edit.
func NewAgentVersionService(
	repo interfaces.AgentVersionRepository, agents interfaces.CustomAgentService,
) interfaces.AgentVersionService {
	return &agentVersionService{repo: repo, agents: agents}
}

func (s *agentVersionService) Snapshot(
	ctx context.Context, agent *types.CustomAgent, source, changelog, userID string,
) (*types.AgentVersion, error) {
	if agent == nil {
		return nil, errors.New("agent is required")
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	configJSON, err := json.Marshal(agent.Config)
	if err != nil {
		return nil, fmt.Errorf("marshal agent config: %w", err)
	}
	no, err := s.repo.NextVersionNo(ctx, tenantID, agent.ID)
	if err != nil {
		return nil, err
	}
	if source == "" {
		source = types.AgentVersionSourceUpdate
	}
	if len([]rune(changelog)) > 512 {
		changelog = string([]rune(changelog)[:512])
	}
	v := &types.AgentVersion{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		AgentID:     agent.ID,
		VersionNo:   no,
		Name:        agent.Name,
		Description: agent.Description,
		Avatar:      agent.Avatar,
		Config:      types.JSON(configJSON),
		Changelog:   changelog,
		CreatedBy:   userID,
		Source:      source,
		CreatedAt:   time.Now(),
	}
	if err := s.repo.Create(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *agentVersionService) List(
	ctx context.Context, agentID string, limit, offset int,
) ([]*types.AgentVersion, int64, error) {
	limit, offset = normalizePage(limit, offset)
	return s.repo.List(ctx, types.MustTenantIDFromContext(ctx), agentID, limit, offset)
}

func (s *agentVersionService) Get(
	ctx context.Context, agentID string, versionNo int,
) (*types.AgentVersion, error) {
	v, err := s.repo.Get(ctx, types.MustTenantIDFromContext(ctx), agentID, versionNo)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrVersionNotFound
	}
	return v, nil
}

func (s *agentVersionService) Diff(
	ctx context.Context, agentID string, from, to int,
) (*types.AgentVersionDiff, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	a, err := s.repo.Get(ctx, tenantID, agentID, from)
	if err != nil {
		return nil, err
	}
	b, err := s.repo.Get(ctx, tenantID, agentID, to)
	if err != nil {
		return nil, err
	}
	if a == nil || b == nil {
		return nil, ErrVersionNotFound
	}

	var ca, cb map[string]any
	if err := json.Unmarshal(a.Config, &ca); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b.Config, &cb); err != nil {
		return nil, err
	}

	changed := make([]string, 0, 8)
	seen := map[string]struct{}{}
	for k, av := range ca {
		seen[k] = struct{}{}
		if bv, ok := cb[k]; !ok || !reflect.DeepEqual(av, bv) {
			changed = append(changed, k)
		}
	}
	for k := range cb {
		if _, ok := seen[k]; !ok {
			changed = append(changed, k)
		}
	}

	return &types.AgentVersionDiff{
		From:               from,
		To:                 to,
		ChangedKeys:        changed,
		NameChanged:        a.Name != b.Name,
		DescriptionChanged: a.Description != b.Description,
		AvatarChanged:      a.Avatar != b.Avatar,
	}, nil
}

func (s *agentVersionService) Rollback(
	ctx context.Context, agentID string, versionNo int, userID string,
) (*types.CustomAgent, error) {
	v, err := s.Get(ctx, agentID, versionNo)
	if err != nil {
		return nil, err
	}
	var cfg types.CustomAgentConfig
	if err := json.Unmarshal(v.Config, &cfg); err != nil {
		return nil, fmt.Errorf("decode snapshot config: %w", err)
	}
	restored := &types.CustomAgent{
		ID:          agentID,
		Name:        v.Name,
		Description: v.Description,
		Config:      cfg,
	}
	restored.EnsureDefaults()
	avatar := v.Avatar
	updated, err := s.agents.UpdateAgent(ctx, restored, &avatar)
	if err != nil {
		return nil, err
	}
	// History stays append-only: the restore is recorded as a new version
	// whose source says why it exists.
	if _, err := s.Snapshot(ctx, updated,
		types.AgentVersionSourceRollback,
		fmt.Sprintf("restored from v%d", versionNo), userID); err != nil {
		// The agent is already restored; a failed bookkeeping write must not
		// turn a successful rollback into an error the caller retries.
		return updated, nil
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Feedback
// ---------------------------------------------------------------------------

type messageFeedbackService struct {
	repo     interfaces.MessageFeedbackRepository
	messages interfaces.MessageService
}

// NewMessageFeedbackService validates votes against the stored message so a
// client cannot attach feedback to an arbitrary id or claim an agent.
func NewMessageFeedbackService(
	repo interfaces.MessageFeedbackRepository, messages interfaces.MessageService,
) interfaces.MessageFeedbackService {
	return &messageFeedbackService{repo: repo, messages: messages}
}

func (s *messageFeedbackService) Submit(
	ctx context.Context, tenantID uint64, userID, messageID, sessionID, rating string,
	reasons []string, comment string,
) (*types.MessageFeedback, error) {
	if !types.IsValidFeedbackRating(rating) {
		return nil, ErrFeedbackInvalidRating
	}
	if strings.TrimSpace(messageID) == "" || strings.TrimSpace(sessionID) == "" {
		return nil, ErrFeedbackMessageNotFound
	}

	msg, err := s.messages.GetMessage(ctx, sessionID, messageID)
	if err != nil || msg == nil {
		return nil, ErrFeedbackMessageNotFound
	}
	if msg.Role != "assistant" {
		return nil, ErrFeedbackNotAssistant
	}

	clean := make([]string, 0, len(reasons))
	for _, r := range reasons {
		if types.IsValidFeedbackReason(r) {
			clean = append(clean, r)
		}
	}
	tagsJSON, err := json.Marshal(clean)
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(comment) > maxFeedbackCommentRunes {
		comment = string([]rune(comment)[:maxFeedbackCommentRunes])
	}

	// An upvote carries no reason tags: keeping stale "wrong_answer" tags on
	// a corrected vote would poison the reason histogram.
	if rating == types.FeedbackRatingUp {
		tagsJSON = []byte("[]")
		comment = ""
	}

	f := &types.MessageFeedback{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		MessageID:  messageID,
		UserID:     userID,
		SessionID:  sessionID,
		AgentID:    msg.AgentID,
		Rating:     rating,
		ReasonTags: types.JSON(tagsJSON),
		Comment:    comment,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := s.repo.Upsert(ctx, f); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, tenantID, messageID, userID)
}

func (s *messageFeedbackService) GetMine(
	ctx context.Context, tenantID uint64, userID, messageID string,
) (*types.MessageFeedback, error) {
	return s.repo.Get(ctx, tenantID, messageID, userID)
}

func (s *messageFeedbackService) List(
	ctx context.Context, tenantID uint64, agentID, rating string, limit, offset int,
) ([]*types.MessageFeedback, int64, error) {
	if rating != "" && !types.IsValidFeedbackRating(rating) {
		return nil, 0, ErrFeedbackInvalidRating
	}
	limit, offset = normalizePage(limit, offset)
	return s.repo.List(ctx, tenantID, agentID, rating, limit, offset)
}

func (s *messageFeedbackService) Summary(
	ctx context.Context, tenantID uint64, agentID string, since time.Time,
) (types.FeedbackSummary, error) {
	return s.repo.Summary(ctx, tenantID, agentID, since)
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

type agentUsageService struct {
	usage    interfaces.AgentUsageRepository
	feedback interfaces.MessageFeedbackRepository
}

// NewAgentUsageService reports token usage and joins it with satisfaction so
// the overview answers "cheap but bad" and "good but expensive" alike.
func NewAgentUsageService(
	usage interfaces.AgentUsageRepository, feedback interfaces.MessageFeedbackRepository,
) interfaces.AgentUsageService {
	return &agentUsageService{usage: usage, feedback: feedback}
}

func (s *agentUsageService) Summary(
	ctx context.Context, tenantID uint64, agentID string, since time.Time,
) (types.AgentUsageSummary, error) {
	return s.usage.Summary(ctx, tenantID, agentID, since)
}

func (s *agentUsageService) Timeseries(
	ctx context.Context, tenantID uint64, agentID string, since time.Time,
) ([]types.AgentUsagePoint, error) {
	return s.usage.Timeseries(ctx, tenantID, agentID, since)
}

func (s *agentUsageService) Report(
	ctx context.Context, tenantID uint64, agentID string, since time.Time,
) (*types.AgentQualityReport, error) {
	usage, err := s.usage.Summary(ctx, tenantID, agentID, since)
	if err != nil {
		return nil, err
	}
	feedback, err := s.feedback.Summary(ctx, tenantID, agentID, since)
	if err != nil {
		return nil, err
	}
	return &types.AgentQualityReport{
		AgentID:  agentID,
		Since:    since,
		Usage:    usage,
		Feedback: feedback,
	}, nil
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

type agentTemplateService struct {
	repo   interfaces.AgentTemplateRepository
	agents interfaces.CustomAgentService
}

// NewAgentTemplateService installs templates through the normal create path so
// validation, defaulting and audit behave exactly like a hand-made agent.
func NewAgentTemplateService(
	repo interfaces.AgentTemplateRepository, agents interfaces.CustomAgentService,
) interfaces.AgentTemplateService {
	return &agentTemplateService{repo: repo, agents: agents}
}

func (s *agentTemplateService) List(
	ctx context.Context, tenantID uint64, category string,
) ([]*types.AgentTemplate, error) {
	return s.repo.List(ctx, tenantID, category)
}

func (s *agentTemplateService) Install(
	ctx context.Context, tenantID uint64, userID, templateID string,
) (*types.CustomAgent, error) {
	t, err := s.repo.Get(ctx, templateID, tenantID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTemplateNotFound
	}
	var cfg types.CustomAgentConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		return nil, fmt.Errorf("decode template config: %w", err)
	}
	name := strings.TrimSpace(t.AgentName)
	if name == "" {
		name = t.Name
	}
	agent := &types.CustomAgent{
		Name:        name,
		Description: t.AgentDescription,
		Avatar:      t.AgentAvatar,
		Config:      cfg,
	}
	agent.EnsureDefaults()
	created, err := s.agents.CreateAgent(ctx, agent)
	if err != nil {
		return nil, err
	}
	_ = s.repo.IncrementInstalls(ctx, t.ID)
	return created, nil
}

func (s *agentTemplateService) SaveFromAgent(
	ctx context.Context, tenantID uint64, userID, agentID, name, category, icon, description string,
) (*types.AgentTemplate, error) {
	agent, err := s.agents.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, ErrTemplateNotFound
	}
	cfgJSON, err := json.Marshal(agent.Config)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		name = agent.Name
	}
	if strings.TrimSpace(category) == "" {
		category = "general"
	}
	t := &types.AgentTemplate{
		ID:               uuid.NewString(),
		TenantID:         tenantID,
		Name:             name,
		Category:         category,
		Icon:             icon,
		Description:      description,
		Config:           types.JSON(cfgJSON),
		AgentName:        agent.Name,
		AgentDescription: agent.Description,
		AgentAvatar:      agent.Avatar,
		Source:           types.AgentTemplateSourceTenant,
		CreatedBy:        userID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *agentTemplateService) Delete(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.Delete(ctx, id, tenantID)
}

// SeedBuiltins upserts the product-shipped templates.
//
// It runs on every boot, so a template's prompt can be improved in a release
// and existing deployments pick the change up. Rows are matched by ID, which is
// why those IDs are constants rather than generated values.
func (s *agentTemplateService) SeedBuiltins(ctx context.Context) (int, error) {
	templates, err := types.BuiltinAgentTemplates()
	if err != nil {
		return 0, err
	}
	seeded := 0
	for _, t := range templates {
		if err := s.repo.UpsertBuiltin(ctx, t); err != nil {
			return seeded, fmt.Errorf("seed builtin template %q: %w", t.ID, err)
		}
		seeded++
	}
	return seeded, nil
}

// ---------------------------------------------------------------------------
// Public pages
// ---------------------------------------------------------------------------

type agentPublicPageService struct {
	repo   interfaces.AgentPublicPageRepository
	agents interfaces.CustomAgentService
}

// NewAgentPublicPageService publishes an agent at a shareable slug. The page
// is metadata only; anonymous chat still runs through the embed channel.
func NewAgentPublicPageService(
	repo interfaces.AgentPublicPageRepository, agents interfaces.CustomAgentService,
) interfaces.AgentPublicPageService {
	return &agentPublicPageService{repo: repo, agents: agents}
}

// slugPattern keeps public URLs copy-paste safe: lowercase alphanumerics and
// single hyphens, 2-63 characters, and it must start and end with an
// alphanumeric. A trailing hyphen ("my-agent-") is technically a valid URL but
// looks like a typo in a shared link, so it is rejected here.
var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])$`)

func (s *agentPublicPageService) Upsert(
	ctx context.Context, page *types.AgentPublicPage, password string,
) (*types.AgentPublicPage, error) {
	page.Slug = strings.ToLower(strings.TrimSpace(page.Slug))
	if !slugPattern.MatchString(page.Slug) {
		return nil, ErrPublicPageInvalidSlug
	}
	if page.AccessMode == "" {
		page.AccessMode = types.PublicPageAccessOpen
	}
	if page.AccessMode != types.PublicPageAccessOpen && page.AccessMode != types.PublicPageAccessPassword {
		return nil, errors.New("access_mode must be 'open' or 'password'")
	}

	// Slug is globally unique: two tenants cannot share one public URL. Check
	// before writing so the caller gets a 409 instead of a driver error.
	if existing, err := s.repo.GetBySlug(ctx, page.Slug); err != nil {
		return nil, err
	} else if existing != nil && existing.AgentID != page.AgentID {
		return nil, ErrPublicPageSlugTaken
	}

	if page.AccessMode == types.PublicPageAccessPassword {
		switch {
		case password != "":
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return nil, err
			}
			page.PasswordHash = string(hash)
		case page.PasswordHash == "":
			// Creating a password-protected page without a password would make
			// it unopenable, which is worse than rejecting the request.
			return nil, ErrPublicPagePasswordRequired
		}
	} else {
		page.PasswordHash = ""
	}

	if page.ID == "" {
		page.ID = uuid.NewString()
	}
	now := time.Now()
	page.CreatedAt, page.UpdatedAt = now, now
	if err := s.repo.Upsert(ctx, page); err != nil {
		return nil, err
	}
	return s.repo.GetByAgent(ctx, page.TenantID, page.AgentID)
}

func (s *agentPublicPageService) GetByAgent(
	ctx context.Context, tenantID uint64, agentID string,
) (*types.AgentPublicPage, error) {
	return s.repo.GetByAgent(ctx, tenantID, agentID)
}

func (s *agentPublicPageService) Delete(ctx context.Context, tenantID uint64, agentID string) error {
	return s.repo.Delete(ctx, tenantID, agentID)
}

func (s *agentPublicPageService) ResolvePublic(
	ctx context.Context, slug, password string,
) (*types.AgentPublicPage, *types.CustomAgent, error) {
	page, err := s.repo.GetBySlug(ctx, strings.ToLower(strings.TrimSpace(slug)))
	if err != nil {
		return nil, nil, err
	}
	if page == nil || !page.Enabled {
		return nil, nil, ErrPublicPageNotFound
	}
	if page.AccessMode == types.PublicPageAccessPassword {
		if password == "" {
			return nil, nil, ErrPublicPagePassword
		}
		if bcrypt.CompareHashAndPassword([]byte(page.PasswordHash), []byte(password)) != nil {
			return nil, nil, ErrPublicPageWrongPassword
		}
	}
	// Anonymous callers have no tenant in context, so the agent is resolved by
	// the page's own tenant rather than the request's.
	agent, err := s.agents.GetAgentByIDAndTenant(ctx, page.AgentID, page.TenantID)
	if err != nil {
		return nil, nil, err
	}
	return page, agent, nil
}

// normalizePage clamps pagination so a caller cannot ask for the whole table.
func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
