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
// These methods are deliberately separate from the legacy Journey interfaces
// so the synchronization calls can be deleted during the cutover.
type BookDispositionStore interface {
	GetBookDisposition(context.Context, string, string) (domain.BookDisposition, error)
	SetBookDisposition(context.Context, string, string, domain.BookDisposition) error
	SetBookAsideAtJourneyRevision(context.Context, string, string, string, int64) error
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
	if err = synchronizeBookDisposition(ctx, sqlcgen.New(tx), owner, bookID, disposition); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetBookAsideAtJourneyRevision sets an out-of-Journey Book aside while
// fencing the write against concurrent Journey changes in that language.
func (s *PostgresStore) SetBookAsideAtJourneyRevision(ctx context.Context, owner, language, bookID string, expectedRevision int64) (err error) {
	language = canonicalization.NormalizeLanguage(language)
	// Create and lock the language's Journey header even when it is empty. This
	// serializes an Inbox disposition write with the first concurrent Journey add.
	createJourney := language != ""
	tx, revision, members, _, cleaned, err := s.beginReadingJourneyMutation(ctx, owner, language, createJourney)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if expectedRevision != revision {
		return ErrJourneyStale
	}
	for _, member := range members {
		if member.bookID == bookID {
			return ErrJourneyStale
		}
	}
	goalBookID, err := readingJourneyGoalBookID(ctx, tx, owner, language)
	if err != nil {
		return err
	}
	if goalBookID == bookID {
		return ErrJourneyStale
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	q := sqlcgen.New(tx)
	if cleaned {
		if err = rewriteReadingJourneyPositions(ctx, q, owner, language, members); err != nil {
			return err
		}
		if _, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
			return err
		}
	}
	if err = synchronizeBookDisposition(ctx, q, owner, bookID, domain.BookDispositionSetAside); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// synchronizeBookDisposition is the temporary compatibility fence for shipped
// Journey and My Books removal mutations. New disposition callers should use
// SetBookDisposition instead.
func synchronizeBookDisposition(ctx context.Context, q interface {
	UpsertBookDisposition(context.Context, sqlcgen.UpsertBookDispositionParams) error
}, owner, bookID string, disposition domain.BookDisposition) error {
	return q.UpsertBookDisposition(ctx, sqlcgen.UpsertBookDispositionParams{OwnerID: owner, BookID: bookID, Disposition: string(disposition)})
}
