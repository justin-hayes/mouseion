package webapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Empty(t, response.DownloadURL)
}

func TestDeckPreparationReturnURLUsesResolvedJourneyBookID(t *testing.T) {
	tests := []struct {
		name   string
		action deckJourneyActionView
		want   string
	}{
		{name: "journey member", action: deckJourneyActionView{BookID: "book-1", State: deckJourneyMember}, want: "/journey/book-1"},
		{name: "primary goal", action: deckJourneyActionView{BookID: "book-1", State: deckJourneyGoal}, want: "/journey/book-1"},
		{name: "unresolved", action: emptyDeckJourneyAction()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, preparationReturnURL(test.action), test.name)
		})
	}
}

func TestReachablePreparationReturnURLRequiresCurrentAnalysisAndBookLanguageJourney(t *testing.T) {
	store := &journeyIntentStore{
		deckJourneyActionStore: &deckJourneyActionStore{journey: domain.ReadingJourney{Entries: []domain.ReadingJourneyEntry{{BookID: "book-1", Position: 1}}}},
		detail:                 domain.MyBook{Book: domain.Book{ID: "book-1", LanguageTag: "de"}, Acquired: &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-1", MediaType: "application/epub+zip", ContentRevisionID: "revision-1", ContentSnapshotID: "snapshot-1"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "run-1", CorpusID: "corpus-1"}},
	}
	h := &Handler{services: Services{Store: store}}
	action := deckJourneyActionView{BookID: "book-1", State: deckJourneyNotMember}
	got, err := h.reachablePreparationReturnURL(context.Background(), "owner-1", action)
	require.NoError(t, err)
	assert.Equal(t, "/journey/book-1", got)

	store.detail.Acquired.AnalysisState = "failed"
	got, err = h.reachablePreparationReturnURL(context.Background(), "owner-1", action)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}
