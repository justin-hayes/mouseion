package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// BookDispositionStore exposes the durable owner-and-Book workflow state.
type BookDispositionStore interface {
	GetBookDisposition(context.Context, string, string) (domain.BookDisposition, error)
	SetBookDisposition(context.Context, string, string, domain.BookDisposition) error
	TransitionBookDisposition(context.Context, string, string, int64, domain.BookDisposition) (bool, error)
}

var (
	ErrStaleBookDisposition   = domain.ErrStaleBookDisposition
	ErrMyBooksBackfillBlocked = domain.ErrMyBooksBackfillBlocked
)

func (s *PostgresStore) GetBookDisposition(ctx context.Context, owner, bookID string) (domain.BookDisposition, error) {
	disposition, err := s.queries().GetBookDisposition(ctx, sqlcgen.GetBookDispositionParams{OwnerID: owner, BookID: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return domain.BookDisposition(disposition), nil
}

func (s *PostgresStore) SetBookDisposition(ctx context.Context, owner, bookID string, disposition domain.BookDisposition) (err error) {
	if err := disposition.Validate(); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err := ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	q := sqlcgen.New(tx)
	if err := refuseUnchosenDisposition(ctx, q, owner, bookID, disposition); err != nil {
		return err
	}
	if err := upsertBookDisposition(ctx, q, owner, bookID, disposition); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// refuseUnchosenDisposition rejects the stored not-To-Read value for a Book
// that is neither the current reading nor has a recorded completion. That state
// was the retired Inbox; My Books no longer has a bucket for it.
func refuseUnchosenDisposition(ctx context.Context, q *sqlcgen.Queries, owner, bookID string, disposition domain.BookDisposition) error {
	if disposition != domain.BookDispositionInbox {
		return nil
	}
	completion, err := q.GetMyBookCompletionSummary(ctx, sqlcgen.GetMyBookCompletionSummaryParams{Owner: owner, Book: bookID})
	if err != nil {
		return err
	}
	if completion.CompletionCount > 0 {
		return nil
	}
	current, err := q.BookHasActiveCurrentReading(ctx, sqlcgen.BookHasActiveCurrentReadingParams{Owner: owner, Book: bookID})
	if err != nil {
		return err
	}
	if current {
		return nil
	}
	return domain.ErrBookNotChosen
}

// TransitionBookDisposition applies a My Books decision against the revision
// rendered with its form. A single-step replay of the accepted action is safe;
// an intervening decision (including a change away and back) is stale.
func (s *PostgresStore) TransitionBookDisposition(ctx context.Context, owner, bookID string, expectedRevision int64, disposition domain.BookDisposition) (applied bool, err error) {
	if err := disposition.Validate(); err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if _, err = q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID}); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, err
	}
	state, err := q.GetBookDispositionStateForUpdate(ctx, sqlcgen.GetBookDispositionStateForUpdateParams{OwnerID: owner, BookID: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, err
	}
	if state.Revision == expectedRevision+1 && domain.BookDisposition(state.Disposition) == disposition {
		return false, tx.Commit(ctx)
	}
	if state.Revision != expectedRevision {
		return false, ErrStaleBookDisposition
	}
	if err := refuseUnchosenDisposition(ctx, q, owner, bookID, disposition); err != nil {
		return false, err
	}
	rows, err := q.TransitionBookDisposition(ctx, sqlcgen.TransitionBookDispositionParams{
		Owner: owner, Book: bookID, Disposition: string(disposition), ExpectedRevision: expectedRevision,
	})
	if err != nil {
		return false, err
	}
	if rows == 0 {
		return false, ErrStaleBookDisposition
	}
	return true, tx.Commit(ctx)
}

// upsertBookDisposition is the one persistence path for a Book's learner-owned
// workflow disposition; no legacy membership state is mirrored.
func upsertBookDisposition(ctx context.Context, q interface {
	UpsertBookDisposition(context.Context, sqlcgen.UpsertBookDispositionParams) error
}, owner, bookID string, disposition domain.BookDisposition) error {
	return q.UpsertBookDisposition(ctx, sqlcgen.UpsertBookDispositionParams{OwnerID: owner, BookID: bookID, Disposition: string(disposition)})
}
