package webapp

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadingDeckPreparationPageShowsRequiredProviderAndNoConsentControl(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-1126", Title: "The Exact Journey Book", Language: "de", ContentSnapshotID: "snapshot-1126"}}
	task := readingDeckPreparationView{Book: book, BookID: "book-1126", AnalysisRunID: "run-1126"}
	var output bytes.Buffer
	require.NoError(t, ReadingDeckPreparationPage(domain.User{Username: "learner"}, "csrf", task, "/reading#journey-book-book-1126").Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		"The Exact Journey Book",
		"run-1126",
		"snapshot-1126",
		`action="/reading/books/book-1126/deck/preparations"`,
		"configured translation provider",
		"public lexical evidence",
		`href="/reading#journey-book-book-1126"`,
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, "external_translation_consent")
	assert.NotContains(t, html, "Declining still permits local preparation")
}

func TestReadingDeckPreparationPageKeepsCurrentReadingRetrySnapshotBound(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-goal-1126", Title: "Goal Book", Language: "de", ContentSnapshotID: "snapshot-goal"}}
	task := readingDeckPreparationView{Book: book, BookID: "book-goal-1126", AnalysisRunID: "run-goal-1126", CurrentReading: true, CurrentReadingSnapshotID: "goal-snapshot-1126", CurrentReadingSnapshotSize: 2, Preparation: &domain.DeckPreparation{ID: "goal-prep-1126", SourceMaterialID: "source-goal-1126", AnalysisRunID: "run-goal-1126", SnapshotID: "goal-snapshot-1126", State: domain.DeckPreparationFailed}, Actions: deckPreparationActions{BookID: "book-goal-1126", OnCurrentReading: true, Submit: true}}
	var output bytes.Buffer
	require.NoError(t, ReadingDeckPreparationPage(domain.User{Username: "learner"}, "csrf", task, "/reading#journey-book-book-goal-1126").Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "goal-snapshot-1126")
	assert.Contains(t, html, `action="/reading/books/book-goal-1126/deck/retry"`)
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="goal-snapshot-1126"`)
	assert.NotContains(t, html, `action="/deck-preparations/goal-prep-1126/retry"`)
}

func TestReadingDeckPreparationPageRepreparesReadyCurrentReadingDeckBySnapshot(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-goal-reprepare", Title: "Goal Book", Language: "de", ContentSnapshotID: "snapshot-goal"}}
	task := readingDeckPreparationView{Book: book, BookID: "book-goal-reprepare", AnalysisRunID: "run-goal-reprepare", CurrentReading: true, CurrentReadingSnapshotID: "goal-snapshot-reprepare", CurrentReadingSnapshotSize: 2, Preparation: &domain.DeckPreparation{ID: "goal-prep-reprepare", SourceMaterialID: "source-goal-reprepare", AnalysisRunID: "run-goal-reprepare", SnapshotID: "goal-snapshot-reprepare", State: domain.DeckPreparationReady, Error: domain.DeckPreparationRequiresRepreparationError}, Actions: deckPreparationActions{BookID: "book-goal-reprepare", OnCurrentReading: true, Submit: true}}
	var output bytes.Buffer
	require.NoError(t, ReadingDeckPreparationPage(domain.User{Username: "learner"}, "csrf", task, "/reading#journey-book-book-goal-reprepare").Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "Re-preparation required")
	assert.Contains(t, html, `action="/reading/books/book-goal-reprepare/deck/retry"`)
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="goal-snapshot-reprepare"`)
	assert.NotContains(t, html, `action="/deck-preparations/goal-prep-reprepare/retry"`)
	assert.NotContains(t, html, "external_translation_consent")
}

