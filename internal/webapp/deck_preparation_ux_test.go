package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func testCompletedAnalysisForDeck() analysis.CompletedAnalysis {
	return analysis.CompletedAnalysis{
		RunID: "run-deck-372", SourceMaterialID: "book-deck-372", ScopeID: "scope-deck-372",
		Source: domain.SourceMaterial{ID: "book-deck-372", Title: "The Exact Book", Language: "de"},
		Corpus: domain.Corpus{ID: "corpus-deck-372", SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "unit-1", Title: "Chapter One"}}},
	}
}

func renderDeckResult(t *testing.T, preparation *domain.DeckPreparation) string {
	t.Helper()
	var output bytes.Buffer
	if err := AnalysisResultPageWithPreparation(domain.User{Username: "learner"}, "csrf-372", testCompletedAnalysisForDeck(), nil, true, preparation).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestAnalysisResultProvidesExactNativeDeckPreparationForm(t *testing.T) {
	html := renderDeckResult(t, nil)
	for _, want := range []string{
		`method="post" action="/books/book-deck-372/analyses/run-deck-372/deck/preparations"`,
		`name="external_translation_consent"`,
		"English translation is optional",
		"sends each selected lemma and its example sentence",
		"does not start a learning campaign",
		"data-deck-preparation",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("exact result missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `action="/jobs/`) || strings.Contains(html, "Generate vocabulary deck") {
		t.Fatalf("result preparation is not bound to the exact result: %s", html)
	}
}

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
			want: []string{"Deck preparation queued", "The exact analysis is queued", `action="/deck-preparations/prep-queued/cancel"`, "Return to analysis result"},
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
			want: []string{"Deck preparation failed", "temporarily unavailable", `action="/deck-preparations/prep-failed/retry"`, "Optional external translation for the retry"},
			omit: []string{"Cancel preparation", "Download deck"},
		},
		{
			name: "ready",
			prep: domain.DeckPreparation{ID: "prep-ready", SourceMaterialID: "book-deck-372", AnalysisRunID: "run-deck-372", State: domain.DeckPreparationReady, DeckName: "Mouseion::de::The Exact Book", Filename: "The Exact Book.apkg", TotalCards: 12, CardsWithEnglish: 11, CardsWithContextualSentenceTranslations: 9, QualityOmissions: 1},
			want: []string{"Deck ready", "12 cards", "Download deck", `href="/deck-preparations/prep-ready/download"`, "Add to learning queue"},
			omit: []string{"Cancel preparation", "Retry preparation"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			html := renderDeckResult(t, &test.prep)
			statusStart := strings.Index(html, `<section data-deck-preparation`)
			if statusStart < 0 {
				t.Fatal("rendered result has no deck preparation status")
			}
			statusHTML := html[statusStart:]
			for _, want := range test.want {
				if !strings.Contains(statusHTML, want) {
					t.Errorf("status missing %q: %s", want, statusHTML)
				}
			}
			for _, unwanted := range test.omit {
				if strings.Contains(statusHTML, unwanted) {
					t.Errorf("status unexpectedly contains %q: %s", unwanted, statusHTML)
				}
			}
			if test.name == "ready" && strings.Contains(html, `method="post" action="/books/book-deck-372/analyses/run-deck-372/deck/preparations"`) {
				t.Error("ready preparation unexpectedly retained the submission form")
			}
		})
	}
}

func TestPreparationProgressUsesDurableTranslationCounts(t *testing.T) {
	if got := preparationProgress(domain.DeckPreparation{State: domain.DeckPreparationPreparing, TranslationEligible: 4, TranslationDone: 2, TranslationFailed: 1}); got != 75 {
		t.Fatalf("progress=%d, want 75", got)
	}
	if got := preparationProgress(domain.DeckPreparation{State: domain.DeckPreparationReady}); got != 100 {
		t.Fatalf("ready progress=%d, want 100", got)
	}
}
