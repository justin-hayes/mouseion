package persistence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// CurrentDeckPreparationForBookTx returns the Book's current (unretired) deck
// preparation in any state. The operator idiom review re-freezes it in memory,
// so it does not need a ready deck. A Book without one is ErrNotFound.
func (s *PostgresStore) CurrentDeckPreparationForBookTx(ctx context.Context, tx pgx.Tx, bookID string) (domain.DeckPreparation, error) {
	var owner, id string
	err := tx.QueryRow(ctx, `
		SELECT owner_id::text, id::text
		FROM deck_preparations
		WHERE book_id = $1 AND retired_at IS NULL
		ORDER BY updated_at DESC, id DESC
		LIMIT 1`, bookID).Scan(&owner, &id)
	if err != nil {
		return domain.DeckPreparation{}, fmt.Errorf("find current deck preparation for Book %s: %w", bookID, missing(err))
	}
	model, err := sqlcgen.New(tx).GetDeckPreparation(ctx, sqlcgen.GetDeckPreparationParams{Owner: owner, ID: id})
	if err != nil {
		return domain.DeckPreparation{}, fmt.Errorf("load current deck preparation for Book %s: %w", bookID, missing(err))
	}
	return deckPreparationFromModel(model), nil
}
