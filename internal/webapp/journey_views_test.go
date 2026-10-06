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

func renderJourney(t *testing.T, view journeyPageView, message, pageError string) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, JourneyPage(domain.User{ID: "owner-1", Username: "learner"}, "csrf-token", view, message, pageError).Render(context.Background(), &output))
	return output.String()
}

func testJourneyBook(id, title, status string) journeyBookView {
	return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: id, Title: title, Language: "de"}, AnalysisStatus: status}}
}

func TestJourneyPageRendersGoalAndProvisionalOrder(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "ready")
	provisional := []journeyBookView{
		testJourneyBook("first", "First provisional book", "ready"),
		testJourneyBook("second", "Second provisional book", "ready"),
	}
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: provisional}, "", "")
	goalIndex := strings.Index(html, "Goal book")
	provisionalIndex := strings.Index(html, `id="provisional-journey-heading"`)
	firstIndex := strings.Index(html, "First provisional book")
	secondIndex := strings.Index(html, "Second provisional book")
	assert.True(t, goalIndex >= 0 && provisionalIndex >= 0 && firstIndex >= 0 && secondIndex >= 0 && goalIndex <= provisionalIndex && firstIndex <= secondIndex, "journey order was not goal-first and learner-ordered: goal=%d provisional=%d first=%d second=%d", goalIndex, provisionalIndex, firstIndex, secondIndex)
	assert.Contains(t, html, `<ol class="journey-list" aria-label="To Read books">`)
	assert.NotContains(t, html, "vocabulary-efficient")
	assert.NotContains(t, html, "advisory order")
}

func TestJourneyPageRendersAlignedCoverMediaForGoalAndProvisionalBooks(t *testing.T) {
	tests := []struct {
		name, goalID, provisionalID string
		goalCover, provisionalCover domain.BookCover
		goalImage, provisionalImage bool
	}{
		{
			name: "available Goal and placeholder provisional", goalID: "goal-cover", provisionalID: "placeholder-cover",
			goalCover: domain.BookCover{State: domain.BookCoverAvailable, Width: 600, Height: 900}, provisionalCover: domain.BookCover{State: domain.BookCoverPending},
			goalImage: true,
		},
		{
			name: "placeholder Goal and available provisional", goalID: "placeholder-goal", provisionalID: "provisional-cover",
			goalCover: domain.BookCover{State: domain.BookCoverUnavailable}, provisionalCover: domain.BookCover{State: domain.BookCoverAvailable, Width: 600, Height: 900},
			provisionalImage: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			goal := testJourneyBook(test.goalID, "Goal cover book", "ready")
			goal.Cover = test.goalCover
			provisional := testJourneyBook(test.provisionalID, "Provisional cover book", "ready")
			provisional.Cover = test.provisionalCover
			html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: []journeyBookView{provisional}}, "", "")
			goalStart := strings.Index(html, `id="journey-book-`+test.goalID+`"`)
			goalEnd := strings.Index(html[goalStart:], "</article>")
			provisionalStart := strings.Index(html, `id="journey-book-`+test.provisionalID+`"`)
			provisionalEnd := strings.Index(html[provisionalStart:], "</article>")
			require.GreaterOrEqual(t, goalStart, 0)
			require.Greater(t, goalEnd, 0)
			require.GreaterOrEqual(t, provisionalStart, 0)
			require.Greater(t, provisionalEnd, 0)

			goalHTML := html[goalStart : goalStart+goalEnd]
			provisionalHTML := html[provisionalStart : provisionalStart+provisionalEnd]
			assertCoverMedia(t, goalHTML, test.goalID, test.goalImage)
			assertCoverMedia(t, provisionalHTML, test.provisionalID, test.provisionalImage)
		})
	}
}

func assertCoverMedia(t *testing.T, html, bookID string, image bool) {
	t.Helper()
	assert.Contains(t, html, `class="journey-book__cover"`)
	assert.Less(t, strings.Index(html, `class="journey-book__cover"`), strings.Index(html, `class="journey-book__identity"`))
	if image {
		assert.Contains(t, html, `<img class="book-cover-media__image" src="/books/`+bookID+`/cover" alt=""`)
		return
	}
	assert.Contains(t, html, `class="book-cover-media__placeholder" aria-hidden="true"`)
	assert.NotContains(t, html, `<img`)
}

