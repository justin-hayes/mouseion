package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestMyBooksBrowseMatchesQuery(t *testing.T) {
	classical := domain.MyBook{
		Book:        domain.Book{ID: "1", Title: "Histories, Book 1", Author: "Herodotus"},
		ReadingUnit: &domain.BookReadingUnit{WorkTitle: "Iliad", WorkAuthor: "Homer", GreekTitle: "Ἰλιάς"},
	}
	ordinary := domain.MyBook{Book: domain.Book{ID: "2", Title: "Something else", Author: "Nobody"}}
	for _, tt := range []struct {
		name    string
		book    domain.MyBook
		query   string
		matches bool
	}{
		{name: "empty", book: ordinary, query: "", matches: true},
		{name: "ordinary title", book: ordinary, query: "something", matches: true},
		{name: "ordinary author", book: ordinary, query: "nobody", matches: true},
		{name: "ordinary no match", book: ordinary, query: "iliad", matches: false},
		{name: "classical Book title", book: classical, query: "histories", matches: true},
		{name: "classical English title", book: classical, query: "iliad", matches: true},
		{name: "classical author", book: classical, query: "homer", matches: true},
		{name: "greek title ignoring accents", book: classical, query: "ιλιασ", matches: true},
		{name: "trailing number is ordinary text", book: classical, query: "iliad 6", matches: false},
		{name: "no match", book: classical, query: "something", matches: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matches, myBooksBrowseMatchesQuery(tt.book, tt.query))
		})
	}
}

func TestMyBooksBrowseFinishPagesEveryBookAsOneRow(t *testing.T) {
	workBook := func(id string, position int) domain.MyBook {
		return domain.MyBook{
			Book: domain.Book{ID: id, Title: "Iliad"},
			ReadingUnit: &domain.BookReadingUnit{
				WorkKey: "homer-iliad", WorkTitle: "Iliad", ConstituentBook: position, ConstituentBooks: 24,
			},
			Disposition: domain.BookDispositionToRead,
		}
	}
	ordinary := domain.MyBook{Book: domain.Book{ID: "ordinary", Title: "A German Book", LanguageTag: "de"}, Disposition: domain.BookDispositionToRead}

	accumulator := &myBooksBrowseAccumulator{
		window: myBooksBrowsePageWindow(1, 2),
		books:  []domain.MyBook{workBook("iliad-1", 1), workBook("iliad-2", 2), ordinary},
	}
	accumulator.finish()

	assert.Equal(t, 3, accumulator.result.Total, "every matching Book is its own row")
	if assert.Len(t, accumulator.result.Items, 2) {
		assert.Equal(t, "iliad-2", accumulator.result.Items[0].Book.ID)
		assert.Equal(t, "ordinary", accumulator.result.Items[1].Book.ID)
	}
}
