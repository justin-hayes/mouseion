package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestCampaignsRedirectsPermanentlyToJourneyPreservingQuery(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/campaigns?message=deep+link&error=keep", nil)
	(&Handler{}).campaigns(recorder, request)
	if recorder.Code != http.StatusMovedPermanently {
		t.Fatalf("campaign redirect status=%d, want 301", recorder.Code)
	}
	if got := recorder.Header().Get("Location"); got != "/journey?message=deep+link&error=keep" {
		t.Fatalf("campaign redirect location=%q", got)
	}
}

func renderJourney(t *testing.T, view journeyPageView, message, pageError, activeCampaignID string) string {
	t.Helper()
	var output bytes.Buffer
	if err := JourneyPage(domain.User{ID: "owner-1", Username: "learner"}, "csrf-token", view, message, pageError, activeCampaignID).Render(context.Background(), &output); err != nil {
		t.Fatalf("render journey page: %v", err)
	}
	return output.String()
}

func renderCampaignPage(t *testing.T, campaigns []campaignView, prepared []preparedCampaignOption, message, pageError, activeCampaignID string) string {
	t.Helper()
	return renderJourney(t, journeyPageView{Campaigns: campaigns, Prepared: prepared}, message, pageError, activeCampaignID)
}

func testJourneyBook(id, title, status string) journeyBookView {
	return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: id, Title: title, Language: "de"}, AnalysisStatus: status}}
}

func testCampaign(id string, book domain.BookProgress, deck domain.DeckProgress) campaignView {
	return campaignView{
		Campaign: domain.LearningCampaign{ID: id, SourceMaterialID: "book-" + id, DeckPreparationID: "deck-" + id, BookProgress: book, DeckProgress: deck, Status: domain.DeriveCampaignStatus(book, deck)},
		Book:     domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-" + id, Title: "Book " + id, Language: "de"}},
		Deck:     domain.DeckPreparation{ID: "deck-" + id, DeckName: "Mouseion::de::Book " + id},
	}
}

func testQueuedCampaign(id string) campaignView {
	return testCampaign(id, domain.BookQueued, domain.DeckQueued)
}

func TestJourneyPageRendersEmptyGoalAndProvisionalStates(t *testing.T) {
	prepared := []preparedCampaignOption{{
		Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "prepared-book", Title: "Prepared Book"}},
		Deck: domain.DeckPreparation{ID: "prepared-deck", DeckName: "Mouseion::de::Prepared Book", TotalCards: 9},
	}}
	html := renderJourney(t, journeyPageView{Prepared: prepared}, "", "", "")
	for _, expected := range []string{
		`<h2 id="primary-goal-heading">Primary Goal</h2>`,
		"No Primary Goal yet",
		`<h2 id="provisional-journey-heading">Provisional Journey</h2>`,
		"No provisional books yet",
		`<h2 id="campaign-operations-heading">Campaign history &amp; operations</h2>`,
		"Prepared books and decks",
		"Download deck",
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("empty journey page missing %q: %s", expected, html)
		}
	}
	for _, forbidden := range []string{"campaign-queue-heading", "Queue position", "Add to learning queue", "Your learning queue"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("retired queue UI still rendered via %q", forbidden)
		}
	}
	for _, forbidden := range []string{"Make available for campaign operations", `method="post" action="/campaigns"`} {
		if strings.Contains(html, forbidden) {
			t.Errorf("ready deck still exposed Campaign creation via %q", forbidden)
		}
	}
}

func TestJourneyPageRendersEmptyPreparedDeckWithoutDownload(t *testing.T) {
	prepared := []preparedCampaignOption{{
		Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "empty-book", Title: "Empty Book"}},
		Deck: domain.DeckPreparation{ID: "empty-deck", State: domain.DeckPreparationReady},
	}}
	html := renderJourney(t, journeyPageView{Prepared: prepared}, "", "", "")
	for _, expected := range []string{"No recurring vocabulary", "no cards were created"} {
		if !strings.Contains(html, expected) {
			t.Errorf("empty prepared deck missing %q: %s", expected, html)
		}
	}
	for _, forbidden := range []string{`status-badge status-badge--success">Deck ready`, "0 cards", "Download deck"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("empty prepared deck contains %q: %s", forbidden, html)
		}
	}
}

