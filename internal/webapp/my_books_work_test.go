package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func classicalWorkBook(id, workKey, workTitle, author, greek string, position, total int, disposition domain.BookDisposition, current bool) domain.MyBook {
	return domain.MyBook{
		Book: domain.Book{
			ID: id, OwnerID: "owner", Title: workTitle, Author: author,
			LanguageState: domain.LanguageChosen, LanguageTag: "grc",
		},
		ReadingUnit: &domain.BookReadingUnit{
			WorkKey: workKey, WorkTitle: workTitle, WorkAuthor: author, GreekTitle: greek,
			ConstituentBook: position, ConstituentBooks: total,
		},
		Disposition:      disposition,
		IsCurrentReading: current,
	}
}

func TestMyBooksGroupedWorkRowRendersAccessibleCellsAndOneLegend(t *testing.T) {
	first := classicalWorkBook("iliad-1", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", 1, 24, domain.BookDispositionToRead, true)
	second := classicalWorkBook("iliad-2", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", 2, 24, domain.BookDispositionInbox, false)
	second.CompletionCount = 1
	third := classicalWorkBook("anabasis-1", "xenophon-anabasis", "Anabasis", "Xenophon", "Ἀνάβασις", 1, 7, domain.BookDispositionInbox, false)
	fourth := classicalWorkBook("anabasis-2", "xenophon-anabasis", "Anabasis", "Xenophon", "Ἀνάβασις", 2, 7, domain.BookDispositionInbox, false)

	rows := []domain.MyBooksBrowseRow{
		{
			Key: "work:homer-iliad", Grouped: true, WorkKey: "homer-iliad", WorkTitle: "Iliad",
			WorkAuthor: "Homer", GreekTitle: "Ἰλιάς", ConstituentBooks: 24, OwnedBooks: 2,
			Books: []domain.MyBook{first, second},
		},
		{
			Key: "work:xenophon-anabasis", Grouped: true, WorkKey: "xenophon-anabasis", WorkTitle: "Anabasis",
			WorkAuthor: "Xenophon", GreekTitle: "Ἀνάβασις", ConstituentBooks: 7, OwnedBooks: 2,
			Books: []domain.MyBook{third, fourth},
		},
	}
	browse := MyBooksBrowseState{
		Enabled: true, Language: "grc", LanguageLabel: "Ancient Greek",
		AllCount: 4, ScopeTotal: 4, Total: 4, BookTotal: 4, WorkTotal: 2, RowCount: 2, HasWorks: true,
		Rows: rows,
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{first, second, third, fourth}, "", "", "", false, browse).Render(context.Background(), &output))
	html := output.String()

	assert.Equal(t, 1, strings.Count(html, `class="library-work-legend"`), "one legend per page")
	assert.Contains(t, html, "4 Books in 2 works")
	assert.Contains(t, html, "2 of 24 books")
	assert.Contains(t, html, "2 of 7 books")
	assert.Contains(t, html, "Currently reading Book 1")
	assert.Contains(t, html, "1 Read")
	assert.Contains(t, html, `aria-label="Iliad, Book 1 of 24, Currently reading"`)
	assert.Contains(t, html, `aria-label="Iliad, Book 2 of 24, Read"`)
	assert.Contains(t, html, `id="book-row-iliad-1"`)
	assert.Contains(t, html, `id="book-row-iliad-2"`)
	assert.Contains(t, html, `href="/reading#journey-book-iliad-1"`)
	assert.Contains(t, html, `href="/reading#journey-book-iliad-2"`)
	assert.Contains(t, html, `class="library-work__provenance">Classical catalog</span>`)
	assert.Contains(t, html, `lang="grc">Ἰλιάς</span>`)
	assert.Contains(t, html, `status-badge__shape--dot`)
	assert.Contains(t, html, `status-badge__shape--bar`)
	assert.Contains(t, html, `status-badge__shape--square`)

	groupedStart := strings.Index(html, `id="work-row-homer-iliad"`)
	require.GreaterOrEqual(t, groupedStart, 0)
	grouped := html[groupedStart:]
	if end := strings.Index(grouped, "</li>"); end >= 0 {
		grouped = grouped[:end]
	}
	for _, forbidden := range []string{"Move to To Read", "More actions", "Hide", "Unhide", "Refresh metadata"} {
		assert.NotContains(t, grouped, forbidden, "grouped Work row exposed a per-Book action %q", forbidden)
	}
}

func TestMyBooksSingleBookWorkRowKeepsItsPositionAndOrdinaryActions(t *testing.T) {
	book := classicalWorkBook("histories-1", "herodotus-histories", "Histories", "Herodotus", "Ἱστορίαι", 1, 9, domain.BookDispositionInbox, false)
	browse := MyBooksBrowseState{
		Enabled: true, Language: "grc", LanguageLabel: "Ancient Greek",
		AllCount: 1, ScopeTotal: 1, Total: 1, BookTotal: 1, WorkTotal: 1, RowCount: 1, HasWorks: true,
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, browse).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{"Histories, Book 1", "Book 1 of 9", "Ἱστορίαι", "Classical catalog", "Move to To Read", "More actions"} {
		assert.Contains(t, html, want, "single-Book Work row missing %q", want)
	}
	assert.NotContains(t, html, `class="library-work"`)
	assert.NotContains(t, html, "library-work-legend")
}

func TestMyBooksWorkSizeLabel(t *testing.T) {
	for _, tt := range []struct {
		name string
		row  domain.MyBooksBrowseRow
		want string
	}{
		{name: "partial ownership", row: domain.MyBooksBrowseRow{OwnedBooks: 2, ConstituentBooks: 24}, want: "2 of 24 books"},
		{name: "all books", row: domain.MyBooksBrowseRow{OwnedBooks: 24, ConstituentBooks: 24}, want: "all 24 books"},
		{name: "whole work", row: domain.MyBooksBrowseRow{OwnedBooks: 1, ConstituentBooks: 1}, want: "1 book"},
		{name: "owned exceeds reviewed total", row: domain.MyBooksBrowseRow{OwnedBooks: 3, ConstituentBooks: 2}, want: "all 2 books"},
		{name: "owned falls back to visible cells", row: domain.MyBooksBrowseRow{Books: []domain.MyBook{{}, {}}, ConstituentBooks: 9}, want: "2 of 9 books"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, myBooksWorkSizeLabel(tt.row))
		})
	}
}

