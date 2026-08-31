package webapp

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestAnalysisResultOffersExactDeckPreparationWithConsentDisclosure(t *testing.T) {
	var output bytes.Buffer
	result := analysis.CompletedAnalysis{
		RunID: "run-372", SourceMaterialID: "book-372", ScopeID: "scope-372",
		Source: domain.SourceMaterial{ID: "book-372", Title: "A Book", Language: "de"},
		Corpus: domain.Corpus{ID: "corpus-372"},
	}
	if err := AnalysisResultPageWithPreparation(domain.User{Username: "learner"}, "csrf", result, nil, true, nil).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		`id="result-deck-heading"`,
		`action="/books/book-372/analyses/run-372/deck/preparations"`,
		`name="external_translation_consent"`,
		"outside Mouseion",
		"configured translation provider",
		"Without consent",
		"does not start a learning campaign",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("exact result missing deck contract %q: %s", want, html)
		}
	}
	if strings.Contains(html, `action="/jobs/`) {
		t.Error("exact result must not submit deck preparation through an operational job route")
	}
}

func TestDeckPreparationStatusHasServerRenderedLifecycle(t *testing.T) {
	tests := []struct {
		name     string
		state    domain.DeckPreparationState
		want     []string
		unwanted []string
		progress int
	}{
		{
			name:     "queued",
			state:    domain.DeckPreparationQueued,
			want:     []string{"Deck preparation queued", "exact analysis", "Cancel preparation"},
			unwanted: []string{"Retry preparation", `download=""`},
			progress: 0,
		},
		{
			name:     "preparing",
			state:    domain.DeckPreparationPreparing,
			want:     []string{"Deck preparation running", "Cancel preparation", `aria-busy="true"`},
			unwanted: []string{"Retry preparation"},
			progress: 50,
		},
		{
			name:     "ready",
			state:    domain.DeckPreparationReady,
			want:     []string{"Deck ready", "Download deck", "Review campaign operations", "Completeness"},
			unwanted: []string{"Cancel preparation", "Retry preparation", `hx-trigger="every 3s"`},
			progress: 100,
		},
		{
			name:     "failed",
			state:    domain.DeckPreparationFailed,
			want:     []string{"Deck preparation failed", "Action needed", "Retry preparation", "temporarily unavailable"},
			unwanted: []string{"Cancel preparation", `hx-trigger="every 3s"`},
			progress: 100,
		},
		{
			name:     "cancelled",
			state:    domain.DeckPreparationCancelled,
			want:     []string{"Deck preparation cancelled", "You can retry this exact analysis"},
			unwanted: []string{"Cancel preparation", `hx-trigger="every 3s"`},
			progress: 100,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			preparation := domain.DeckPreparation{ID: "preparation-372", SourceMaterialID: "book-372", AnalysisRunID: "run-372", State: test.state, FailureClass: "provider", TotalCards: 10, CardsWithEnglish: 8, CardsWithContextualSentenceTranslations: 6, QualityOmissions: 1}
			if err := DeckPreparationStatus("csrf", preparation, "/books/book-372/analyses/run-372").Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			html := output.String()
			for _, want := range test.want {
				if !strings.Contains(html, want) {
					t.Errorf("status missing %q: %s", want, html)
				}
			}
			for _, unwanted := range test.unwanted {
				if strings.Contains(html, unwanted) {
					t.Errorf("status contains %q: %s", unwanted, html)
				}
			}
			if !strings.Contains(html, `value="`+strconv.Itoa(test.progress)+`"`) {
				t.Errorf("status progress does not include %d: %s", test.progress, html)
			}
		})
	}
}
