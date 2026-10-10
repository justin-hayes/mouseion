package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repreparingPreparedDeck struct {
	fixtures.PreparedDeck
}

func (repreparingPreparedDeck) Get(_ context.Context, owner, id string) (domain.DeckPreparation, error) {
	return domain.DeckPreparation{
		ID: id, OwnerID: owner, SourceMaterialID: fixtures.SourceID,
		AnalysisRunID: fixtures.ResultRunID, BookID: fixtures.BookID,
		SnapshotID: "fixture-de-goal-snapshot", State: domain.DeckPreparationReady,
	}, nil
}

func (repreparingPreparedDeck) Retry(context.Context, string, string) (prepareddeck.Handle, error) {
	return prepareddeck.Handle{Preparation: domain.DeckPreparation{ID: "new-preparation", State: domain.DeckPreparationQueued}, JobID: 9}, nil
}

func (repreparingPreparedDeck) Reprepare(context.Context, string, string) (prepareddeck.Handle, error) {
	return prepareddeck.Handle{Preparation: domain.DeckPreparation{ID: "new-preparation", State: domain.DeckPreparationQueued}, JobID: 9}, nil
}

func TestPreparationResponseExposesPhaseCountsWithoutProviderIdentity(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{
		ID: "preparation-1", State: domain.DeckPreparationPreparing, Phase: "waiting",
		Error: "raw provider id file-secret and source sentence", FailureClass: "reconciliation", BatchAge: 2 * time.Hour,
		TotalCards: 4, CardsWithEnglish: 2, CardsWithContextualSentenceTranslations: 1, CardsWithFallbackGloss: 1,
		TranslationEligible: 4, TranslationDone: 2, TranslationPending: 1, TranslationRetrying: 1, TranslationFailed: 1,
		BatchChunkCount: 2, BatchPollingChunks: 1, BatchRequestCount: 4, BatchCompletedRequests: 2, BatchFailedRequests: 1, BatchExpiredRequests: 1,
	})
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	text := strings.ToLower(string(encoded))
	for _, prohibited := range []string{"batch_id", "input_file_id", "output_file_id", "error_file_id", "raw", "prompt", "response"} {
		assert.False(t, strings.Contains(text, prohibited), "status contains prohibited data %q: %s", prohibited, encoded)
	}
	assert.Equal(t, "waiting", response.Phase)
	assert.Equal(t, int64(7200), response.Batch.AgeSeconds)
	assert.Equal(t, 1, response.Translation.Retrying)
	assert.Equal(t, 1, response.Batch.Expired)
	assert.Nil(t, response.Completeness.CardsWithFallbackGloss)
	assert.NotContains(t, string(encoded), `"cards_with_fallback_gloss"`)
}

func TestReadyPreparationResponseExposesFallbackGlossCount(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{State: domain.DeckPreparationReady, CardsWithFallbackGloss: 1})

	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Equal(t, 1, *response.Completeness.CardsWithFallbackGloss)
	assert.Contains(t, string(encoded), `"cards_with_fallback_gloss":1`)
}

func TestReadyPreparationResponseExposesContextualGlossInferenceCount(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{
		State: domain.DeckPreparationReady, ContextualGlossesReported: true,
		ContextualGlosses: 4, ContextOnlyGlosses: 2,
	})
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Equal(t, 4, *response.Completeness.CardsWithContextualGloss)
	assert.Equal(t, 2, *response.Completeness.ContextOnlyGlosses)
	assert.Contains(t, string(encoded), `"cards_with_contextual_gloss":4`)
	assert.Contains(t, string(encoded), `"context_only_glosses":2`)
	assert.NotContains(t, string(encoded), `"cards_with_fallback_gloss"`)
}

