package webapp

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/riverqueue/river/rivertype"
)

func TestBrowseURLRoundTripsBreadcrumbTrail(t *testing.T) {
	want := []CatalogCrumb{{Title: "Authors", URL: "https://catalog.example/authors"}, {Title: "A–C", URL: "https://catalog.example/a-c"}}
	request := httptest.NewRequest("GET", browseURL("connection-1", "https://catalog.example/books", want), nil)
	got := decodeTrail(request.URL.Query()["trail"])
	if len(got) != len(want) {
		t.Fatalf("trail=%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("trail[%d]=%+v want %+v", i, got[i], want[i])
		}
	}
}

func TestCatalogUsesReadyNLPCodeSeparatelyFromCatalogLanguageID(t *testing.T) {
	feed := opds.Feed{Entries: []opds.Entry{
		{Title: "Spanish", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/4"}}},
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/7"}}},
	}}
	capability := domain.SupportedLanguage{Language: "de", DisplayName: "German"}
	if got := catalogLanguageID(capability, feed); got != "7" {
		t.Fatalf("catalog language ID=%q", got)
	}
	var output bytes.Buffer
	if err := CatalogPage(domain.User{}, "csrf", domain.OpdsConnection{ID: "connection-1", Name: "Library"}, []domain.SupportedLanguage{capability}, false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Browse EPUBs by language", "German", `value="de"`, `method="get"`, `action="/opds/language"`, `hx-get="/opds/language"`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("catalog page missing %q: %s", want, output.String())
		}
	}
}

func TestConnectionFormsHaveNoBookLanguageField(t *testing.T) {
	var output bytes.Buffer
	connections := []domain.OpdsConnection{{ID: "connection-1", Name: "Library", URL: "https://catalog.example/opds"}}
	if err := ConnectionsPage(domain.User{}, "csrf", connections, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if strings.Contains(html, "Book language") || strings.Contains(html, `name="language"`) {
		t.Fatalf("connection forms still contain a language field: %s", html)
	}
	for _, want := range []string{`class="confirmation confirmation--danger"`, "Delete catalog connection", "Confirm deletion"} {
		if !strings.Contains(html, want) {
			t.Errorf("connection deletion missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "window.confirm") || strings.Contains(html, "onsubmit=") {
		t.Fatalf("connection deletion must use the server-rendered confirmation pattern: %s", html)
	}
}

func TestLanguageResultsShowOnlyProvidedEPUBEntries(t *testing.T) {
	feed := opds.Feed{Title: "German", Entries: []opds.Entry{{ID: "book", Title: "Book", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}
	var output bytes.Buffer
	if err := LanguageResults("csrf", "connection-1", "de", "/opds/language?connection=connection-1&language=de", "", feed).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"German", "Showing EPUB editions only", "Book", "Add to library", "Analysis starts separately"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("language results missing %q: %s", want, output.String())
		}
	}
	if !strings.Contains(output.String(), `name="language" value="de"`) {
		t.Fatalf("language results did not carry NLP code: %s", output.String())
	}
}

func TestCatalogRootFragmentShowsSearchWithoutCategories(t *testing.T) {
	feed := opds.Feed{Title: "Catalog", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}}, Entries: []opds.Entry{{Title: "Authors"}, {Title: "Newest books"}}}
	var output bytes.Buffer
	if err := CatalogRootFragment("connection-1", "de", feed).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Search this catalog", `method="get"`, `action="/opds/search"`, `hx-get="/opds/search"`} {
		if !strings.Contains(html, want) {
			t.Errorf("root fragment missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{"Catalog root", "Authors", "Newest books"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("root fragment unexpectedly contains %q: %s", unwanted, html)
		}
	}
}

func TestFeedFragmentShowsBreadcrumbsAndEmptyState(t *testing.T) {
	feed := opds.Feed{Title: "A–C", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}}}
	trail := []CatalogCrumb{{Title: "Authors", URL: "https://catalog.example/authors"}, {Title: "A–C", URL: "https://catalog.example/a-c"}}
	var output bytes.Buffer
	if err := FeedFragment("csrf", "connection-1", "de", "/opds/browse?connection=connection-1&language=de", "", feed, trail).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Catalog root", "Authors", `aria-current="page"`, "No books or collections were found here"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered fragment missing %q: %s", want, html)
		}
	}
}