func TestIsCurrentReadingShowsSnapshotBoundDeckRecoveryWithoutConsent(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "analyzed")
	goal.GoalSnapshotSize = 2
	goal.GoalSnapshotID = "snapshot"
	goal.GoalVocabularyEligible = 1
	goal.GoalPreparation = &domain.DeckPreparation{ID: "goal-preparation", GoalSnapshotID: "snapshot", State: domain.DeckPreparationFailed, FailureClass: "provider"}
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Reserved vocabulary</strong>: <span class=\"numeric\">2</span> frozen identities.")
	assert.Contains(t, html, "1 currently eligible frozen Reserved identities")
	assert.Contains(t, html, "Deck preparation")
	assert.Contains(t, html, "Retry deck preparation")
	assert.Contains(t, html, `action="/reading/books/goal/deck/retry"`)
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="snapshot"`)
	assert.NotContains(t, html, `action="/deck-preparations/goal-preparation/retry"`)
	goalCardStart := strings.Index(html, `id="journey-book-goal"`)
	goalCardEnd := strings.Index(html[goalCardStart:], "</article>")
	require.GreaterOrEqual(t, goalCardStart, 0)
	require.Greater(t, goalCardEnd, 0)
	assert.NotContains(t, html[goalCardStart:goalCardStart+goalCardEnd], "external_translation_consent")
}

func TestIsCurrentReadingRendersEmptyReservedVocabularyCount(t *testing.T) {
	goal := testJourneyBook("empty-goal", "Empty Goal book", "analyzed")
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Reserved vocabulary</strong>: <span class=\"numeric\">0</span> frozen identities.")
	assert.NotContains(t, html, "Deck preparation unavailable")
	assert.NotContains(t, html, "Retry deck preparation")
}

func TestIsCurrentReadingShowsReservedVocabularyWhenDeckIsUnavailable(t *testing.T) {
	goal := testJourneyBook("unavailable-goal", "Unavailable Goal book", "analyzed")
	goal.GoalSnapshotSize = 2
	goal.GoalDeckUnavailable = true
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Reserved vocabulary</strong>: <span class=\"numeric\">2</span> frozen identities.")
	assert.Contains(t, html, "Deck state: Unavailable")
	assert.Contains(t, html, "Retry deck preparation")
}

func TestIsCurrentReadingShowsMissingDeckWithoutChangingGoalFacts(t *testing.T) {
	goal := testJourneyBook("missing-goal", "Missing Goal deck", "analyzed")
	goal.GoalSnapshotSize = 2
	goal.GoalSnapshotID = "missing-snapshot"
	goal.GoalDeckMissing = true
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Deck state: Missing")
	assert.Contains(t, html, "The current reading and snapshot remain unchanged")
	assert.Contains(t, html, "Prepare deck")
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="missing-snapshot"`)
}

func TestIsCurrentReadingShowsReadingStateAndPageAction(t *testing.T) {
	goal := testJourneyBook("goal-state", "Goal state book", "analyzed")
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Reading state")
	assert.Contains(t, html, "Not yet marked finished")
	assert.Contains(t, html, `href="/library">Add books from My Books</a>`)
}

func TestJourneyPageRendersCanonicalBookTitle(t *testing.T) {
	book := testJourneyBook("canonical-book", "Acquisition title", "ready")
	book.Book.BookTitle = "Catalogue title"
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{book}}, "", "")
	assert.True(t, strings.Contains(html, "Catalogue title") && !strings.Contains(html, "Acquisition title"), "Journey did not use canonical Book title: %s", html)
}

func TestJourneyPageRendersAvailableAuthorWithoutInventingMissingMetadata(t *testing.T) {
	withAuthor := testJourneyBook("with-author", "Book with author", "ready")
	withAuthor.Book.BookAuthor = "A. Reader"
	withoutAuthor := testJourneyBook("without-author", "Book without author", "ready")
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{withAuthor, withoutAuthor}}, "", "")
	assert.Contains(t, html, "By A. Reader")
	missingStart := strings.Index(html, `id="journey-book-without-author"`)
	missingEnd := strings.Index(html[missingStart:], "</article>")
	require.GreaterOrEqual(t, missingStart, 0)
	require.Greater(t, missingEnd, 0)
	assert.NotContains(t, html[missingStart:missingStart+missingEnd], "journey-book__author")
}

