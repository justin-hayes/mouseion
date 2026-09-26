package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// BookDispositionStore exposes the durable owner-and-Book workflow state.
type BookDispositionStore interface {
	GetBookDisposition(context.Context, string, string) (domain.BookDisposition, error)
	SetBookDisposition(context.Context, string, string, domain.BookDisposition) error
	SetBookAside(context.Context, string, string, string) error
}

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
	if err = disposition.Validate(); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	if err = upsertBookDisposition(ctx, sqlcgen.New(tx), owner, bookID, disposition); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetBookAside changes only the Book's disposition and refuses to change the
// current reading. The Book lock serializes this transition with starting or
// switching current reading.
func (s *PostgresStore) SetBookAside(ctx context.Context, owner, language, bookID string) (err error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if _, err = q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID}); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	goal, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		goal.GBookID = ""
	} else if err != nil {
		return err
	}
	if goal.GBookID == bookID {
		return ErrBookIsPrimaryGoal
	}
	if err = upsertBookDisposition(ctx, q, owner, bookID, domain.BookDispositionSetAside); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// upsertBookDisposition is the one persistence path for a Book's learner-owned
// workflow disposition; no legacy membership state is mirrored.
func upsertBookDisposition(ctx context.Context, q interface {
	UpsertBookDisposition(context.Context, sqlcgen.UpsertBookDispositionParams) error
}, owner, bookID string, disposition domain.BookDisposition) error {
	return q.UpsertBookDisposition(ctx, sqlcgen.UpsertBookDispositionParams{OwnerID: owner, BookID: bookID, Disposition: string(disposition)})
}
