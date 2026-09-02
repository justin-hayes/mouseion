package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
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
