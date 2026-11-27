package study

import (
	"context"
	"fmt"
	"slices"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/idiom"
)

// IdiomAvoidance lets a word's Study material and Fluency contexts prefer a
// sentence where the word is not part of an Idiom candidate (ADR 0137 §3).
// A nil value, or a language whose switch is off, leaves selection unchanged.
type IdiomAvoidance struct {
	// Index is the dictionary's idiom lookup. A nil Index finds no candidates.
	Index idiom.Index
	// Enabled reports whether a language's idiom switch is on: the operator
	// enabled it and its idiom-card payload assessment is recorded.
	Enabled func(language string) bool
	// Parameters are the matching rules; zero means idiom.DefaultParameters.
	Parameters idiom.Parameters
}

// active reports whether the language's switch is on.
func (a *IdiomAvoidance) active(language string) bool {
	if a == nil || a.Index == nil || a.Enabled == nil {
		return false
	}
	return a.Enabled(canonicalization.NormalizeLanguage(language))
}

// reader returns a per-run cached view, so the entries of a common word are
// read once for a whole search.
func (a *IdiomAvoidance) reader(language string) *idiomReader {
	if !a.active(language) {
		return nil
	}
	return &idiomReader{avoidance: a, index: idiom.NewCachedIndex(a.Index), language: canonicalization.NormalizeLanguage(language)}
}

// idiomReader answers whether a token is inside an Idiom candidate, caching
// dictionary lookups. A nil reader reports no candidate.
type idiomReader struct {
	avoidance *IdiomAvoidance
	index     *idiom.CachedIndex
	language  string
}

// inCandidate reports whether the token at ordinal lies inside any Idiom
// candidate assembled for the sentence.
func (r *idiomReader) inCandidate(ctx context.Context, tokens []analyzer.Token, ordinal int64) (bool, error) {
	if r == nil || ordinal < 0 || ordinal >= int64(len(tokens)) {
		return false, nil
	}
	candidates, err := idiom.CandidatesInSentence(ctx, r.index, r.avoidance.Parameters, r.language, tokens)
	if err != nil {
		return false, fmt.Errorf("study: idiom candidates: %w", err)
	}
	for _, candidate := range candidates {
		if slices.Contains(candidate.MatchedTokenOrdinals, int(ordinal)) {
			return true, nil
		}
	}
	return false, nil
}
