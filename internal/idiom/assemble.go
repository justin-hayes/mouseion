package idiom

import (
	"context"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

// Index is the dictionary capability idiom assembly reads. The local
// dictionary index implements it.
type Index interface {
	IdiomEntries(ctx context.Context, language, word string) ([]Entry, error)
}

// TokensOf converts analyzed sentence tokens to matcher tokens. Each token's
// Ordinal is its position in the sentence.
func TokensOf(tokens []analyzer.Token) []Token {
	result := make([]Token, len(tokens))
	for index, token := range tokens {
		result[index] = Token{Ordinal: index, Surface: token.Surface, CanonicalLemma: token.CanonicalLemma}
	}
	return result
}

// SentenceEntries looks up the idiom entries of every token's lemma and
// surface form once per distinct word.
func SentenceEntries(ctx context.Context, index Index, language string, tokens []analyzer.Token) ([]Entry, error) {
	seen := make(map[string]struct{}, len(tokens))
	entries := make([]Entry, 0)
	for _, token := range tokens {
		for _, word := range []string{token.CanonicalLemma, token.Surface} {
			word = strings.TrimSpace(word)
			if word == "" {
				continue
			}
			if _, ok := seen[word]; ok {
				continue
			}
			seen[word] = struct{}{}
			found, err := index.IdiomEntries(ctx, language, word)
			if err != nil {
				return nil, err
			}
			entries = append(entries, found...)
		}
	}
	return entries, nil
}

// CandidatesInSentence assembles a sentence's idiom candidates with no
// anchor: it looks up the entries of its tokens and matches them with
// MatchUnanchored. The tokens' CanonicalLemma should be the effective lemma.
func CandidatesInSentence(ctx context.Context, index Index, parameters Parameters, language string, tokens []analyzer.Token) ([]Candidate, error) {
	matcherTokens := TokensOf(tokens)
	if len(matcherTokens) == 0 {
		return nil, nil
	}
	entries, err := SentenceEntries(ctx, index, language, tokens)
	if err != nil {
		return nil, err
	}
	return MatchUnanchored(parameters, matcherTokens, entries), nil
}

// CachedIndex remembers each word's idiom entries. Common words such as
// Italian "di" belong to well over a thousand idioms and recur in nearly
// every sentence of a Book, so an uncached caller would re-read and re-decode
// those entries for every sentence. It is not safe for concurrent use.
type CachedIndex struct {
	index   Index
	entries map[string][]Entry
}

// NewCachedIndex wraps index with a per-word cache.
func NewCachedIndex(index Index) *CachedIndex {
	return &CachedIndex{index: index, entries: make(map[string][]Entry)}
}

// IdiomEntries returns the word's entries, reading the index once per word.
func (c *CachedIndex) IdiomEntries(ctx context.Context, language, word string) ([]Entry, error) {
	key := language + "\x00" + word
	if entries, ok := c.entries[key]; ok {
		return entries, nil
	}
	entries, err := c.index.IdiomEntries(ctx, language, word)
	if err != nil {
		return nil, err
	}
	c.entries[key] = entries
	return entries, nil
}
