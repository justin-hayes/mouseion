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

func TestRetiredCampaignRoutesAreNotRegistered(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "campaign-route-test-secret-0123456789")
	h, err := NewWithError(Services{})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/campaigns"},
		{http.MethodPost, "/campaigns/example/activate"},
		{http.MethodPost, "/campaigns/example/book-finished"},
		{http.MethodPost, "/campaigns/example/deck-reviewed"},
		{http.MethodPost, "/campaigns/example/abandon"},
	} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s %s returned %d, want 404", request.method, request.path, recorder.Code)
		}
	}
}

func renderJourney(t *testing.T, view journeyPageView, message, pageError string) string {
	t.Helper()
	var output bytes.Buffer
	if err := JourneyPage(domain.User{ID: "owner-1", Username: "learner"}, "csrf-token", view, message, pageError).Render(context.Background(), &output); err != nil {
		t.Fatalf("render journey page: %v", err)
	}
	return output.String()
}

func testJourneyBook(id, title, status string) journeyBookView {
	return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: id, Title: title, Language: "de"}, AnalysisStatus: status}}
}

func TestJourneyPageDoesNotRenderRetiredCampaignSurface(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "analyzed")
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	for _, forbidden := range []string{
		"Campaign history &amp; operations",
		"campaign-operations-heading",
		"Start learning",
		"Mark book finished",
		"Mark deck reviewed",
		"Abandon campaign",
		"/campaigns/",
		"Queue position",
	} {
		if strings.Contains(html, forbidden) {
			t.Errorf("Journey rendered retired campaign surface %q: %s", forbidden, html)
		}
	}
}

func TestJourneyPageRendersGoalAndProvisionalOrder(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "ready")
	provisional := []journeyBookView{
		testJourneyBook("first", "First provisional book", "ready"),
		testJourneyBook("second", "Second provisional book", "ready"),
	}
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: provisional}, "", "")
	goalIndex := strings.Index(html, "Goal book")
	provisionalIndex := strings.Index(html, "Provisional Journey")
	firstIndex := strings.Index(html, "First provisional book")
	secondIndex := strings.Index(html, "Second provisional book")
	if goalIndex < 0 || provisionalIndex < 0 || firstIndex < 0 || secondIndex < 0 || goalIndex > provisionalIndex || firstIndex > secondIndex {
		t.Fatalf("journey order was not goal-first and learner-ordered: goal=%d provisional=%d first=%d second=%d", goalIndex, provisionalIndex, firstIndex, secondIndex)
	}
}

func TestJourneyPageRendersCanonicalBookTitle(t *testing.T) {
	book := testJourneyBook("canonical-book", "Acquisition title", "ready")
	book.Book.BookTitle = "Catalogue title"
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{book}}, "", "")
	if !strings.Contains(html, "Catalogue title") || strings.Contains(html, "Acquisition title") {
		t.Fatalf("Journey did not use canonical Book title: %s", html)
	}
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
	if action.Status != "Stale analysis" || action.Label != "Re-analyze" || action.URL != "/journey/books/stale/reanalyze" || !action.Submit {
		t.Fatalf("stale Journey action=%+v", action)
	}
	unassessed := testJourneyBook("unassessed", "Unassessed book", "not analyzed")
	unassessed.Book.Source.MediaType = "application/epub+zip"
	unassessed.Book.Source.ContentRevisionID = "revision"
	unassessed.Book.Source.ContentSnapshotID = "snapshot"
	action = journeyAnalysisAction(unassessed)
	if action.Status != "Analysis not started" || action.Label != "Retry analysis" || action.URL != "/journey/books/unassessed/reanalyze" || !action.Submit {
		t.Fatalf("unassessed Journey action=%+v", action)
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
	if !strings.Contains(html, `href="/journey/canonical-book"`) || strings.Contains(html, `href="/books/source-book"`) {
		t.Fatalf("Journey card used a non-canonical entry link: %s", html)
	}
}

func TestJourneyCoverageLabelsConditionalVocabulary(t *testing.T) {
	item := testJourneyBook("conditional", "Conditional book", "analyzed")
	item.Coverage = &domain.AnalysisCoverage{
		AnalyzableTokenCount: 100,
		KnownTokenCount:      50,
		ReservedTokenCount:   30,
		Projections:          []domain.CoverageProjection{{TopLemmaCount: 2, ProjectedTokenCount: 75}},
	}
	if got := journeyCurrentCoverage(item); got != "50.0%" {
		t.Fatalf("current coverage=%q", got)
	}
	if got := journeyProjectedCoverage(item); got != "80.0% if reserved vocabulary graduates; 75.0% after the top 2 deck-eligible lemmas" {
		t.Fatalf("conditional projection=%q", got)
	}
}
