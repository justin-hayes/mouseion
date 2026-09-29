//go:build integration

package webapp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
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
	book, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "lemma-review", "Lemma review", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "drach", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "drache", UPOS: "NOUN", OccurrenceCount: 2},
	})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	var analysisRun string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpus.ID).Scan(&analysisRun))
	unitID := domain.EPUBUnitID(0, "lemma-review")
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,0,'Ein Drache sieht einen Drachen und noch einen Drachen.',0,51)`, owner.ID, analysisRun, corpus.ID, unitID)
	require.NoError(t, err)
	for _, token := range []struct {
		raw, lemma string
		start, end int64
		ordinal    int64
	}{{"Drach", "drach", 10, 17, 0}, {"Drache", "drache", 22, 29, 1}, {"Drache", "drache", 39, 46, 2}} {
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,start_offset,end_offset,dependency,head)
			VALUES($1,'de',$2,$3,0,$4,'Drachen',$5,$6,'NOUN','{}',$7,$8,'root',0)`, owner.ID, analysisRun, corpus.ID, token.ordinal, token.raw, token.lemma, token.start, token.end)
		require.NoError(t, err)
	}

	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, occurrences, 3)
	assert.Empty(t, occurrences[0].CorrectedLemma)
	require.NoError(t, store.PutLemmaCorrection(ctx, occurrences[0], "drache", "german-post-1996", "6"))
	require.ErrorIs(t, store.PutLemmaCorrection(ctx, occurrences[0], "drachenwesen", "german-post-1996", "6"), persistence.ErrNotFound, "a stale occurrence decision cannot overwrite a newer correction")
	updated, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, updated, 3)
	assert.Equal(t, "drache", updated[0].CorrectedLemma)
	assert.Empty(t, updated[1].CorrectedLemma, "matching surface elsewhere must not inherit the decision")
	assert.Empty(t, updated[2].CorrectedLemma, "a second matching surface must remain untouched")
	require.NoError(t, store.PutLemmaCorrection(ctx, updated[0], updated[0].CanonicalLemma, "german-post-1996", "6"), "keeping the analyzer lemma removes an existing correction")
	reverted, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Empty(t, reverted[0].CorrectedLemma)
	require.NoError(t, store.PutLemmaCorrection(ctx, reverted[0], "drache", "german-post-1996", "6"))
	updated, err = store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	other, err := store.ListLemmaReviewOccurrences(ctx, otherOwner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Empty(t, other)
	var analyzerLemma string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT canonical_lemma FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND start_offset=10`, owner.ID, corpus.ID).Scan(&analyzerLemma))
	assert.Equal(t, "drach", analyzerLemma)
	assert.Equal(t, owner.ID, updated[0].OwnerID)

	insights, err := store.GetAnalysisCorpusVocabulary(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	require.Len(t, insights.Lemmas, 1)
	assert.Equal(t, "drache", insights.Lemmas[0].CanonicalLemma)
	assert.Equal(t, int64(3), insights.Lemmas[0].OccurrenceCount)
	assert.Equal(t, int64(3), insights.Statistics.AnalyzableTokenCount, "the source-derived coverage denominator is unchanged")

	directTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = directTx.Rollback(ctx) }()
	directProjections, _, err := prepareddeck.NewInputAssembler(store).AssemblePreparedDeckInputs(ctx, directTx, domain.DeckPreparation{
		ID: uuid.NewString(), OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRun,
	})
	require.NoError(t, err)
	require.NoError(t, directTx.Rollback(ctx))
	require.Len(t, directProjections, 1, "direct preparation selects the corrected identity across the recurrence floor")
	assert.Equal(t, "drache", directProjections[0].Candidate.CanonicalLemma)
	assert.Equal(t, 3, directProjections[0].Candidate.OccurrenceCount)
	assert.Equal(t, "Ein Drache sieht einen Drachen und noch einen Drachen.", directProjections[0].Entry.Sentence)

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	snapshot, err := store.ListPrimaryGoalSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	require.Len(t, snapshot, 1)
	assert.Equal(t, "drache", snapshot[0].CanonicalLemma)
	assert.Equal(t, 3, snapshot[0].OccurrenceCount, "the one corrected occurrence joins the two existing occurrences across the recurrence floor")
	assert.Contains(t, string(snapshot[0].SentenceReferences), "Ein Drache sieht einen Drachen und noch einen Drachen.", "the reading snapshot freezes a representative sentence from the effective candidate")
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	deckFacts, err := store.LoadPreparedDeckCandidateFactsTx(ctx, tx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRun,
	}, snapshot)
	require.NoError(t, err)
	require.Len(t, deckFacts, 1)
	assert.Equal(t, "Ein Drache sieht einen Drachen und noch einen Drachen.", deckFacts[0].Entry.Sentence, "deck preparation uses a corrected representative source sentence, not a stale analyzer candidate row")
	require.ErrorIs(t, store.PutLemmaCorrection(ctx, updated[1], "drago", "german-post-1996", "6"), persistence.ErrNotFound, "an active snapshot rejects later identity changes")
	unchanged, err := store.ListPrimaryGoalSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, snapshot, unchanged, "frozen reading evidence remains immutable")
	reservedCoverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), reservedCoverage.ReservedTokenCount, "the effective corrected identity matches the active reading reservation")
	assert.Zero(t, reservedCoverage.KnownTokenCount, "Reserved vocabulary is not counted as Known")
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "drache", "NOUN")
	require.NoError(t, err)
	coverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), coverage.KnownTokenCount, "Known matches the corrected effective identity")
	assert.Zero(t, coverage.UnknownTokenCount)
	assert.Equal(t, int64(3), coverage.AnalyzableTokenCount, "correction does not change the source-derived denominator")
}
