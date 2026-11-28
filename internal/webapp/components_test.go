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
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
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
		"Sync completed. The Catalog index is current; no Book metadata changed.",
	)
}

func TestLemmaReviewPageShowsNonAuthoritativeFlagWithSourceEvidence(t *testing.T) {
	occurrence := domain.LemmaReviewOccurrence{
		Surface: "Drachen", RawLemma: "Drach", CanonicalLemma: "drach", UPOS: "NOUN",
		SentenceText:         "Ein Reiter zielt mit seinem Speer auf einen Drachen.",
		ReviewFlagReason:     "The analyzer lemma is an index miss and a plausible competitor is supported.",
		ReviewFlagProvenance: map[string]any{"alternative_lemma": "drache", "source": "wiktionary", "version": "fixture-1", "evidence_id": "fixture-evidence"},
	}
	html := renderPattern(t, LemmaReviewPage(domain.User{}, "csrf", "book", "Book", "", "", true, []domain.LemmaReviewOccurrence{occurrence}, nil, lemmareview.Recovery{ReferenceAssessed: true}, nil, nil), "")
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
		LastLanguage: "de",
		PageLanguage: "de",
		PageKind:     pageKindMyBooks,
		Options: []activeStudyLanguageOption{
			{Language: "de", DisplayName: "German", HasBooks: true},
			{Language: "it", DisplayName: "Italian", HasBooks: true, NewArrival: true},
			{Language: "el", DisplayName: "Greek", HasBooks: true},
			{Language: "fr", DisplayName: "French"},
		},
	}
	html := renderPattern(t, ActiveStudyLanguageSwitcher(view, NavigationLibrary), "")
	requireMarkup(t, html,
		`<select id="active-study-language" name="lang"`,
		`<label for="active-study-language"><span class="site-header__language-label">Study language</span>`,
		`Deutsch`,
		`Italiano (new)`,
		`Ελληνικά`,
		`French (fr) (no books)`,
		`value="fr"`,
		`<noscript><button class="btn btn-primary" type="submit">Switch language</button></noscript>`,
		`method="get" action="/study-language"`,
	)
	assert.NotContains(t, html, "German (de)")
	assert.NotContains(t, html, "Greek (el)")
	assert.NotContains(t, html, `value="">Choose a study language`, "active language switcher exposes a selectable empty option")
}

func TestActiveStudyLanguageSwitcherIsHiddenWithoutAPageLanguage(t *testing.T) {
	view := &shellView{
		LastLanguage: "de",
		Options:      []activeStudyLanguageOption{{Language: "de", DisplayName: "German", HasBooks: true}},
	}
	html := renderPattern(t, ActiveStudyLanguageSwitcher(view, NavigationLibrary), "")
	assert.Empty(t, html, "job status and other pages without a language have no picker")
}

func TestActiveStudyLanguageSwitcherIsHiddenOnBookAddressedPages(t *testing.T) {
	// A Book-addressed page (lemma review, deck preparation) has the Book's
	// language as its page language but is not a per-language page kind.
	view := &shellView{
		LastLanguage: "it",
		PageLanguage: "de",
		Options:      []activeStudyLanguageOption{{Language: "de", DisplayName: "German", HasBooks: true}, {Language: "it", DisplayName: "Italian", HasBooks: true}},
	}
	html := renderPattern(t, ActiveStudyLanguageSwitcher(view, NavigationLibrary), "")
	assert.Empty(t, html, "Book-addressed pages have no picker")
}

