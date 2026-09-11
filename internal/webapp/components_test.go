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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderPattern(t *testing.T, component templ.Component, child string) string {
	t.Helper()
	ctx := context.Background()
	if child != "" {
		ctx = templ.WithChildren(ctx, templ.Raw(child))
	}
	var output bytes.Buffer
	require.NoError(t, component.Render(ctx, &output), "render component")
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
	assert.False(t, strings.Contains(html, `value="">Choose a study language`), "active language switcher exposes a selectable empty option")
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
	assert.Equal(t, "", html, "empty language switcher rendered markup")
}

func TestActiveStudyLanguageReturnPathKeepsScopedLanguageInTransition(t *testing.T) {
	assert.Equal(t, "/library?q=title", activeStudyLanguageReturnPath("/library?language=de&q=title", "it"))
	assert.Equal(t, "/journey/book-1", activeStudyLanguageReturnPath("/books/book-1", "it"))
	assert.Equal(t, "/journey/book-1?message=updated", activeStudyLanguageReturnPath("/books/book-1?message=updated", "it"))
	assert.Equal(t, "/vocabulary", activeStudyLanguageReturnPath("/vocabulary?language=de", "it"))
}

func TestCatalogueSyncConnectionViewDoesNotExposeSyncScope(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	view := catalogueSyncConnectionViewFor(domain.OpdsConnection{ID: "catalog", Name: "Home"}, map[string]domain.CatalogueSyncStatus{
		"catalog": {State: domain.CatalogueSyncSynced, LastSyncedAt: &now},
	})
	assert.False(t, strings.Contains(view.Message, "study language") || strings.Contains(view.Message, "German"), "sync view exposes a learner scope=%+v", view)
}

func requireMarkup(t *testing.T, html string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		assert.True(t, strings.Contains(html, fragment), "markup missing %q\n%s", fragment, html)
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
	empty := renderPattern(t, EmptyState("Your library is empty", "Add a book to begin."), `<a href="/catalogs">Acquire an EPUB</a>`)
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

	confirmation := renderPattern(t, Confirmation("Release study", "The prepared deck remains available.", StatusDanger), `<form><button>Confirm release</button></form>`)
	requireMarkup(t, confirmation,
		`<details class="confirmation confirmation--danger">`,
		`<summary>Release study</summary>`,
		`class="confirmation__body"`,
		`Confirm release`,
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
		AnalyzableTokenCount: 100,
		KnownTokenCount:      80,
		ReservedTokenCount:   5,
	}
	stats := coverageStatItems(coverage)
	assert.Equal(t, "80.0%", stats[0].Value)
	assert.Equal(t, "85.0%", stats[1].Value)

	projection := projectionStatItems([]domain.CoverageProjection{{
		TopLemmaCount: 3, EligibleTokenCount: 10, ProjectedTokenCount: 90,
	}}, coverage.AnalyzableTokenCount)
	require.Len(t, projection, 1)
	assert.Equal(t, "90.0%", projection[0].Value)

	thresholds := thresholdStatItems([]domain.CoverageThreshold{{TargetPercent: 97, LemmaCount: 2, Reachable: true}})
	require.Len(t, thresholds, 1)
	assert.Equal(t, "lemmas for 97% of analyzed text", thresholds[0].Label)
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
	assert.Equal(t, http.StatusOK, response.Code)

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
		assert.True(t, strings.Contains(css, selector), "application CSS missing component selector %q", selector)
	}
	compactActions := regexp.MustCompile(`(?s)@media \(max-width: 40rem\).*?\.action-group\s*\{[^}]*flex-direction:\s*column`)
	assert.True(t, compactActions.MatchString(css), "compact component styles must stack action groups")
	assert.True(t, strings.Contains(css, ".confirmation--danger summary"), "danger confirmations need a distinct semantic treatment")
}
