package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeckPreparationStatusRendersLifecycleAndRecoveryForms(t *testing.T) {
	tests := []struct {
		name string
		prep domain.DeckPreparation
		want []string
		omit []string
	}{
		{
			name: "queued",
			prep: domain.DeckPreparation{ID: "prep-queued", SourceMaterialID: "book-deck-372", AnalysisRunID: "run-deck-372", State: domain.DeckPreparationQueued, Phase: "queued"},
			want: []string{"Deck preparation queued", "Analysis is queued for Book deck preparation", `action="/deck-preparations/prep-queued/cancel"`},
			omit: []string{"Retry preparation", "Download deck"},
		},
		{
			name: "waiting",
			prep: domain.DeckPreparation{ID: "prep-waiting", SourceMaterialID: "book-deck-372", AnalysisRunID: "run-deck-372", State: domain.DeckPreparationPreparing, Phase: "waiting", TranslationEligible: 10, TranslationDone: 6, TranslationFailed: 1, TranslationPending: 3, BatchRequestCount: 10, BatchCompletedRequests: 7},
			want: []string{"Deck preparation running", "Waiting for Batch translation", "can take hours (up to 24h)", "6 complete", "7 of 10 requests complete", `action="/deck-preparations/prep-waiting/cancel"`},
			omit: []string{"Retry preparation", "Download deck"},
		},
		{
			name: "failed",
			prep: domain.DeckPreparation{ID: "prep-failed", SourceMaterialID: "book-deck-372", AnalysisRunID: "run-deck-372", State: domain.DeckPreparationFailed, FailureClass: "provider"},
			want: []string{"Deck preparation failed", "temporarily unavailable", "no retry can be submitted"},
			omit: []string{"Cancel preparation", "Download deck", `action="/deck-preparations/prep-failed/retry"`},
		},
		{
			name: "ready",
			prep: domain.DeckPreparation{ID: "prep-ready", SourceMaterialID: "book-deck-372", AnalysisRunID: "run-deck-372", State: domain.DeckPreparationReady, DeckName: "Mouseion::de::The Exact Book", Filename: "The Exact Book.apkg", TotalCards: 12, CardsWithEnglish: 11, CardsWithContextualSentenceTranslations: 9, QualityOmissions: 1},
			want: []string{"Deck ready", "12 cards", "Download deck", `href="/deck-preparations/prep-ready/download"`, "no new preparation can be submitted"},
			omit: []string{"Cancel preparation", "Retry preparation", "Re-prepare with current Meaning evidence", `action="/deck-preparations/prep-ready/reprepare"`},
		},
		{
			name: "re-preparation required",
			prep: domain.DeckPreparation{ID: "prep-reprepare", SourceMaterialID: "book-deck-372", AnalysisRunID: "run-deck-372", State: domain.DeckPreparationReady, Error: domain.DeckPreparationRequiresRepreparationError},
			want: []string{"Re-preparation required", "cannot be submitted again"},
			omit: []string{"Download deck", "Deck ready", "Re-prepare deck", `action="/deck-preparations/prep-reprepare/retry"`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, DeckPreparationStatus("csrf-372", test.prep, "", emptyDeckReadingAction()).Render(context.Background(), &output))
			statusHTML := output.String()
			for _, want := range test.want {
				assert.True(t, strings.Contains(statusHTML, want), "status missing %q: %s", want, statusHTML)
			}
			for _, unwanted := range test.omit {
				assert.False(t, strings.Contains(statusHTML, unwanted), "status unexpectedly contains %q: %s", unwanted, statusHTML)
			}
		})
	}
}

func TestPreparationProgressUsesDurableTranslationCounts(t *testing.T) {
	assert.Equal(t, 75, preparationProgress(domain.DeckPreparation{State: domain.DeckPreparationPreparing, TranslationEligible: 4, TranslationDone: 2, TranslationFailed: 1}))
	assert.Equal(t, 100, preparationProgress(domain.DeckPreparation{State: domain.DeckPreparationReady}))
}
