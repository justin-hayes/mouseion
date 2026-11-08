// Package idiom finds dictionary-evidenced idioms in an analyzed sentence.
package idiom

import (
	"slices"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"golang.org/x/text/unicode/norm"
)

const (
	// MatcherVersion identifies the matching rules frozen into a candidate's
	// provenance. A rule change requires a new version.
	MatcherVersion = "idiom-matcher/2"
	// DefaultGapBound is the initial bounded number of other tokens allowed
	// between consecutive words of a headword.
	DefaultGapBound = 2

	minWords = 2
	maxWords = 4
)

// Parameters are the named, versioned matching rules a candidate records.
type Parameters struct {
	Version  string
	GapBound int
}

// DefaultParameters returns the reviewed matching rules.
func DefaultParameters() Parameters {
	return Parameters{Version: MatcherVersion, GapBound: DefaultGapBound}
}

// Token is one analyzed sentence token. Ordinal is its position in the
// sentence. CanonicalLemma is the analyzer's normalized lemma.
type Token struct {
	Ordinal        int
	Surface        string
	CanonicalLemma string
}

// AnchorIdentity is one frozen vocabulary identity of a deck.
type AnchorIdentity struct {
	Language             string
	CanonicalLemma       string
	UPOS                 string
	LexicalDiscriminator string
}

// Anchor marks a sentence token that is a frozen vocabulary occurrence.
type Anchor struct {
	TokenOrdinal int
	Identity     AnchorIdentity
}

// Entry is one multi-word dictionary or reviewed-list entry that could be an
// idiom. Words are the language-normalized component words used for matching;
// Headword keeps the source's display form.
type Entry struct {
	Language string
	Headword string
	UPOS     string
	Words    []string
	Senses   []enrichment.LexicalSense
	Source   string
	Version  string
}

// Provenance records why a candidate exists: the matching rules and the entry's
// dictionary or reviewed-list evidence.
type Provenance struct {
	MatcherVersion string
	GapBound       int
	Source         string
	Version        string
}

// Candidate is one idiom entry matched in a sentence and anchored on a frozen
// vocabulary occurrence. MatchedTokenOrdinals are in sentence order.
type Candidate struct {
	Entry                Entry
	MatchedTokenOrdinals []int
	Anchor               AnchorIdentity
	AnchorTokenOrdinal   int
	Provenance           Provenance
}

// Match returns the idiom candidates in tokens. Every word of an entry's
// headword must match a token by case-folded surface form or canonical lemma,
// in headword order, with at most parameters.GapBound other tokens between
// consecutive words. Entries that share a headword and source are pooled into
// one candidate carrying every part of speech's senses. A candidate without an
// anchor is dropped. Overlapping candidates resolve to the longest headword,
// then the earliest sentence position. Match is pure: it performs no I/O and
// does not mutate its inputs.
func Match(parameters Parameters, tokens []Token, anchors []Anchor, entries []Entry) []Candidate {
	if len(tokens) == 0 || len(entries) == 0 {
		return nil
	}
	if parameters.Version == "" {
		parameters = DefaultParameters()
	}
	if parameters.GapBound < 0 {
		parameters.GapBound = 0
	}
	anchorsByOrdinal := make(map[int]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorsByOrdinal[anchor.TokenOrdinal] = anchor
	}
	candidates := make([]Candidate, 0)
	for _, entry := range poolEntries(entries) {
		candidate, found := matchEntry(parameters, tokens, anchorsByOrdinal, entry)
		if found {
			candidates = append(candidates, candidate)
		}
	}
	return resolveOverlaps(candidates)
}

// poolEntries merges the entries that share a language, headword and source
// into one entry. A dictionary can list one headword under several parts of
// speech (di fronte is both an adjective and an adverb); matching them
// separately would keep only one entry's senses, so the provider could not see
// the sense the sentence uses. The pooled senses follow part-of-speech order,
// then each entry's own sense order, so the result does not depend on the
// lookup order. A repeated part of speech is dropped. UPOS lists every pooled
// part of speech, joined with "+".
func poolEntries(entries []Entry) []Entry {
	type poolKey struct{ language, headword, source string }
	groups := make(map[poolKey][]Entry, len(entries))
	order := make([]poolKey, 0, len(entries))
	for _, entry := range entries {
		key := poolKey{entry.Language, entry.Headword, entry.Source}
		group, seen := groups[key]
		if !seen {
			order = append(order, key)
		}
		if slices.ContainsFunc(group, func(member Entry) bool { return member.UPOS == entry.UPOS }) {
			continue
		}
		groups[key] = append(group, entry)
	}
	pooled := make([]Entry, 0, len(order))
	for _, key := range order {
		group := groups[key]
		sort.SliceStable(group, func(left, right int) bool { return group[left].UPOS < group[right].UPOS })
		entry := group[0]
		if len(group) > 1 {
			parts := make([]string, 0, len(group))
			var senses []enrichment.LexicalSense
			for _, member := range group {
				parts = append(parts, member.UPOS)
				senses = append(senses, member.Senses...)
			}
			entry.UPOS = strings.Join(parts, "+")
			entry.Senses = senses
		}
		pooled = append(pooled, entry)
	}
	return pooled
}

