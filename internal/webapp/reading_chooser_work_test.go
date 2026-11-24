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
// Hellenica, so constituent-Book and whole-work candidates can be exercised
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

func chooserUpdateOffer() domain.AnalysisUpdateCandidate {
	return domain.AnalysisUpdateCandidate{PublishedRunID: "published-run", Target: testUpdateTarget}
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

// chooserWorkBooks builds four Iliad To Read Books covering the numeric,
// no-comparison, in-progress and not-assessed states, plus one Anabasis Book.
func chooserWorkBooks() ([]domain.MyBook, chooserWorkInsights) {
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

func TestReadingChooserBandLabelsAndTitleTieBreaks(t *testing.T) {
	assert.Equal(t, "97% to below 99%", readingChooserBandLabel(domain.CoverageBand97To99))
	assert.Equal(t, "No vocabulary comparison", readingChooserBandLabel(domain.CoverageBandNoComparison))

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

func TestReadingChooserListsOneOrdinaryEntryPerBookInTitleOrder(t *testing.T) {
	books, insights := chooserWorkBooks()
	handler := newChooserHandler(books, nil, insights)
	handler.services.Classical = workCatalog(t)

	view, err := handler.buildReadingChooser(context.Background(), fixtures.OwnerID, classical.Language, "Ancient Greek", "")
	require.NoError(t, err)
	require.Len(t, view.Entries, 5, "every To Read Book is its own entry, even Books of one Work")
	var ids []string
	for _, entry := range view.Entries {
		require.Len(t, entry.Books, 1)
		ids = append(ids, entry.Books[0].Book.Book.ID)
	}
	assert.Equal(t, []string{"anabasis-1", "iliad-1", "iliad-2", "iliad-3", "iliad-4"}, ids, "neutral title order")
	assert.Equal(t, "99% or above", readingChooserBookBandLabel(view.Entries[1].Books[0]))
	assert.Equal(t, "Below 95%", readingChooserBookBandLabel(view.Entries[2].Books[0]))
	assert.Equal(t, "Analysis in progress", readingChooserBookBandLabel(view.Entries[3].Books[0]))
	assert.Equal(t, "Not assessed", readingChooserBookBandLabel(view.Entries[4].Books[0]))
}

func TestReadingChooserSwitchModeExcludesTheCurrentBook(t *testing.T) {
	books, insights := chooserWorkBooks()
	handler := newChooserHandler(books, nil, insights)
	handler.services.Classical = workCatalog(t)

	view, err := handler.buildReadingChooser(context.Background(), fixtures.OwnerID, classical.Language, "Ancient Greek", "iliad-2")
	require.NoError(t, err)
	require.Len(t, view.Entries, 4)
	for _, entry := range view.Entries {
		assert.NotEqual(t, "iliad-2", entry.Books[0].Book.Book.ID, "the current Book is not offered")
	}
}

func chooserWorkBody(t *testing.T, books []domain.MyBook, insights chooserWorkInsights, offers map[string]domain.AnalysisUpdateCandidate) string {
	t.Helper()
	h, cookies, _, store := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = workCatalog(t)
	})
	endFixtureReading(t, store, "it")
	require.NoError(t, store.SetActiveStudyLanguage(t.Context(), fixtures.OwnerID, "it"))
	handler, ok := h.(*Handler)
	require.True(t, ok)
	handler.services.Store = storeDependencies(chooserEvidenceBooks{Store: store, books: fixtureLanguageClassicalBooks(books)})
	handler.services.AnalysisInsights = insights
	if offers != nil {
		handler.services.AnalysisUpdates = &fakeAnalysisUpdates{offers: offers}
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	return response.Body.String()
}

func TestReadingChooserRendersEachBookAsAnOrdinaryEntry(t *testing.T) {
	books, insights := chooserWorkBooks()
	body := chooserWorkBody(t, books, insights, nil)

	for _, want := range []string{
		"99% or above",
		"Below 95%",
		`id="reading-chooser-book-iliad-1"`,
		`id="reading-chooser-book-iliad-3"`,
		`action="/reading/books/iliad-1/start"`,
		`action="/reading/books/iliad-4/reanalyze"`,
	} {
		assert.Contains(t, body, want)
	}
	for _, gone := range []string{"reading-chooser-work", "reading-chooser-band-book", "To Read Books, ", "Known coverage 9", "Book 1 of", "24 books"} {
		assert.NotContains(t, body, gone, "the chooser has no grouped Work entry")
	}
	assert.Less(t, strings.Index(body, "Anabasis"), strings.Index(body, "Iliad"), "entries share neutral title order")
}

func TestReadingChooserDistinguishesAWithdrawnBookFromTheWholeWork(t *testing.T) {
	leftover := chooserWorkBook("leftover", "Iliad", "Homer", "homer-iliad-3", "leftover-corpus")
	leftover.ReadingUnit.WorkTitle = "Iliad"
	leftover.ReadingUnit.ConstituentBook = 3
	leftover.ReadingUnit.ConstituentBooks = 24
	leftover.ReadingUnit.Withdrawn = true
	whole := chooserWorkBook("whole", "Iliad", "Homer", "homer-iliad", "whole-corpus")
	whole.ReadingUnit.ConstituentBooks = 24
	insights := chooserWorkInsights{coverage: map[string]domain.AnalysisCoverage{
		"leftover-corpus": chooserCoverage(99, 100), "whole-corpus": chooserCoverage(93, 100),
	}}

	body := chooserWorkBody(t, []domain.MyBook{leftover, whole}, insights, nil)
	require.Contains(t, body, ">Iliad, Book 3</")
	require.Contains(t, body, ">Iliad</")
}

func TestReadingChooserOffersTheOrdinaryAnalysisUpdateOnEachBook(t *testing.T) {
	books, insights := chooserWorkBooks()
	offers := map[string]domain.AnalysisUpdateCandidate{}
	for i := range books {
		if books[i].Acquired == nil {
			continue
		}
		books[i].Acquired.Source.ID = "source-" + books[i].Book.ID
		if books[i].Book.ID == "iliad-1" || books[i].Book.ID == "iliad-2" {
			offers[books[i].Acquired.Source.ID] = chooserUpdateOffer()
		}
	}
	body := chooserWorkBody(t, books, insights, offers)

	assert.Equal(t, 2, strings.Count(body, "A newer analysis is available."), "each outdated Book shows its own notice")
	assert.Equal(t, 2, strings.Count(body, "Confirm analysis update"))
	assert.Contains(t, body, `action="/reading/books/iliad-1/analysis-update"`)
	assert.Contains(t, body, `action="/reading/books/iliad-2/analysis-update"`)
	assert.NotContains(t, body, "/library/works/", "there is no Work-level update action")
}

func TestReadingChooserOffersAWholeWorkBookTheOrdinaryAnalysisUpdate(t *testing.T) {
	book := chooserWorkBook("hellenica", "Hellenica", "Xenophon", "xenophon-hellenica-1", "corpus-hellenica")
	book.Acquired.Source.ID = "source-hellenica"
	insights := chooserWorkInsights{coverage: map[string]domain.AnalysisCoverage{"corpus-hellenica": chooserCoverage(99, 100)}}
	body := chooserWorkBody(t, []domain.MyBook{book}, insights, map[string]domain.AnalysisUpdateCandidate{"source-hellenica": chooserUpdateOffer()})

	assert.Equal(t, 1, strings.Count(body, "A newer analysis is available."))
	assert.Contains(t, body, `action="/reading/books/hellenica/analysis-update"`)
	assert.NotContains(t, body, "/library/works/")
}
