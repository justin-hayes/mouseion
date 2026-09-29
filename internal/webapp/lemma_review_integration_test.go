//go:build integration

package webapp

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLemmaCorrectionPersistsOnlyForExactOwnedOccurrence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner := createAccount(t, ctx, store, "lemma-review-owner", "learner-password", false)
	otherOwner, err := store.CreateUser(ctx, "lemma-review-other", false)
	require.NoError(t, err)
	book, _, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "lemma-review", "Lemma review", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "drach", UPOS: "NOUN", OccurrenceCount: 2}})
	var analysisRun string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpus.ID).Scan(&analysisRun))
	unitID := domain.EPUBUnitID(0, "lemma-review")
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,0,'Ein Drache sieht einen Drachen.',0,32)`, owner.ID, analysisRun, corpus.ID, unitID)
	require.NoError(t, err)
	for _, token := range []struct {
		raw, lemma string
		start, end int64
		ordinal    int64
	}{{"Drach", "drach", 10, 17, 0}, {"Drache", "drache", 22, 29, 1}} {
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,start_offset,end_offset,dependency,head)
			VALUES($1,'de',$2,$3,0,$4,'Drachen',$5,$6,'NOUN','{}',$7,$8,'root',0)`, owner.ID, analysisRun, corpus.ID, token.ordinal, token.raw, token.lemma, token.start, token.end)
		require.NoError(t, err)
	}

	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	assert.Empty(t, occurrences[0].CorrectedLemma)
	require.NoError(t, store.PutLemmaCorrection(ctx, occurrences[0], "drache", "german-post-1996", "6"))
	updated, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, updated, 2)
	assert.Equal(t, "drache", updated[0].CorrectedLemma)
	assert.Empty(t, updated[1].CorrectedLemma, "matching surface elsewhere must not inherit the decision")
	other, err := store.ListLemmaReviewOccurrences(ctx, otherOwner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Empty(t, other)
	var analyzerLemma string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT canonical_lemma FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND start_offset=10`, owner.ID, corpus.ID).Scan(&analyzerLemma))
	assert.Equal(t, "drach", analyzerLemma)
	assert.Equal(t, owner.ID, updated[0].OwnerID)
}
