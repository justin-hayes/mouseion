package webapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestPreparationResponseExposesPhaseCountsWithoutProviderIdentity(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{
		ID: "preparation-1", State: domain.DeckPreparationPreparing, Phase: "waiting",
		Error: "raw provider id file-secret and source sentence", FailureClass: "reconciliation", BatchAge: 2 * time.Hour,
		TotalCards: 4, CardsWithEnglish: 2, CardsWithContextualSentenceTranslations: 1,
		TranslationEligible: 4, TranslationDone: 2, TranslationPending: 1, TranslationRetrying: 1, TranslationFailed: 1,
		BatchChunkCount: 2, BatchPollingChunks: 1, BatchRequestCount: 4, BatchCompletedRequests: 2, BatchFailedRequests: 1, BatchExpiredRequests: 1,
	})
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(encoded))
	for _, prohibited := range []string{"batch_id", "input_file_id", "output_file_id", "error_file_id", "raw", "prompt", "response"} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("status contains prohibited data %q: %s", prohibited, encoded)
		}
	}
	if response.Phase != "waiting" || response.Batch.AgeSeconds != 7200 || response.Translation.Retrying != 1 || response.Batch.Expired != 1 {
		t.Fatalf("status=%+v", response)
	}
}

func TestPreparationFailureMessageUsesBoundedActionableClasses(t *testing.T) {
	for class, want := range map[string]string{
		"ambiguous_submission": "could not be confirmed",
		"validation":           "could not be verified",
		"provider":             "temporarily unavailable",
		"unknown":              "could not be completed",
	} {
		if got := preparationFailureMessage(class); !strings.Contains(got, want) {
			t.Errorf("class=%q message=%q want %q", class, got, want)
		}
	}
	response := preparationResponse(domain.DeckPreparation{State: domain.DeckPreparationFailed, Error: "raw provider response file-secret", FailureClass: "provider"})
	if strings.Contains(strings.ToLower(response.Error), "raw provider response") || strings.Contains(response.Error, "file-secret") {
		t.Fatalf("failure response leaked raw error: %q", response.Error)
	}
}

func TestEmptyReadyPreparationDoesNotExposeDownload(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{ID: "empty", State: domain.DeckPreparationReady})
	if response.DownloadURL != "" {
		t.Fatalf("empty preparation download URL = %q", response.DownloadURL)
	}
}

func TestQualityOmittedZeroCardPreparationExposesDownload(t *testing.T) {
	response := preparationResponse(domain.DeckPreparation{ID: "omitted", State: domain.DeckPreparationReady, QualityOmissions: 1})
	if response.DownloadURL == "" {
		t.Fatal("quality-omitted zero-card preparation has no download URL")
	}
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
			if got := preparationReturnURL(test.action); got != test.want {
				t.Fatalf("return URL=%q, want %q", got, test.want)
			}
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
	if err != nil || got != "/journey/book-1" {
		t.Fatalf("reachable URL=%q err=%v", got, err)
	}

	store.detail.Acquired.AnalysisState = "failed"
	got, err = h.reachablePreparationReturnURL(context.Background(), "owner-1", action)
	if err != nil || got != "" {
		t.Fatalf("incomplete analysis URL=%q err=%v", got, err)
	}
}