func TestPreparationResponseExposesTargetAndReasonForMeaningOmissions(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{
		State:            domain.DeckPreparationReady,
		MeaningOmissions: []domain.DeckPreparationMeaningOmission{{TargetWord: "Bank", Reason: "The sentence does not distinguish the meanings."}},
	})
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"meaning_omissions":[{"target":"Bank","reason":"The sentence does not distinguish the meanings."}]`)
}

func TestPreparationResponseExposesFrozenEvidenceCoverageAndTruncation(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{State: domain.DeckPreparationPreparing, EvidenceCoverage: []domain.DeckPreparationEvidenceCoverage{{Source: "wiktionary", Configured: true, Selected: 5, Matched: 3, Candidates: 12, Omitted: 4}}})
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"evidence_coverage":[{"source":"wiktionary","configured":true,"selected":5,"matched":3,"candidates":12,"omitted_candidates":4}]`)
	assert.NotContains(t, string(encoded), "private corpus")
}

func TestPreparationFailureMessageUsesBoundedActionableClasses(t *testing.T) {
	for class, want := range map[string]string{
		"ambiguous_submission": "could not be confirmed",
		"validation":           "could not be verified",
		"provider":             "temporarily unavailable",
		"unknown":              "could not be completed",
	} {
		got := preparationFailureMessage(class)
		assert.True(t, strings.Contains(got, want), "class=%q message=%q want %q", class, got, want)
	}
	response := preparationResponse(domain.DeckPreparation{State: domain.DeckPreparationFailed, Error: "raw provider response file-secret", FailureClass: "provider"})
	assert.False(t, strings.Contains(strings.ToLower(response.Error), "raw provider response") || strings.Contains(response.Error, "file-secret"), "failure response leaked raw error: %q", response.Error)
}

func TestEmptyReadyPreparationDoesNotExposeDownload(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{ID: "empty", State: domain.DeckPreparationReady})
	assert.Equal(t, "", response.DownloadURL)
}

func TestQualityOmittedZeroCardPreparationExposesDownload(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{ID: "omitted", State: domain.DeckPreparationReady, QualityOmissions: 1})
	assert.NotEmpty(t, response.DownloadURL, "quality-omitted zero-card preparation has no download URL")
}

func TestPreparationResponseSurfacesUnrecoverableRenderInputs(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{
		ID: "legacy", State: domain.DeckPreparationReady,
		Error: domain.DeckPreparationRequiresRepreparationError, TotalCards: 1,
	})

	assert.Equal(t, domain.DeckPreparationRequiresRepreparationError, response.Error)
	assert.False(t, response.Ready)
	assert.Empty(t, response.DownloadURL)
}

func TestExplicitReprepareRedirectsToTheNewPreparationGeneration(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)
	requireHandler(t, h).services.PreparedDeck = repreparingPreparedDeck{}

	response := readingTestRequest(t, h, "/deck-preparations/old-preparation/reprepare", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_snapshot_id": {"fixture-de-goal-snapshot"},
	}, cookies)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/deck-preparations/new-preparation/status", response.Header().Get("Location"))
}

func TestReachablePreparationReturnURLRequiresCurrentAnalysisAndBookLanguageReading(t *testing.T) {
	store := &readingIntentStore{
		deckReadingActionStore: &deckReadingActionStore{},
		detail:                 domain.MyBook{Book: domain.Book{ID: "book-1", LanguageTag: "de"}, Disposition: domain.BookDispositionToRead, Acquired: &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-1", MediaType: "application/epub+zip", ContentRevisionID: "revision-1", ContentSnapshotID: "snapshot-1"}, Signals: testAnalyzed, AnalysisRunID: "run-1", CorpusID: "corpus-1"}},
	}
	h := &Handler{services: Services{Store: storeDependencies(store)}}
	action := deckReadingActionView{BookID: "book-1", State: deckReadingNotMember}
	got, err := h.reachablePreparationReturnURL(context.Background(), "owner-1", action)
	require.NoError(t, err)
	assert.Equal(t, "/reading#journey-book-book-1", got)

	crossLanguageContext := context.WithValue(context.Background(), shellViewContextKey{}, &shellView{ActiveLanguage: "it"})
	got, err = h.reachablePreparationReturnURL(crossLanguageContext, "owner-1", action)
	require.ErrorIs(t, err, errPreparationOtherLanguage)
	assert.Equal(t, "", got)

	store.detail.Acquired.Signals = testFailed
	got, err = h.reachablePreparationReturnURL(context.Background(), "owner-1", action)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}
