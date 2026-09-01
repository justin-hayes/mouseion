//go:build integration

package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

func TestMigrationScenarioCoversFreshFlowAndEpistemicBoundaries(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "migration-scenario-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "migration-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "migration-bob", "bob-password", false)
	capabilities := staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", DisplayName: "German", Ready: true},
		{Language: "it", DisplayName: "Italian", Ready: true},
	}}}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store,
		Analysis: fixtures.Analysis{}, AnalysisInsights: analysisinsights.NewService(store), Capabilities: capabilities,
		SessionLifetime: time.Hour,
	})
	aliceCookies, csrf := loginCookies(t, h, "migration-alice", "alice-password")

	metadata, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Migration metadata book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	metadataView := migrationMyBook(t, ctx, store, alice.ID, metadata.ID)
	if metadataView.Acquired != nil || metadataView.EvidenceState != domain.MyBookNotAcquired {
		t.Fatalf("metadata-only book before acquisition=%+v", metadataView)
	}

	book, source, corpus, preparation := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "primary", "Migrated primary goal", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "residual", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "graduated", UPOS: "VERB", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "legacy", UPOS: "ADJ", OccurrenceCount: 1},
	})
	if err = store.LinkSourceToBook(ctx, alice.ID, book.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	metadataView = migrationMyBook(t, ctx, store, alice.ID, book.ID)
	if metadataView.Acquired == nil || metadataView.Acquired.Source.ID != source.ID || metadataView.Book.LanguageState != domain.LanguageChosen {
		t.Fatalf("acquisition promotion=%+v", metadataView)
	}

	secondBook, secondSource, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "later", "Later Journey book", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1},
	})
	if err = store.LinkSourceToBook(ctx, alice.ID, secondBook.ID, secondSource.ID); err != nil {
		t.Fatal(err)
	}

	// Language selection and the initial My Books form are server-rendered
	// controls; these requests deliberately omit HX-Request.
	settings := perform(t, h, http.MethodGet, "/settings", nil, aliceCookies)
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `name="language"`) || !strings.Contains(settings.Body.String(), `name="csrf_token"`) {
		t.Fatalf("initial settings=%d %s", settings.Code, settings.Body.String())
	}
	for _, language := range []string{"de", "it"} {
		added := perform(t, h, http.MethodPost, "/settings/languages", url.Values{"csrf_token": {csrf}, "language": {language}}, aliceCookies)
		if added.Code != http.StatusSeeOther {
			t.Fatalf("add %s language=%d %s", language, added.Code, added.Body.String())
		}
	}
	if _, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "Haus", "NOUN"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutKnownVocabulary(ctx, alice.ID, "it", "casa", "NOUN"); err != nil {
		t.Fatal(err)
	}

	legacySource := (*string)(nil)
	legacyDeck, err := store.PutDeck(ctx, alice.ID, "de", "Legacy generated deck")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "legacy", UPOS: "ADJ", FirstDeckID: legacyDeck.ID, FirstSourceMaterialID: legacySource}); err != nil {
		t.Fatal(err)
	}
	primaryGenerated := []string{"residual", "graduated"}
	primaryDeck, err := store.PutDeck(ctx, alice.ID, "de", "Migration primary deck")
	if err != nil {
		t.Fatal(err)
	}
	for _, lemma := range primaryGenerated {
		if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", FirstDeckID: primaryDeck.ID, FirstSourceMaterialID: &source.ID}); err != nil {
			t.Fatal(err)
		}
	}
	// Keep the intended verb identity distinct from the residual noun while
	// using the same generated source/deck provenance.
	if _, err = store.Pool().Exec(ctx, `UPDATE generated_vocabulary SET upos='VERB' WHERE owner_id=$1 AND canonical_lemma='graduated'`, alice.ID); err != nil {
		t.Fatal(err)
	}
	campaign, err := store.CreateLearningCampaign(ctx, alice.ID, source.ID, preparation.ID)
	if err != nil {
		t.Fatal(err)
	}
	campaign, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, campaign.ID, migrationCampaignExpectedState(campaign), domain.BookReading, domain.DeckStudying)
	if err != nil {
		t.Fatal(err)
	}

	journey, err := store.GetReadingJourney(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddToReadingJourney(ctx, alice.ID, book.ID, journey.Revision); err != nil {
		t.Fatal(err)
	}
	journey, _ = store.GetReadingJourney(ctx, alice.ID)
	if _, err = store.AddToReadingJourney(ctx, alice.ID, secondBook.ID, journey.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, alice.ID, book.ID); err != nil {
		t.Fatal(err)
	}

	beforeCoverage, err := analysisinsights.NewService(store).Coverage(ctx, alice.ID, corpus.ID)
	if err != nil || beforeCoverage.KnownTokenCount != 1 || beforeCoverage.ActiveCampaignTokenCount != 2 {
		t.Fatalf("current versus conditional coverage before finish=%+v err=%v", beforeCoverage, err)
	}
	projectionBefore, err := analysisinsights.NewService(store).JourneyProjection(ctx, alice.ID, "de")
	if err != nil || len(projectionBefore.LearnerOrder) != 2 || len(projectionBefore.ConditionalAdvisoryOrder) != 2 {
		t.Fatalf("projection before finish=%+v err=%v", projectionBefore, err)
	}
	learnerOrderBefore := []string{projectionBefore.LearnerOrder[0].BookID, projectionBefore.LearnerOrder[1].BookID}

	// Stale writes, missing CSRF, cross-owner references, and invalid progress
	// are rejected without changing the accepted state.
	if _, err = store.AddToReadingJourney(ctx, alice.ID, secondBook.ID, journey.Revision); !errors.Is(err, persistence.ErrJourneyStale) {
		t.Fatalf("stale Journey write=%v", err)
	}
	if _, err = store.ChangePrimaryGoal(ctx, alice.ID, secondBook.ID, "stale-book"); !errors.Is(err, persistence.ErrGoalStale) {
		t.Fatalf("stale Goal write=%v", err)
	}
	if _, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, campaign.ID, migrationCampaignExpectedState(campaign), domain.BookReading, domain.DeckQueued); !errors.Is(err, persistence.ErrInvalidTransition) {
		t.Fatalf("invalid Campaign transition=%v", err)
	}
	if missingCSRF := perform(t, h, http.MethodPost, "/goal/finish", url.Values{"expected_goal_book_id": {book.ID}}, aliceCookies); missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("finish without CSRF=%d", missingCSRF.Code)
	}
	bobCookies, bobCSRF := loginCookies(t, h, "migration-bob", "bob-password")
	foreign := perform(t, h, http.MethodPost, "/goal/books/"+book.ID, url.Values{"csrf_token": {bobCSRF}, "expected_goal_book_id": {""}}, bobCookies)
	if foreign.Code != http.StatusSeeOther || !strings.Contains(foreign.Header().Get("Location"), "not+available") {
		t.Fatalf("cross-owner Goal mutation=%d location=%q", foreign.Code, foreign.Header().Get("Location"))
	}
	if goal, goalErr := store.GetPrimaryGoal(ctx, bob.ID); goalErr != nil || goal.BookID != "" {
		t.Fatalf("cross-owner Goal state=%+v err=%v", goal, goalErr)
	}
	if bobJourney, journeyErr := store.GetReadingJourney(ctx, bob.ID); journeyErr != nil || len(bobJourney.Entries) != 0 {
		t.Fatalf("cross-owner Journey state=%+v err=%v", bobJourney, journeyErr)
	}

	finished := perform(t, h, http.MethodPost, "/goal/finish", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {book.ID}}, aliceCookies)
	if finished.Code != http.StatusOK {
		t.Fatalf("reading finish=%d %s", finished.Code, finished.Body.String())
	}
	for _, want := range []string{"Reading finished", "No vocabulary was added to known vocabulary", "Where next?", "No new Primary Goal has been selected"} {
		if !strings.Contains(finished.Body.String(), want) {
			t.Errorf("finish receipt missing %q: %s", want, finished.Body.String())
		}
	}
	afterReading, err := store.GetLearningCampaign(ctx, alice.ID, campaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterReading.Status != domain.CampaignActive || afterReading.BookProgress != domain.BookFinished || afterReading.DeckProgress != domain.DeckStudying || afterReading.VocabularyGraduatedAt != nil {
		t.Fatalf("reading finish changed vocabulary state=%+v", afterReading)
	}
	active, err := store.ListActiveLearningCampaignVocabulary(ctx, alice.ID, "de")
	if err != nil || len(active) != 2 {
		t.Fatalf("residual reservation after reading=%+v err=%v", active, err)
	}
	knownAfterReading, err := store.ListKnownVocabulary(ctx, alice.ID, "de")
	if err != nil || len(knownAfterReading) != 1 {
		t.Fatalf("reading finish changed known vocabulary=%+v err=%v", knownAfterReading, err)
	}

	review := perform(t, h, http.MethodPost, "/campaigns/"+campaign.ID+"/deck-reviewed", url.Values{
		"csrf_token": {csrf}, "expected_campaign_status": {string(afterReading.Status)}, "expected_book_progress": {string(afterReading.BookProgress)}, "expected_deck_progress": {string(afterReading.DeckProgress)},
	}, aliceCookies)
	if review.Code != http.StatusSeeOther || !strings.Contains(review.Header().Get("Location"), "Campaign+complete") {
		t.Fatalf("review transition=%d location=%q", review.Code, review.Header().Get("Location"))
	}
	completed, err := store.GetLearningCampaign(ctx, alice.ID, campaign.ID)
	if err != nil || completed.Status != domain.CampaignComplete || completed.VocabularyGraduatedAt == nil {
		t.Fatalf("graduation campaign=%+v err=%v", completed, err)
	}
	graduatedAt := *completed.VocabularyGraduatedAt
	known, err := store.ListKnownVocabulary(ctx, alice.ID, "de")
	if err != nil || len(known) != 3 {
		t.Fatalf("graduated known vocabulary=%+v err=%v", known, err)
	}
	provenance := map[string]string{}
	for _, item := range known {
		provenance[item.CanonicalLemma] = item.Provenance
	}
	if provenance["Haus"] != "Explicitly recorded" || provenance["residual"] != "Graduated from completed campaign" || provenance["graduated"] != "Graduated from completed campaign" || provenance["legacy"] != "" {
		t.Fatalf("graduation provenance=%v", provenance)
	}
	legacy, err := store.ListLegacyGeneratedVocabulary(ctx, alice.ID, "de")
	if err != nil || len(legacy) != 1 || legacy[0].CanonicalLemma != "legacy" {
		t.Fatalf("legacy generated state after graduation=%+v err=%v", legacy, err)
	}
	graduatedRows, err := store.ListLearningCampaignVocabulary(ctx, alice.ID, campaign.ID)
	if err != nil || len(graduatedRows) != 2 {
		t.Fatalf("campaign snapshot=%+v err=%v", graduatedRows, err)
	}
	for _, row := range graduatedRows {
		if row.GraduatedAt == nil || row.CampaignID != campaign.ID {
			t.Fatalf("unlinked graduation row=%+v", row)
		}
	}

	completedAgain, err := store.UpdateLearningCampaignProgress(ctx, alice.ID, campaign.ID, migrationCampaignExpectedState(completed), domain.BookFinished, domain.DeckReviewed)
	if err != nil || completedAgain.VocabularyGraduatedAt == nil || !completedAgain.VocabularyGraduatedAt.Equal(graduatedAt) {
		t.Fatalf("idempotent graduation=%+v err=%v", completedAgain, err)
	}
	knownAgain, _ := store.ListKnownVocabulary(ctx, alice.ID, "de")
	if len(knownAgain) != len(known) {
		t.Fatalf("idempotent graduation duplicated known vocabulary=%+v", knownAgain)
	}
	afterCoverage, err := analysisinsights.NewService(store).Coverage(ctx, alice.ID, corpus.ID)
	if err != nil || afterCoverage.KnownTokenCount != 3 || afterCoverage.ActiveCampaignTokenCount != 0 {
		t.Fatalf("current coverage after graduation=%+v err=%v", afterCoverage, err)
	}
	projectionAfter, err := analysisinsights.NewService(store).JourneyProjection(ctx, alice.ID, "de")
	if err != nil || len(projectionAfter.ConditionalAdvisoryOrder) != 0 || len(projectionAfter.LearnerOrder) != len(learnerOrderBefore) {
		t.Fatalf("projection after graduation=%+v err=%v", projectionAfter, err)
	}
	for i, item := range projectionAfter.LearnerOrder {
		if item.BookID != learnerOrderBefore[i] {
			t.Fatalf("projection changed learner order[%d]=%q want %q", i, item.BookID, learnerOrderBefore[i])
		}
	}
	goal, err := store.GetPrimaryGoal(ctx, alice.ID)
	if err != nil || goal.BookID != book.ID || goal.ReadingFinishedAt == nil {
		t.Fatalf("finish auto-advanced Goal=%+v err=%v", goal, err)
	}

	// Failed analysis retry and prepared-deck retry remain explicit operations.
	failedJob := perform(t, h, http.MethodPost, "/jobs/43/retry", url.Values{"csrf_token": {csrf}}, aliceCookies)
	if failedJob.Code != http.StatusSeeOther || !strings.Contains(failedJob.Header().Get("Location"), "Analysis+retry+submitted") {
		t.Fatalf("analysis retry=%d location=%q", failedJob.Code, failedJob.Header().Get("Location"))
	}
	retryPrep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: secondSource.ID, Filename: "retry.apkg", DeckName: "Retry deck", ContentHash: "retry-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimDeckPreparation(ctx, alice.ID, retryPrep.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.FailDeckPreparation(ctx, alice.ID, retryPrep.ID, "temporary provider failure"); err != nil {
		t.Fatal(err)
	}
	requeued, err := store.RetryDeckPreparation(ctx, alice.ID, retryPrep.ID)
	if err != nil || requeued.State != domain.DeckPreparationQueued || requeued.Error != "" {
		t.Fatalf("deck retry=%+v err=%v", requeued, err)
	}

	itSettings := perform(t, h, http.MethodGet, "/settings?language=it", nil, aliceCookies)
	if itSettings.Code != http.StatusOK || !strings.Contains(itSettings.Body.String(), "casa") || !strings.Contains(itSettings.Body.String(), "Explicitly recorded") {
		t.Fatalf("explicit Italian settings=%d %s", itSettings.Code, itSettings.Body.String())
	}
	degradedHandler := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, Capabilities: staticCapabilities{err: errors.New("NLP unavailable")}, SessionLifetime: time.Hour})
	degraded := perform(t, degradedHandler, http.MethodGet, "/settings", nil, aliceCookies)
	if degraded.Code != http.StatusOK || !strings.Contains(degraded.Body.String(), "NLP language discovery is temporarily unavailable") || strings.Contains(degraded.Body.String(), `action="/settings/languages"`) {
		t.Fatalf("degraded settings=%d %s", degraded.Code, degraded.Body.String())
	}
	if blocked := perform(t, degradedHandler, http.MethodPost, "/settings/languages", url.Values{"csrf_token": {csrf}, "language": {"fr"}}, aliceCookies); blocked.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded language mutation=%d", blocked.Code)
	}
}

func seedMigrationAnalyzedBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, suffix, title string, lemmas []domain.LemmaOccurrence) (domain.Book, domain.SourceMaterial, domain.Corpus, domain.DeckPreparation) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	text := suffix + " content"
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner, Language: "de", SourceIdentifier: "migration-" + suffix, Title: title, MediaType: "application/epub+zip", Content: []byte(text), FullText: text}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, suffix), Order: 0, SpineIndex: 0, ManifestID: suffix, Text: text, EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, units, err := store.GetExtractedUnitSnapshot(ctx, owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.EPUBReviewedScopeSnapshot{SchemaVersion: 1, ScopeID: uuid.NewMD5(uuid.Nil, []byte("migration-scope-"+owner+"-"+suffix)).String(), OwnerID: owner, SourceMaterialID: source.ID, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: units.SchemaVersion}, Classifier: domain.EPUBClassifierIdentity{Name: "migration-fixture", Version: "1"}, SelectionMode: domain.EPUBScopeSelectionRecommended, SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: units.Units[0].ID, Order: 0}}}
	if _, err = store.CreateEPUBReviewedScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if err = store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "migration", NormalizationVersion: "1", AnalyzerName: "migration-fixture", AnalyzerVersion: "1"}, toSharedLemmas(lemmas)); err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, owner, source.ID, source.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	var tokenCount int64
	for _, lemma := range lemmas {
		tokenCount += lemma.OccurrenceCount
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE corpora SET reviewed_scope_id=$2,analyzable_token_count=$3,distinct_lemma_count=$4,sentence_count=1,normalized_token_count=$3,empty_sentence_count=0,median_sentence_token_count=1,p90_sentence_token_count=1,long_sentence_count=0 WHERE owner_id=$1 AND id=$5`, owner, scope.ScopeID, tokenCount, len(lemmas), corpus.ID); err != nil {
		t.Fatal(err)
	}
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source.ID, Filename: suffix + ".apkg", DeckName: "Migration " + suffix, ContentHash: source.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	if preparation, err = store.ClaimDeckPreparation(ctx, owner, preparation.ID); err != nil {
		t.Fatal(err)
	}
	preparation, err = store.CompleteDeckPreparation(ctx, owner, preparation.ID, domain.DeckPreparation{Artifact: []byte("migration-" + suffix), Filename: suffix + ".apkg", DeckName: "Migration " + suffix, TotalCards: len(lemmas)})
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	for _, book := range books {
		if book.Book.ID == bookID {
			return book
		}
	}
	t.Fatalf("book %q absent from My Books: %+v", bookID, books)
	return domain.MyBook{}
}

func migrationCampaignExpectedState(campaign domain.LearningCampaign) persistence.LearningCampaignExpectedState {
	return persistence.LearningCampaignExpectedState{Status: campaign.Status, BookProgress: campaign.BookProgress, DeckProgress: campaign.DeckProgress}
}
