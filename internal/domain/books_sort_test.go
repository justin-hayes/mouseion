package domain

import "testing"

func TestSortMyBooksMostReadableFirst(t *testing.T) {
	covered := func(id, title string, mature, total int64) MyBook {
		return MyBook{Book: Book{ID: id, Title: title}, CoverageMatureTokens: mature, CoverageTotalTokens: total}
	}
	tests := []struct {
		name  string
		books []MyBook
		want  []string
	}{
		{
			name: "the highest Mature share comes first",
			books: []MyBook{
				covered("half", "Half", 1, 2),
				covered("full", "Full", 4, 5),
			},
			want: []string{"full", "half"},
		},
		{
			name: "an equal share is broken by title order",
			books: []MyBook{
				covered("beta", "Beta", 2, 4),
				covered("alpha", "Alpha", 1, 2),
			},
			want: []string{"alpha", "beta"},
		},
		{
			name: "books without a coverage figure sort last",
			books: []MyBook{
				covered("none", "None", 0, 0),
				covered("low", "Low", 1, 4),
			},
			want: []string{"low", "none"},
		},
		{
			name: "books without a coverage figure keep title order among themselves",
			books: []MyBook{
				covered("zeta-none", "Zeta", 0, 0),
				covered("alpha-none", "Alpha", 0, 0),
				covered("covered", "Covered", 1, 4),
			},
			want: []string{"covered", "alpha-none", "zeta-none"},
		},
		{
			name: "an updating Book with a zero total is last even beside Mature zeros",
			books: []MyBook{
				covered("zero-mature", "Zero Mature", 0, 4),
				covered("updating", "Updating", 0, 0),
			},
			want: []string{"zero-mature", "updating"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortMyBooksMostReadableFirst(tt.books)
			got := make([]string, 0, len(tt.books))
			for _, book := range tt.books {
				got = append(got, book.Book.ID)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("position %d: got %v, want %v", i, got, tt.want)
				}
			}
		})
	}
}
