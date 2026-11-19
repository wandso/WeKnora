// Package aclevel implements document-level clearance filtering for WeKnora.
// Productization 2026-11-18.
//
// Goal: an intern must not be able to retrieve the CEO's salary document.
//
// Design:
//   - Each knowledge document carries a clearance level in its CustomMetadata
//     under the key "clearance" (one of public/internal/confidential/secret).
//   - Each caller has a clearance derived from their TenantRole.
//   - Filtering happens at TWO layers (defense in depth):
//       1. Pre-retrieval: inject allowed knowledge_ids into RetrieveParams, so
//          high-clearance chunks are never fetched from the vector store.
//       2. Post-rerank: drop any surviving chunk whose clearance exceeds the
//          caller's level, before it is assembled into the LLM context.
//
// This file is dependency-free and unit-testable. Wire it into the pipeline as
// described in scaleup/productization/PERMISSION-ISOLATION.md.
package aclevel

import "strings"

// Level is a clearance tier. Higher value = more sensitive.
type Level int

const (
	Public Level = iota
	Internal
	Confidential
	Secret
)

var levelNames = map[Level]string{
	Public: "public", Internal: "internal", Confidential: "confidential", Secret: "secret",
}

// ParseLevel maps a metadata string to a Level. Unknown/empty values default to
// Public — we deliberately FAIL CLOSED for the caller but FAIL OPEN for data
// tagging only in the sense that unlabeled docs are treated as least-sensitive;
// see WithStrictMode to force unlabeled docs to Secret instead.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "internal":
		return Internal
	case "confidential":
		return Confidential
	case "secret", "绝密", "top-secret":
		return Secret
	default:
		return Public
	}
}

func (l Level) String() string {
	if n, ok := levelNames[l]; ok {
		return n
	}
	return "public"
}

// RoleLevel maps WeKnora's existing TenantRole to a clearance.
// viewer/intern -> Internal, contributor -> Internal, admin -> Confidential,
// owner -> Secret. Adjust to your org policy.
func RoleLevel(role string) Level {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "owner":
		return Secret
	case "admin":
		return Confidential
	case "contributor", "viewer", "intern", "":
		return Internal
	default:
		return Public
	}
}

// CanAccess reports whether a caller at callerLevel may read data at dataLevel.
func CanAccess(callerLevel, dataLevel Level) bool {
	return callerLevel >= dataLevel
}

// ExtractClearance reads the "clearance" key from a JSON-like metadata map.
// We accept map[string]interface{} because WeKnora's types.JSON unmarshals to that.
func ExtractClearance(meta map[string]interface{}) Level {
	if meta == nil {
		return Public
	}
	for k, v := range meta {
		if strings.EqualFold(k, "clearance") || strings.EqualFold(k, "密级") {
			if s, ok := v.(string); ok {
				return ParseLevel(s)
			}
		}
	}
	return Public
}

// FilterResult is a single retrieval/rerank result carrying its clearance.
type Result struct {
	ID        string
	Clearance Level
}

// FilterResults removes results the caller is not allowed to see.
// Returns (allowed, blockedIDs). blockedIDs is useful for audit logging.
func FilterResults(callerLevel Level, results []Result) ([]Result, []string) {
	allowed := make([]Result, 0, len(results))
	blocked := make([]string, 0)
	for _, r := range results {
		if CanAccess(callerLevel, r.Clearance) {
			allowed = append(allowed, r)
		} else {
			blocked = append(blocked, r.ID)
		}
	}
	return allowed, blocked
}
