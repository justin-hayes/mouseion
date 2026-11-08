package cardexport

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
)

// idiomLookup is the optional lexical-provider capability that exposes the
// dictionary's word-to-multi-word lookup. A provider without it, including a
// nil provider, leaves representative-sentence ranking unchanged.
type idiomLookup interface {
	IdiomEntries(context.Context, string, string) ([]idiom.Entry, error)
}

// idiomCapableProvider narrows a lexical provider to the optional idiom
// capability. A provider that does not implement it is treated as having no
// idiom entries.
func idiomCapableProvider(lexical enrichment.LexicalProvider) idiomLookup {
	lookup, ok := lexical.(idiomLookup)
	if !ok {
		return nil
	}
	return lookup
}

// idiomSentencePreferenceFor returns the leading ranking rule for one candidate
// when its language's idiom-card switch is on. It loads the dictionary entries
// for every observed form once, then marks a sentence preferred when its target
// occurrence is not part of an idiom candidate. It returns a preference that
// never leads when the switch is off, the language or provider cannot supply
// entries, or the candidate has no observed form that could begin an idiom.
func idiomSentencePreferenceFor(ctx context.Context, lexical idiomLookup, enabled map[string]bool, candidate domain.SelectionCandidate) (sentencequality.SentencePreference, error) {
	language := canonicalization.NormalizeLanguage(candidate.Language)
	if lexical == nil || language == "" || !enabled[language] {
		return noIdiomPreference, nil
	}
	words := idiomCandidateWords(candidate)
	entriesByWord := make(map[string][]idiom.Entry, len(words))
	found := false
	for _, word := range words {
		entries, lookupErr := lexical.IdiomEntries(ctx, language, word)
		if lookupErr != nil {
			return nil, fmt.Errorf("%w: idiom lookup %q: %w", ErrInvalidInput, word, lookupErr)
		}
		if len(entries) == 0 {
			continue
		}
		entriesByWord[word] = append(entriesByWord[word], entries...)
		found = true
	}
	if !found {
		return noIdiomPreference, nil
	}
	return func(evidence sentencequality.SentenceEvidence) bool {
		return targetNotInIdiomCandidate(language, candidate, entriesByWord, evidence)
	}, nil
}

// noIdiomPreference is the inert preference used when the switch is off or no
// idiom entry can apply. Every accepted sentence keeps the existing order.
func noIdiomPreference(sentencequality.SentenceEvidence) bool {
	return false
}

// idiomCandidateWords returns the candidate's observed forms plus its canonical
// lemma, deduplicated. A malformed observed-form list yields no preference
// instead of failing the deck; the sentence selector already treats the
// candidate as unrankable.
func idiomCandidateWords(candidate domain.SelectionCandidate) []string {
	var forms []string
	if len(candidate.ObservedForms) > 0 {
		if err := json.Unmarshal(candidate.ObservedForms, &forms); err != nil {
			return nil
		}
	}
	forms = append(forms, candidate.CanonicalLemma)
	seen := make(map[string]bool, len(forms))
	words := make([]string, 0, len(forms))
	for _, form := range forms {
		form = strings.TrimSpace(form)
		if form == "" || seen[form] {
			continue
		}
		seen[form] = true
		words = append(words, form)
	}
	return words
}

// targetNotInIdiomCandidate reports whether the sentence's anchored target
// token is outside every accepted idiom candidate. A target that cannot be
// anchored does not claim an idiom-free rank rather than guessing.
func targetNotInIdiomCandidate(language string, candidate domain.SelectionCandidate, entriesByWord map[string][]idiom.Entry, evidence sentencequality.SentenceEvidence) bool {
	if !evidence.Quality.Accepted || evidence.Target == "" || len(evidence.Tokens) == 0 {
		return false
	}
	targetIndex, ok := sentencequality.TargetTokenIndex(evidence.Sentence, evidence.Target, evidence.Tokens, evidence.SentenceStartOffset)
	if !ok {
		return false
	}
	entries := idiomEntriesForTarget(candidate, entriesByWord, evidence.Target)
	if len(entries) == 0 {
		return true
	}
	tokens := make([]idiom.Token, len(evidence.Tokens))
	for index, token := range evidence.Tokens {
		tokens[index] = idiom.Token{Ordinal: index, Surface: token.Surface, CanonicalLemma: token.CanonicalLemma}
	}
	anchors := []idiom.Anchor{{
		TokenOrdinal: targetIndex,
		Identity: idiom.AnchorIdentity{
			Language:             language,
			CanonicalLemma:       candidate.CanonicalLemma,
			UPOS:                 strings.ToUpper(strings.TrimSpace(candidate.UPOS)),
			LexicalDiscriminator: strings.TrimSpace(evidence.Tokens[targetIndex].LexicalDiscriminator),
		},
	}}
	for _, matched := range idiom.Match(idiom.DefaultParameters(), tokens, anchors, entries) {
		if slices.Contains(matched.MatchedTokenOrdinals, targetIndex) {
			return false
		}
	}
	return true
}

// idiomEntriesForTarget combines the entries loaded for the sentence's target
// form with those loaded for the candidate's canonical lemma, so a surface form
// and its lemma reach the same entries.
func idiomEntriesForTarget(candidate domain.SelectionCandidate, entriesByWord map[string][]idiom.Entry, target string) []idiom.Entry {
	entries := append([]idiom.Entry(nil), entriesByWord[target]...)
	if candidate.CanonicalLemma != target {
		entries = append(entries, entriesByWord[candidate.CanonicalLemma]...)
	}
	return entries
}
