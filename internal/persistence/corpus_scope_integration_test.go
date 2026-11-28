//go:build integration

package persistence

import (
	"context"
	"testing"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcordanceSearchesTheCorpus holds that Concordance's library scope is
// the Corpus, the analysed Books still in My Books (ADR 0136 §2): a Book
// removed from My Books is no longer searched, and a Read Book still is. The
// Fluency worker draws its candidate contexts from this lookup, so it never
// draws a new context from a Book outside the Corpus.
func TestConcordanceSearchesTheCorpus(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "concordance-corpus", false)
	require.NoError(t, err)
	books := seedScaleLibrary(t, ctx, store, owner.ID, scaleLibrary{books: 2, units: 1, sentencesPerUnit: 10})
	require.NoError(t, store.MarkBookPreviouslyRead(ctx, owner.ID, books[0]))

	searched := func() map[string]bool {
		found := map[string]bool{}
		for page := 1; ; page++ {
			result, lookupErr := store.ListVocabularyConcordance(ctx, owner.ID, "de", domain.ConcordanceLookup{Term: "der", Match: domain.ConcordanceMatchLemma, Page: page})
			require.NoError(t, lookupErr)
			for _, occurrence := range result.Occurrences {
				found[occurrence.BookID] = true
			}
			if !result.HasNext {
				return found
			}
			require.Less(t, page, 50)
		}
	}
	assert.Equal(t, map[string]bool{books[0]: true, books[1]: true}, searched(), "a Read Book is in the Corpus")

	require.NoError(t, store.RemoveBookFromMyBooks(ctx, owner.ID, books[0]))
	assert.Equal(t, map[string]bool{books[1]: true}, searched(), "a removed Book is not searched")

	evidenced, err := sqlcgen.New(store.Pool()).ConcordanceLemmaEvidenced(ctx, sqlcgen.ConcordanceLemmaEvidencedParams{Owner: owner.ID, Language: "de", Term: "der"})
	require.NoError(t, err)
	assert.True(t, evidenced, "the remaining Book still evidences the lemma")

	require.NoError(t, store.RemoveBookFromMyBooks(ctx, owner.ID, books[1]))
	assert.Empty(t, searched(), "a Corpus with no Book has no occurrences")
	evidenced, err = sqlcgen.New(store.Pool()).ConcordanceLemmaEvidenced(ctx, sqlcgen.ConcordanceLemmaEvidencedParams{Owner: owner.ID, Language: "de", Term: "der"})
	require.NoError(t, err)
	assert.False(t, evidenced, "a lemma only a removed Book holds is not evidenced")
}