func TestJourneyPageAnchorsGoalAndPreservesProvisionalOrder(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "ready")
	provisional := []journeyBookView{
		testJourneyBook("first", "First provisional book", "ready"),
		testJourneyBook("second", "Second provisional book", "ready"),
	}
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: provisional}, "", "", "")
	goalIndex := strings.Index(html, "Goal book")
	provisionalIndex := strings.Index(html, "Provisional Journey")
	firstIndex := strings.Index(html, "First provisional book")
	secondIndex := strings.Index(html, "Second provisional book")
	if goalIndex < 0 || provisionalIndex < 0 || firstIndex < 0 || secondIndex < 0 || goalIndex > provisionalIndex || firstIndex > secondIndex {
		t.Fatalf("journey order was not goal-first and learner-ordered: goal=%d provisional=%d first=%d second=%d", goalIndex, provisionalIndex, firstIndex, secondIndex)
	}
	if !strings.Contains(html, `class="resource-card journey-book journey-book--goal"`) || !strings.Contains(html, "Primary Goal") {
		t.Fatalf("goal was not visually distinct: %s", html)
	}
	if strings.Count(html, "Goal book") != 1 {
		t.Fatalf("goal was duplicated instead of anchored once: %s", html)
	}
	if strings.Count(html, "Provisional — your order") != 2 || strings.Count(html, "Your order") != 2 {
		t.Fatalf("provisional order labels missing: %s", html)
	}
	if strings.Contains(html, "Queue position") || strings.Contains(html, "queued behind") {
		t.Fatal("journey rendered queue-position semantics")
	}
}

func TestJourneyPageRendersCanonicalBookTitle(t *testing.T) {
	book := testJourneyBook("canonical-book", "Acquisition-internal title", "ready")
	book.Book.BookTitle = "Refreshed catalogue title"

	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{book}}, "", "", "")
	if !strings.Contains(html, "Refreshed catalogue title") {
		t.Fatalf("Journey omitted canonical Book title: %s", html)
	}
	if strings.Contains(html, "Acquisition-internal title") {
		t.Fatalf("Journey rendered acquisition-internal title: %s", html)
	}
}

