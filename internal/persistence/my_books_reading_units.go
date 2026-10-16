package persistence

import (
	"context"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// populateMyBooksReadingUnits attaches the Reading unit association of an
// operator-registered Book. Ordinary catalogue and acquisition Books keep a nil
// association, so their My Books row is unchanged.
func (s *PostgresStore) populateMyBooksReadingUnits(ctx context.Context, owner string, books []domain.MyBook) error {
	if len(books) == 0 {
		return nil
	}
	ids := make([]string, 0, len(books))
	byID := make(map[string]*domain.MyBook, len(books))
	for i := range books {
		books[i].ReadingUnit = nil
		ids = append(ids, books[i].Book.ID)
		byID[books[i].Book.ID] = &books[i]
	}
	rows, err := s.queries().ListBookReadingUnits(ctx, sqlcgen.ListBookReadingUnitsParams{Owner: owner, BookIds: ids})
	if err != nil {
		return err
	}
	for _, row := range rows {
		book := byID[row.BookID]
		if book == nil {
			continue
		}
		book.ReadingUnit = &domain.BookReadingUnit{
			OwnerID:       owner,
			BookID:        row.BookID,
			ReadingUnitID: row.ReadingUnitID,
			CitationStart: row.CitationStart,
			CitationEnd:   row.CitationEnd,
			EditionLabel:  row.EditionLabel,
		}
	}
	return nil
}
