package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"

	"github.com/google/uuid"
)

// Content moderation, observation phase.
//
// This service never changes what a user sees. It scans stored conversation
// turns against a rule set and appends matches to an audit log, so a workspace
// can look at real traffic and decide which rules are worth enforcing. Blocking
// is deliberately not implemented: a false positive that truncates a paying
// user's answer is worse than a missed detection while the rules are still
// being tuned.

var (
	// ErrModerationRuleNameRequired rejects a rule with no name: the hit log
	// stores the name, so an unnamed rule produces unreadable audit rows.
	ErrModerationRuleNameRequired = errors.New("moderation rule name is required")
	// ErrModerationRulePatternInvalid rejects an unusable pattern at write time
	// rather than silently skipping it on every scan.
	ErrModerationRulePatternInvalid = errors.New("moderation rule pattern is invalid")
	// ErrModerationRuleNotFound is returned when the rule does not belong to
	// the caller's tenant.
	ErrModerationRuleNotFound = errors.New("moderation rule not found")
)

const (
	moderationDefaultScanLimit = 200
	moderationMaxScanLimit     = 500
	moderationSnippetRadius    = 24
	moderationSnippetMaxRunes  = 80
)

// builtinPIIRules returns the structural detectors that ship with the product.
//
// These are patterns, not vocabulary: a phone number is a phone number in every
// language and jurisdiction, so the list does not age and does not encode a
// policy decision about what words are acceptable. They are kept in code rather
// than seeded into moderation_rules because a tenant must not be able to edit
// them away and then believe the workspace is covered.
func builtinPIIRules() []*types.ModerationRule {
	specs := []struct {
		id      string
		name    string
		pattern string
	}{
		{"builtin-pii-phone-cn", "中国大陆手机号", `\b1[3-9]\d{9}\b`},
		{"builtin-pii-idcard-cn", "中国大陆身份证号", `\b\d{17}[\dXx]\b`},
		{"builtin-pii-bankcard", "银行卡号", `\b\d{16,19}\b`},
		{"builtin-pii-email", "电子邮箱", `[\w.+-]+@[\w-]+\.[\w.]{2,}`},
	}
	rules := make([]*types.ModerationRule, 0, len(specs))
	for _, spec := range specs {
		rules = append(rules, &types.ModerationRule{
			ID:       spec.id,
			Name:     spec.name,
			Category: types.ModerationCategoryPII,
			Pattern:  spec.pattern,
			IsRegex:  true,
			Action:   types.ModerationActionRecord,
			Enabled:  true,
		})
	}
	return rules
}

type moderationService struct {
	rules interfaces.ModerationRuleRepository
	hits  interfaces.ModerationHitRepository
}

// NewModerationService builds the scanner.
func NewModerationService(
	rules interfaces.ModerationRuleRepository, hits interfaces.ModerationHitRepository,
) interfaces.ModerationService {
	return &moderationService{rules: rules, hits: hits}
}

func (s *moderationService) ListRules(
	ctx context.Context, tenantID uint64,
) ([]*types.ModerationRule, error) {
	return s.rules.List(ctx, tenantID)
}

func (s *moderationService) UpsertRule(
	ctx context.Context, tenantID uint64, rule *types.ModerationRule,
) (*types.ModerationRule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Pattern = strings.TrimSpace(rule.Pattern)
	if rule.Name == "" {
		return nil, ErrModerationRuleNameRequired
	}
	if rule.Pattern == "" {
		return nil, ErrModerationRulePatternInvalid
	}
	if rule.IsRegex {
		// Compile now: a bad pattern must fail here, not silently skip on every
		// scan for the rest of the deployment's life.
		if _, err := compileModerationPattern(rule.Pattern); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrModerationRulePatternInvalid, err)
		}
	}
	if rule.Category == "" {
		rule.Category = types.ModerationCategoryCustom
	}
	if rule.Action == "" {
		rule.Action = types.ModerationActionRecord
	}
	rule.TenantID = tenantID

	if rule.ID == "" {
		rule.ID = uuid.New().String()
		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()
		return rule, s.rules.Create(ctx, rule)
	}
	rule.UpdatedAt = time.Now()
	if err := s.rules.Update(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *moderationService) DeleteRule(ctx context.Context, tenantID uint64, id string) error {
	return s.rules.Delete(ctx, tenantID, id)
}

