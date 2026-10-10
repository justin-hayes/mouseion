//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationScenarioCoversFreshFlowAndEpistemicBoundaries(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "migration-scenario-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	var languageProfilesExists, languageProfilesForeignKeyExists bool
	err = store.Pool().QueryRow(ctx, `SELECT to_regclass('public.language_profiles') IS NOT NULL`).Scan(&languageProfilesExists)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='language_profiles_supported_language_fkey')`).Scan(&languageProfilesForeignKeyExists)
	require.NoError(t, err)
	assert.False(t, languageProfilesExists, "language_profiles migration left table")
	assert.False(t, languageProfilesForeignKeyExists, "language_profiles migration left foreign key")

	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "migration-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "migration-bob", "bob-password", false)
	capabilities := staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", DisplayName: "German", Ready: true},
		{Language: "it", DisplayName: "Italian", Ready: true},
	}}}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: fixtures.Analysis{}, AnalysisInsights: analysisinsights.NewService(store), Capabilities: capabilities,
		CatalogueSync:   fixtures.NewCatalogueSync(fixtures.NewStore()),
		PreparedDeck:    &recordingPreparedDeck{preparations: make(map[string]domain.DeckPreparation)},
		SessionLifetime: time.Hour,
	})
	aliceCookies, csrf := loginCookies(t, h, "migration-alice", "alice-password")

	metadata, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Migration metadata book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	metadataView := migrationMyBook(t, ctx, store, alice.ID, metadata.ID)
	assert.Nil(t, metadataView.Acquired)
	assert.Equal(t, domain.BookNotAcquired, metadataView.EvidenceState())

	book, source, corpus, preparation := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "primary", "Migrated primary goal", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "residual", UPOS: "NOUN", OccurrenceCount: 3},
		{Language: "de", CanonicalLemma: "graduated", UPOS: "VERB", OccurrenceCount: 3},
		{Language: "de", CanonicalLemma: "legacy", UPOS: "ADJ", OccurrenceCount: 1},
	})
	require.NoError(t, store.LinkSourceToBook(ctx, alice.ID, book.ID, source.ID))
	metadataView = migrationMyBook(t, ctx, store, alice.ID, book.ID)
	require.NotNil(t, metadataView.Acquired)
	assert.Equal(t, source.ID, metadataView.Acquired.Source.ID)
	assert.NotEmpty(t, metadataView.Acquired.CorpusID)
	assert.Equal(t, domain.LanguageChosen, metadataView.Book.LanguageState)
	for _, candidate := range []domain.SelectionCandidate{
		{OwnerID: alice.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: "residual", UPOS: "NOUN", OccurrenceCount: 3, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`)},
		{OwnerID: alice.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: "graduated", UPOS: "VERB", OccurrenceCount: 3, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`)},
		{OwnerID: alice.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: "legacy-state", UPOS: "ADJ", OccurrenceCount: 3, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`)},
	} {
		_, err = store.PutSelectionCandidate(ctx, candidate)
		require.NoError(t, err)
	}

	secondBook, secondSource, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "later", "Later Journey book", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1},
	})
	require.NoError(t, store.LinkSourceToBook(ctx, alice.ID, secondBook.ID, secondSource.ID))

	_, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "Haus", "NOUN")
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, alice.ID, "it", "casa", "NOUN")
	require.NoError(t, err)

	legacySource := (*string)(nil)
	legacyDeck, err := store.PutDeck(ctx, alice.ID, "de", "Legacy generated deck")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "legacy", UPOS: "ADJ", FirstDeckID: legacyDeck.ID, FirstSourceMaterialID: legacySource})
	require.NoError(t, err)
	primaryGenerated := []string{"residual", "graduated"}
	primaryDeck, err := store.PutDeck(ctx, alice.ID, "de", "Migration primary deck")
	require.NoError(t, err)
	for _, lemma := range primaryGenerated {
		_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", FirstDeckID: primaryDeck.ID, FirstSourceMaterialID: &source.ID})
		require.NoError(t, err)
	}
	// Keep the intended verb identity distinct from the residual noun while
	// using the same generated source/deck provenance.
	_, err = store.Pool().Exec(ctx, `UPDATE generated_vocabulary SET upos='VERB' WHERE owner_id=$1 AND canonical_lemma='graduated'`, alice.ID)
	require.NoError(t, err)
	// Preserve an unconfirmed legacy preparation while the Goal-owned model is
	// exercised. Completion must not silently graduate or release this history.
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET studying_at=now() WHERE owner_id=$1 AND id=$2`, alice.ID, preparation.ID)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, book.ID, domain.BookDispositionToRead))
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, secondBook.ID, domain.BookDispositionToRead))
	started := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, aliceCookies)
	require.Equal(t, http.StatusSeeOther, started.Code)
	assert.Contains(t, started.Header().Get("Location"), "/reading?message=")
	goal, err := store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, goal.BookID)
	queuedDecks, ok := h.services.PreparedDeck.(*recordingPreparedDeck)
	require.True(t, ok)
	require.Empty(t, queuedDecks.preparations, "starting a Reading must not auto-submit deck preparation")
	currentPage := perform(t, h, http.MethodGet, "/reading", nil, aliceCookies)
	require.Equal(t, http.StatusOK, currentPage.Code)
	assert.Contains(t, currentPage.Body.String(), "Migrated primary goal")
	assert.Contains(t, currentPage.Body.String(), "Reserved vocabulary")
	toReadPage := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, aliceCookies)
	require.Equal(t, http.StatusOK, toReadPage.Code)
	assert.Contains(t, toReadPage.Body.String(), "Migrated primary goal")
	assert.Contains(t, toReadPage.Body.String(), "Currently reading")
	assert.Contains(t, toReadPage.Body.String(), "Later Journey book")
	assert.Contains(t, toReadPage.Body.String(), "To Read (2)")
	_, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "legacy-state", "ADJ")
	require.NoError(t, err)
	beforeCoverage, err := analysisinsights.NewService(store).Coverage(ctx, alice.ID, corpus.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), beforeCoverage.KnownTokenCount)
	assert.Equal(t, int64(6), beforeCoverage.ReservedTokenCount)

	// Stale writes, missing CSRF, cross-owner references, and invalid progress
	// are rejected without changing the accepted state.
	_, err = store.SwitchCurrentReading(ctx, alice.ID, "de", secondBook.ID, "stale-book", goal.SnapshotID)
	assert.ErrorIs(t, err, persistence.ErrCurrentReadingStale) //nolint:testifylint // Stale goal rejection is independently asserted before HTTP checks.
	bobCookies, bobCSRF := loginCookies(t, h, "migration-bob", "bob-password")
	foreign := perform(t, h, http.MethodPost, "/goal/books/"+book.ID, url.Values{"csrf_token": {bobCSRF}, "expected_goal_book_id": {""}}, bobCookies)
	assert.Equal(t, http.StatusNotFound, foreign.Code)
	bobGoal, goalErr := store.GetCurrentReading(ctx, bob.ID, "de")
	require.NoError(t, goalErr)
	assert.Empty(t, bobGoal.BookID)

	finished := perform(t, h, http.MethodPost, "/reading/finish", url.Values{"csrf_token": {csrf}, "expected_current_book_id": {book.ID}, "expected_current_snapshot_id": {goal.SnapshotID}}, aliceCookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	for _, want := range []string{"Reading finished", "Vocabulary: 2 identities newly Known; 1 identities already Known", `href="/reading">Choose a To Read book</a>`} {
		assert.True(t, strings.Contains(finished.Body.String(), want), "finish receipt missing %q: %s", want, finished.Body.String())
	}
	reserved, err := store.ListReservedVocabulary(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, reserved)
	knownAfterReading, err := store.ListKnownVocabulary(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Len(t, knownAfterReading, 4)
	knownLemmas := map[string]bool{}
	for _, item := range knownAfterReading {
		knownLemmas[item.CanonicalLemma] = true
	}
	assert.True(t, knownLemmas["Haus"])
	assert.True(t, knownLemmas["residual"])
	assert.True(t, knownLemmas["graduated"])
	assert.True(t, knownLemmas["legacy-state"])
	assert.False(t, knownLemmas["legacy"])
	legacy, err := store.ListUnattachedGeneratedVocabulary(ctx, alice.ID, "de")
	require.NoError(t, err)
	legacyFound := false
	for _, item := range legacy {
		legacyFound = legacyFound || item.CanonicalLemma == "legacy" && item.FirstSourceMaterialID == nil
	}
	assert.True(t, legacyFound)
	knownAgain, err := store.ListKnownVocabulary(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Len(t, knownAgain, len(knownAfterReading))
	afterCoverage, err := analysisinsights.NewService(store).Coverage(ctx, alice.ID, corpus.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(7), afterCoverage.KnownTokenCount)
	assert.Equal(t, int64(0), afterCoverage.ReservedTokenCount)
	completion, err := store.FinishCurrentReading(ctx, alice.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, 3, completion.Completion.SnapshotVocabularyCount)
	assert.Equal(t, 2, completion.Completion.EligibleVocabularyCount)
	assert.Equal(t, 2, completion.Completion.GraduatedVocabularyCount)
	assert.Equal(t, 1, completion.Completion.AlreadyKnownVocabularyCount)
	legacyPreparation, err := store.GetDeckPreparation(ctx, alice.ID, preparation.ID)
	require.NoError(t, err)
	assert.NotNil(t, legacyPreparation.StudyingAt, "legacy preparation was silently graduated")
	assert.Nil(t, legacyPreparation.ReleasedAt, "legacy preparation was silently released")
	assert.Nil(t, legacyPreparation.GraduatedAt, "legacy preparation was silently graduated")
	goal, err = store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, goal.BookID)
	laterGoal := perform(t, h, http.MethodPost, "/reading/books/"+secondBook.ID+"/start", url.Values{
		"csrf_token": {csrf},
	}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, laterGoal.Code)
	assert.NotContains(t, laterGoal.Header().Get("Location"), "error=")
	finishedDisposition, err := store.GetBookDisposition(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, finishedDisposition, "choosing a later current reading changed the finished Book disposition")
	goal, err = store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, secondBook.ID, goal.BookID)

	// Failed analysis retry and prepared-deck retry remain explicit operations.
	failedJob := perform(t, h, http.MethodPost, "/jobs/43/retry", url.Values{"csrf_token": {csrf}}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, failedJob.Code)
	assert.True(t, strings.Contains(failedJob.Header().Get("Location"), "Analysis+retry+submitted"), "location=%q", failedJob.Header().Get("Location"))
	retryPrep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: secondSource.ID, Filename: "retry.apkg", DeckName: "Retry deck", ContentHash: "retry-hash"})
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, alice.ID, retryPrep.ID)
	require.NoError(t, err)
	_, err = store.FailDeckPreparation(ctx, alice.ID, retryPrep.ID, "temporary provider failure")
	require.NoError(t, err)
	requeued, err := store.RetryDeckPreparation(ctx, alice.ID, retryPrep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, requeued.State)
	assert.Empty(t, requeued.Error)

	deVocabulary := perform(t, h, http.MethodGet, "/vocabulary/import", nil, aliceCookies)
	assert.Equal(t, http.StatusOK, deVocabulary.Code)
	body := deVocabulary.Body.String()
	assert.True(t, strings.Contains(body, `enctype="multipart/form-data"`), "import form missing: body=%s", body)
	assert.False(t, strings.Contains(body, "Known vocabulary</h2>"), "known-vocabulary heading rendered: body=%s", body)
	assert.False(t, strings.Contains(body, "Haus"), "known-vocabulary row rendered: body=%s", body)
	assert.False(t, strings.Contains(body, "Explicitly recorded"), "known-vocabulary provenance rendered: body=%s", body)
}

func seedMigrationAnalyzedBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, suffix, title string, lemmas []domain.LemmaOccurrence) (domain.Book, domain.SourceMaterial, domain.Corpus, domain.DeckPreparation) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	text := suffix + " content"
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner, Language: "de", SourceIdentifier: "migration-" + suffix, Title: title, MediaType: "application/epub+zip", Content: []byte(text), FullText: text}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, suffix), Order: 0, SpineIndex: 0, ManifestID: suffix, Text: text, EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true}}})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner, book.ID, source.ID))
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "migration", NormalizationVersion: "1", AnalyzerName: "migration-fixture", AnalyzerVersion: "1"}, toSharedLemmas(lemmas)))
	corpus, err := store.PutCorpus(ctx, owner, source.ID, source.ContentHash)
	require.NoError(t, err)
	var tokenCount int64
	for _, lemma := range lemmas {
		tokenCount += lemma.OccurrenceCount
	}
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analyzable_token_count=$2,distinct_lemma_count=$3,sentence_count=1,normalized_token_count=$2,empty_sentence_count=0,median_sentence_token_count=1,p90_sentence_token_count=1,long_sentence_count=0 WHERE owner_id=$1 AND id=$4`, owner, tokenCount, len(lemmas), corpus.ID)
	require.NoError(t, err)
	var snapshotID string
	err = store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, source.ID).Scan(&snapshotID)
	require.NoError(t, err)
	var runID string
	err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'migration-fixture','1',$5,'completed',now()) RETURNING id::text`, owner, source.ID, source.ContentRevisionID, snapshotID, suffix).Scan(&runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, owner, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, owner, runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner, book.ID, source.ID, runID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source.ID, Filename: suffix + ".apkg", DeckName: "Migration " + suffix, ContentHash: source.ContentHash})
	require.NoError(t, err)
	preparation, err = store.ClaimDeckPreparation(ctx, owner, preparation.ID)
	require.NoError(t, err)
	preparation, err = store.CompleteDeckPreparation(ctx, owner, preparation.ID, domain.DeckPreparation{Artifact: []byte("migration-" + suffix), Filename: suffix + ".apkg", DeckName: "Migration " + suffix, TotalCards: len(lemmas)})
	require.NoError(t, err)
	return book, source, corpus, preparation
}

func toSharedLemmas(lemmas []domain.LemmaOccurrence) []domain.SharedLemma {
	result := make([]domain.SharedLemma, 0, len(lemmas))
	for _, lemma := range lemmas {
		result = append(result, domain.SharedLemma{Language: lemma.Language, CanonicalLemma: lemma.CanonicalLemma, UPOS: lemma.UPOS, Frequency: lemma.OccurrenceCount, Morphology: []byte(`{}`)})
	}
	return result
}

func migrationMyBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, bookID string) domain.MyBook {
	t.Helper()
	books, err := store.ListMyBooksWithEvidence(ctx, owner)
	require.NoError(t, err)
	for _, book := range books {
		if book.Book.ID == bookID {
			return book
		}
	}
	require.Failf(t, "book absent from My Books", "book %q absent from My Books: %+v", bookID, books)
	return domain.MyBook{}
}
