package cardexport

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/justin-hayes/mouseion/internal/textmatch"
	"github.com/stretchr/testify/require"
)

// frenchGoldenCorpus mirrors the committed pinned-model golden corpus the
// opt-in Python check replays against the real French pipeline
// (nlp/tests/test_french_golden.py). Reading the same file here proves the Go
// anchor implementation reaches every recorded target on the recorded persisted
// tokens, so the corpus and the product anchor cannot drift.
type frenchGoldenCorpus struct {
	Strings []struct {
		ID        string `json:"id"`
		Text      string `json:"text"`
		Sentences []struct {
			Text   string `json:"text"`
			Tokens []struct {
				Surface string    `json:"surface"`
				Span    [2]uint64 `json:"span"`
			} `json:"tokens"`
			Targets []struct {
				Form        string    `json:"form"`
				Span        [2]uint64 `json:"span"`
				WordBounded bool      `json:"word_bounded"`
			} `json:"targets"`
		} `json:"sentences"`
	} `json:"strings"`
}

// TestFrenchGoldenCorpusAnchorsEveryTarget pins the ADR 0102 acceptance target:
// all 112 eligible corpus targets anchor on their persisted token offsets, with
// byte-identical persisted sentences. Nineteen of them have no word-bounded
// match, so they exercise the fallback path.
func TestFrenchGoldenCorpusAnchorsEveryTarget(t *testing.T) {
	raw, err := os.ReadFile("../../nlp/tests/testdata/french_golden_corpus.json")
	require.NoError(t, err)
	var corpus frenchGoldenCorpus
	require.NoError(t, json.Unmarshal(raw, &corpus))
	require.Len(t, corpus.Strings, 44, "the committed corpus keeps all 44 strings")

	anchored, offsetAnchored := 0, 0
	for _, entry := range corpus.Strings {
		for _, sentence := range entry.Sentences {
			require.Equal(t, entry.Text, sentence.Text, entry.ID)
			tokens := make([]analyzer.Token, 0, len(sentence.Tokens))
			for _, token := range sentence.Tokens {
				tokens = append(tokens, analyzer.Token{
					Surface: token.Surface,
					Location: analyzer.SourceLocation{
						StartOffset: token.Span[0],
						EndOffset:   token.Span[1],
					},
				})
			}
			for _, target := range sentence.Targets {
				cleaned := textmatch.CleanLexicalSurface(target.Form)
				_, _, wordBounded := textmatch.FoldedWordSpan(sentence.Text, cleaned)
				require.Equal(t, target.WordBounded, wordBounded, "%s/%s recorded match", entry.ID, target.Form)

				start, end, ok := sentencequality.TargetSpan(sentence.Text, cleaned, tokens, 0)
				require.True(t, ok, "%s/%s must anchor", entry.ID, target.Form)
				require.EqualValues(t, target.Span[0], utf8.RuneCountInString(sentence.Text[:start]), "%s/%s start", entry.ID, target.Form)
				require.EqualValues(t, target.Span[1], utf8.RuneCountInString(sentence.Text[:end]), "%s/%s end", entry.ID, target.Form)
				require.True(t, strings.EqualFold(cleaned, textmatch.CleanLexicalSurface(sentence.Text[start:end])), "%s/%s anchored text", entry.ID, target.Form)

				anchored++
				if !target.WordBounded {
					offsetAnchored++
				}
			}
		}
	}
	require.Equal(t, 112, anchored, "112/112 eligible targets anchor")
	require.Equal(t, 19, offsetAnchored, "19 targets need the persisted-token anchor")
}