func TestJourneyPageRendersEvidenceStates(t *testing.T) {
	current := testJourneyBook("current", "Current book", "analyzed")
	current.Book.Source.MediaType = "application/epub+zip"
	current.Book.Source.ContentRevisionID = "revision-current"
	current.Book.Source.ContentSnapshotID = "snapshot-current"
	current.Book.AnalysisState = "completed"
	current.Book.AnalysisRunID = "run-current"
	current.Book.CorpusID = "corpus-current"
	coverage := domain.AnalysisCoverage{AnalyzableTokenCount: 100, KnownTokenCount: 60, Projections: []domain.CoverageProjection{{TopLemmaCount: 3, ProjectedTokenCount: 80}}}
	current.Coverage = &coverage
	stale := testJourneyBook("stale", "Stale book", "stale")
	stale.Book.Source.MediaType = "application/epub+zip"
	stale.Book.Source.ContentRevisionID = "revision-stale"
	stale.Book.Source.ContentSnapshotID = "snapshot-stale"
	stale.Book.AnalysisState = "completed"
	stale.Book.AnalysisRunID = "stale-run"
	stale.Book.CorpusID = "stale-corpus"
	unassessed := testJourneyBook("unassessed", "Unassessed book", "not analyzed")
	unassessed.Book.Source.MediaType = "application/epub+zip"
	unassessed.Book.Source.ContentRevisionID = "revision-unassessed"
	unassessed.Book.Source.ContentSnapshotID = "snapshot-unassessed"
	unavailable := testJourneyBook("unavailable", "Unavailable book", "ready")
	unavailable.StatisticsUnavailable = true
	unavailableEPUB := testJourneyBook("unavailable-epub", "Unavailable EPUB book", "ready")
	unavailableEPUB.Book.Source.MediaType = "application/epub+zip"
	queued := testJourneyBook("queued", "Queued book", "queued")
	queued.Book.Source.MediaType = "application/epub+zip"
	queued.Book.Source.ContentRevisionID = "revision-queued"
	queued.Book.Source.ContentSnapshotID = "snapshot-queued"
	queued.Book.AnalysisState = "queued"
	queued.Book.AnalysisJobID = 101
	running := testJourneyBook("running", "Running book", "running")
	running.Book.Source.MediaType = "application/epub+zip"
	running.Book.Source.ContentRevisionID = "revision-running"
	running.Book.Source.ContentSnapshotID = "snapshot-running"
	running.Book.AnalysisState = "running"
	running.Book.AnalysisJobID = 102
	failed := testJourneyBook("failed", "Failed book", "failed")
	failed.Book.Source.MediaType = "application/epub+zip"
	failed.Book.Source.ContentRevisionID = "revision-failed"
	failed.Book.Source.ContentSnapshotID = "snapshot-failed"
	failed.Book.AnalysisState = "failed"
	failed.Book.AnalysisJobID = 103
	cancelled := testJourneyBook("cancelled", "Cancelled book", "cancelled")
	cancelled.Book.Source.MediaType = "application/epub+zip"
	cancelled.Book.Source.ContentRevisionID = "revision-cancelled"
	cancelled.Book.Source.ContentSnapshotID = "snapshot-cancelled"
	cancelled.Book.AnalysisState = "cancelled"
	cancelled.Book.AnalysisJobID = 104
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{current, stale, unassessed, unavailable, unavailableEPUB, queued, running, failed, cancelled}}, "", "", "")
	for _, expected := range []string{"Current evidence", "Analysis result ready", "Current coverage:", "Projected coverage:", "Stale evidence", "Not yet assessed", "Coverage unavailable", "60.0%", "Assessment unavailable", "Analysis queued", "Analysis running", "Analysis failed", "Analysis cancelled"} {
		if !strings.Contains(html, expected) {
			t.Errorf("evidence state missing %q: %s", expected, html)
		}
	}
	for _, expected := range []string{`href="/journey/current"`, `href="/jobs/101"`, `href="/jobs/102"`, `href="/jobs/103"`, `href="/jobs/104"`} {
		if !strings.Contains(html, expected) {
			t.Errorf("Journey state missing link %q: %s", expected, html)
		}
	}
	for _, forbidden := range []string{`href="/books/current"`, `href="/books/queued"`, `href="/books/running"`, `href="/books/failed"`, `href="/books/cancelled"`, "Start analysis"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("Journey state rendered forbidden detail/action %q: %s", forbidden, html)
		}
	}
}

func TestJourneyStaleEvidenceOffersExplicitReanalysis(t *testing.T) {
	book := testJourneyBook("stale", "Stale book", "stale")
	book.Book.Source.MediaType = "application/epub+zip"
	book.Book.Source.ContentRevisionID = "current-revision"
	book.Book.Source.ContentSnapshotID = "current-snapshot"
	book.Book.AnalysisState = "completed"
	book.Book.AnalysisRunID = "old-run"
	book.Book.CorpusID = "old-corpus"
	action := journeyAnalysisAction(book)
	if action.Status != "Stale analysis" || action.Label != "Re-analyze" || action.URL != "/journey/books/stale/reanalyze" || !action.Submit {
		t.Fatalf("stale Journey action=%+v", action)
	}
}

func TestJourneyUnavailableEvidenceOffersAcquisitionRetry(t *testing.T) {
	book := testJourneyBook("unavailable", "Unavailable book", "not analyzed")
	action := journeyAnalysisAction(book)
	if action.Status != "Assessment unavailable" || action.Label != "Retry acquisition" || action.URL != "/journey/books/unavailable/reanalyze" || !action.Submit {
		t.Fatalf("unavailable Journey action=%+v", action)
	}
}

func TestJourneyUnassessedEvidenceOffersAnalysisRetry(t *testing.T) {
	book := testJourneyBook("unassessed", "Unassessed book", "not analyzed")
	book.Book.Source.MediaType = "application/epub+zip"
	book.Book.Source.ContentRevisionID = "revision"
	book.Book.Source.ContentSnapshotID = "snapshot"
	action := journeyAnalysisAction(book)
	if action.Status != "Analysis not started" || action.Label != "Retry analysis" || action.URL != "/journey/books/unassessed/reanalyze" || !action.Submit {
		t.Fatalf("unassessed Journey action=%+v", action)
	}
}

