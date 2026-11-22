package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// MarkClassicalReadingUnitPreviouslyRead creates or restores the learner's Book
// for one current Reading unit as Read. It records one Previously read
// assertion in the same transaction and starts no preparation.
func (s *PostgresStore) MarkClassicalReadingUnitPreviouslyRead(ctx context.Context, owner, readingUnitKey string) (result domain.CatalogueReadResult, err error) {
	if owner == "" || readingUnitKey == "" {
		return domain.CatalogueReadResult{}, errors.New("persistence: classical previously read identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err := lockClassicalAddKeys(ctx, tx, owner, []string{readingUnitKey}); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := refuseClassicalAddOnLegacyRows(ctx, q); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if _, err := q.PutSupportedLanguage(ctx, sqlcgen.PutSupportedLanguageParams{
		Language: classical.Language, DisplayName: classical.LanguageDisplayName,
	}); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	unit, err := q.GetCurrentClassicalUnitByKey(ctx, readingUnitKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CatalogueReadResult{}, errors.New("persistence: classical Reading unit " + readingUnitKey + " is not current")
	}
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	bookID, err := q.GetRegisteredBookID(ctx, sqlcgen.GetRegisteredBookIDParams{OwnerID: owner, ReadingUnitID: unit.ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		result, err = createReadClassicalBookTx(ctx, q, owner, unit)
	case err != nil:
		return domain.CatalogueReadResult{}, err
	default:
		result, err = restoreReadClassicalBookTx(ctx, q, owner, bookID)
	}
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	return result, nil
}

// createReadClassicalBookTx creates the metadata-only Book linked to the
// Reading unit and records it as Read in one transaction.
func createReadClassicalBookTx(ctx context.Context, q *sqlcgen.Queries, owner string, unit sqlcgen.GetCurrentClassicalUnitByKeyRow) (domain.CatalogueReadResult, error) {
	book, err := q.InsertBook(ctx, sqlcgen.InsertBookParams{
		OwnerID:            owner,
		Title:              unit.WorkTitle,
		Author:             unit.WorkAuthor,
		MetadataProvenance: domain.MetadataProvenanceOperatorRegistration,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        textArg(classical.Language),
	})
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := q.InsertBookMembership(ctx, sqlcgen.InsertBookMembershipParams{OwnerID: owner, BookID: book.ID}); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := upsertBookDisposition(ctx, q, owner, book.ID, domain.BookDispositionInbox); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := q.InsertBookReadingUnit(ctx, sqlcgen.InsertBookReadingUnitParams{OwnerID: owner, BookID: book.ID, ReadingUnitID: unit.ID}); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if _, err := insertPreviouslyReadTx(ctx, q, owner, book.ID); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	return domain.CatalogueReadResult{Book: domain.Book(book), Created: true, Changed: true}, nil
}

// restoreReadClassicalBookTx makes an existing Book reachable again and records
// it as Read. Its Reading unit, analysis, corrections and reading history stay
// untouched.
func restoreReadClassicalBookTx(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) (domain.CatalogueReadResult, error) {
	changed, err := restoreReadBookMembershipTx(ctx, q, owner, bookID)
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	book, err := q.GetBook(ctx, sqlcgen.GetBookParams{OwnerID: owner, ID: bookID})
	if err != nil {
		return domain.CatalogueReadResult{}, missing(err)
	}
	recorded, err := insertPreviouslyReadTx(ctx, q, owner, bookID)
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	return domain.CatalogueReadResult{Book: domain.Book(book), Changed: changed || recorded}, nil
}