func matchEntry(parameters Parameters, tokens []Token, anchors map[int]Anchor, entry Entry) (Candidate, bool) {
	if len(entry.Words) < minWords || len(entry.Words) > maxWords {
		return Candidate{}, false
	}
	positions, found := searchWords(parameters.GapBound, tokens, anchors, entry.Words, 0, 0, -1, nil)
	if !found {
		return Candidate{}, false
	}
	anchorPosition, anchor, found := firstAnchor(tokens, anchors, positions)
	if !found {
		return Candidate{}, false
	}
	ordinals := make([]int, len(positions))
	for index, position := range positions {
		ordinals[index] = tokens[position].Ordinal
	}
	return Candidate{
		Entry:                entry,
		MatchedTokenOrdinals: ordinals,
		Anchor:               anchor.Identity,
		AnchorTokenOrdinal:   tokens[anchorPosition].Ordinal,
		Provenance: Provenance{
			MatcherVersion: parameters.Version,
			GapBound:       parameters.GapBound,
			Source:         entry.Source,
			Version:        entry.Version,
		},
	}, true
}

// searchWords finds the earliest token sequence that matches every headword
// word and contains an anchor. It backtracks over candidate positions so an
// unanchored earliest match never hides a later anchored one.
func searchWords(
	gapBound int,
	tokens []Token,
	anchors map[int]Anchor,
	words []string,
	wordIndex, start, lastPosition int,
	matched []int,
) ([]int, bool) {
	if wordIndex == len(words) {
		if _, _, found := firstAnchor(tokens, anchors, matched); found {
			return clonePositions(matched), true
		}
		return nil, false
	}
	for position := start; position < len(tokens); position++ {
		if lastPosition >= 0 && position-lastPosition-1 > gapBound {
			break
		}
		if !tokenMatches(words[wordIndex], tokens[position]) {
			continue
		}
		next := append(clonePositions(matched), position)
		if sequence, found := searchWords(gapBound, tokens, anchors, words, wordIndex+1, position+1, position, next); found {
			return sequence, true
		}
	}
	return nil, false
}

func clonePositions(positions []int) []int {
	return append([]int(nil), positions...)
}

func tokenMatches(word string, token Token) bool {
	word = norm.NFC.String(word)
	return strings.EqualFold(norm.NFC.String(token.Surface), word) ||
		strings.EqualFold(norm.NFC.String(token.CanonicalLemma), word)
}

// firstAnchor returns the first matched position that is a frozen vocabulary
// occurrence. When several words are anchored, the earliest matched one is the
// candidate's anchor.
func firstAnchor(tokens []Token, anchors map[int]Anchor, positions []int) (int, Anchor, bool) {
	for _, position := range positions {
		if anchor, ok := anchors[tokens[position].Ordinal]; ok {
			return position, anchor, true
		}
	}
	return 0, Anchor{}, false
}

func resolveOverlaps(candidates []Candidate) []Candidate {
	sort.SliceStable(candidates, func(left, right int) bool {
		leftLength, rightLength := len(candidates[left].Entry.Words), len(candidates[right].Entry.Words)
		if leftLength != rightLength {
			return leftLength > rightLength
		}
		leftStart, rightStart := firstOrdinal(candidates[left]), firstOrdinal(candidates[right])
		if leftStart != rightStart {
			return leftStart < rightStart
		}
		if candidates[left].Entry.Headword != candidates[right].Entry.Headword {
			return candidates[left].Entry.Headword < candidates[right].Entry.Headword
		}
		return candidates[left].Entry.Source < candidates[right].Entry.Source
	})
	used := make(map[int]bool)
	accepted := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if overlapsUsed(candidate, used) {
			continue
		}
		for _, ordinal := range candidate.MatchedTokenOrdinals {
			used[ordinal] = true
		}
		accepted = append(accepted, candidate)
	}
	sort.SliceStable(accepted, func(left, right int) bool {
		leftStart, rightStart := firstOrdinal(accepted[left]), firstOrdinal(accepted[right])
		if leftStart != rightStart {
			return leftStart < rightStart
		}
		return accepted[left].Entry.Headword < accepted[right].Entry.Headword
	})
	return accepted
}

func firstOrdinal(candidate Candidate) int {
	if len(candidate.MatchedTokenOrdinals) == 0 {
		return 0
	}
	return candidate.MatchedTokenOrdinals[0]
}

func overlapsUsed(candidate Candidate, used map[int]bool) bool {
	for _, ordinal := range candidate.MatchedTokenOrdinals {
		if used[ordinal] {
			return true
		}
	}
	return false
}