func TestOPDSErrorsAreActionable(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errors.New("opds: HTTP 401 Unauthorized"), "rejected the credentials"},
		{errors.New("opds: parse Atom feed: unexpected EOF"), "web page instead of an OPDS feed"},
		{opds.ErrNoEPUB, "not available as an EPUB"},
		{errors.New("opds: fetch feed: dial tcp: connection refused"), "could not be reached"},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		opdsFail(response, test.err)
		if response.Code != 502 || !strings.Contains(response.Body.String(), test.want) {
			t.Errorf("error %q produced %d %q", test.err, response.Code, response.Body.String())
		}
	}
}

func TestPrepareDeckFormRendersAccessibleAsynchronousWorkflow(t *testing.T) {
	var output bytes.Buffer
	status := analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, LogicalState: "completed", ScopeID: "scope-1", CorpusID: "corpus-1"}
	if err := JobPage(domain.User{Username: "learner"}, "csrf", status).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		`form[data-prepare-deck]`,
		`action="/jobs/42/deck/preparations"`,
		`name="external_translation_consent"`,
		`data-deck-preparation`,
		`aria-live="polite"`,
		`evt.preventDefault()`,
		`await fetch(form.action`,
		`if (!response.ok)`,
		`window.pollDeckPreparation(response.url`,
		`preparation.state === 'queued'`,
		`preparation.state === 'preparing'`,
		`data-cancel-preparation`,
		`data-retry-preparation`,
		`preparation.completeness`,
		`preparation.deck_name`,
		`preparation.filename`,
		`preparation.download_url`,
		`Download deck`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("deck page missing client behavior %q", want)
		}
	}
	for _, unwanted := range []string{`action="/books/book-1/deck/preparations"`, `action="/books/book-1/deck"`, `await response.blob()`, `URL.createObjectURL`, `The deck downloads immediately`} {
		if strings.Contains(html, unwanted) {
			t.Errorf("deck page still contains legacy immediate-download behavior %q", unwanted)
		}
	}
}

