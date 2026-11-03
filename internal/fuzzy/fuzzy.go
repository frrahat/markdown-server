// Package fuzzy ranks file paths against a typed query, in the spirit of
// fzf: the query's characters must appear in order, and matches at word
// boundaries, in consecutive runs, and inside the file name rank higher.
package fuzzy

import (
	"slices"
	"strings"
	"unicode"
)

// Scoring weights. Only their relative sizes matter.
const (
	scorePerMatchedChar   = 16
	bonusConsecutive      = 8
	bonusSegmentBoundary  = 12
	bonusCamelCase        = 8
	bonusFirstCharFactor  = 2
	bonusInFileName       = 24
	penaltyGapStart       = 3
	penaltyGapPerExtraGap = 1
)

// Match is a candidate that matched every term of a query.
type Match struct {
	Score int
	// Positions are rune (not byte) offsets of the matched characters in
	// ascending order, so a UI can highlight them by code point.
	Positions []int
}

// Query is a parsed query: whitespace-separated terms that must all match.
type Query struct {
	terms []term
}

type term struct {
	runes []rune
	// caseSensitive follows smartcase: on only if the term has an uppercase
	// letter.
	caseSensitive bool
}

func ParseQuery(rawQuery string) Query {
	var query Query
	for _, field := range strings.Fields(rawQuery) {
		query.terms = append(query.terms, term{
			runes:         []rune(field),
			caseSensitive: strings.ToLower(field) != field,
		})
	}
	return query
}

func (q Query) IsEmpty() bool { return len(q.terms) == 0 }

// Match scores candidate (a slash-separated path) against every term.
// ok is false if any term fails to match.
func (q Query) Match(candidate string) (match Match, ok bool) {
	// Rejecting non-matches before the []rune conversion keeps them
	// allocation-free, which matters when filtering a large index.
	for _, queryTerm := range q.terms {
		if !containsSubsequence(candidate, queryTerm) {
			return Match{}, false
		}
	}
	candidateRunes := []rune(candidate)
	fileNameStart := 0
	for i, r := range candidateRunes {
		if r == '/' {
			fileNameStart = i + 1
		}
	}
	for _, queryTerm := range q.terms {
		termScore, positions, termOK := matchTerm(queryTerm, candidateRunes, fileNameStart)
		if !termOK {
			return Match{}, false
		}
		match.Score += termScore
		match.Positions = append(match.Positions, positions...)
	}
	// Terms can match overlapping characters; report each position once.
	slices.Sort(match.Positions)
	match.Positions = slices.Compact(match.Positions)
	return match, true
}

func containsSubsequence(candidate string, queryTerm term) bool {
	termIndex := 0
	for _, candidateRune := range candidate {
		if runesEqual(candidateRune, queryTerm.runes[termIndex], queryTerm.caseSensitive) {
			termIndex++
			if termIndex == len(queryTerm.runes) {
				return true
			}
		}
	}
	return false
}

func runesEqual(candidateRune, termRune rune, caseSensitive bool) bool {
	if caseSensitive || candidateRune == termRune {
		return candidateRune == termRune
	}
	return unicode.ToLower(candidateRune) == unicode.ToLower(termRune)
}

// matchTerm tries the file name first and falls back to the full path.
// Otherwise "gd" against "go/ui/de/guide.md" would match the "g" of "go" and
// the "d" of "de" rather than letters in "guide".
func matchTerm(queryTerm term, candidateRunes []rune, fileNameStart int) (int, []int, bool) {
	if positions, ok := findPositions(queryTerm, candidateRunes, fileNameStart); ok {
		return scorePositions(candidateRunes, positions) + bonusInFileName, positions, true
	}
	if positions, ok := findPositions(queryTerm, candidateRunes, 0); ok {
		return scorePositions(candidateRunes, positions), positions, true
	}
	return 0, nil, false
}

// findPositions locates the term as a subsequence of candidateRunes starting
// at searchFrom. It scans forward to find where the earliest match ends, then
// backward from there to find the latest possible start, which yields the
// shortest window ending at that point; positions are then taken greedily
// within the window. This follows fzf's v1 algorithm. It runs in linear time
// but doesn't always find the best-scoring positions.
func findPositions(queryTerm term, candidateRunes []rune, searchFrom int) ([]int, bool) {
	termRunes := queryTerm.runes
	equal := func(candidateRune, termRune rune) bool {
		return runesEqual(candidateRune, termRune, queryTerm.caseSensitive)
	}

	termIndex := 0
	windowEnd := -1
	for i := searchFrom; i < len(candidateRunes); i++ {
		if equal(candidateRunes[i], termRunes[termIndex]) {
			termIndex++
			if termIndex == len(termRunes) {
				windowEnd = i
				break
			}
		}
	}
	if windowEnd < 0 {
		return nil, false
	}

	termIndex = len(termRunes) - 1
	windowStart := windowEnd
	for i := windowEnd; i >= searchFrom; i-- {
		if equal(candidateRunes[i], termRunes[termIndex]) {
			termIndex--
			if termIndex < 0 {
				windowStart = i
				break
			}
		}
	}

	positions := make([]int, 0, len(termRunes))
	termIndex = 0
	for i := windowStart; i <= windowEnd && termIndex < len(termRunes); i++ {
		if equal(candidateRunes[i], termRunes[termIndex]) {
			positions = append(positions, i)
			termIndex++
		}
	}
	return positions, true
}

func scorePositions(candidateRunes []rune, positions []int) int {
	score := 0
	for i, position := range positions {
		charScore := scorePerMatchedChar
		boundary := boundaryBonus(candidateRunes, position)
		if i == 0 {
			boundary *= bonusFirstCharFactor
		}
		charScore += boundary
		if i > 0 {
			gap := position - positions[i-1] - 1
			if gap == 0 {
				charScore += bonusConsecutive
			} else {
				charScore -= penaltyGapStart + (gap-1)*penaltyGapPerExtraGap
			}
		}
		score += charScore
	}
	return score
}

func boundaryBonus(candidateRunes []rune, position int) int {
	if position == 0 {
		return bonusSegmentBoundary
	}
	previous, current := candidateRunes[position-1], candidateRunes[position]
	switch {
	case strings.ContainsRune("/-_. ", previous):
		return bonusSegmentBoundary
	case unicode.IsLower(previous) && unicode.IsUpper(current):
		return bonusCamelCase
	}
	return 0
}
