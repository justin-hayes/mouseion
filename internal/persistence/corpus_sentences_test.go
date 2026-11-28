package persistence

import (
	"testing"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorpusSentenceFromRowDecodesPersistedLocations pins the offset and
// morphology decoding that keeps a persisted token's span available to the
// card-export anchor.
func TestCorpusSentenceFromRowDecodesPersistedLocations(t *testing.T) {
	result := map[int64]analyzer.Sentence{}

	withoutToken, err := corpusSentenceFromRow(result, sqlcgen.ListCorpusSentencesRow{
		SentenceOrdinal: 4, SentenceText: "Das Haus steht.", StartOffset: 10, TokenOrdinal: -1,
	})
	require.NoError(t, err)
	assert.Equal(t, "Das Haus steht.", withoutToken.Text)
	assert.Equal(t, uint64(10), withoutToken.Location.StartOffset)
	assert.Empty(t, withoutToken.Tokens)

	result[4] = withoutToken
	withToken, err := corpusSentenceFromRow(result, sqlcgen.ListCorpusSentencesRow{
		SentenceOrdinal: 4, SentenceText: "Das Haus steht.", StartOffset: 10, TokenOrdinal: 1,
		Surface: "Haus", RawLemma: "haus", CanonicalLemma: "haus", Upos: "NOUN",
		Dependency: "nsubj", Head: 2, Morphology: []byte(`{"Case":"Nom"}`),
		TokenStartOffset: 14, TokenEndOffset: 18,
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(10), withToken.Location.StartOffset, "sentence start offset is not re-read from a token row")
	require.Len(t, withToken.Tokens, 1)
	assert.Equal(t, "Haus", withToken.Tokens[0].Surface)
	assert.Equal(t, uint64(14), withToken.Tokens[0].Location.StartOffset)
	assert.Equal(t, uint64(18), withToken.Tokens[0].Location.EndOffset)
	assert.Equal(t, map[string]string{"Case": "Nom"}, withToken.Tokens[0].Morphology)
}

// TestCorpusSentenceFromRowRejectsInvalidRows pins the defensive decoders for a
// negative persisted offset or an unreadable morphology payload.
func TestCorpusSentenceFromRowRejectsInvalidRows(t *testing.T) {
	for _, test := range []struct {
		name string
		row  sqlcgen.ListCorpusSentencesRow
	}{
		{name: "negative sentence start", row: sqlcgen.ListCorpusSentencesRow{SentenceOrdinal: 1, StartOffset: -1, TokenOrdinal: -1}},
		{name: "negative head", row: sqlcgen.ListCorpusSentencesRow{SentenceOrdinal: 1, TokenOrdinal: 0, Head: -1}},
		{name: "negative token start", row: sqlcgen.ListCorpusSentencesRow{SentenceOrdinal: 1, TokenOrdinal: 0, Head: 0, TokenStartOffset: -1}},
		{name: "negative token end", row: sqlcgen.ListCorpusSentencesRow{SentenceOrdinal: 1, TokenOrdinal: 0, Head: 0, TokenEndOffset: -1}},
		{name: "malformed morphology", row: sqlcgen.ListCorpusSentencesRow{SentenceOrdinal: 1, TokenOrdinal: 0, Head: 0, Morphology: []byte(`{`)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := corpusSentenceFromRow(map[int64]analyzer.Sentence{}, test.row)
			require.Error(t, err)
		})
	}
}