func TestJourneyPageUsesBookIDForCompletedEntryLink(t *testing.T) {
	book := testJourneyBook("source-book", "Completed book", "analyzed")
	book.BookID = "canonical-book"
	book.Book.Source.MediaType = "application/epub+zip"
	book.Book.Source.ContentRevisionID = "revision"
	book.Book.Source.ContentSnapshotID = "snapshot"
	book.Book.AnalysisState = "completed"
	book.Book.AnalysisRunID = "run"
	book.Book.CorpusID = "corpus"

	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{book}}, "", "", "")
	if !strings.Contains(html, `href="/journey/canonical-book"`) || strings.Contains(html, `href="/books/source-book"`) {
		t.Fatalf("completed Journey card used acquisition identity for entry link: %s", html)
	}
}

func TestJourneyProjectedCoverageLabelsUnfinishedVocabularyConditionally(t *testing.T) {
	item := testJourneyBook("conditional", "Conditional book", "analyzed")
	item.Coverage = &domain.AnalysisCoverage{
		AnalyzableTokenCount: 100,
		KnownTokenCount:      50,
		ReservedTokenCount:   30,
		Projections:          []domain.CoverageProjection{{TopLemmaCount: 2, ProjectedTokenCount: 75}},
	}

	if got := journeyCurrentCoverage(item); got != "50.0%" {
		t.Fatalf("current coverage = %q, want 50.0%%", got)
	}
	if got := journeyProjectedCoverage(item); got != "80.0% if reserved vocabulary graduates; 75.0% after the top 2 deck-eligible lemmas" {
		t.Fatalf("conditional projection = %q", got)
	}
}

func TestJourneyPageDemotesCampaignHistoryAndKeepsOperations(t *testing.T) {
	active := testCampaign("active", domain.BookReading, domain.DeckStudying)
	preparedCampaign := testQueuedCampaign("prepared")
	completed := testCampaign("completed", domain.BookFinished, domain.DeckReviewed)
	abandoned := testCampaign("abandoned", domain.BookAbandoned, domain.DeckAbandoned)
	prepared := []preparedCampaignOption{{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{Title: "Prepared"}}, Deck: domain.DeckPreparation{ID: "prepared", DeckName: "Prepared deck"}}}
	html := renderJourney(t, journeyPageView{Campaigns: []campaignView{active, preparedCampaign, completed, abandoned}, Prepared: prepared}, "", "", "")
	operations := strings.Index(html, `id="campaign-operations-heading"`)
	if operations < 0 || strings.Index(html, "Active campaign") >= 0 {
		t.Fatalf("campaign operations were not demoted into one secondary section: %s", html)
	}
	for _, expected := range []string{"Book active", "Book prepared", "Book completed", "Book abandoned", "Prepared", "Complete", "Abandoned", "Start learning", "Mark book finished", "Mark deck reviewed", "Abandon campaign", "Download prepared deck"} {
		if !strings.Contains(html, expected) {
			t.Errorf("secondary campaign operation missing %q: %s", expected, html)
		}
	}
	for _, forbidden := range []string{"Queue position", "Queued behind the single active campaign", "Add to learning queue"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("secondary surface retained retired queue semantics %q", forbidden)
		}
	}
}

func TestJourneyPageRendersIndependentActiveProgressPermutations(t *testing.T) {
	for _, test := range []struct {
		name      string
		book      domain.BookProgress
		deck      domain.DeckProgress
		remaining []string
		complete  bool
	}{
		{name: "both incomplete", book: domain.BookReading, deck: domain.DeckStudying, remaining: []string{"Mark book finished — campaign remains active", "Mark deck reviewed — campaign remains active"}},
		{name: "book finished", book: domain.BookFinished, deck: domain.DeckStudying, remaining: []string{"Book: Finished", "Deck: Studying", "Complete campaign and add its vocabulary to known"}, complete: true},
		{name: "deck reviewed", book: domain.BookReading, deck: domain.DeckReviewed, remaining: []string{"Book: Reading", "Deck: Reviewed", "Complete campaign and add its vocabulary to known"}, complete: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderCampaignPage(t, []campaignView{testCampaign("active", test.book, test.deck)}, nil, "", "", "")
			if !strings.Contains(html, "Campaign history &amp; operations") || !strings.Contains(html, "Remaining before completion") {
				t.Fatalf("active hierarchy/progress missing: %s", html)
			}
			for _, expected := range test.remaining {
				if !strings.Contains(html, expected) {
					t.Errorf("active page missing %q", expected)
				}
			}
			if test.complete != strings.Contains(html, "Complete campaign and add its vocabulary to known") {
				t.Errorf("completion confirmation presence=%t", strings.Contains(html, "Complete campaign and add its vocabulary to known"))
			}
		})
	}
}

