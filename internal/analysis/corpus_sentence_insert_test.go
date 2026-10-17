package analysis

import (
	"math"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorpusSentenceInsertEncodesCitations(t *testing.T) {
	sentences := []analyzer.Sentence{
		{Text: "one", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 0, EndOffset: 3}, Citations: []string{"1.2.17", "1.2.18"}},
		{Text: "two", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 4, EndOffset: 7}},
	}
	query, args, err := corpusSentenceInsert(sentences, 0, 2, "owner", "run", "corpus")
	require.NoError(t, err)
	assert.Contains(t, query, "citations")
	require.Len(t, args, 18)
	assert.Equal(t, []string{"1.2.17", "1.2.18"}, args[8])
	assert.Equal(t, []string{}, args[17], "an absent citation list is stored empty, never NULL")
}

func TestCorpusSentenceInsertRejectsOverflowingOffsets(t *testing.T) {
	_, _, err := corpusSentenceInsert([]analyzer.Sentence{{Location: analyzer.SourceLocation{StartOffset: math.MaxUint64}}}, 0, 1, "owner", "run", "corpus")
	require.ErrorContains(t, err, "start offset overflows bigint")
	_, _, err = corpusSentenceInsert([]analyzer.Sentence{{Location: analyzer.SourceLocation{EndOffset: math.MaxUint64}}}, 0, 1, "owner", "run", "corpus")
	require.ErrorContains(t, err, "end offset overflows bigint")
}
