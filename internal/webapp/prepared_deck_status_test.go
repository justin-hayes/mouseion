package webapp

import (
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
