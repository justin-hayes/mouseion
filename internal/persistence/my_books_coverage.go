package persistence

import (
	"context"
	"errors"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// populateMyBooksCoverage enriches a page from the durable Vocabulary Browse
// inventory counts joined to the learner's Study progress at read time. It never
// reads corpus tokens: a Book whose counts are not ready for its current
// analysis keeps a zero figure instead of falling back to a scan.
func (s *PostgresStore) populateMyBooksCoverage(ctx context.Context, owner string, books []*domain.MyBook) error {
	if len(books) == 0 {
		return nil
	}
	ids := make([]string, 0, len(books))
	byID := make(map[string]*domain.MyBook, len(books))
	for _, book := range books {
		book.CoverageMatureTokens = 0
		book.CoverageInStudyTokens = 0
		book.CoverageTotalTokens = 0
		ids = append(ids, book.Book.ID)
		byID[book.Book.ID] = book
	}
	rows, err := s.queries().ListMyBooksCoverage(ctx, sqlcgen.ListMyBooksCoverageParams{Owner: owner, BookIds: ids, MatureDays: domain.FluencyMaturityDays})
	if err != nil {
		return err
	}
	for _, row := range rows {
		book := byID[row.BookID]
		if book == nil {
			return errors.New("coverage query returned a book outside the requested page")
		}
		book.CoverageTotalTokens = row.TotalTokens
		book.CoverageMatureTokens = min(max(row.MatureTokens, 0), row.TotalTokens)
		book.CoverageInStudyTokens = min(max(row.InStudyTokens, book.CoverageMatureTokens), row.TotalTokens)
	}
	return nil
}