func TestMyBooksShapeLabel(t *testing.T) {
	for _, tt := range []struct {
		name   string
		browse MyBooksBrowseState
		want   string
	}{
		{name: "one book one work", browse: MyBooksBrowseState{BookTotal: 1, WorkTotal: 1}, want: "1 Book in 1 work"},
		{name: "one book many works", browse: MyBooksBrowseState{BookTotal: 1, WorkTotal: 2}, want: "1 Book in 2 works"},
		{name: "many books one work", browse: MyBooksBrowseState{BookTotal: 2, WorkTotal: 1}, want: "2 Books in 1 work"},
		{name: "many books many works", browse: MyBooksBrowseState{BookTotal: 57, WorkTotal: 5}, want: "57 Books in 5 works"},
		{name: "falls back to total and rows", browse: MyBooksBrowseState{Total: 3, RowCount: 2}, want: "3 Books in 2 works"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, myBooksShapeLabel(tt.browse))
		})
	}
}

func TestMyBooksWorkLeadingBadgeAndCounts(t *testing.T) {
	current := classicalWorkBook("current", "work", "Iliad", "Homer", "Ἰλιάς", 3, 24, domain.BookDispositionToRead, true)
	wholeCurrent := classicalWorkBook("whole", "work", "Hellenica", "Xenophon", "", 0, 1, domain.BookDispositionToRead, true)
	toRead := classicalWorkBook("to-read", "work", "Iliad", "Homer", "Ἰλιάς", 1, 24, domain.BookDispositionToRead, false)
	read := classicalWorkBook("read", "work", "Iliad", "Homer", "Ἰλιάς", 2, 24, domain.BookDispositionInbox, false)
	read.CompletionCount = 1
	inbox := classicalWorkBook("inbox", "work", "Iliad", "Homer", "Ἰλιάς", 3, 24, domain.BookDispositionInbox, false)

	for _, tt := range []struct {
		name  string
		books []domain.MyBook
		label string
		tone  StatusTone
		shape string
	}{
		{name: "current constituent", books: []domain.MyBook{current}, label: "Currently reading Book 3", tone: StatusInfo, shape: "dot"},
		{name: "current whole work", books: []domain.MyBook{wholeCurrent}, label: "Currently reading", tone: StatusInfo, shape: "dot"},
		{name: "to read", books: []domain.MyBook{toRead}, label: "To Read", tone: StatusNeutral, shape: "ring"},
		{name: "read", books: []domain.MyBook{read}, label: "Read", tone: StatusNeutral, shape: "bar"},
		{name: "inbox", books: []domain.MyBook{inbox}, label: "Inbox", tone: StatusNeutral, shape: "square"},
		{name: "empty", books: nil, label: "Inbox", tone: StatusNeutral, shape: "square"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			label, tone, shape := myBooksWorkLeadingBadge(tt.books)
			assert.Equal(t, tt.label, label)
			assert.Equal(t, tt.tone, tone)
			assert.Contains(t, shape, "--"+tt.shape)
		})
	}

	moreToRead := make([]domain.MyBook, 0, 23)
	for i := range 23 {
		book := toRead
		book.Book.ID = "to-read-" + string(rune('a'+i))
		moreToRead = append(moreToRead, book)
	}
	assert.Equal(t, "22 more To Read · 1 Read", myBooksWorkCountsLabel(append(moreToRead, read)))
	currentAndMore := append([]domain.MyBook{current}, moreToRead[1:]...)
	currentAndMore = append(currentAndMore, read)
	assert.Equal(t, "22 To Read · 1 Read", myBooksWorkCountsLabel(currentAndMore))
	assert.Equal(t, "2 To Read", myBooksWorkCountsLabel([]domain.MyBook{current, toRead, toRead}))
	assert.Equal(t, "1 more Read", myBooksWorkCountsLabel([]domain.MyBook{read, read}))
	assert.Empty(t, myBooksWorkCountsLabel(nil))
}

