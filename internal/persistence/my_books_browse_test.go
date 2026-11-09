package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestMyBooksWorkMatchesQuery(t *testing.T) {
	unit := &domain.BookReadingUnit{WorkTitle: "Iliad", WorkAuthor: "Homer", GreekTitle: "Ἰλιάς"}
	books := []domain.MyBook{{Book: domain.Book{ID: "1", Title: "Iliad", Author: "Homer"}}}
	for _, tt := range []struct {
		name    string
		query   string
		matches bool
		narrow  int
	}{
		{name: "empty", query: "", matches: true},
		{name: "english title", query: "iliad", matches: true},
		{name: "author", query: "homer", matches: true},
		{name: "greek title ignoring accents", query: "ιλιασ", matches: true},
		{name: "trailing number", query: "iliad 6", matches: true, narrow: 6},
		{name: "trailing number with nonmatching text", query: "odyssey 6", matches: false},
		{name: "no match", query: "something", matches: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			matches, narrow := myBooksWorkMatchesQuery(unit, books, tt.query)
			assert.Equal(t, tt.matches, matches)
			assert.Equal(t, tt.narrow, narrow)
		})
	}

	fallbackBooks := []domain.MyBook{{Book: domain.Book{ID: "2", Title: "Something else", Author: "Nobody"}}}
	matches, narrow := myBooksWorkMatchesQuery(unit, fallbackBooks, "something")
	assert.True(t, matches, "an individual Book title still matches")
	assert.Zero(t, narrow)
}

func TestMyBooksBrowseFinishCountsOnlyWorkBooksInTheShapeTotals(t *testing.T) {
	workBook := func(id string, position int) domain.MyBook {
		return domain.MyBook{
			Book: domain.Book{ID: id, Title: "Iliad"},
			ReadingUnit: &domain.BookReadingUnit{
				WorkKey: "homer-iliad", WorkTitle: "Iliad", ConstituentBook: position, ConstituentBooks: 24,
			},
			Disposition: domain.BookDispositionInbox,
		}
	}
	ordinary := domain.MyBook{Book: domain.Book{ID: "ordinary", Title: "A German Book", LanguageTag: "de"}, Disposition: domain.BookDispositionInbox}

	accumulator := &myBooksBrowseAccumulator{
		window:          myBooksBrowsePageWindow(0, 25),
		ownedWorkCounts: map[string]int{"homer-iliad": 2},
		books:           []domain.MyBook{workBook("iliad-1", 1), workBook("iliad-2", 2), ordinary},
	}
	accumulator.finish()

	assert.Equal(t, 3, accumulator.result.Total, "every matching Book counts toward the result total")
	assert.Equal(t, 2, accumulator.result.BookTotal, "the shape line counts only Books in reviewed Works")
	assert.Equal(t, 1, accumulator.result.WorkTotal)
	assert.Equal(t, 2, accumulator.result.RowCount, "the Work row and the ordinary row each page once")
	assert.True(t, accumulator.result.HasWorks)
}

func TestSplitTrailingBookNumber(t *testing.T) {
	for _, tt := range []struct {
		query  string
		text   string
		number int
		ok     bool
	}{
		{query: "iliad 6", text: "iliad", number: 6, ok: true},
		{query: "  iliad   6  ", text: "iliad", number: 6, ok: true},
		{query: "iliad", text: "iliad"},
		{query: "6", text: "6"},
		{query: "iliad zero", text: "iliad zero"},
		{query: "iliad 0", text: "iliad 0"},
	} {
		t.Run(tt.query, func(t *testing.T) {
			text, number, ok := splitTrailingBookNumber(tt.query)
			assert.Equal(t, tt.text, text)
			assert.Equal(t, tt.number, number)
			assert.Equal(t, tt.ok, ok)
		})
	}
}
