package webapp

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookPageOffersDeckPreparationWithConsentDisclosure(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{
		Source:         domain.SourceMaterial{ID: "book-372", Title: "A Book", Language: "de"},
		AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "run-372", CorpusID: "corpus-372",
	}
	require.NoError(t, BookPageWithOptions(domain.User{Username: "learner"}, "csrf", book, nil, true, "", journeyBookPageOptions(book), nil, emptyDeckJourneyAction()).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		`id="deck-preparation-heading"`,
		`action="/journey/books/book-372/deck/preparations"`,
		`name="external_translation_consent"`,
		"outside Mouseion",
		"configured translation provider",
		"Without consent",
		"does not start vocabulary study",
	} {
		assert.True(t, strings.Contains(html, want), "exact result missing deck contract %q: %s", want, html)
	}
	assert.False(t, strings.Contains(html, `action="/jobs/`), "exact result must not submit deck preparation through an operational job route")
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
			preparation := domain.DeckPreparation{ID: "preparation-372", SourceMaterialID: "book-372", AnalysisRunID: "run-372", State: test.state, FailureClass: "provider", TotalCards: 10, CardsWithEnglish: 8, CardsWithContextualSentenceTranslations: 6, CardsWithFallbackGloss: 2, QualityOmissions: 1}
			require.NoError(t, DeckPreparationStatus("csrf", preparation, "/journey/book-372", emptyDeckJourneyAction()).Render(context.Background(), &output))
			html := output.String()
			assert.True(t, strings.Contains(html, "Return to book") || test.name != "queued", "status missing return-to-book link: %s", html)
			for _, want := range test.want {
				assert.True(t, strings.Contains(html, want), "status missing %q: %s", want, html)
			}
			for _, unwanted := range test.unwanted {
				assert.False(t, strings.Contains(html, unwanted), "status contains %q: %s", unwanted, html)
			}
			assert.True(t, strings.Contains(html, `value="`+strconv.Itoa(test.progress)+`"`), "status progress does not include %d: %s", test.progress, html)
		})
	}
}

func TestDeckPreparationStatusPageUsesJourneyEntryForBothBackLinks(t *testing.T) {
	preparation := domain.DeckPreparation{ID: "prep-1", SourceMaterialID: "source-1", State: domain.DeckPreparationReady, TotalCards: 1}
	action := deckJourneyActionView{BookID: "book-1", State: deckJourneyMember}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatusPage(domain.User{Username: "learner"}, "csrf", preparation, "/journey/book-1", action).Render(context.Background(), &output))
	html := output.String()
	assert.Equal(t, 2, strings.Count(html, `href="/journey/book-1"`), "Journey entry link count=%d: %s", strings.Count(html, `href="/journey/book-1"`), html)
	assert.False(t, strings.Contains(html, "/books/source-1"), "status page contains retired book link: %s", html)
}

func TestDeckPreparationStatusPageOmitsBackLinksWithoutJourneyEntry(t *testing.T) {
	preparation := domain.DeckPreparation{ID: "prep-1", SourceMaterialID: "source-1", State: domain.DeckPreparationFailed}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatusPage(domain.User{Username: "learner"}, "csrf", preparation, "", emptyDeckJourneyAction()).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, "Return to book") || strings.Contains(html, `href="/books/source-1"`), "unreachable deck status page contains back link: %s", html)
}

func TestEmptyReadyDeckShowsRecurringVocabularyEmptyState(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-empty", SourceMaterialID: "book-empty", State: domain.DeckPreparationReady,
		DeckName: "Mouseion::de::A Book", Filename: "A Book.apkg",
	}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckJourneyAction()).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		"No recurring vocabulary",
		"This book has no recurring vocabulary to study",
	} {
		assert.True(t, strings.Contains(html, want), "empty deck state missing %q: %s", want, html)
	}
	for _, unwanted := range []string{"Completeness", "Download deck", "0 cards"} {
		assert.False(t, strings.Contains(html, unwanted), "empty deck state contains %q: %s", unwanted, html)
	}
}