func TestMyBooksFallbackRowsGroupsAndSorts(t *testing.T) {
	second := classicalWorkBook("iliad-2", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", 2, 24, domain.BookDispositionInbox, false)
	first := classicalWorkBook("iliad-1", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", 1, 24, domain.BookDispositionInbox, false)
	ordinary := domain.MyBook{Book: domain.Book{ID: "de-1", Title: "Ein Buch", LanguageState: domain.LanguageChosen, LanguageTag: "de"}}

	rows := myBooksFallbackRows(MyBooksBrowseState{Language: "grc"}, []domain.MyBook{second, ordinary, first})
	require.Len(t, rows, 1)
	assert.True(t, rows[0].Grouped)
	assert.Equal(t, "iliad-1", rows[0].Books[0].Book.ID)
	assert.Equal(t, "iliad-2", rows[0].Books[1].Book.ID)
	assert.Equal(t, 2, rows[0].OwnedBooks)

	single := myBooksFallbackRows(MyBooksBrowseState{Language: "grc"}, []domain.MyBook{first})
	require.Len(t, single, 1)
	assert.False(t, single[0].Grouped)

	explicit := []domain.MyBooksBrowseRow{{Key: "explicit", Books: []domain.MyBook{first}}}
	assert.Equal(t, explicit, myBooksBrowseRows(MyBooksBrowseState{Rows: explicit}, nil))
}

func TestMyBooksWorkCellLabelFallsBackToBookIdentity(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "book", Title: "Standalone"}}
	row := domain.MyBooksBrowseRow{}
	assert.Equal(t, "Standalone, Inbox", myBooksWorkCellLabel(row, book))
	assert.Equal(t, "1", myBooksWorkCellNumber(book))

	book.ReadingUnit = &domain.BookReadingUnit{WorkTitle: "Iliad", ConstituentBook: 1, ConstituentBooks: 24}
	assert.Equal(t, "Iliad, Book 1 of 24, Inbox", myBooksWorkCellLabel(domain.MyBooksBrowseRow{WorkTitle: "Iliad"}, book))
	assert.Equal(t, "1", myBooksWorkCellNumber(book))
}

func TestMyBooksGroupedWorkStylesWrapCellsAtCompactWidths(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()
	StaticHandler().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	css := response.Body.String()
	for _, want := range []string{
		".library-work__strip",
		"flex-wrap: wrap",
		"min-width: 44px",
		"min-height: 44px",
		".library-work__cell",
	} {
		assert.Contains(t, css, want, "grouped Work CSS missing %q", want)
	}
	assert.Contains(t, css, "@media (max-width: 40rem)")
	assert.Contains(t, css, ".library-work__identity", "compact layout must keep cover beside title and author")
}
