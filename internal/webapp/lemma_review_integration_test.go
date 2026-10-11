//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
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
		VALUES($1,$2,$3,$4,0,'Ein Drache sieht einen Drachen.',0,30),
		      ($1,$2,$3,$4,1,'Ein Drachen fliegt als Drachen.',31,61),
		      ($1,$2,$3,$4,2,'Drachen liegen im Sand.',62,85)`, owner.ID, analysisRun, corpus.ID, unitID)
	require.NoError(t, err)
	for _, token := range []struct {
		raw, lemma string
		start, end int64
		ordinal    int64
	}{{"Drach", "drach", 22, 29, 0}, {"Drache", "drache", 35, 42, 1}, {"Drache", "drache", 62, 69, 2}} {
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,start_offset,end_offset,dependency,head)
			VALUES($1,'de',$2,$3,$9,$4,'Drachen',$5,$6,'NOUN','{}',$7,$8,'root',0)`, owner.ID, analysisRun, corpus.ID, token.ordinal, token.raw, token.lemma, token.start, token.end, token.ordinal)
		require.NoError(t, err)
	}

	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, occurrences, 3)
	assert.Equal(t, "Ein Drache sieht einen Drachen.", occurrences[0].SentenceText)
	assert.Equal(t, "Ein Drachen fliegt als Drachen.", occurrences[1].SentenceText, "the exact same observed form can have a different contextual meaning")
	assert.Equal(t, "Drachen liegen im Sand.", occurrences[2].SentenceText)
	flag := domain.LemmaReviewFlag{Occurrence: occurrences[0], Reason: "A local lexical alternative could affect recurring vocabulary.", Provenance: map[string]any{"alternative_lemma": "drache", "source": "kaikki", "version": "fixture-1", "evidence_id": "drache-fixture"}}
	require.NoError(t, store.SaveLemmaReviewFlags(ctx, []domain.LemmaReviewFlag{flag}))
	flagged, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Equal(t, flag.Reason, flagged[0].ReviewFlagReason)
	assert.Empty(t, flagged[0].ReviewFlagResolution)
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.ErrorIs(t, err, persistence.ErrUnresolvedLemmaReviewFlags, "an unresolved exact-analysis flag blocks the snapshot freeze")
	flaggedOccurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Equal(t, flag.Reason, flaggedOccurrences[0].ReviewFlagReason)
	assert.Equal(t, "drache", flaggedOccurrences[0].ReviewFlagProvenance["alternative_lemma"])
	readyDeck, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRun, Filename: "historical.apkg", DeckName: "Historical", ContentHash: source.ContentHash})
	require.NoError(t, err)
	readyDeck, err = store.ClaimDeckPreparation(ctx, owner.ID, readyDeck.ID)
	require.NoError(t, err)
	readyDeck, err = store.CompleteDeckPreparation(ctx, owner.ID, readyDeck.ID, domain.DeckPreparation{Artifact: []byte("historical-an identity-deck"), Filename: "historical.apkg", DeckName: "Historical", TotalCards: 1})
	require.NoError(t, err)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: occurrences[0].CanonicalLemma, Excluded: false, NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}), "an explicit keep resolves the review flag")
	require.NoError(t, store.SaveLemmaReviewFlags(ctx, []domain.LemmaReviewFlag{flag}), "later reference refreshes cannot reopen a resolved occurrence flag")
	resolved, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Equal(t, "keep", resolved[0].ReviewFlagResolution)
	buildBrowseProjection(t, ctx, store, book.ID)
	// The proposal moves "unrelated-known", so making that identity Known after
	// the preview is an affected-identity change that must reject the confirm.
	proposal := domain.LemmaReviewProposal{
		OwnerID: owner.ID, BookID: book.ID, Language: "de", Surface: "Drachen", Action: "correct", Lemma: "unrelated-known",
		Occurrences: occurrences[:1], NormalizationProfile: "german-post-1996", NormalizationVersion: "6",
	}
	preview, err := store.ReadLemmaReviewProposal(ctx, proposal)
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "unrelated-known", "NOUN")
	require.NoError(t, err)
	err = store.PutLemmaDecisionProposal(ctx, proposal, preview.Fingerprint)
	require.ErrorIs(t, err, domain.ErrLemmaReviewPreviewStale, "a Known change to an identity the proposal affects after preview is rejected inside the decision transaction")
	unchangedAfterStaleProposal, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.Empty(t, unchangedAfterStaleProposal[0].CorrectedLemma, "a stale preview cannot persist its proposed identity")
	assert.Empty(t, occurrences[0].CorrectedLemma)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "drache", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	historicalDeck, err := store.GetDeckPreparation(ctx, owner.ID, readyDeck.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("historical-an identity-deck"), historicalDeck.Artifact, "accepting a later identity decision preserves the ready historical artifact")
	assert.Equal(t, domain.DeckPreparationReady, historicalDeck.State)
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
	assert.Zero(t, excludedCoverage.KnownTokenCount, "an excluded occurrence cannot become Known")
	assert.Zero(t, excludedCoverage.ReservedTokenCount, "an excluded occurrence cannot become Reserved")
	excludedTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	excludedDeck, _, assembleErr := prepareddeck.NewInputAssembler(store).AssemblePreparedDeckInputs(ctx, excludedTx, domain.DeckPreparation{
		ID: uuid.NewString(), OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRun,
	})
	require.NoError(t, excludedTx.Rollback(ctx))
	require.NoError(t, assembleErr)
	assert.Empty(t, excludedDeck, "effective direct-deck candidates must omit the excluded occurrence and honor the three-occurrence floor")
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

	directTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	directProjections, _, assembleErr := prepareddeck.NewInputAssembler(store).AssemblePreparedDeckInputs(ctx, directTx, domain.DeckPreparation{
		ID: uuid.NewString(), OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRun,
	})
	rollbackErr := directTx.Rollback(ctx)
	require.NoError(t, rollbackErr)
	require.NoError(t, assembleErr)
	require.Len(t, directProjections, 1, "direct preparation selects the corrected identity across the recurrence floor")
	assert.Equal(t, "drache", directProjections[0].Candidate.CanonicalLemma)
	assert.Equal(t, 3, directProjections[0].Candidate.OccurrenceCount)
	assert.Equal(t, "Ein Drache sieht einen Drachen.", directProjections[0].Entry.Sentence)

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	snapshot, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	require.Len(t, snapshot, 1)
	assert.Equal(t, "drache", snapshot[0].CanonicalLemma)
	assert.Equal(t, 3, snapshot[0].OccurrenceCount, "the one corrected occurrence joins the two existing occurrences across the recurrence floor")
	assert.Contains(t, string(snapshot[0].SentenceReferences), "Ein Drache sieht einen Drachen.", "the reading snapshot freezes a representative sentence from the effective candidate")
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	deckFacts, err := store.LoadPreparedDeckCandidateFactsTx(ctx, tx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRun,
	}, snapshot)
	require.NoError(t, err)
	require.Len(t, deckFacts, 1)
	assert.Equal(t, "Ein Drache sieht einen Drachen.", deckFacts[0].Entry.Sentence, "deck preparation uses a corrected representative source sentence, not a stale analyzer candidate row")
	require.ErrorIs(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: updated[1], CanonicalLemma: "", Excluded: true, NormalizationProfile: "", NormalizationVersion: ""}}), persistence.ErrNotFound, "an active snapshot rejects later occurrence exclusions")
	stillCurrent, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	assert.False(t, stillCurrent[1].Excluded, "a rejected stale exclusion leaves the learner decision unchanged")
	unchanged, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, snapshot, unchanged, "frozen reading evidence remains immutable")
	buildBrowseProjection(t, ctx, store, book.ID)
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
	// Decisions retire the Book's Browse counts until the durable rebuild runs;
	// My Books shows no coverage without them, so publish them as the job would.
	rebuildTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, persistence.BuildVocabularyBrowseCountsTx(ctx, rebuildTx, owner.ID, book.ID, source.ID, analysisRun, corpus.ID, "de"))
	require.NoError(t, rebuildTx.Commit(ctx))
	myBooksAuth := auth.New(store, time.Hour)
	myBooksHandler := New(Services{Auth: myBooksAuth, WebAuth: webauth.New(myBooksAuth, false, time.Hour), Store: storeDependencies(store), PreparedDeck: prepareddeck.NewService(store, nil), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, myBooksHandler, "lemma-review-owner", "learner-password")
	myBooksPage := perform(t, myBooksHandler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, myBooksPage.Code)
	assert.Contains(t, myBooksPage.Body.String(), fmt.Sprintf("%.1f%% of running words Known.", float64(coverage.KnownTokenCount)*100/float64(coverage.AnalyzableTokenCount)), "My Books margin coverage must match analysisinsights.Coverage after the exact-occurrence correction and exclusion decisions")
	assert.NotContains(t, myBooksPage.Body.String(), ">0.0% of running words Known.")

	oldSnapshotID := reading.SnapshotID
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, oldSnapshotID))
	currentOccurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: currentOccurrences[0], CanonicalLemma: "drachen", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}), "stopping permits an identity change without completing the old snapshot")
	rereading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.NotEqual(t, oldSnapshotID, rereading.SnapshotID, "restarting freezes a new snapshot after the correction")
	oldFrozen, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, oldSnapshotID)
	require.NoError(t, err)
	assert.Equal(t, snapshot, oldFrozen, "stopping and restarting does not rewrite the old frozen snapshot")
	finished, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, rereading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, rereading.SnapshotID, finished.Completion.SnapshotID)
	knownAfterFinish, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, knownAfterFinish, 2)
	assert.Contains(t, []string{knownAfterFinish[0].CanonicalLemma, knownAfterFinish[1].CanonicalLemma}, "drache", "the pre-existing Known identity remains unchanged after the later reading")
	assert.Contains(t, []string{knownAfterFinish[0].CanonicalLemma, knownAfterFinish[1].CanonicalLemma}, "unrelated-known")
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
	owner := createAccount(t, ctx, store, "drachen-review-owner", "learner-password", false)
	book, _, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "drachen-context", "Drachen contexts", []domain.LemmaOccurrence{
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
