//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLemmaCorrectionPersistsOnlyForExactOwnedOccurrence(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "lemma-review-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner := createAccount(t, ctx, store, "lemma-review-owner", "learner-password")
	otherOwner, err := store.CreateUser(ctx, "lemma-review-other", false)
	require.NoError(t, err)
	book, source, corpus := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "lemma-review", "Lemma review", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "drach", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "drache", UPOS: "NOUN", OccurrenceCount: 2},
	})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	var analysisRun string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpus.ID).Scan(&analysisRun))
	unitID := domain.EPUBUnitID(0, "lemma-review")
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,0,'Ein Drache sieht einen Drachen heute.',0,30),
		      ($1,$2,$3,$4,1,'Ein Drachen fliegt als Drachen.',31,61),
		      ($1,$2,$3,$4,2,'Drachen liegen im Sand.',62,85)`, owner.ID, analysisRun, corpus.ID, unitID)
	require.NoError(t, err)
	for _, token := range []struct {
		surface, raw, lemma, upos, dependency, morphology string
		head, start, end, sentence, ordinal               int64
	}{
		{"Drachen", "Drach", "drach", "NOUN", "root", "{}", 0, 22, 29, 0, 0},
		{"Drache", "Drache", "Drache", "PROPN", "nsubj", "{}", 2, 4, 10, 0, 1},
		{"ist", "ist", "sein", "AUX", "root", `{"VerbForm":"Fin"}`, 2, 11, 14, 0, 2},
		{"Drachen", "Drache", "drache", "NOUN", "root", "{}", 0, 35, 42, 1, 0},
		{"Drachen", "Drache", "drache", "NOUN", "root", "{}", 0, 62, 69, 2, 0},
	} {
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,start_offset,end_offset,dependency,head)
			VALUES($1,'de',$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13,$14)`, owner.ID, analysisRun, corpus.ID, token.sentence, token.ordinal, token.surface, token.raw, token.lemma, token.upos, token.morphology, token.start, token.end, token.dependency, token.head)
		require.NoError(t, err)
	}

	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, occurrences, 3)
	assert.Equal(t, "Ein Drache sieht einen Drachen heute.", occurrences[0].SentenceText)
	assert.Equal(t, "Ein Drachen fliegt als Drachen.", occurrences[1].SentenceText, "the exact same observed form can have a different contextual meaning")
	assert.Equal(t, "Drachen liegen im Sand.", occurrences[2].SentenceText)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: occurrences[0].CanonicalLemma, Excluded: false, NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}), "an explicit keep is a decision")
	buildBrowseProjection(t, ctx, store, book.ID)
	assert.Empty(t, occurrences[0].CorrectedLemma)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "drache", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	require.ErrorIs(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "drachenwesen", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}), persistence.ErrNotFound, "a stale occurrence decision cannot overwrite a newer correction")
	updated, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, updated, 3)
	assert.Equal(t, "drache", updated[0].CorrectedLemma)
	assert.Empty(t, updated[1].CorrectedLemma, "matching surface elsewhere must not inherit the decision")
	assert.Empty(t, updated[2].CorrectedLemma, "a second matching surface must remain untouched")
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: updated[0], CanonicalLemma: updated[0].CanonicalLemma, NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}), "keeping the analyzer lemma removes an existing correction")
	reverted, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Empty(t, reverted[0].CorrectedLemma)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: reverted[0], CanonicalLemma: "drache", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	updated, err = store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	staleMember := updated[2]
	staleMember.CorrectedLemma = "not-current"
	err = store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{
		{Occurrence: updated[1], Excluded: true},
		{Occurrence: staleMember, CanonicalLemma: "drachenwesen", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"},
	})
	require.ErrorIs(t, err, persistence.ErrNotFound, "a stale member rejects an entire multi-occurrence decision")
	unchangedBatch, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.False(t, unchangedBatch[1].Excluded, "the valid first member rolls back when a later selected member is stale")
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: updated[0], CanonicalLemma: "", Excluded: true, NormalizationProfile: "", NormalizationVersion: ""}}), "exclusion is persisted as an exact-occurrence decision")
	excluded, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.True(t, excluded[0].Excluded)
	assert.False(t, excluded[1].Excluded, "another occurrence with the same observed form remains eligible")
	otherOwnerOccurrences, err := store.ListLemmaReviewOccurrences(ctx, otherOwner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Empty(t, otherOwnerOccurrences, "exclusion is private to the Book owner")
	buildBrowseProjection(t, ctx, store, book.ID)
	excludedProjected, err := store.GetProjectedCorpusVocabulary(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	require.True(t, excludedProjected.Ready)
	excludedInsights := excludedProjected.AnalysisCorpusVocabulary
	require.Len(t, excludedInsights.Lemmas, 1)
	assert.Equal(t, int64(3), excludedInsights.Statistics.AnalyzableTokenCount, "excluded content words remain in the original source-derived denominator")
	assert.Equal(t, int64(2), excludedInsights.Lemmas[0].OccurrenceCount, "the excluded occurrence is absent from effective insights")
	excludedCoverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), excludedCoverage.AnalyzableTokenCount)
	assert.Zero(t, excludedCoverage.MatureTokenCount, "an excluded occurrence cannot become Mature")
	assert.Zero(t, excludedCoverage.InStudyTokenCount, "an excluded occurrence cannot be in study")
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: excluded[0], CanonicalLemma: "drache", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}), "a correction replaces the exclusion on that exact occurrence")
	updated, err = store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.False(t, updated[0].Excluded)
	assert.Equal(t, "drache", updated[0].CorrectedLemma)
	other, err := store.ListLemmaReviewOccurrences(ctx, otherOwner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Empty(t, other)
	var analyzerLemma string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT canonical_lemma FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND start_offset=22`, owner.ID, corpus.ID).Scan(&analyzerLemma))
	assert.Equal(t, "drach", analyzerLemma)
	assert.Equal(t, owner.ID, updated[0].OwnerID)

	buildBrowseProjection(t, ctx, store, book.ID)
	projected, err := store.GetProjectedCorpusVocabulary(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	require.True(t, projected.Ready)
	insights := projected.AnalysisCorpusVocabulary
	require.Len(t, insights.Lemmas, 1)
	assert.Equal(t, "drache", insights.Lemmas[0].CanonicalLemma)
	assert.Equal(t, int64(3), insights.Lemmas[0].OccurrenceCount)
	assert.Equal(t, int64(3), insights.Statistics.AnalyzableTokenCount, "the source-derived coverage denominator is unchanged")

	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	buildBrowseProjection(t, ctx, store, book.ID)
	reservedCoverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	assert.Zero(t, reservedCoverage.InStudyTokenCount, "an active reading reservation is not study")
	assert.Zero(t, reservedCoverage.MatureTokenCount, "an active reading reservation is not Mature")
	// The Mature word is what moves coverage.
	seedStudyWord(t, ctx, store, owner.ID, "drache", true)
	coverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), coverage.MatureTokenCount, "Mature matches the corrected effective identity")
	assert.Zero(t, coverage.UnknownTokenCount)
	assert.Equal(t, int64(3), coverage.AnalyzableTokenCount, "correction does not change the source-derived denominator")
	// Decisions retire the Book's Browse counts until the durable rebuild runs;
	// Reading shows no coverage without them, so publish them as the job would.
	rebuildTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, persistence.BuildVocabularyBrowseCountsTx(ctx, rebuildTx, owner.ID, book.ID, source.ID, analysisRun, corpus.ID, "de"))
	require.NoError(t, rebuildTx.Commit(ctx))
	myBooksAuth := auth.New(store, time.Hour)
	myBooksHandler := New(Services{Auth: myBooksAuth, WebAuth: webauth.New(myBooksAuth, false, time.Hour), Store: storeDependencies(store), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, myBooksHandler, "lemma-review-owner", "learner-password")
	myBooksPage := perform(t, myBooksHandler, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, myBooksPage.Code)
	assert.Contains(t, myBooksPage.Body.String(), fmt.Sprintf("%.1f%% of running words Mature.", float64(coverage.MatureTokenCount)*100/float64(coverage.AnalyzableTokenCount)), "Reading margin coverage must match analysisinsights.Coverage after the exact-occurrence correction and exclusion decisions")
	assert.NotContains(t, myBooksPage.Body.String(), ">0.0% of running words Mature.")
	assert.Equal(t, coverage.InStudyTokenCount, coverage.MatureTokenCount)

	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de"))
	currentOccurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: currentOccurrences[0], CanonicalLemma: "drachen", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}), "ending the reading permits an identity change")
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	finished, err := store.FinishCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, finished.Completion.BookID)
	assert.Empty(t, testutil.LegacyKnownLemmas(t, ctx, store.Pool(), owner.ID, "de"), "Finish adds no Known identity")
	completedBook, err := store.GetBookDetail(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, completedBook.CompletionCount)
}

func TestLemmaReviewCanSelectContextSpecificDrachenOccurrences(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner := createAccount(t, ctx, store, "drachen-review-owner", "learner-password")
	book, _, corpus := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "drachen-context", "Drachen contexts", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "drach", UPOS: "NOUN", OccurrenceCount: 2},
	})
	var analysisRun string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpus.ID).Scan(&analysisRun))
	unitID := domain.EPUBUnitID(0, "drachen-context")
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,0,'Der Drache sieht einen Drachen.',0,31),
		      ($1,$2,$3,$4,1,'Der Drachen steigt als Drachen.',32,63)`, owner.ID, analysisRun, corpus.ID, unitID)
	require.NoError(t, err)
	for _, token := range []struct {
		sentence   int64
		start, end int64
	}{{0, 22, 29}, {1, 36, 43}} {
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,start_offset,end_offset,dependency,head)
			VALUES($1,'de',$2,$3,$4,0,'Drachen','Drach','drach','NOUN','{}',$5,$6,'root',0)`, owner.ID, analysisRun, corpus.ID, token.sentence, token.start, token.end)
		require.NoError(t, err)
	}
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	assert.Contains(t, occurrences[0].SentenceText, "Drache sieht", "the first context is a dragon")
	assert.Contains(t, occurrences[1].SentenceText, "steigt als Drachen", "the same observed form is used as a kite in the second context")
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{
		{Occurrence: occurrences[0], CanonicalLemma: "drache", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"},
		{Occurrence: occurrences[1], CanonicalLemma: "drachen", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"},
	}))
	updated, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, updated, 2)
	assert.Equal(t, "drache", updated[0].CorrectedLemma)
	assert.Equal(t, "drachen", updated[1].CorrectedLemma)
	assert.Equal(t, "drach", updated[0].CanonicalLemma, "analyzer evidence remains immutable")
	assert.Equal(t, "drach", updated[1].CanonicalLemma, "the second context keeps its own analyzer evidence")
}
