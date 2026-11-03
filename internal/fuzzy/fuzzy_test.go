package fuzzy

import (
	"slices"
	"testing"
)

func TestMatchPositions(t *testing.T) {
	tests := []struct {
		query         string
		candidate     string
		wantOK        bool
		wantPositions []int
	}{
		{"readme", "docs/README.md", true, []int{5, 6, 7, 8, 9, 10}},
		{"gd", "docs/guide.md", true, []int{5, 8}},
		{"dg", "docs/guide.md", true, []int{0, 5}},
		{"xyz", "docs/guide.md", false, nil},
		{"Guide", "docs/guide.md", false, nil},
		{"Guide", "docs/Guide.md", true, []int{5, 6, 7, 8, 9}},
		{"notes jan", "notes/2026/jan.md", true, []int{0, 1, 2, 3, 4, 11, 12, 13}},
		{"notes zzz", "notes/2026/jan.md", false, nil},
		{"é", "café.md", true, []int{3}},
	}
	for _, tc := range tests {
		match, ok := ParseQuery(tc.query).Match(tc.candidate)
		if ok != tc.wantOK {
			t.Errorf("%q vs %q: ok=%v, want %v", tc.query, tc.candidate, ok, tc.wantOK)
			continue
		}
		if ok && !slices.Equal(match.Positions, tc.wantPositions) {
			t.Errorf("%q vs %q: positions %v, want %v", tc.query, tc.candidate, match.Positions, tc.wantPositions)
		}
	}
}

func TestShortestWindowIsPreferred(t *testing.T) {
	// The greedy first "a" is at 0, but the backward pass tightens the window
	// to the "ab" run.
	match, ok := ParseQuery("ab").Match("a-x-ab.md")
	if !ok || !slices.Equal(match.Positions, []int{4, 5}) {
		t.Fatalf("positions %v, ok %v", match.Positions, ok)
	}
}

func TestRanking(t *testing.T) {
	tests := []struct {
		query  string
		better string
		worse  string
	}{
		{"guide", "docs/guide.md", "go/ui/de/notes.md"},
		{"readme", "README.md", "docs/deep/README-old/notes.md"},
		{"api", "docs/api.md", "docs/rapid-iteration.md"},
		{"sg", "setup-guide.md", "settings.md"},
		{"sg", "SetupGuide.md", "setupguide.md"},
		{"jan", "notes/2026/jan.md", "jan/notes/2026/feb.md"},
	}
	for _, tc := range tests {
		query := ParseQuery(tc.query)
		betterMatch, okBetter := query.Match(tc.better)
		worseMatch, okWorse := query.Match(tc.worse)
		if !okBetter || !okWorse {
			t.Errorf("%q: expected both %q and %q to match", tc.query, tc.better, tc.worse)
			continue
		}
		if betterMatch.Score <= worseMatch.Score {
			t.Errorf("%q: %q scored %d, not above %q at %d", tc.query, tc.better, betterMatch.Score, tc.worse, worseMatch.Score)
		}
	}
}

func TestEmptyQuery(t *testing.T) {
	if !ParseQuery("   ").IsEmpty() {
		t.Error("whitespace query is not empty")
	}
}
