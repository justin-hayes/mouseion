//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// newBrokenIdiomStore is a store whose database a test can damage.
func newBrokenIdiomStore(t *testing.T) (context.Context, *PostgresStore, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "idiom-failures-"+uuid.NewString(), false)
	require.NoError(t, err)
	return ctx, store, owner.ID
}

func TestStudyIdiomReadsReportAStorageFailure(t *testing.T) {
	cancelled := func(ctx context.Context) context.Context {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		return ctx
	}
	ref := StudyIdiomSentenceRef{BookID: uuid.NewString(), AnalysisRunID: uuid.NewString(), CorpusID: uuid.NewString(), SentenceOrdinal: 1}

	t.Run("a headword with no component words has no sentences", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		refs, err := store.ListStudyIdiomSentenceRefs(ctx, owner, "de", "mit dem haus", nil)
		require.NoError(t, err)
		require.Empty(t, refs)
	})
	t.Run("a sentence list that cannot be read fails", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		_, err := store.ListStudyIdiomSentenceRefs(cancelled(ctx), owner, "de", "mit dem haus", []string{"mit", "haus"})
		require.Error(t, err)
	})
	t.Run("sentences that cannot be loaded fail", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		_, err := store.LoadStudyIdiomSentences(cancelled(ctx), owner, []StudyIdiomSentenceRef{ref})
		require.Error(t, err)
	})
	t.Run("decisions that cannot be read fail the load", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		_, err := store.pool.Exec(ctx, `DROP TABLE occurrence_lemma_corrections CASCADE`)
		require.NoError(t, err)
		_, err = store.LoadStudyIdiomSentences(ctx, owner, []StudyIdiomSentenceRef{ref})
		require.Error(t, err)
	})
	t.Run("a sentence that is not in the Corpus is left out", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		sentences, err := store.LoadStudyIdiomSentences(ctx, owner, []StudyIdiomSentenceRef{ref})
		require.NoError(t, err)
		require.Empty(t, sentences)
	})
	t.Run("effective tokens of an unknown Book are none", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		tokens, err := store.ListStudyEffectiveTokens(ctx, owner, "not-a-uuid", ref.AnalysisRunID, ref.CorpusID, []int64{1})
		require.NoError(t, err)
		require.Empty(t, tokens)
	})
	t.Run("effective tokens that cannot be read fail", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		_, err := store.ListStudyEffectiveTokens(cancelled(ctx), owner, ref.BookID, ref.AnalysisRunID, ref.CorpusID, []int64{1})
		require.Error(t, err)
	})
	t.Run("effective-token decisions that cannot be read fail", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		_, err := store.pool.Exec(ctx, `DROP TABLE occurrence_lemma_corrections CASCADE`)
		require.NoError(t, err)
		_, err = store.ListStudyEffectiveTokens(ctx, owner, ref.BookID, ref.AnalysisRunID, ref.CorpusID, []int64{1})
		require.Error(t, err)
	})
	t.Run("Idiom candidates that cannot be counted fail", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		_, err := store.pool.Exec(ctx, `DROP TABLE idiom_candidate_counts CASCADE`)
		require.NoError(t, err)
		_, _, err = store.ListStudyIdiomCandidates(ctx, owner, "de", "v1")
		require.Error(t, err)
	})
	t.Run("restoring material that is not a record is not found", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		err := store.RestoreStudyMaterial(ctx, owner, domain.StudyProgress{MaterialID: "not-a-uuid"})
		require.ErrorIs(t, err, ErrNotFound)
	})
	t.Run("restoring material that cannot be written fails", func(t *testing.T) {
		ctx, store, owner := newBrokenIdiomStore(t)
		_, err := store.pool.Exec(ctx, `DROP TABLE study_material CASCADE`)
		require.NoError(t, err)
		err = store.RestoreStudyMaterial(ctx, owner, domain.StudyProgress{MaterialID: uuid.NewString(), StudyItem: domain.IdiomItem("de", "mit dem haus")})
		require.Error(t, err)
	})
}
