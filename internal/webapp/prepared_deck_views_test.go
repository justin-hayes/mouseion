package webapp

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestBookPageOffersDeckPreparationWithConsentDisclosure(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{
		Source:         domain.SourceMaterial{ID: "book-372", Title: "A Book", Language: "de"},
		AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "run-372", CorpusID: "corpus-372",
	}
	if err := BookPageWithHistoryAndPreparation(domain.User{Username: "learner"}, "csrf", book, nil, true, nil, "", nil, emptyDeckJourneyAction()).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		`id="deck-preparation-heading"`,
		`action="/books/book-372/deck/preparations"`,
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
			want:     []string{"Deck ready", "Download deck", "Completeness"},
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
			if err := DeckPreparationStatus("csrf", preparation, "/books/book-372", emptyDeckJourneyAction()).Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			html := output.String()
			if !strings.Contains(html, "Return to book") && test.name == "queued" {
				t.Errorf("status missing return-to-book link: %s", html)
			}
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

func TestEmptyReadyDeckShowsRecurringVocabularyEmptyState(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-empty", SourceMaterialID: "book-empty", State: domain.DeckPreparationReady,
		DeckName: "Mouseion::de::A Book", Filename: "A Book.apkg",
	}
	var output bytes.Buffer
	if err := DeckPreparationStatus("csrf", preparation, "/books/book-empty", emptyDeckJourneyAction()).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		"No recurring vocabulary",
		"This book has no recurring vocabulary to study",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("empty deck state missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{"Completeness", "Download deck", "0 cards"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("empty deck state contains %q: %s", unwanted, html)
		}
	}
}

func TestZeroCardReadyDeckWithQualityOmissionsKeepsCompleteness(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-omitted", SourceMaterialID: "book-omitted", State: domain.DeckPreparationReady,
		DeckName: "Mouseion::de::A Book", Filename: "A Book.apkg", QualityOmissions: 1,
	}
	var output bytes.Buffer
	if err := DeckPreparationStatus("csrf", preparation, "", emptyDeckJourneyAction()).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if strings.Contains(html, "No recurring vocabulary") {
		t.Fatalf("quality omissions were presented as missing vocabulary: %s", html)
	}
	for _, want := range []string{"Completeness", "0 cards", "Download deck"} {
		if !strings.Contains(html, want) {
			t.Errorf("quality omission state missing %q: %s", want, html)
		}
	}
}

func TestReadyDeckRendersTruthfulJourneyStates(t *testing.T) {
	preparation := domain.DeckPreparation{ID: "preparation-372", SourceMaterialID: "book-372", AnalysisRunID: "run-372", State: domain.DeckPreparationReady, DeckName: "Mouseion::de::The Exact Book", Filename: "The Exact Book.apkg", TotalCards: 10}
	tests := []struct {
		name  string
		state deckJourneyState
		want  []string
		omit  []string
	}{
		{name: "not in Journey", state: deckJourneyNotMember, want: []string{"Not in Reading Journey", "Add to Reading Journey", `method="post" action="/journey/books/book-372/add"`, `name="expected_revision"`}, omit: []string{"View this Journey entry", "Primary Goal"}},
		{name: "already in Journey", state: deckJourneyMember, want: []string{"In Reading Journey", "This book is already in your Reading Journey", `href="/journey#journey-book-book-372"`}, omit: []string{"Add to Reading Journey", "Primary Goal"}},
		{name: "Primary Goal", state: deckJourneyGoal, want: []string{"Primary Goal", "This deck is preparation for your current Primary Goal", "View Primary Goal in Reading Journey"}, omit: []string{"Add to Reading Journey", "View this Journey entry"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			action := deckJourneyActionView{BookID: preparation.SourceMaterialID, PreparationID: preparation.ID, Revision: 9, State: test.state}
			if err := DeckPreparationStatus("csrf", preparation, "", action).Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			html := output.String()
			for _, want := range test.want {
				if !strings.Contains(html, want) {
					t.Errorf("state missing %q: %s", want, html)
				}
			}
			for _, omit := range test.omit {
				if strings.Contains(html, omit) {
					t.Errorf("state contains %q: %s", omit, html)
				}
			}
			if strings.Contains(html, "campaign operations") {
				t.Errorf("ready state exposed Campaign queue copy: %s", html)
			}
		})
	}
}
