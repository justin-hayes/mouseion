package persistence

import (
	"context"
	"errors"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// populateMyBooksCoverage enriches a page from the durable Vocabulary Browse
// inventory counts joined to current Known vocabulary at read time. It never
// reads corpus tokens: a Book whose counts are not ready for its current
// analysis keeps a zero figure instead of falling back to a scan.
func (s *PostgresStore) populateMyBooksCoverage(ctx context.Context, owner string, books []domain.MyBook) error {
	if len(books) == 0 {
		return nil
	}
	ids := make([]string, 0, len(books))
	byID := make(map[string]*domain.MyBook, len(books))
	for i := range books {
		books[i].CoverageKnownTokens = 0
		books[i].CoverageTotalTokens = 0
		ids = append(ids, books[i].Book.ID)
		byID[books[i].Book.ID] = &books[i]
	}
	rows, err := s.queries().ListMyBooksCoverage(ctx, sqlcgen.ListMyBooksCoverageParams{Owner: owner, BookIds: ids})
	if err != nil {
		return err
	}
	for _, row := range rows {
		book := byID[row.BookID]
		if book == nil {
			return errors.New("coverage query returned a book outside the requested page")
		}
		book.CoverageTotalTokens = row.TotalTokens
		book.CoverageKnownTokens = min(max(row.KnownTokens, 0), row.TotalTokens)
	}
	return nil
}