func TestJourneyEvidenceActionsRemainAvailable(t *testing.T) {
	stale := testJourneyBook("stale", "Stale book", "stale")
	stale.Book.Source.MediaType = "application/epub+zip"
	stale.Book.Source.ContentRevisionID = "current-revision"
	stale.Book.Source.ContentSnapshotID = "current-snapshot"
	stale.Book.AnalysisState = "completed"
	stale.Book.AnalysisRunID = "old-run"
	stale.Book.CorpusID = "old-corpus"
	action := journeyAnalysisAction(stale)
	assert.Equal(t, "Stale analysis", action.Status)
	assert.Equal(t, "Re-analyze", action.Label)
	assert.Equal(t, "/reading/books/stale/reanalyze", action.URL)
	assert.True(t, action.Submit)
	unassessed := testJourneyBook("unassessed", "Unassessed book", "not analyzed")
	unassessed.Book.Source.MediaType = "application/epub+zip"
	unassessed.Book.Source.ContentRevisionID = "revision"
	unassessed.Book.Source.ContentSnapshotID = "snapshot"
	action = journeyAnalysisAction(unassessed)
	assert.Equal(t, "Analysis incomplete", action.Status)
	assert.Equal(t, "Retry analysis", action.Label)
	assert.Equal(t, "/reading/books/unassessed/reanalyze", action.URL)
	assert.True(t, action.Submit)

	missingSnapshot := testJourneyBook("missing-snapshot", "Missing snapshot", "analyzed")
	missingSnapshot.Book.Source.MediaType = "application/epub+zip"
	missingSnapshot.Book.Source.ContentRevisionID = "current-revision"
	action = journeyAnalysisAction(missingSnapshot)
	assert.Equal(t, "Assessment unavailable", action.Status)
	assert.Equal(t, "Retry acquisition", action.Label)

	for _, test := range []struct {
		status, wantStatus string
	}{
		{status: "analysis queued", wantStatus: "Analysis queued"},
		{status: "analysis running", wantStatus: "Analysis running"},
	} {
		item := testJourneyBook(test.status, test.status, test.status)
		item.Book.Source.MediaType = "application/epub+zip"
		item.Book.Source.ContentRevisionID = "revision"
		item.Book.Source.ContentSnapshotID = "snapshot"
		item.Book.AnalysisJobID = 42
		queuedAction := journeyAnalysisAction(item)
		assert.Equal(t, test.wantStatus, queuedAction.Status)
		assert.Equal(t, "View analysis status", queuedAction.Label)
		assert.False(t, queuedAction.Submit)
	}
}

func TestJourneyPageUsesCanonicalJourneyEntryLink(t *testing.T) {
	book := testJourneyBook("source-book", "Completed book", "analyzed")
	book.BookID = "canonical-book"
	book.Book.Source.MediaType = "application/epub+zip"
	book.Book.Source.ContentRevisionID = "revision"
	book.Book.Source.ContentSnapshotID = "snapshot"
	book.Book.AnalysisState = "completed"
	book.Book.AnalysisRunID = "run"
	book.Book.CorpusID = "corpus"
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{book}}, "", "")
	assert.True(t, strings.Contains(html, `href="/reading#journey-book-canonical-book"`) && !strings.Contains(html, `href="/books/source-book"`), "Journey card used a non-canonical book link: %s", html)
}

func TestJourneyCoverageLabelsConditionalVocabulary(t *testing.T) {
	item := testJourneyBook("conditional", "Conditional book", "analyzed")
	item.Coverage = &domain.AnalysisCoverage{
		AnalyzableTokenCount: 100,
		KnownTokenCount:      50,
		ReservedTokenCount:   30,
		Projections:          []domain.CoverageProjection{{TopLemmaCount: 2, ProjectedTokenCount: 75}},
	}
	assert.Equal(t, "50.0%", journeyCurrentCoverage(item))
}

func TestJourneyHealthyEvidenceStaysQuiet(t *testing.T) {
	item := testJourneyBook("healthy", "Healthy book", "analyzed")
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisState = "completed"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.Coverage = &domain.AnalysisCoverage{AnalyzableTokenCount: 10}
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{item}}, "", "")
	assert.NotContains(t, html, "Current evidence")
	assert.NotContains(t, html, "Assessment:")
	assert.NotContains(t, html, "Analysis result ready")
	assert.NotContains(t, html, "Current coverage")
	assert.NotContains(t, html, "On arrival")
}

