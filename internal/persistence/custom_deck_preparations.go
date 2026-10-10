package persistence

import (
	"context"
)

// FailCustomDeckPreparation marks a preparation queued before Custom decks were
// retired (ADR 0085) as failed. It is the only Custom deck preparation write
// left; stored preparation, identity, and artifact rows are never rewritten.
func (s *PostgresStore) FailCustomDeckPreparation(ctx context.Context, owner, id, message string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE custom_vocabulary_deck_preparations SET state='failed',error=$3,completed_at=now()
 WHERE owner_id=$1 AND id=$2 AND state IN ('queued','preparing')`, owner, id, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}