func TestAnalysisResultPageUsesExactIdentityAndDocumentedOrder(t *testing.T) {
	completed := time.Date(2026, time.August, 28, 12, 34, 56, 0, time.UTC)
	result := analysis.CompletedAnalysis{
		RunID: "run-it-370", OwnerID: "owner-1", SourceMaterialID: "book-it-370", ScopeID: "scope-it-370",
		AnalyzerName: "mouseion-scoped-analyzer", AnalyzerVersion: "3", ConfigIdentity: "selection-default-v1", CompletedAt: &completed,
		JobID: 370, DisplayNumber: 4,
		Source:   domain.SourceMaterial{ID: "book-it-370", Title: "Il lettore", Language: "it", MediaType: "application/epub+zip"},
		Corpus:   domain.Corpus{ID: "corpus-it-370", SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "unit-1", Order: 0, Title: "Capitolo primo", ResolvedHref: "capitolo.xhtml"}}},
		Scope:    domain.EPUBReviewedScopeSnapshot{Classifier: domain.EPUBClassifierIdentity{Name: "epub-classifier", Version: "2"}, SelectionMode: domain.EPUBScopeSelectionRecommended},
		Artifact: domain.NormalizedArtifact{NormalizationProfile: "italian-standard", NormalizationVersion: "1"},
	}
	coverage := domain.AnalysisCoverage{
		AnalyzableTokenCount: 100, DistinctLemmaCount: 40, KnownTokenCount: 70, KnownLemmaCount: 28, UnknownTokenCount: 30, UnknownLemmaCount: 12,
		ActiveCampaignTokenCount: 8, ActiveCampaignLemmaCount: 3,
		Thresholds:           []domain.CoverageThreshold{{TargetPercent: 95, LemmaCount: 5, EligibleTokenCount: 30}, {TargetPercent: 97, LemmaCount: 8, EligibleTokenCount: 30}, {TargetPercent: 99, Reachable: false, EligibleTokenCount: 30}},
		TopUnknownLemmas:     []domain.LemmaOccurrence{{Language: "it", CanonicalLemma: "casa", UPOS: "NOUN", OccurrenceCount: 6}},
		UnknownConcentration: domain.CoverageProjection{TopLemmaCount: 10, EligibleTokenCount: 30, OccurrenceCount: 20},
		TextProfile:          &domain.TextProfile{SentenceCount: 12, NormalizedTokenCount: 130, MedianSentenceTokenCount: 9.5, P90SentenceTokenCount: 18, LongSentenceCount: 1},
	}
	var output bytes.Buffer
	if err := AnalysisResultPage(domain.User{Username: "learner"}, "csrf", result, &coverage, false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Analysis result", "Il lettore", "run-it-370", "scope-it-370", "2026-08-28 12:34:56 UTC", "Identity and trust", "Decision summary", "Vocabulary investment", "Structural and text signals", "Provenance and history", "mouseion-scoped-analyzer", "casa", "Vocabulary state and exclusions", "What next?"} {
		if !strings.Contains(html, want) {
			t.Errorf("analysis result missing %q", want)
		}
	}
	for _, pair := range [][2]string{{"Identity and trust", "Decision summary"}, {"Decision summary", "Vocabulary investment"}, {"Vocabulary investment", "Structural and text signals"}, {"Structural and text signals", "Provenance and history"}, {"Provenance and history", "What next?"}} {
		if strings.Index(html, pair[0]) > strings.Index(html, pair[1]) {
			t.Errorf("result sections out of order: %q before %q", pair[0], pair[1])
		}
	}
}

func TestCompletedJobStatusLinksToExactResult(t *testing.T) {
	status := analysis.Status{ID: 370, DisplayNumber: 4, SourceMaterialID: "book-de-370", RunID: "run-de-370", ScopeID: "scope-de-370", LogicalState: "completed", State: rivertype.JobStateCompleted, CorpusID: "corpus-de-370"}
	var output bytes.Buffer
	if err := JobPage(domain.User{Username: "learner"}, "csrf", status).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `href="/books/book-de-370/analyses/run-de-370"`) || !strings.Contains(html, "View analysis result") {
		t.Fatalf("completed job did not link exact result: %s", html)
	}
	if strings.Contains(html, `<form method="post" action="/jobs/370/deck/preparations"`) {
		t.Error("completed operational job must not own deck preparation")
	}
}

func TestAnalysisResultPageShowsDegradedInsightsWithoutMetrics(t *testing.T) {
	completed := time.Date(2026, time.August, 28, 12, 34, 56, 0, time.UTC)
	result := analysis.CompletedAnalysis{
		RunID: "run-unavailable", SourceMaterialID: "book-unavailable", ScopeID: "scope-unavailable", CompletedAt: &completed,
		Source: domain.SourceMaterial{ID: "book-unavailable", Title: "Unavailable insights", Language: "de"},
		Corpus: domain.Corpus{ID: "corpus-unavailable", SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "unit-1", Title: "Chapter"}}},
	}
	var output bytes.Buffer
	if err := AnalysisResultPage(domain.User{Username: "learner"}, "csrf", result, nil, true).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, "Analysis insights unavailable") || strings.Contains(html, "Decision summary") || strings.Contains(html, "current-known coverage") {
		t.Fatalf("degraded result rendering=%s", html)
	}
}