func TestJourneyExceptionalEvidenceNamesStateAndRecovery(t *testing.T) {
	tests := []struct {
		name, status, wantState, wantLabel, wantAction string
	}{
		{name: "stale", status: "stale", wantState: "stale", wantLabel: "Stale evidence", wantAction: "Re-analyze"},
		{name: "unavailable", status: "", wantState: "unavailable", wantLabel: "Evidence unavailable", wantAction: "Retry acquisition"},
		{name: "incomplete", status: "not analyzed", wantState: "incomplete", wantLabel: "Incomplete evidence", wantAction: "Retry analysis"},
		{name: "failed", status: "analysis failed", wantState: "failed", wantLabel: "Analysis failed", wantAction: "Retry analysis"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := testJourneyBook(tt.name, tt.name, tt.status)
			if tt.name != "unavailable" {
				item.Book.Source.MediaType = "application/epub+zip"
				item.Book.Source.ContentRevisionID = "revision"
				item.Book.Source.ContentSnapshotID = "snapshot"
			}
			assert.Equal(t, tt.wantState, journeyEvidenceState(item))
			html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{item}}, "", "")
			assert.Contains(t, html, tt.wantLabel)
			assert.Contains(t, html, tt.wantAction)
		})
	}
}

func TestJourneyIncompleteAnalyzedEvidenceOffersReanalysis(t *testing.T) {
	item := testJourneyBook("incomplete", "Incomplete analyzed book", "analyzed")
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisState = "completed"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.StatisticsUnavailable = true

	action := journeyAnalysisAction(item)
	assert.Equal(t, "Analysis incomplete", action.Status)
	assert.Equal(t, "Retry analysis", action.Label)
	assert.Equal(t, "/reading/books/incomplete/reanalyze", action.URL)
	assert.True(t, action.Submit)
}

func TestJourneyTreatsAnalyzedEvidenceAndEligibleGoalsAsCurrent(t *testing.T) {
	item := testJourneyBook("analyzed", "Analyzed book", "analyzed")
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisState = "completed"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.Coverage = &domain.AnalysisCoverage{AnalyzableTokenCount: 10}

	assert.Equal(t, "current", journeyEvidenceState(item))
	eligible, message := journeyGoalEligibility(item.Book)
	assert.True(t, eligible)
	assert.Empty(t, message)
}

func TestActiveReadingDoesNotExposeLegacyGoalChoiceAndKeepsSecondaryActionsDisclosed(t *testing.T) {
	eligible := testJourneyBook("eligible", "Eligible book", "analyzed")
	eligible.CanChooseGoal = true
	ineligible := testJourneyBook("ineligible", "Ineligible book", "not analyzed")
	ineligible.GoalEligibilityReason = "This book needs a successfully completed current analysis before it can become a current reading."

	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{eligible, ineligible}}, "", "")
	eligibleStart := strings.Index(html, `id="journey-book-eligible"`)
	ineligibleStart := strings.Index(html, `id="journey-book-ineligible"`)
	require.GreaterOrEqual(t, eligibleStart, 0)
	require.Greater(t, ineligibleStart, eligibleStart)
	eligibleCard := html[eligibleStart:ineligibleStart]
	ineligibleCard := html[ineligibleStart:]

	assert.NotContains(t, eligibleCard, ">Start reading</button>")
	assert.NotContains(t, eligibleCard, `action="/goal/books/`)
	assert.Contains(t, eligibleCard, `<details class="more-actions"><summary>More actions</summary>`)
	assert.Contains(t, eligibleCard, `href="/library"`)
	assert.NotContains(t, eligibleCard, `action="/reading/books/eligible/set-aside"`)
	assert.NotContains(t, eligibleCard, "Remove from To Read")
	assert.NotContains(t, ineligibleCard, "Start reading")
	assert.Contains(t, ineligibleCard, ineligible.GoalEligibilityReason)
}

func analyzedJourneyBookView(id string) journeyBookView {
	item := testJourneyBook(id, "Analyzed "+id, "analyzed")
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision-" + id
	item.Book.Source.ContentSnapshotID = "snapshot-" + id
	item.Book.AnalysisState = "completed"
	item.Book.AnalysisRunID = "run-" + id
	item.Book.CorpusID = "corpus-" + id
	return item
}

func TestJourneyProvisionalBooksDoNotExposeDeckPreparationActions(t *testing.T) {
	books := []journeyBookView{
		analyzedJourneyBookView("missing-deck"), analyzedJourneyBookView("ready-deck"),
		analyzedJourneyBookView("active-deck"), analyzedJourneyBookView("failed-deck"),
	}
	html := renderJourney(t, journeyPageView{Provisional: books}, "", "")
	start := strings.Index(html, `<div id="provisional-journey-content"`)
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(html[start:], `</div></div></section>`)
	require.Greater(t, end, 0)
	provisional := html[start : start+end]
	assert.Contains(t, provisional, "Analyzed missing-deck")
	assert.NotContains(t, provisional, "/deck/preparations")
	assert.NotContains(t, provisional, "/deck-preparations/")
	assert.NotContains(t, provisional, "Prepare deck")
}
