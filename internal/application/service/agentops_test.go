package service

import (
	"testing"
)

// The operations centre has a few pure decisions worth pinning down: page
// clamping (so a caller cannot dump a whole table) and slug validation (so a
// public URL cannot be crafted into something that breaks routing or SEO).
// Everything else in this package is exercised through the HTTP layer.

func TestNormalizePage(t *testing.T) {
	cases := []struct {
		limit, offset         int
		wantLimit, wantOffset int
	}{
		{0, 0, 20, 0},
		{-5, -3, 20, 0},
		{500, 10, 100, 10},
		{50, 40, 50, 40},
	}
	for _, c := range cases {
		gotLimit, gotOffset := normalizePage(c.limit, c.offset)
		if gotLimit != c.wantLimit || gotOffset != c.wantOffset {
			t.Fatalf("normalizePage(%d,%d) = (%d,%d), want (%d,%d)",
				c.limit, c.offset, gotLimit, gotOffset, c.wantLimit, c.wantOffset)
		}
	}
}

func TestSlugPattern(t *testing.T) {
	valid := []string{"support", "sales-bot", "a1", "agent-2026"}
	for _, s := range valid {
		if !slugPattern.MatchString(s) {
			t.Fatalf("expected %q to be a valid slug", s)
		}
	}
	invalid := []string{"", "A", "Sales", "has space", "-leading", "trailing-", "中文", "a"}
	for _, s := range invalid {
		if slugPattern.MatchString(s) {
			t.Fatalf("expected %q to be rejected", s)
		}
	}
}
