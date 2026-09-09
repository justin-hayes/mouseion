package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func renderPattern(t *testing.T, component templ.Component, child string) string {
	t.Helper()
	ctx := context.Background()
	if child != "" {
		ctx = templ.WithChildren(ctx, templ.Raw(child))
	}
	var output bytes.Buffer
	if err := component.Render(ctx, &output); err != nil {
		t.Fatalf("render component: %v", err)
	}
	return output.String()
}

func TestCatalogueSyncConnectionViewExplainsEmptySyncResult(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	view := catalogueSyncConnectionViewFor(domain.OpdsConnection{ID: "catalog", Name: "Home"}, map[string]domain.CatalogueSyncStatus{
		"catalog": {State: domain.CatalogueSyncSynced, LastSyncedAt: &now},
	})
	html := renderPattern(t, CatalogueSyncConnectionStatus(view), "")
	requireMarkup(t, html,
		"Sync completed, but no eligible EPUB entries were found; the library was unchanged.",
	)
}

func TestActiveStudyLanguageSwitcherMarksReadOnlyAndNewOptions(t *testing.T) {
	view := &shellView{
		ActiveLanguage: "de",
		ReturnTo:       "/library",
		Options: []activeStudyLanguageOption{
			{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true},
			{StudyLanguage: domain.StudyLanguage{Language: "it", DisplayName: "Italian"}, HasBooks: true, NewArrival: true},
			{StudyLanguage: domain.StudyLanguage{Language: "fr", DisplayName: "French"}},
		},
	}
	html := renderPattern(t, ActiveStudyLanguageSwitcher(view, "csrf"), "")
	requireMarkup(t, html,
		`<select id="active-study-language" name="language"`,
		`German (de)`,
		`Italian (it) (new)`,
		`French (fr) (no books)`,
		`value="fr"`,
		`<noscript><button type="submit">Switch language</button></noscript>`,
		`action="/active-study-language"`,
	)
	if strings.Contains(html, `value="">Choose a study language`) {
		t.Fatal("active language switcher exposes a selectable empty option")
	}
}

func TestActiveStudyLanguageSwitcherDisplaysUnselectedPrompt(t *testing.T) {
	view := &shellView{
		ReturnTo: "/library",
		Options: []activeStudyLanguageOption{
			{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true},
		},
	}
	html := renderPattern(t, ActiveStudyLanguageSwitcher(view, "csrf"), "")
	requireMarkup(t, html, `<option value="" disabled selected>Choose a study language</option>`)
}

func TestActiveStudyLanguageSwitcherIsHiddenWithoutOptions(t *testing.T) {
	html := renderPattern(t, ActiveStudyLanguageSwitcher(&shellView{}, "csrf"), "")
	if html != "" {
		t.Fatalf("empty language switcher rendered markup: %s", html)
	}
}

func TestActiveStudyLanguageReturnPathKeepsScopedLanguageInTransition(t *testing.T) {
	if got := activeStudyLanguageReturnPath("/library?language=de&q=title", "it"); got != "/library?q=title" {
		t.Fatalf("library return path=%q", got)
	}
	if got := activeStudyLanguageReturnPath("/books/book-1", "it"); got != "/books/book-1" {
		t.Fatalf("book return path=%q", got)
	}
	if got := activeStudyLanguageReturnPath("/vocabulary?language=de", "it"); got != "/vocabulary" {
		t.Fatalf("vocabulary return path=%q", got)
	}
}

func TestCatalogueSyncConnectionViewDoesNotExposeSyncScope(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	view := catalogueSyncConnectionViewFor(domain.OpdsConnection{ID: "catalog", Name: "Home"}, map[string]domain.CatalogueSyncStatus{
		"catalog": {State: domain.CatalogueSyncSynced, LastSyncedAt: &now},
	})
	if strings.Contains(view.Message, "study language") || strings.Contains(view.Message, "German") {
		t.Fatalf("sync view exposes a learner scope=%+v", view)
	}
}

func requireMarkup(t *testing.T, html string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(html, fragment) {
			t.Errorf("markup missing %q\n%s", fragment, html)
		}
	}
}

func TestPageHeaderPattern(t *testing.T) {
	html := renderPattern(t, PageHeader("My Books", "Choose a book to continue.", StatusBadge("Analyzed", StatusSuccess)), `<button type="button">Choose a book</button>`)
	requireMarkup(t, html,
		`<header class="page-header">`,
		`<h1>My Books</h1>`,
		`<p>Choose a book to continue.</p>`,
		`class="page-header__status"`,
		`Analyzed`,
		`<div class="page-header__actions">`,
		`<button type="button">Choose a book</button>`,
	)
}

func TestBreadcrumbPattern(t *testing.T) {
	html := renderPattern(t, Breadcrumb("/library", "My Books"), "")
	requireMarkup(t, html,
		`<nav class="breadcrumb" aria-label="Breadcrumb">`,
		`href="/library"`,
		`← My Books`,
	)
}

func TestStatusBadgePattern(t *testing.T) {
	html := renderPattern(t, StatusBadge("Analysis running", StatusInfo), "")
	requireMarkup(t, html,
		`class="status-badge status-badge--info"`,
		`Analysis running`,
	)
}