func TestCurrentReadingDeckPreparationStatusRetriesBySnapshot(t *testing.T) {
	var output bytes.Buffer
	preparation := domain.DeckPreparation{ID: "goal-prep-failed", State: domain.DeckPreparationFailed}
	require.NoError(t, CurrentReadingDeckPreparationStatus("csrf", "book-goal", "snapshot-goal", preparation, deckPreparationActions{BookID: "book-goal", OnCurrentReading: true, Submit: true, Reprepare: true}).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `action="/reading/books/book-goal/deck/retry"`)
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="snapshot-goal"`)
	assert.NotContains(t, html, `action="/deck-preparations/goal-prep-failed/retry"`)
}

func TestCurrentReadingDeckPreparationStatusCancelsBySnapshot(t *testing.T) {
	var output bytes.Buffer
	preparation := domain.DeckPreparation{ID: "goal-prep-active", State: domain.DeckPreparationPreparing}
	require.NoError(t, CurrentReadingDeckPreparationStatus("csrf", "book-goal", "snapshot-goal", preparation, deckPreparationActions{BookID: "book-goal", OnCurrentReading: true, Submit: true, Reprepare: true}).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `action="/reading/books/book-goal/deck/cancel"`)
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="snapshot-goal"`)
	assert.NotContains(t, html, `action="/deck-preparations/goal-prep-active/cancel"`)
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
			want:     []string{"Deck preparation queued", "Analysis is queued for Book deck preparation", "Cancel preparation"},
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
			want:     []string{"Deck preparation failed", "Action needed", "temporarily unavailable"},
			unwanted: []string{"Cancel preparation", `hx-trigger="every 3s"`},
			progress: 100,
		},
		{
			name:     "cancelled",
			state:    domain.DeckPreparationCancelled,
			want:     []string{"Deck preparation cancelled", "You can try again when you want to continue"},
			unwanted: []string{"Cancel preparation", `hx-trigger="every 3s"`},
			progress: 100,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			preparation := domain.DeckPreparation{ID: "preparation-372", SourceMaterialID: "book-372", AnalysisRunID: "run-372", State: test.state, FailureClass: "provider", TotalCards: 10, CardsWithEnglish: 8, CardsWithContextualSentenceTranslations: 6, CardsWithFallbackGloss: 2, QualityOmissions: 1}
			require.NoError(t, DeckPreparationStatus("csrf", preparation, "/reading#journey-book-book-372", emptyDeckReadingAction()).Render(context.Background(), &output))
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

func TestDeckPreparationFallbackUsesLearnerFacingCopy(t *testing.T) {
	preparation := domain.DeckPreparation{State: domain.DeckPreparationPreparing, Phase: "unrecognized"}
	assert.Equal(t, "Preparing the Book deck. You can leave and return later.", deckPreparationSummary(preparation))
}

func TestDeckPreparationStatusIndicatesUpdatedRevision(t *testing.T) {
	updated := domain.DeckPreparation{
		ID: "preparation-372", State: domain.DeckPreparationReady,
		DeckRevision: 2, TotalCards: 1,
	}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatus("csrf", updated, "", emptyDeckReadingAction()).Render(context.Background(), &output))
	assert.Contains(t, output.String(), "Updated deck revision available")
	assert.Contains(t, output.String(), "revision 2")
}

func TestMissingTranslationProviderFailureOffersConfigurationAndRetry(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-unconfigured", State: domain.DeckPreparationFailed,
		Error: "prepareddeck: a configured translation provider is required for contextual Glosses",
	}
	response := preparationResponse(preparation)
	assert.Contains(t, response.Error, "Configure the translation provider, then retry")
	assert.False(t, response.Ready)
	assert.Empty(t, response.DownloadURL)
}

func TestDeckPreparationStatusReportsEvidenceCoverageAndMissingIndex(t *testing.T) {
	for _, test := range []struct {
		name     string
		coverage domain.DeckPreparationEvidenceCoverage
		want     string
	}{
		{name: "configured with truncation", coverage: domain.DeckPreparationEvidenceCoverage{Source: "wiktionary", Configured: true, Selected: 5, Matched: 3, Candidates: 12, Omitted: 4}, want: "wiktionary matched 3 of 5 targets; 12 candidates frozen, 4 omitted by the candidate bound"},
		{name: "index not configured", coverage: domain.DeckPreparationEvidenceCoverage{Source: "wiktionary", Selected: 5}, want: "wiktionary index not configured; no local candidates were available for 5 targets"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			preparation := domain.DeckPreparation{ID: "preparation-evidence", State: domain.DeckPreparationReady, TotalCards: 1, EvidenceCoverage: []domain.DeckPreparationEvidenceCoverage{test.coverage}}
			require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckReadingAction()).Render(context.Background(), &output))
			assert.Contains(t, output.String(), test.want)
		})
	}
}

func TestDeckPreparationStatusOmitsUpdatedRevisionIndicatorForCurrentDeck(t *testing.T) {
	// Legacy sentence translations with no structured alignment remain current
	// artifacts; absence of alignment is not itself a presentation revision.
	for _, preparation := range []domain.DeckPreparation{
		{ID: "initial", State: domain.DeckPreparationReady, DeckRevision: 1, TotalCards: 1},
		{ID: "legacy-alignment-missing", State: domain.DeckPreparationReady, DeckRevision: 1, TotalCards: 1, CardsWithContextualSentenceTranslations: 1},
		{ID: "unrecoverable", State: domain.DeckPreparationReady, DeckRevision: 2, TotalCards: 1, Error: domain.DeckPreparationRequiresRepreparationError},
	} {
		var output bytes.Buffer
		require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckReadingAction()).Render(context.Background(), &output))
		assert.NotContains(t, output.String(), "Updated deck revision available", preparation.ID)
	}
}

