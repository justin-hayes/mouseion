package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chooserWorkInsights serves one coverage reading per corpus.
type chooserWorkInsights struct {
	fixtures.Insights
	coverage map[string]domain.AnalysisCoverage
}

func (i chooserWorkInsights) Coverage(_ context.Context, _, corpusID string) (domain.AnalysisCoverage, error) {
	return i.coverage[corpusID], nil
}

// chooserWorkBook is an assessed Book linked to a reviewed unit.
func chooserWorkBook(bookID, title, author, unitKey, corpusID string) domain.MyBook {
	book := chooserReadyCandidate()
	book.Book.ID = bookID
	book.Book.Title = title
	book.Book.Author = author
	book.Book.LanguageTag = classical.Language
	book.ReadingUnit = &domain.BookReadingUnit{ReadingUnitID: "unit-" + bookID, ReadingUnitKey: unitKey}
	book.Acquired.CorpusID = corpusID
	return book
}

// workDefinitions define six Iliad Books, one Anabasis Book and a whole-work
// Hellenica, so grouped, ordinary and whole-work candidates can be exercised
// together.
func workDefinitions() []classical.Definition {
	one, two, three, four, five, six := 1, 2, 3, 4, 5, 6
	return []classical.Definition{
		catalogFixtureDefinition("xenophon-anabasis-1", "xenophon-anabasis", "Anabasis", "Xenophon", "Ἀνάβασις", "xenophon-anabasis-marchant-1904", "1.1.1", "1.10.19", &one, 7, 9305, false, true),
		catalogFixtureDefinition("xenophon-hellenica-1", "xenophon-hellenica", "Hellenica", "Xenophon", "Ἑλληνικά", "xenophon-hellenica-dindorf-1866", "1.1.1", "1.1.10", nil, 1, 500, false, false),
		catalogFixtureDefinition("homer-iliad-1", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", "homer-iliad-monro-1902", "1.1.1", "1.1.10", &one, 24, 100, false, false),
		catalogFixtureDefinition("homer-iliad-2", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", "homer-iliad-monro-1902", "2.1.1", "2.1.10", &two, 24, 200, false, false),
		catalogFixtureDefinition("homer-iliad-3", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", "homer-iliad-monro-1902", "3.1.1", "3.1.10", &three, 24, 300, false, false),
		catalogFixtureDefinition("homer-iliad-4", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", "homer-iliad-monro-1902", "4.1.1", "4.1.10", &four, 24, 400, false, false),
		catalogFixtureDefinition("homer-iliad-5", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", "homer-iliad-monro-1902", "5.1.1", "5.1.10", &five, 24, 500, false, false),
		catalogFixtureDefinition("homer-iliad-6", "homer-iliad", "Iliad", "Homer", "Ἰλιάς", "homer-iliad-monro-1902", "6.1.1", "6.1.10", &six, 24, 600, false, false),
	}
}

func workCatalog(t *testing.T) *classical.Catalog {
	t.Helper()
	catalog, err := classical.NewCatalog(workDefinitions())
	require.NoError(t, err)
	return catalog
}

func chooserCoverage(known, total int64) domain.AnalysisCoverage {
	return domain.AnalysisCoverage{KnownTokenCount: known, AnalyzableTokenCount: total}
}

// endFixtureReading ends the fixture's current reading in one language.
func endFixtureReading(t *testing.T, store *fixtures.Store, language string) {
	t.Helper()
	current, err := store.GetCurrentReading(t.Context(), fixtures.OwnerID, language)
	require.NoError(t, err)
	if current.IsActive() {
		require.NoError(t, store.EndCurrentReading(t.Context(), fixtures.OwnerID, language, current.BookID, current.SnapshotID))
	}
}

// fixtureLanguageClassicalBooks makes the synthetic classical Books candidates
// in the fixture's active Italian language. The reviewed Work facts still come
// from the shared catalog definitions.
func fixtureLanguageClassicalBooks(books []domain.MyBook) []domain.MyBook {
	for i := range books {
		books[i].Book.LanguageTag = "it"
	}
	return books
}

// groupedChooserBooks builds one work with four To Read Books covering the
// numeric, no-comparison, in-progress and not-assessed states, plus a
// single-book work.
func groupedChooserBooks() ([]domain.MyBook, chooserWorkInsights) {
	books := []domain.MyBook{
		chooserWorkBook("iliad-1", "Iliad", "Homer", "homer-iliad-1", "corpus-1"),
		chooserWorkBook("iliad-2", "Iliad", "Homer", "homer-iliad-2", "corpus-2"),
		chooserWorkBook("iliad-3", "Iliad", "Homer", "homer-iliad-3", "corpus-3"),
		chooserWorkBook("iliad-4", "Iliad", "Homer", "homer-iliad-4", "corpus-4"),
		chooserWorkBook("anabasis-1", "Anabasis", "Xenophon", "xenophon-anabasis-1", "corpus-anabasis"),
	}
	books[2].Acquired.Signals = testQueued
	books[3].Acquired = nil
	insights := chooserWorkInsights{coverage: map[string]domain.AnalysisCoverage{
		"corpus-1":        chooserCoverage(99, 100),
		"corpus-2":        chooserCoverage(92, 100),
		"corpus-3":        chooserCoverage(1, 1),
		"corpus-4":        chooserCoverage(1, 1),
		"corpus-anabasis": chooserCoverage(96, 100),
	}}
	return books, insights
}

func TestReadingChooserWorkHelpersCoverFallbacks(t *testing.T) {
	book := chooserWorkBook("fallback-1", "Fallback", "Author", "", "corpus")
	entry := readingChooserWorkEntry("fallback-work", []readingChooserBookView{{Book: book}})
	assert.Equal(t, "Fallback", entry.Title)
	assert.Equal(t, "Author", entry.Author)
	assert.Equal(t, 1, entry.WorkBooks)

	assert.Equal(t, "97% to below 99%", readingChooserBandLabel(domain.CoverageBand97To99))
	assert.Equal(t, "No vocabulary comparison", readingChooserBandLabel(domain.CoverageBandNoComparison))
	assert.Empty(t, readingChooserWorkCoverageRange(nil))
	assert.Equal(t, "Known coverage 94.9%", readingChooserWorkCoverageRange([]readingChooserBookView{{
		Coverage: &domain.AnalysisCoverage{KnownTokenCount: 949, AnalyzableTokenCount: 1000},
	}}))
	assert.Empty(t, readingChooserBookNumber(readingChooserBookView{Book: book}), "a whole-work unit has no book number cell")

	noComparison := readingChooserEntryView{Books: []readingChooserBookView{{
		Coverage: &domain.AnalysisCoverage{},
	}}}
	assert.Equal(t, "1 To Read Book, 1 assessed", readingChooserWorkCounts(noComparison), "a no-comparison Book has an assessment but no numeric range")
	assert.Empty(t, readingChooserWorkCoverageRange(noComparison.Books))

	first := readingChooserEntryView{Key: "book:a", Title: "Same", Author: "Alpha"}
	second := readingChooserEntryView{Key: "book:b", Title: "Same", Author: "Beta"}
	assert.True(t, readingChooserEntryLess(first, second), "author breaks a title tie")
	assert.True(t, readingChooserEntryLess(first, readingChooserEntryView{Key: "book:c", Title: "Same", Author: "Alpha"}), "key breaks a full tie")
}

func TestReadingChooserModernLanguageCandidatesStayOrdinary(t *testing.T) {
	for _, language := range []string{"de", "it", "fr", "el"} {
		t.Run(language, func(t *testing.T) {
			book := chooserReadyCandidate()
			book.Book.LanguageTag = language
			book.Book.Title = "Modern " + language
			view, err := newChooserHandler([]domain.MyBook{book}, nil, chooserWorkInsights{
				coverage: map[string]domain.AnalysisCoverage{"corpus-book-1": chooserCoverage(960, 1000)},
			}).buildReadingChooser(context.Background(), fixtures.OwnerID, language, language, "")
			require.NoError(t, err)
			require.Len(t, view.Entries, 1)
			assert.False(t, view.Entries[0].Grouped, "a modern-language Book has no reviewed Work to group")
			assert.Equal(t, "95% to below 97%", readingChooserBookBandLabel(view.Entries[0].Books[0]))
		})
	}
}

func TestReadingChooserCompletionReceiptNeverPointsToTheNextBook(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, CurrentReadingFinish(currentReadingFinishView{BookTitle: "Finished Book"}).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `href="/reading">Choose a To Read book</a>`)
	for _, unwanted := range []string{"next Book", "Book 2", "Continue", "Start reading"} {
		assert.NotContains(t, html, unwanted)
	}
}

func TestReadingChooserGroupsAWorkAndOrdersBands(t *testing.T) {
	books, insights := groupedChooserBooks()
	handler := newChooserHandler(books, nil, insights)
	handler.services.Classical = workCatalog(t)

	view, err := handler.buildReadingChooser(context.Background(), fixtures.OwnerID, classical.Language, "Ancient Greek", "")
	require.NoError(t, err)
	require.Len(t, view.Entries, 2, "Iliad is one entry and the single Anabasis Book stays ordinary")

	anabasis := view.Entries[0]
	assert.False(t, anabasis.Grouped)
	assert.Equal(t, "Anabasis", anabasis.Title)
	assert.Equal(t, "95% to below 97%", readingChooserBookBandLabel(anabasis.Books[0]), "a single-Book work's margin names its band")
	assert.Equal(t, "Book 1 of 7", readingChooserWorkPosition(anabasis.Books[0]))
	assert.Empty(t, readingChooserWorkPosition(readingChooserBookView{Position: 0, WorkBooks: 1}), "a whole-work unit shows no position")

	iliad := view.Entries[1]
	require.True(t, iliad.Grouped)
	assert.Equal(t, "Iliad", iliad.Title)
	assert.Equal(t, "Homer", iliad.Author)
	assert.Equal(t, "Ἰλιάς", iliad.GreekTitle)
	assert.Equal(t, 24, iliad.WorkBooks)
	assert.Equal(t, "4 To Read Books, 2 assessed", readingChooserWorkCounts(iliad))
	assert.Equal(t, "Known coverage 92.0%–99.0%", readingChooserWorkCoverageRange(iliad.Books))

	var labels []string
	for _, band := range iliad.Bands {
		labels = append(labels, band.Label)
	}
	assert.Equal(t, []string{"99% or above", "Below 95%", "Analysis in progress or Updating", "Not assessed"}, labels, "bands keep the shipped order and omit empty rows")
	require.Len(t, iliad.Bands[0].Books, 1)
	assert.Equal(t, 1, iliad.Bands[0].Books[0].Position)
	assert.Equal(t, 2, iliad.Bands[1].Books[0].Position)
	assert.Equal(t, "Iliad, Book 1, Known coverage 99.0%", readingChooserBookAccessibleLabel(iliad.Bands[0].Books[0]))
	assert.Equal(t, "1", readingChooserBookNumber(iliad.Bands[0].Books[0]), "only the book number is visible")
}

func TestReadingChooserWorkCellsExcludeCurrentAndHiddenBooks(t *testing.T) {
	books, insights := groupedChooserBooks()
	books[0].Hidden = true
	handler := newChooserHandler(books, nil, insights)
	handler.services.Classical = workCatalog(t)

	view, err := handler.buildReadingChooser(context.Background(), fixtures.OwnerID, classical.Language, "Ancient Greek", "iliad-2")
	require.NoError(t, err)
	require.Len(t, view.Entries, 2)
	var iliad readingChooserEntryView
	for _, entry := range view.Entries {
		if entry.Grouped {
			iliad = entry
		}
	}
	require.True(t, iliad.Grouped)
	for _, entry := range view.Entries {
		for _, book := range entry.Books {
			assert.NotEqual(t, "iliad-1", book.Book.Book.ID, "hidden Book must not be a cell")
			assert.NotEqual(t, "iliad-2", book.Book.Book.ID, "current Book must not be a cell")
		}
	}
	assert.Equal(t, "2 To Read Books, 0 assessed", readingChooserWorkCounts(iliad))
}

func TestReadingChooserRendersGroupedWorkEntryAndInlineEntries(t *testing.T) {
	h, cookies, _, store := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = workCatalog(t)
	})
	endFixtureReading(t, store, "it")
	require.NoError(t, store.SetActiveStudyLanguage(t.Context(), fixtures.OwnerID, "it"))
	handler, ok := h.(*Handler)
	require.True(t, ok)
	books, insights := groupedChooserBooks()
	handler.services.Store = storeDependencies(chooserEvidenceBooks{Store: store, books: fixtureLanguageClassicalBooks(books)})
	handler.services.AnalysisInsights = insights

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()

	for _, want := range []string{
		"<h3 class=\"bibliographic-title\" id=\"reading-chooser-work-work:homer-iliad\">Iliad</h3>",
		"Ἰλιάς",
		"24 books",
		"4 To Read Books, 2 assessed",
		"Known coverage 92.0%–99.0%",
		"99% or above",
		"Below 95%",
		"Analysis in progress or Updating",
		"Not assessed",
		`href="#reading-chooser-book-iliad-1"`,
		`href="#reading-chooser-book-iliad-3"`,
		`aria-label="Iliad, Book 1, Known coverage 99.0%"`,
		`action="/reading/books/iliad-1/start"`,
		`action="/reading/books/iliad-4/reanalyze"`,
		"Book 1 of 24",
	} {
		assert.Contains(t, body, want)
	}
	marginStart := strings.Index(body, "reading-chooser-work__margin")
	booksStart := strings.Index(body, "reading-chooser-work__books")
	require.True(t, marginStart >= 0 && booksStart > marginStart, "grouped margin is missing")
	iliadBands := body[marginStart:booksStart]
	for _, absent := range []string{"97% to below 99%", "95% to below 97%", "No vocabulary comparison"} {
		assert.NotContains(t, iliadBands, absent, "an empty band row is omitted")
	}
	assert.Less(t, strings.Index(body, "Anabasis"), strings.Index(body, "Iliad"), "grouped and ordinary entries share neutral title order")
}

func TestReadingChooserGroupedInlineEntryOffersStartAndRetry(t *testing.T) {
	h, cookies, _, store := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = workCatalog(t)
	})
	endFixtureReading(t, store, "it")
	require.NoError(t, store.SetActiveStudyLanguage(t.Context(), fixtures.OwnerID, "it"))
	handler, ok := h.(*Handler)
	require.True(t, ok)
	books, insights := groupedChooserBooks()
	handler.services.Store = storeDependencies(chooserEvidenceBooks{Store: store, books: fixtureLanguageClassicalBooks(books)})
	handler.services.AnalysisInsights = insights

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `action="/reading/books/iliad-1/start"`, "assessed inline entries start a Reading")
	assert.Contains(t, body, `action="/reading/books/iliad-4/reanalyze"`, "a not-assessed inline entry offers Retry")
}