func TestFeedbackPatterns(t *testing.T) {
	errorHTML := renderPattern(t, Feedback(FeedbackError, "Scope not saved", "Review the selected units and try again."), "")
	requireMarkup(t, errorHTML,
		`class="feedback feedback--error"`,
		`role="alert"`,
		`tabindex="-1"`,
		`<strong>Scope not saved</strong>`,
	)

	noticeHTML := renderPattern(t, Feedback(FeedbackInfo, "Analysis queued", "Mouseion will update this page."), "")
	requireMarkup(t, noticeHTML,
		`class="feedback feedback--info"`,
		`role="status"`,
		`aria-live="polite"`,
	)
}

func TestEmptyStateAndResourceCardPatterns(t *testing.T) {
	empty := renderPattern(t, EmptyState("Your library is empty", "Add a book to begin."), `<a href="/connections">Acquire an EPUB</a>`)
	requireMarkup(t, empty,
		`class="empty-state"`,
		`<h2>Your library is empty</h2>`,
		`<div class="empty-state__actions">`,
	)

	card := renderPattern(t, ResourceCard(), `<h2>Die Verwandlung</h2>`)
	requireMarkup(t, card,
		`<article class="resource-card">`,
		`<h2>Die Verwandlung</h2>`,
	)
}

func TestActionAndConfirmationPatterns(t *testing.T) {
	actions := renderPattern(t, ActionGroup(), `<button>Retry</button><button>Cancel</button>`)
	requireMarkup(t, actions,
		`<div class="action-group">`,
		`<button>Retry</button>`,
	)

	confirmation := renderPattern(t, Confirmation("Abandon campaign", "The prepared deck remains available.", StatusDanger), `<form><button>Confirm abandonment</button></form>`)
	requireMarkup(t, confirmation,
		`<details class="confirmation confirmation--danger">`,
		`<summary>Abandon campaign</summary>`,
		`class="confirmation__body"`,
		`Confirm abandonment`,
	)
}

func TestDataDisplayPatterns(t *testing.T) {
	stats := renderPattern(t, StatGroup([]StatItem{{Label: "Current-known coverage", Value: "93.4%", Detail: "Analyzed scope"}}), "")
	requireMarkup(t, stats,
		`class="stat-group"`,
		`class="stat-group__value numeric"`,
		`93.4%`,
		`Current-known coverage`,
		`Analyzed scope`,
	)

	metadata := renderPattern(t, MetadataList([]MetadataItem{{Term: "Language", Description: "German"}}), "")
	requireMarkup(t, metadata,
		`<dl class="metadata-list">`,
		`<dt>Language</dt>`,
		`<dd>German</dd>`,
	)

	table := renderPattern(t, ResponsiveTable("Analysis history"), `<thead><tr><th>Run</th></tr></thead>`)
	requireMarkup(t, table,
		`class="table-region"`,
		`role="region"`,
		`aria-label="Analysis history"`,
		`tabindex="0"`,
		`<table>`,
	)
}

func TestCoveragePercentagesUseWholeAnalyzableDenominator(t *testing.T) {
	coverage := domain.AnalysisCoverage{
		AnalyzableTokenCount:     100,
		KnownTokenCount:          80,
		ActiveCampaignTokenCount: 5,
	}
	stats := coverageStatItems(coverage)
	if stats[0].Value != "80.0%" || stats[1].Value != "85.0%" {
		t.Fatalf("coverage stats = %+v, want 80%% and 85%% of all analyzable tokens", stats)
	}

	projection := projectionStatItems([]domain.CoverageProjection{{
		TopLemmaCount: 3, EligibleTokenCount: 10, ProjectedTokenCount: 90,
	}}, coverage.AnalyzableTokenCount)
	if len(projection) != 1 || projection[0].Value != "90.0%" {
		t.Fatalf("projected coverage = %+v, want 90%% of all analyzable tokens", projection)
	}

	thresholds := thresholdStatItems([]domain.CoverageThreshold{{TargetPercent: 97, LemmaCount: 2, Reachable: true}})
	if len(thresholds) != 1 || thresholds[0].Label != "lemmas for 97% of analyzed text" {
		t.Fatalf("threshold label = %+v, want whole-text basis", thresholds)
	}
}

func TestAsyncStatusPattern(t *testing.T) {
	html := renderPattern(t, AsyncStatus(nil, "analysis-progress", "Analysis running", "42% complete", 42, 100, true), `<button>Cancel analysis</button>`)
	requireMarkup(t, html,
		`id="analysis-progress"`,
		`class="async-status"`,
		`role="status"`,
		`aria-live="polite"`,
		`aria-busy="true"`,
		`aria-label="Analysis running progress"`,
		`value="42"`,
		`max="100"`,
		`class="async-status__actions"`,
	)
}

func TestComponentStylesAvailable(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()
	StaticHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	css := response.Body.String()
	for _, selector := range []string{
		".page-header",
		".breadcrumb",
		".status-badge",
		".feedback",
		".empty-state",
		".action-group",
		".resource-card",
		".confirmation",
		".stat-group",
		".metadata-list",
		".table-region",
		".async-status",
	} {
		if !strings.Contains(css, selector) {
			t.Errorf("application CSS missing component selector %q", selector)
		}
	}
	compactActions := regexp.MustCompile(`(?s)@media \(max-width: 40rem\).*?\.action-group\s*\{[^}]*flex-direction:\s*column`)
	if !compactActions.MatchString(css) {
		t.Error("compact component styles must stack action groups")
	}
	if !strings.Contains(css, ".confirmation--danger summary") {
		t.Error("danger confirmations need a distinct semantic treatment")
	}
}