func TestBookVocabularyStudyRendersReachableTransitions(t *testing.T) {
	tests := []struct {
		name        string
		preparation domain.DeckPreparation
		want        []string
		unwanted    []string
	}{
		{
			name:        "ready",
			preparation: domain.DeckPreparation{ID: "prep-study", State: domain.DeckPreparationReady, TotalCards: 2, VocabularyCount: 2},
			want:        []string{"Ready to study", "Start vocabulary study", "vocabulary for review", "Study this Book's vocabulary", `action="/journey/books/book-1/vocabulary-study"`},
			unwanted:    []string{"Confirm deck review", "Release study"},
		},
		{
			name:        "studying",
			preparation: domain.DeckPreparation{ID: "prep-study", State: domain.DeckPreparationReady, TotalCards: 2, VocabularyCount: 2, StudyingAt: studyTimePtr()},
			want:        []string{"Studying", "reserved", "not counted as known", "Confirm deck review", "Release study", `action="/journey/books/book-1/vocabulary-study/confirm"`, `action="/journey/books/book-1/vocabulary-study/release"`, "consequential transition"},
		},
		{
			name:        "reviewed",
			preparation: domain.DeckPreparation{ID: "prep-study", State: domain.DeckPreparationReady, TotalCards: 2, ReviewedAt: studyTimePtr(), GraduatedAt: studyTimePtr()},
			want:        []string{"Reviewed and graduated", "Vocabulary graduated"},
			unwanted:    []string{"Study this Book's vocabulary", "Confirm deck review", "Release study"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, BookVocabularyStudy("book-1", "csrf", test.preparation).Render(context.Background(), &output))
			html := output.String()
			for _, want := range test.want {
				assert.True(t, strings.Contains(html, want), "study view missing %q: %s", want, html)
			}
			for _, unwanted := range test.unwanted {
				assert.False(t, strings.Contains(html, unwanted), "study view contains %q: %s", unwanted, html)
			}
		})
	}
}

func TestBookPageRendersVocabularyStudyHistoryAlongsideCurrentStudy(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-history", Language: "de"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "run-history"}
	now := time.Date(2026, time.September, 10, 9, 0, 0, 0, time.UTC)
	page := journeyBookPageOptions(book)
	page.VocabularyStudyHistory = []domain.DeckPreparation{{
		ID: "old-study", DeckName: "Mouseion::de::Old deck", State: domain.DeckPreparationReady, TotalCards: 2, ReleasedAt: &now,
	}}
	current := domain.DeckPreparation{ID: "current-study", State: domain.DeckPreparationReady, TotalCards: 2, VocabularyCount: 2, StudyingAt: &now}
	var output bytes.Buffer
	require.NoError(t, BookPageWithOptions(domain.User{Username: "learner"}, "csrf", book, nil, false, "", page, &current, emptyDeckJourneyAction()).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{"This Book's vocabulary study", "Studying", "This Book's vocabulary-study history", "Mouseion::de::Old deck", "Released", "2026-09-10 09:00 UTC", `href="/deck-preparations/old-study/download"`} {
		assert.True(t, strings.Contains(html, want), "Book page missing %q: %s", want, html)
	}
}

func TestEmptyReadyDeckDoesNotOfferVocabularyStudy(t *testing.T) {
	var output bytes.Buffer
	preparation := domain.DeckPreparation{ID: "prep-empty", State: domain.DeckPreparationReady}
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-empty", Language: "de"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "run-empty", CorpusID: "corpus-empty"}
	require.NoError(t, BookPageWithOptions(domain.User{Username: "learner"}, "csrf", book, nil, false, "", journeyBookPageOptions(book), &preparation, emptyDeckJourneyAction()).Render(context.Background(), &output))
	assert.False(t, strings.Contains(output.String(), "Study this Book's vocabulary"), "empty deck offered a study action: %s", output.String())
}

func studyTimePtr() *time.Time {
	now := time.Now()
	return &now
}

func TestZeroCardReadyDeckWithQualityOmissionsKeepsCompleteness(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-omitted", SourceMaterialID: "book-omitted", State: domain.DeckPreparationReady,
		DeckName: "Mouseion::de::A Book", Filename: "A Book.apkg", QualityOmissions: 1,
	}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckJourneyAction()).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, "No recurring vocabulary"), "quality omissions were presented as missing vocabulary: %s", html)
	for _, want := range []string{"Completeness", "0 cards", "Download deck"} {
		assert.True(t, strings.Contains(html, want), "quality omission state missing %q: %s", want, html)
	}
}

func TestReadyDeckShowsFallbackGlossCountInCompleteness(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-fallback", SourceMaterialID: "book-fallback", State: domain.DeckPreparationReady,
		DeckName: "Mouseion::de::A Book", Filename: "A Book.apkg", TotalCards: 3,
		CardsWithEnglish: 3, CardsWithFallbackGloss: 1,
	}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckJourneyAction()).Render(context.Background(), &output))
	assert.Contains(t, output.String(), "1 with fallback gloss")
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
			require.NoError(t, DeckPreparationStatus("csrf", preparation, "", action).Render(context.Background(), &output))
			html := output.String()
			for _, want := range test.want {
				assert.True(t, strings.Contains(html, want), "state missing %q: %s", want, html)
			}
			for _, omit := range test.omit {
				assert.False(t, strings.Contains(html, omit), "state contains %q: %s", omit, html)
			}
			assert.False(t, strings.Contains(html, "campaign operations"), "ready state exposed Campaign queue copy: %s", html)
		})
	}
}
