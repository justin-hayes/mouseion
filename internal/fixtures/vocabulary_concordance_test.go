package fixtures

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureConcordanceServesHausAndMultiBookOccurrences(t *testing.T) {
	store := NewStore()

	haus, err := store.ListVocabularyConcordance(t.Context(), OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "Haus"})
	require.NoError(t, err)
	require.NotEmpty(t, haus.Occurrences)
	for _, occurrence := range haus.Occurrences {
		assert.Equal(t, "haus", occurrence.CanonicalLemma)
	}

	books, err := store.ListVocabularyConcordance(t.Context(), OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "fixture-books"})
	require.NoError(t, err)
	assert.NotEmpty(t, books.Occurrences)
}