func TestBookAnalysisHistoryLinksCompletedRunsToExactResults(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-history", Title: "History", Language: "de"}}
	history := []domain.AnalysisJob{
		{ID: 11, DisplayNumber: 2, SourceMaterialID: book.Source.ID, AnalysisRunID: "run-completed", CorpusID: "corpus-completed", AnalysisState: "completed"},
		{ID: 12, DisplayNumber: 3, SourceMaterialID: book.Source.ID, AnalysisRunID: "run-running", AnalysisState: "running"},
	}
	var output bytes.Buffer
	if err := BookPageWithHistory(domain.User{Username: "learner"}, "csrf", book, nil, false, history, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `href="/books/book-history/analyses/run-completed"`) || !strings.Contains(html, `href="/jobs/12"`) {
		t.Fatalf("analysis history links=%s", html)
	}
}

func TestJobsPageLinksCompletedScopedRunsToExactResults(t *testing.T) {
	jobs := []domain.AnalysisJob{
		{ID: 21, DisplayNumber: 4, SourceMaterialID: "book-jobs", AnalysisRunID: "run-jobs", CorpusID: "corpus-jobs", AnalysisState: "completed"},
		{ID: 22, DisplayNumber: 5, SourceMaterialID: "book-jobs", AnalysisRunID: "run-pending", AnalysisState: "running"},
	}
	var output bytes.Buffer
	if err := JobsPage(domain.User{Username: "learner"}, "csrf", jobs, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `href="/books/book-jobs/analyses/run-jobs"`) || !strings.Contains(html, `href="/jobs/22"`) {
		t.Fatalf("jobs page links=%s", html)
	}
}

func TestAnalyzedBookCoverageSummaryExplainsMetrics(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-1", Title: "Book", Language: "de", MediaType: "application/epub+zip"}, AnalysisStatus: "analyzed", ReviewedScopeID: "scope-264"}
	coverage := domain.AnalysisCoverage{
		ReviewedScopeID: "scope-264", SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "one", Order: 1, Title: "Chapter One"}, {UnitID: "two", Order: 2, SourceHref: "chapter-two.xhtml"}},
		AnalyzableTokenCount: 40, DistinctLemmaCount: 12, KnownTokenCount: 30, KnownLemmaCount: 7, UnknownTokenCount: 10, UnknownLemmaCount: 5,
		TextProfile:          &domain.TextProfile{SentenceCount: 4, NormalizedTokenCount: 50, EmptySentenceCount: 1, MedianSentenceTokenCount: 12.5, P90SentenceTokenCount: 40, LongSentenceCount: 1},
		TopUnknownLemmas:     []domain.LemmaOccurrence{{CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 4}},
		UnknownConcentration: domain.CoverageProjection{TopLemmaCount: 10, SelectedLemmaCount: 5, OccurrenceCount: 10, EligibleTokenCount: 10, ProjectedTokenCount: 40},
		Projections:          []domain.CoverageProjection{{TopLemmaCount: 10, SelectedLemmaCount: 5, OccurrenceCount: 10, EligibleTokenCount: 10, ProjectedTokenCount: 40}},
		Thresholds:           []domain.CoverageThreshold{{TargetPercent: 95, LemmaCount: 3, Reachable: true}, {TargetPercent: 97, LemmaCount: 4, Reachable: true}, {TargetPercent: 99, LemmaCount: 5}},
	}
	if err := BookPage(domain.User{Username: "learner"}, "csrf", book, &coverage, false, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Analyzed scope", "2 selected units", "scope-264", "Chapter One", "chapter-two.xhtml", "not necessarily to the full EPUB", "Reanalysis", "reuses reviewed scope", "new EPUB scope", "Analysis quality", "do not assign a quality grade", "Empty sentences returned", "1 analyzer-provided sentences contained no tokens", "Text profile", "4", "12.5", "40", "25.0%", "long sentences (&gt;35 tokens)", "40 of 50", "not a difficulty score", "75.0%", "current-known coverage", "active-campaign projected coverage", "analyzable tokens", "distinct lemmas", "graduated by completed campaigns", "unknown vocabulary", "lemmas for 95%", "lemmas for 97%", "Unavailable", "99% cannot be reached with deck-eligible vocabulary", "legacy generated history", "deck-eligible vocabulary", "Highest-impact unknown vocabulary", "Haus", "4 occurrences", "top 10 deck-eligible lemmas", "100.0%", "Projected token coverage", "after top 10 lemmas"} {
		if !strings.Contains(html, want) {
			t.Errorf("coverage summary missing %q", want)
		}
	}
	for _, unwanted := range []string{"structural difficulty", "CEFR"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("coverage summary includes unsupported claim %q: %s", unwanted, html)
		}
	}
}