func TestDeckPreparationStatusPageUsesReadingEntryForBothBackLinks(t *testing.T) {
	preparation := domain.DeckPreparation{ID: "prep-1", SourceMaterialID: "source-1", State: domain.DeckPreparationReady, TotalCards: 1}
	action := deckReadingActionView{BookID: "book-1", State: deckIsToRead}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatusPage(domain.User{Username: "learner"}, "csrf", preparation, "/reading#journey-book-book-1", action).Render(context.Background(), &output))
	html := output.String()
	assert.Equal(t, 3, strings.Count(html, `href="/reading#journey-book-book-1"`), "Journey anchor link count=%d: %s", strings.Count(html, `href="/reading#journey-book-book-1"`), html)
	assert.False(t, strings.Contains(html, "/books/source-1"), "status page contains retired book link: %s", html)
}

func TestDeckPreparationStatusPageOmitsBackLinksWithoutReadingEntry(t *testing.T) {
	preparation := domain.DeckPreparation{ID: "prep-1", SourceMaterialID: "source-1", State: domain.DeckPreparationFailed}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatusPage(domain.User{Username: "learner"}, "csrf", preparation, "", emptyDeckReadingAction()).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, "Return to book") || strings.Contains(html, `href="/books/source-1"`), "unreachable deck status page contains back link: %s", html)
}

func TestEmptyReadyDeckShowsRecurringVocabularyEmptyState(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-empty", SourceMaterialID: "book-empty", State: domain.DeckPreparationReady,
		DeckName: "Mouseion::de::A Book", Filename: "A Book.apkg",
	}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckReadingAction()).Render(context.Background(), &output))
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

func TestZeroCardReadyDeckWithQualityOmissionsKeepsCompleteness(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-omitted", SourceMaterialID: "book-omitted", State: domain.DeckPreparationReady,
		DeckName: "Mouseion::de::A Book", Filename: "A Book.apkg", QualityOmissions: 1,
	}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckReadingAction()).Render(context.Background(), &output))
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
	require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckReadingAction()).Render(context.Background(), &output))
	assert.Contains(t, output.String(), "1 with fallback gloss")
}

func TestReadyDeckReportsContextualGlossesAndContextOnlyInference(t *testing.T) {
	preparation := domain.DeckPreparation{
		ID: "prep-contextual", SourceMaterialID: "book-contextual", State: domain.DeckPreparationReady,
		DeckName: "Mouseion::de::A Book", Filename: "A Book.apkg", TotalCards: 3,
		CardsWithEnglish: 3, ContextualGlossesReported: true, ContextualGlosses: 3, ContextOnlyGlosses: 1,
	}
	var output bytes.Buffer
	require.NoError(t, DeckPreparationStatus("csrf", preparation, "", emptyDeckReadingAction()).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "3 with contextual Gloss")
	assert.Contains(t, html, "1 inferred from sentence context")
	assert.NotContains(t, html, "fallback gloss")
}

func TestReadyDeckRendersTruthfulReadingStates(t *testing.T) {
	preparation := domain.DeckPreparation{ID: "preparation-372", SourceMaterialID: "book-372", AnalysisRunID: "run-372", State: domain.DeckPreparationReady, DeckName: "Mouseion::de::The Exact Book", Filename: "The Exact Book.apkg", TotalCards: 10}
	tests := []struct {
		name  string
		state deckReadingState
		want  []string
		omit  []string
	}{
		{name: "not in To Read", state: deckReadingNotMember, want: []string{"Not in To Read", "disposition in", `href="/library"`}, omit: []string{"Move to To Read", `method="post" action="/reading/books/`, "View this book in Reading", "current reading"}},
		{name: "already in Journey", state: deckIsToRead, want: []string{"To Read", "This book is already in To Read", `href="/reading#journey-book-book-372"`}, omit: []string{"Move to To Read", "current reading"}},
		{name: "current reading", state: deckIsCurrentReading, want: []string{"Current Book", "This deck is preparation for the Book you are reading now", "View current book in Reading"}, omit: []string{"Move to To Read", "View in Reading"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			action := deckReadingActionView{BookID: preparation.SourceMaterialID, State: test.state}
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