func (s *moderationService) ListHits(
	ctx context.Context, tenantID uint64, agentID string, limit, offset int,
) ([]*types.ModerationHit, int64, error) {
	limit, offset = normalizePage(limit, offset)
	return s.hits.List(ctx, tenantID, agentID, limit, offset)
}

// ScanAgent inspects the agent's most recent turns and records matches.
//
// It is intentionally read-mostly: the only write is the hit log. Re-running a
// scan is cheap and idempotent because the hit table deduplicates on
// (tenant, message, rule).
func (s *moderationService) ScanAgent(
	ctx context.Context, tenantID uint64, agentID string, limit int,
) (*types.ModerationScanResult, error) {
	if limit <= 0 {
		limit = moderationDefaultScanLimit
	}
	if limit > moderationMaxScanLimit {
		limit = moderationMaxScanLimit
	}

	active, err := s.activeRules(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := &types.ModerationScanResult{Rules: len(active)}
	if len(active) == 0 {
		return result, nil
	}

	messages, err := s.hits.ListMessages(ctx, tenantID, agentID, limit)
	if err != nil {
		return nil, err
	}
	result.Scanned = len(messages)

	hits := make([]*types.ModerationHit, 0, 8)
	for _, msg := range messages {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		direction := types.ModerationDirectionOutput
		if msg.Role == "user" {
			direction = types.ModerationDirectionInput
		}
		for _, rule := range active {
			idx, length, ok := matchModerationRule(rule, msg.Content)
			if !ok {
				continue
			}
			hits = append(hits, &types.ModerationHit{
				ID:        uuid.New().String(),
				TenantID:  tenantID,
				AgentID:   agentID,
				SessionID: msg.SessionID,
				MessageID: msg.ID,
				Direction: direction,
				RuleID:    rule.ID,
				RuleName:  rule.Name,
				Category:  rule.Category,
				Snippet:   moderationSnippet(msg.Content, idx, length),
				Action:    rule.Action,
				CreatedAt: time.Now(),
			})
		}
	}

	inserted, err := s.hits.InsertIgnore(ctx, hits)
	if err != nil {
		return nil, err
	}
	result.Matched = inserted
	return result, nil
}

// activeRules merges the built-in PII detectors with the tenant's enabled rules.
func (s *moderationService) activeRules(
	ctx context.Context, tenantID uint64,
) ([]*types.ModerationRule, error) {
	tenantRules, err := s.rules.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := builtinPIIRules()
	for _, rule := range tenantRules {
		if rule.Enabled {
			out = append(out, rule)
		}
	}
	return out, nil
}

// compileModerationPattern builds a case-insensitive regexp.
func compileModerationPattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("(?i)" + pattern)
}

// matchModerationRule returns the byte offset and length of the first match.
func matchModerationRule(rule *types.ModerationRule, content string) (int, int, bool) {
	if rule.IsRegex {
		re, err := compileModerationPattern(rule.Pattern)
		if err != nil {
			return 0, 0, false
		}
		loc := re.FindStringIndex(content)
		if loc == nil {
			return 0, 0, false
		}
		return loc[0], loc[1] - loc[0], true
	}
	idx := strings.Index(strings.ToLower(content), strings.ToLower(rule.Pattern))
	if idx < 0 {
		return 0, 0, false
	}
	return idx, len(rule.Pattern), true
}

// moderationSnippet returns a short window around the match so a reviewer can
// judge the hit without opening the conversation. The window is deliberately
// small: the audit log must not become a second copy of the corpus.
func moderationSnippet(content string, idx, length int) string {
	start := idx - moderationSnippetRadius
	if start < 0 {
		start = 0
	}
	end := idx + length + moderationSnippetRadius
	if end > len(content) {
		end = len(content)
	}
	// Byte offsets can land inside a multi-byte rune; nudge outward until the
	// slice is valid UTF-8.
	for start > 0 && !isRuneBoundary(content, start) {
		start--
	}
	for end < len(content) && !isRuneBoundary(content, end) {
		end++
	}
	snippet := strings.Join(strings.Fields(content[start:end]), " ")
	runes := []rune(snippet)
	if len(runes) > moderationSnippetMaxRunes {
		snippet = string(runes[:moderationSnippetMaxRunes]) + "…"
	}
	return snippet
}

// isRuneBoundary reports whether i starts a new UTF-8 rune.
func isRuneBoundary(s string, i int) bool {
	if i <= 0 || i >= len(s) {
		return true
	}
	return s[i]&0xC0 != 0x80
}
