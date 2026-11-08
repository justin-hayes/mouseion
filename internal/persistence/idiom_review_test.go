package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/require"
)

func TestMatchedIdiomWordsResolvesFrozenTokens(t *testing.T) {
	payload := idiomSampleRenderPayload{
		SentenceTokens: []analyzer.Token{
			{Surface: "È"}, {Surface: "un"}, {Surface: "ragazzo"}, {Surface: "in"}, {Surface: "gamba."},
		},
		IdiomMatchedTokenOrdinals: []int{3, 4},
	}
	words, err := matchedIdiomWords(payload)
	require.NoError(t, err)
	require.Equal(t, []string{"in", "gamba."}, words)
}

func TestMatchedIdiomWordsRejectsIncompleteEvidence(t *testing.T) {
	for name, payload := range map[string]idiomSampleRenderPayload{
		"no ordinals":      {SentenceTokens: []analyzer.Token{{Surface: "in"}}},
		"ordinal too high": {SentenceTokens: []analyzer.Token{{Surface: "in"}}, IdiomMatchedTokenOrdinals: []int{1}},
		"negative ordinal": {SentenceTokens: []analyzer.Token{{Surface: "in"}}, IdiomMatchedTokenOrdinals: []int{-1}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := matchedIdiomWords(payload)
			require.Error(t, err)
		})
	}
}