func TestActiveStudyLanguageSwitcherIsHiddenWithoutOptions(t *testing.T) {
	html := renderPattern(t, ActiveStudyLanguageSwitcher(&shellView{}, NavigationLibrary), "")
	assert.Empty(t, html, "empty language switcher rendered markup")
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
		assert.Contains(t, html, fragment, "markup missing %q\n%s", fragment, html)
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

func TestStatusBadgeKeepsOrdinaryReadAndInboxRingShapes(t *testing.T) {
	for _, label := range []string{"To Read", "Inbox", "Read"} {
		t.Run(label, func(t *testing.T) {
			html := renderPattern(t, StatusBadge(label, StatusNeutral), "")
			requireMarkup(t, html, `class="status-badge__shape status-badge__shape--ring" aria-hidden="true"`)
		})
	}
}

func TestBookCoverPlaceholderRendersTitlePlateHiddenFromAssistiveTechnology(t *testing.T) {
	plate := newTitlePlate("Die Verwandlung", "", "", "Franz Kafka")
	html := renderPattern(t, BookCoverMedia("book-1", plate, domain.BookCover{State: domain.BookCoverPending}), "")
	requireMarkup(t, html,
		`class="book-cover-media__placeholder" aria-hidden="true"`,
		`class="title-plate__title">Die Verwandlung</span>`,
		`class="title-plate__author">Franz Kafka</span>`,
		`class="book-cover-media__label">Cover pending</span>`,
	)
	assert.NotContains(t, html, "title-plate__greek")
	assert.NotContains(t, html, "book-cover-media__initial")
}

func TestTitlePlateCarriesOriginalTitleForClassicalBook(t *testing.T) {
	plate := newTitlePlate("Iliad", " Ἰλιάς ", "grc", "Homer")
	html := renderPattern(t, BookCoverMedia("book-1", plate, domain.BookCover{}), "")
	requireMarkup(t, html,
		`class="title-plate__title">Iliad</span>`,
		`class="title-plate__greek" lang="grc">Ἰλιάς</span>`,
		`class="title-plate__author">Homer</span>`,
	)
}

func TestTitlePlateDropsAnOriginalTitleThatRepeatsTheEnglishTitle(t *testing.T) {
	for name, original := range map[string]string{"identical": "Germania", "case": "GERMANIA", "spacing": "  Germania "} {
		t.Run(name, func(t *testing.T) {
			plate := newTitlePlate("Germania", original, "la", "Tacitus")

			assert.Empty(t, plate.OriginalTitle)
			assert.NotContains(t, renderPattern(t, BookCoverMedia("book-1", plate, domain.BookCover{}), ""), "title-plate__greek")
		})
	}
}

func TestBookCoverImageReplacesTitlePlate(t *testing.T) {
	html := renderPattern(t, BookCoverMedia("book-1", newTitlePlate("T", "", "", "A"), domain.BookCover{State: domain.BookCoverAvailable, Width: 10, Height: 15}), "")
	assert.Contains(t, html, "book-cover-media__image")
	assert.NotContains(t, html, "title-plate")
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
		assert.Contains(t, css, selector, "application CSS missing component selector %q", selector)
	}
	compactActions := regexp.MustCompile(`(?s)@media \(max-width: 40rem\).*?\.action-group\s*\{[^}]*flex-direction:\s*column`)
	assert.True(t, compactActions.MatchString(css), "compact component styles must stack action groups")
	assert.Contains(t, css, ".confirmation--danger summary", "danger confirmations need a distinct semantic treatment")
}

func TestTitlePlateAppearsOnCatalogMyBooksAndReading(t *testing.T) {
	unit := &domain.BookReadingUnit{WorkTitle: "Iliad", OriginalTitle: "Ἰλιάς"}
	book := domain.MyBook{
		Book:        domain.Book{ID: "book-1", Title: "Iliad", Author: "Homer", LanguageTag: "grc"},
		ReadingUnit: unit,
		Cover:       domain.BookCover{State: domain.BookCoverNone},
	}
	tests := []struct {
		name string
		html string
		want []string
	}{
		{
			name: "Catalog card",
			html: renderPattern(t, CatalogEntryCard("csrf", opdsCatalogCard(domain.CatalogueEntry{Index: domain.CatalogueIndexEntry{
				ConnectionID: "c", EntryIdentifier: "e", Title: "Die Verwandlung", Author: "Franz Kafka",
			}})), ""),
			want: []string{`class="title-plate__title">Die Verwandlung</span>`, `class="title-plate__author">Franz Kafka</span>`},
		},
		{
			name: "Classical card",
			html: renderPattern(t, CatalogEntryCard("csrf", classicalCatalogCard(
				classical.CatalogWork{Key: "iliad", Title: "Iliad", Author: "Homer", OriginalTitle: "Ἰλιάς", Language: "grc"},
				classical.CatalogUnit{Key: "iliad-unit", EditionLabel: "Monro 1902"},
				classicalCatalogView{AddAvailable: true})), ""),
			want: []string{`class="title-plate__title">Iliad</span>`, `class="title-plate__greek" lang="grc">Ἰλιάς</span>`, `class="catalog-entry__detail">Monro 1902</p>`},
		},
		{
			name: "My Books row",
			html: renderPattern(t, MyBookRow("csrf", book, ""), ""),
			want: []string{`class="title-plate__title">Iliad</span>`, `class="title-plate__greek" lang="grc">Ἰλιάς</span>`, `class="title-plate__author">Homer</span>`},
		},
		{
			name: "Reading current Book",
			html: renderPattern(t, ReadingBookCard(readingBookView{
				Book:        domain.SourceMaterialSummary{BookTitle: "Iliad", BookAuthor: "Homer", Source: domain.SourceMaterial{Language: "grc"}},
				BookID:      "book-1",
				ReadingUnit: unit,
			}, "csrf"), ""),
			want: []string{`class="title-plate__title">Iliad</span>`, `class="title-plate__greek" lang="grc">Ἰλιάς</span>`, `class="title-plate__author">Homer</span>`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireMarkup(t, tt.html, append(tt.want, `aria-hidden="true"`)...)
			assert.NotContains(t, tt.html, "book-cover-media__initial")
		})
	}
}

func TestTitlePlateRendersTheOriginalTitleInTheDefinitionLanguage(t *testing.T) {
	plate := newTitlePlate("Aeneid", "Aeneis", "la", "Virgil")
	html := renderPattern(t, BookCoverMedia("book-1", plate, domain.BookCover{}), "")
	requireMarkup(t, html, `class="title-plate__greek" lang="la">Aeneis</span>`)
}
