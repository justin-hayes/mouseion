//go:build integration

package persistence

import (
	"context"
	"strconv"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFinishMatchesSharedFinishCases holds the Postgres store to the same
// outcomes as the fixture store for the same facts.
func TestFinishMatchesSharedFinishCases(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	for index, tc := range testutil.FinishCases() {
		t.Run(tc.Name, func(t *testing.T) {
			suffix := "finish-parity-" + strconv.Itoa(index)
			owner, err := store.CreateUser(ctx, suffix, false)
			require.NoError(t, err)
			book, source, _ := createReadingFixture(t, ctx, store, owner.ID, suffix)
			makeAnalyzedToReadBook(t, ctx, store, book, source)
			var corpusID string
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&corpusID))
			for _, identity := range tc.Snapshot {
				_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
					OwnerID: owner.ID, CorpusID: corpusID, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS,
					OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
				})
				require.NoError(t, err)
			}
			reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
			require.NoError(t, err)
			for _, identity := range tc.Known {
				_, err = store.PutKnownVocabulary(ctx, owner.ID, identity.Language, identity.CanonicalLemma, identity.UPOS)
				require.NoError(t, err)
			}
			other, err := store.CreateUser(ctx, suffix+"-other", false)
			require.NoError(t, err)
			for _, identity := range tc.OtherLearnerKnown {
				_, err = store.PutKnownVocabulary(ctx, other.ID, identity.Language, identity.CanonicalLemma, identity.UPOS)
				require.NoError(t, err)
			}
			expectedSnapshot := reading.SnapshotID
			if tc.Legacy {
				_, err = store.Pool().Exec(ctx, `UPDATE primary_goals SET snapshot_id=NULL WHERE owner_id=$1 AND language='de'`, owner.ID)
				require.NoError(t, err)
				expectedSnapshot = ""
			}

			_, err = store.FinishCurrentReading(ctx, owner.ID, "de", "00000000-0000-0000-0000-000000000000", expectedSnapshot)
			require.ErrorIs(t, err, ErrCurrentReadingStale)

			result, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, expectedSnapshot)
			require.NoError(t, err)
			assert.Equal(t, tc.SnapshotCount, result.Completion.SnapshotVocabularyCount)
			assert.Equal(t, tc.EligibleCount, result.Completion.EligibleVocabularyCount)
			assert.Equal(t, tc.GraduatedCount, result.Completion.GraduatedVocabularyCount)
			assert.Equal(t, tc.AlreadyKnownCount, result.Completion.AlreadyKnownVocabularyCount)
			known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
			require.NoError(t, err)
			assert.Equal(t, tc.KnownAfter, testutil.KnownKeys(known))

			replay, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, expectedSnapshot)
			require.NoError(t, err)
			assert.Equal(t, result, replay)
		})
	}
}
