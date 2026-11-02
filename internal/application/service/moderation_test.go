package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/types"
)

// The moderation scanner is record-only, so its correctness lives in two pure
// decisions: does a rule match, and does the stored snippet stay readable. Both
// are tested here without a database.

func TestMatchModerationRuleKeywordIsCaseInsensitive(t *testing.T) {
	rule := &types.ModerationRule{Pattern: "Acme", IsRegex: false}
	idx, length, ok := matchModerationRule(rule, "we compared with ACME last quarter")
	if !ok {
		t.Fatal("expected a keyword match")
	}
	if got := "we compared with ACME last quarter"[idx : idx+length]; got != "ACME" {
		t.Fatalf("match returned %q, want %q", got, "ACME")
	}
}

func TestMatchModerationRuleKeywordMisses(t *testing.T) {
	rule := &types.ModerationRule{Pattern: "acme", IsRegex: false}
	if _, _, ok := matchModerationRule(rule, "nothing relevant here"); ok {
		t.Fatal("expected no match")
	}
}

func TestMatchModerationRuleRegex(t *testing.T) {
	rule := &types.ModerationRule{Pattern: `\b1[3-9]\d{9}\b`, IsRegex: true}
	idx, length, ok := matchModerationRule(rule, "call 13812345678 now")
	if !ok {
		t.Fatal("expected a regex match")
	}
	if got := "call 13812345678 now"[idx : idx+length]; got != "13812345678" {
		t.Fatalf("match returned %q, want the phone number", got)
	}
}

// A malformed pattern must not panic the scanner: it is validated on write, but
// a row could predate that check or be written directly to the database.
func TestMatchModerationRuleInvalidRegexIsSkipped(t *testing.T) {
	rule := &types.ModerationRule{Pattern: `([a-z`, IsRegex: true}
	if _, _, ok := matchModerationRule(rule, "anything"); ok {
		t.Fatal("an uncompilable pattern must not match")
	}
}

func TestModerationSnippetStaysValidUTF8(t *testing.T) {
	// Chinese content around the match: byte-offset slicing would cut a rune in
	// half and produce mojibake in the audit log.
	content := strings.Repeat("这是一段很长的中文内容", 10) + "手机号13812345678结束" + strings.Repeat("后续内容", 10)
	rule := &types.ModerationRule{Pattern: `\b1[3-9]\d{9}\b`, IsRegex: true}

	idx, length, ok := matchModerationRule(rule, content)
	if !ok {
		t.Fatal("expected a match")
	}
	snippet := moderationSnippet(content, idx, length)
	if !utf8.ValidString(snippet) {
		t.Fatalf("snippet is not valid UTF-8: %q", snippet)
	}
	if !strings.Contains(snippet, "13812345678") {
		t.Fatalf("snippet dropped the match: %q", snippet)
	}
	// Collapsed whitespace and a bounded window: the log must not become a copy
	// of the conversation.
	if n := len([]rune(snippet)); n > moderationSnippetMaxRunes+1 {
		t.Fatalf("snippet too long: %d runes", n)
	}
	if strings.Contains(snippet, "\n") {
		t.Fatal("snippet should collapse newlines")
	}
}

func TestModerationSnippetAtStartOfContent(t *testing.T) {
	content := "13812345678 是号码"
	idx, length, ok := matchModerationRule(
		&types.ModerationRule{Pattern: `\b1[3-9]\d{9}\b`, IsRegex: true}, content,
	)
	if !ok {
		t.Fatal("expected a match")
	}
	snippet := moderationSnippet(content, idx, length)
	if !strings.HasPrefix(snippet, "13812345678") {
		t.Fatalf("snippet should start at the match when there is no leading context: %q", snippet)
	}
}

func TestBuiltinPIIRulesCoverStructuralPatterns(t *testing.T) {
	rules := builtinPIIRules()
	if len(rules) == 0 {
		t.Fatal("expected builtin rules")
	}
	samples := map[string]string{
		"builtin-pii-phone-cn":  "联系电话 13812345678",
		"builtin-pii-idcard-cn": "身份证 110101199003074512",
		"builtin-pii-bankcard":  "卡号 6222021234567890123",
		"builtin-pii-email":     "发到 alice@example.com",
	}
	for _, rule := range rules {
		if rule.Category != types.ModerationCategoryPII {
			t.Fatalf("%s: expected pii category, got %q", rule.ID, rule.Category)
		}
		// Record-only: a builtin must never be able to block on its own.
		if rule.Action != types.ModerationActionRecord {
			t.Fatalf("%s: builtin rules must default to record, got %q", rule.ID, rule.Action)
		}
		sample, ok := samples[rule.ID]
		if !ok {
			t.Fatalf("%s: no sample text in this test", rule.ID)
		}
		if _, _, matched := matchModerationRule(rule, sample); !matched {
			t.Fatalf("%s: did not match its own sample %q", rule.ID, sample)
		}
	}
}

// A plain order number must not be reported as a bank card: over-reporting PII
// during the observation phase would make the whole log untrustworthy.
func TestBuiltinBankCardRuleIgnoresShortNumbers(t *testing.T) {
	var bank *types.ModerationRule
	for _, rule := range builtinPIIRules() {
		if rule.ID == "builtin-pii-bankcard" {
			bank = rule
		}
	}
	if bank == nil {
		t.Fatal("bank card rule missing")
	}
	if _, _, ok := matchModerationRule(bank, "订单号 20241030001"); ok {
		t.Fatal("an 11-digit order number must not match the bank card rule")
	}
}
