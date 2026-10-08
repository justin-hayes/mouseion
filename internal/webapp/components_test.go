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

func TestLemmaReviewPageShowsNonAuthoritativeFlagWithSourceEvidence(t *testing.T) {
	occurrence := domain.LemmaReviewOccurrence{
		Surface: "Drachen", RawLemma: "Drach", CanonicalLemma: "drach", UPOS: "NOUN",
		SentenceText:         "Ein Reiter zielt mit seinem Speer auf einen Drachen.",
		ReviewFlagReason:     "The analyzer lemma is an index miss and a plausible competitor is supported.",
		ReviewFlagProvenance: map[string]any{"alternative_lemma": "drache", "source": "wiktionary", "version": "fixture-1", "evidence_id": "fixture-evidence"},
	}
	html := renderPattern(t, LemmaReviewPage(domain.User{}, "csrf", "book", "Book", "", "", true, []domain.LemmaReviewOccurrence{occurrence}, nil, lemmaReviewRecovery{ReferenceAssessed: true}, nil), "")
	requireMarkup(t, html,
		"Needs learner review — not a verdict.",
		"Ein Reiter zielt mit seinem Speer auf einen Drachen.",
		"drache", "wiktionary", "fixture-1", "fixture-evidence",
		"Review exact form",
	)
	assert.Contains(t, html, "Find occurrences", "review remains a server-rendered form flow without JavaScript")
}

func TestActiveStudyLanguageSwitcherMarksReadOnlyAndNewOptions(t *testing.T) {
	view := &shellView{
		ActiveLanguage: "de",
		Options: []activeStudyLanguageOption{
			{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "German"}, HasBooks: true},
			{StudyLanguage: domain.StudyLanguage{Language: "it", DisplayName: "Italian"}, HasBooks: true, NewArrival: true},
			{StudyLanguage: domain.StudyLanguage{Language: "el", DisplayName: "Greek"}, HasBooks: true},
			{StudyLanguage: domain.StudyLanguage{Language: "fr", DisplayName: "French"}},
		},
	}
	html := renderPattern(t, ActiveStudyLanguageSwitcher(view, "csrf"), "")
	requireMarkup(t, html,
		`<select id="active-study-language" name="language"`,
		`<label for="active-study-language"><span class="site-header__language-label">Study language</span>`,
		`Deutsch`,
		`Italiano (new)`,
		`Ελληνικά`,
		`French (fr) (no books)`,
		`value="fr"`,
		`<noscript><button class="btn btn-primary" type="submit">Switch language</button></noscript>`,
		`action="/active-study-language"`,
	)
	assert.NotContains(t, html, "German (de)")
	assert.NotContains(t, html, "Greek (el)")
	assert.False(t, strings.Contains(html, `value="">Choose a study language`), "active language switcher exposes a selectable empty option")
}

func TestActiveStudyLanguageSwitcherDisplaysUnselectedPrompt(t *testing.T) {
	view := &shellView{
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
		`class="status-badge__shape status-badge__shape--ring" aria-hidden="true"`,
		`Analysis running`,
	)
}

func TestStatusBadgeShapeUsesCurrentReadyAndFailureMeaning(t *testing.T) {
	for _, tt := range []struct {
		label, shape string
		tone         StatusTone
	}{
		{label: "Current reading", tone: StatusInfo, shape: "dot"},
		{label: "Deck ready", tone: StatusSuccess, shape: "dot"},
		{label: "Analysis completed", tone: StatusSuccess, shape: "ring"},
		{label: "Analysis queued", tone: StatusInfo, shape: "ring"},
		{label: "Analysis failed", tone: StatusDanger, shape: "diamond"},
	} {
		t.Run(tt.label, func(t *testing.T) {
			html := renderPattern(t, StatusBadge(tt.label, tt.tone), "")
			requireMarkup(t, html, `class="status-badge__shape status-badge__shape--`+tt.shape+`" aria-hidden="true"`)
		})
	}
}

func TestBookCoverPlaceholderKeepsStateAndDecoratesInitial(t *testing.T) {
	html := renderPattern(t, BookCoverMedia("book-1", "Die Verwandlung", "de", domain.BookCover{State: domain.BookCoverPending}), "")
	requireMarkup(t, html,
		`class="book-cover-media__placeholder" aria-hidden="true"`,
		`class="book-cover-media__initial">V</span>`,
		`class="book-cover-media__label">Cover pending</span>`,
	)
}

func TestBookCoverInitialIgnoresLeadingArticleForBookLanguage(t *testing.T) {
	tests := []struct {
		name, title, language, want string
	}{
		{name: "German definite article", title: "Der Prozess", language: "de", want: "P"},
		{name: "Italian article", title: "Gli indifferenti", language: "it", want: "I"},
		{name: "Greek article", title: "Το σπίτι", language: "el", want: "Σ"},
		{name: "article alone", title: "Die", language: "de", want: "D"},
		{name: "punctuation before article", title: "— Die Verwandlung", language: "de", want: "V"},
		{name: "elided Italian article", title: "L'amica geniale", language: "it", want: "A"},
		{name: "unknown language keeps first letter", title: "Der Prozess", language: "fr", want: "D"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, bookCoverInitial(tt.title, tt.language))
		})
	}
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

	confirmation := renderPattern(t, Confirmation("Remove connection", "The catalog connection can be added again later.", StatusDanger), `<form><button>Confirm removal</button></form>`)
	requireMarkup(t, confirmation,
		`<details class="confirmation confirmation--danger">`,
		`<summary>Remove connection</summary>`,
		`class="confirmation__body"`,
		`Confirm removal`,
	)
}

func TestDataDisplayPatterns(t *testing.T) {
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
	html := renderPattern(t, AsyncStatus(nil, "analysis-progress", "Analysis running", "42% complete", 42, 100, true, nil), `<button>Cancel analysis</button>`)
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
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/app.css", nil)
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
