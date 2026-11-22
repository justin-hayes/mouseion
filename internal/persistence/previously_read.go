package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// MarkBookPreviouslyRead records an unstarted To Read Book as read outside
// Mouseion and clears its To Read disposition in one transaction. It refuses
// the Current reading. A Book that already has reading history is unchanged, so
// a replay is idempotent.
func (s *PostgresStore) MarkBookPreviouslyRead(ctx context.Context, owner, bookID string) (err error) {
	if owner == "" || bookID == "" {
		return errors.New("persistence: previously read identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err := lockBookForUpdate(ctx, q, owner, bookID); err != nil {
		return err
	}
	current, err := q.BookHasActiveCurrentReading(ctx, sqlcgen.BookHasActiveCurrentReadingParams{Owner: owner, Book: bookID})
	if err != nil {
		return err
	}
	if current {
		return domain.ErrBookIsCurrentReading
	}
	record, err := previouslyReadDecisionTx(ctx, q, owner, bookID)
	if err != nil {
		return err
	}
	if !record {
		return tx.Commit(ctx)
	}
	recorded, err := insertPreviouslyReadTx(ctx, q, owner, bookID)
	if err != nil {
		return err
	}
	if !recorded {
		return ErrNotFound
	}
	if err := upsertBookDisposition(ctx, q, owner, bookID, domain.BookDispositionInbox); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// previouslyReadDecisionTx decides whether the learner may record the Book:
// true means insert the assertion, false is an idempotent no-op for an already
// read Book. A To Read Book with history was already read, so it is refused
// rather than cleared again.
func previouslyReadDecisionTx(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) (bool, error) {
	disposition, err := previouslyReadDispositionTx(ctx, q, owner, bookID)
	if err != nil {
		return false, err
	}
	history, err := q.GetMyBookCompletionSummary(ctx, sqlcgen.GetMyBookCompletionSummaryParams{Owner: owner, Book: bookID})
	if err != nil {
		return false, err
	}
	if history.CompletionCount > 0 {
		if disposition == domain.BookDispositionToRead {
			return false, domain.ErrPreviouslyReadIneligible
		}
		return false, nil
	}
	if disposition != domain.BookDispositionToRead {
		return false, domain.ErrPreviouslyReadIneligible
	}
	started, err := q.BookEverCurrentReading(ctx, sqlcgen.BookEverCurrentReadingParams{Owner: owner, Book: bookID})
	if err != nil {
		return false, err
	}
	if started {
		return false, domain.ErrPreviouslyReadIneligible
	}
	return true, nil
}

// previouslyReadDispositionTx loads the Book's disposition under the row lock.
// A Book without a row has never been given a disposition and is Inbox.
func previouslyReadDispositionTx(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) (domain.BookDisposition, error) {
	disposition, err := q.GetBookDispositionStateForUpdate(ctx, sqlcgen.GetBookDispositionStateForUpdateParams{OwnerID: owner, BookID: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BookDispositionInbox, nil
	}
	if err != nil {
		return "", err
	}
	return domain.BookDisposition(disposition.Disposition), nil
}