func TestAnalyzedBookDoesNotRenderNonLexicalTopUnknownLemma(t *testing.T) {
	coverage := domain.AnalysisCoverage{TopUnknownLemmas: []domain.LemmaOccurrence{
		{CanonicalLemma: "5", UPOS: "NOUN", OccurrenceCount: 99},
		{CanonicalLemma: "Straße", UPOS: "NOUN", OccurrenceCount: 1},
	}}
	var output bytes.Buffer
	if err := BookPage(domain.User{Username: "learner"}, "csrf", domain.SourceMaterialSummary{}, &coverage, false, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if strings.Contains(html, ">5</strong>") || !strings.Contains(html, ">Straße</strong>") {
		t.Fatalf("top unknown vocabulary = %s", html)
	}
}

func TestAnalyzedBookReportsOnlyEvidenceBackedQualityWarnings(t *testing.T) {
	tests := []struct {
		name     string
		coverage domain.AnalysisCoverage
		want     string
		unwanted string
	}{
		{
			name:     "no sentences",
			coverage: domain.AnalysisCoverage{TextProfile: &domain.TextProfile{}},
			want:     "No sentences returned.",
			unwanted: "No vocabulary-analyzable tokens.",
		},
		{
			name: "normalized tokens filtered from vocabulary analysis",
			coverage: domain.AnalysisCoverage{TextProfile: &domain.TextProfile{
				SentenceCount: 2, NormalizedTokenCount: 12,
			}},
			want:     "No vocabulary-analyzable tokens.",
			unwanted: "No sentences returned.",
		},
		{
			name: "complete analyzer output",
			coverage: domain.AnalysisCoverage{AnalyzableTokenCount: 10, TextProfile: &domain.TextProfile{
				SentenceCount: 2, NormalizedTokenCount: 12,
			}},
			unwanted: "Analysis quality",
		},
	}
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-1", Title: "Book", Language: "de"}, AnalysisStatus: "analyzed"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := BookPage(domain.User{Username: "learner"}, "csrf", book, &tt.coverage, false, "").Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			html := output.String()
			if tt.want != "" && !strings.Contains(html, tt.want) {
				t.Errorf("page missing warning %q: %s", tt.want, html)
			}
			if strings.Contains(html, tt.unwanted) {
				t.Errorf("page unexpectedly contains %q: %s", tt.unwanted, html)
			}
			for _, unsupported := range []string{"analysis is bad", "CEFR", "proficiency level:"} {
				if strings.Contains(html, unsupported) {
					t.Errorf("page contains unsupported claim %q", unsupported)
				}
			}
		})
	}
}

func TestLegacyAnalyzedBookRequestsReanalysisForAllInsights(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "legacy", Title: "Legacy Book", Language: "de"}, AnalysisStatus: "analyzed"}
	if err := BookPage(domain.User{Username: "learner"}, "csrf", book, nil, true, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Analysis insights unavailable", "reproducible vocabulary and sentence statistics", "Analyze it again", "coverage, projections, and the structural profile"} {
		if !strings.Contains(html, want) {
			t.Errorf("legacy page missing %q", want)
		}
	}
}
