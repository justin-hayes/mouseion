//go:build integration

package webapp

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/require"
)

// publishProjectedCounts marks a seeded Book's Browse count projection ready
// with exactly the supplied effective counts, as data, the way a finished
// durable rebuild would leave it. It is for Books whose corpus tokens are not
// seeded; Books with tokens use buildBrowseProjection.
func publishProjectedCounts(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, bookID string, counts []domain.LemmaOccurrence) {
	t.Helper()
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, ownerID, bookID).Scan(&runID, &corpusID))
	_, err := store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version) VALUES($1,$2,'de',$3,$4,3)
		ON CONFLICT(owner_id,book_id) DO UPDATE SET analysis_run_id=excluded.analysis_run_id,corpus_id=excluded.corpus_id,builder_version=3`, ownerID, bookID, runID, corpusID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2`, ownerID, bookID)
	require.NoError(t, err)
	for _, count := range counts {
		_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			ownerID, bookID, count.Language, runID, corpusID, count.CanonicalLemma, count.UPOS, count.OccurrenceCount)
		require.NoError(t, err)
	}
}
