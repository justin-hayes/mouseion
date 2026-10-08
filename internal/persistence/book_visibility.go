package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// BookVisibilityStore exposes the durable owner-and-Book Hidden choice. It is
// independent of disposition, current reading, history, and evidence.
type BookVisibilityStore interface {
	GetBookVisibility(context.Context, string, string) (hidden bool, revision int64, err error)
	SetBookHidden(context.Context, string, string, int64, bool) (bool, error)
}

var ErrStaleBookVisibility = errors.New("book visibility changed since this page was loaded")

func (s *PostgresStore) GetBookVisibility(ctx context.Context, owner, bookID string) (bool, int64, error) {
	state, err := s.queries().GetBookVisibility(ctx, sqlcgen.GetBookVisibilityParams{Owner: owner, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, ErrNotFound
	}
	if err != nil {
		return false, 0, err
	}
	return state.Hidden, state.Revision, nil
}

// SetBookHidden requests a desired visibility against the revision rendered
// with the form. A Book without a stored choice is visible at revision 0. The
// request applies only to that exact revision; a single-step replay of the
// accepted request is a verified no-op, and any intervening choice (including
// a change away and back) is stale, so an old Hide cannot reverse a newer
// Unhide. Only the visibility row is written: disposition, current role,
// reservations, history, evidence, and queued work are untouched.
func (s *PostgresStore) SetBookHidden(ctx context.Context, owner, bookID string, expectedRevision int64, hidden bool) (applied bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	state, err := q.GetBookVisibilityForUpdate(ctx, sqlcgen.GetBookVisibilityForUpdateParams{Owner: owner, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, err
	}
	if state.Revision == expectedRevision+1 && state.Hidden == hidden {
		return false, tx.Commit(ctx)
	}
	if state.Revision != expectedRevision {
		return false, ErrStaleBookVisibility
	}
	if state.Hidden == hidden {
		return false, tx.Commit(ctx)
	}
	if err = q.WriteBookVisibility(ctx, sqlcgen.WriteBookVisibilityParams{Owner: owner, Book: bookID, Hidden: hidden, Revision: expectedRevision + 1}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