func TestJourneyPageRendersCompletionOutcomeWithAndWithoutCount(t *testing.T) {
	count := 132
	withCount := testCampaign("counted", domain.BookFinished, domain.DeckStudying)
	withCount.GraduatableLemmaCount = &count
	withoutCount := testCampaign("uncounted", domain.BookFinished, domain.DeckStudying)

	withCountHTML := renderCampaignPage(t, []campaignView{withCount}, nil, "", "", "")
	for _, expected := range []string{
		"Complete campaign and add 132 lemmas to known vocabulary",
		"Book</dt><dd>Book counted — Finished",
		"Prepared deck</dt><dd>Mouseion::de::Book counted — Reviewed",
		"Generated provenance remains attached",
		"Coverage evidence will be recalculated",
		"not a mastery claim or a spaced-repetition grade",
		"Completion cannot currently be undone",
		"name=\"csrf_token\"",
	} {
		if !strings.Contains(withCountHTML, expected) {
			t.Errorf("counted confirmation missing %q: %s", expected, withCountHTML)
		}
	}
	if withoutCountHTML := renderCampaignPage(t, []campaignView{withoutCount}, nil, "", "", ""); !strings.Contains(withoutCountHTML, "Complete campaign and add its vocabulary to known") {
		t.Errorf("count-free confirmation did not use fallback: %s", withoutCountHTML)
	}
}

func TestJourneyPageRendersAbandonmentAndRecoveryStates(t *testing.T) {
	active := testCampaign("active", domain.BookReading, domain.DeckStudying)
	html := renderCampaignPage(t, []campaignView{active}, nil, "", "", "")
	if !strings.Contains(html, `name="expected_book_progress"`) || !strings.Contains(html, `name="expected_deck_progress"`) {
		t.Errorf("active abandonment form is missing expected progress state: %s", html)
	}
	preparedHTML := renderCampaignPage(t, []campaignView{testQueuedCampaign("prepared")}, nil, "", "", "")
	for _, expected := range []string{`action="/campaigns/prepared/abandon"`, "This prepared campaign has no active reservation to release", "Confirm abandonment", `value="queued"`} {
		if !strings.Contains(preparedHTML, expected) {
			t.Errorf("prepared abandonment UI missing %q", expected)
		}
	}
	for _, expected := range []string{"Abandon campaign", "without deleting it", "book, prepared deck, generated provenance, and campaign history remain", "active reservation is released", "eligible for future decks again", `action="/campaigns/active/abandon"`, `name="expected_campaign_status"`} {
		if !strings.Contains(html, expected) {
			t.Errorf("abandonment confirmation missing %q", expected)
		}
	}

	for _, test := range []struct {
		name     string
		message  string
		campaign string
		want     []string
	}{
		{name: "blocked activation", message: "Finish or abandon the active campaign before starting another.", campaign: "active", want: []string{"Action needed", "Finish or abandon the active campaign", `href="#campaign-active"`, "Return to the active campaign"}},
		{name: "stale submission", message: "This campaign changed since this page was loaded. Its current state is unchanged by this request; review Reading Journey before trying again.", want: []string{"Action needed", "changed since this page was loaded", "current state is unchanged"}},
		{name: "mutation failure", message: "Campaign progress could not be updated. No progress was changed; review Reading Journey and try again.", want: []string{"Action needed", "No progress was changed", "review Reading Journey and try again"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderCampaignPage(t, []campaignView{active}, nil, "", test.message, test.campaign)
			for _, expected := range test.want {
				if !strings.Contains(html, expected) {
					t.Errorf("recovery page missing %q", expected)
				}
			}
		})
	}
}
