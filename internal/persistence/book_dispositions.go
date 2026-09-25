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

// SetBookAsideAtJourneyRevision sets a non-current Book aside in the same
// transaction that fences Journey membership, revision, and Goal state.
func (s *PostgresStore) SetBookAsideAtJourneyRevision(ctx context.Context, owner, language, bookID string, expectedRevision int64) (err error) {
	language = canonicalization.NormalizeLanguage(language)
	// Creating the language header also serializes this write with the first
	// concurrent Journey add when the language has no Journey yet.
	createJourney := language != ""
	tx, revision, members, _, cleaned, err := s.beginReadingJourneyMutation(ctx, owner, language, createJourney)
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
	goalBookID, err := readingJourneyGoalBookID(ctx, tx, owner, language)
	if err != nil {
		return err
	}
	memberIndex := -1
	for i, member := range members {
		if member.bookID == bookID {
			memberIndex = i
			break
		}
	}
	if expectedRevision != revision {
		if !cleaned && goalBookID != bookID && memberIndex < 0 {
			currentDisposition, dispositionErr := q.GetBookDisposition(ctx, sqlcgen.GetBookDispositionParams{OwnerID: owner, BookID: bookID})
			if dispositionErr != nil {
				return dispositionErr
			}
			if domain.BookDisposition(currentDisposition) == domain.BookDispositionSetAside {
				return tx.Commit(ctx)
			}
		}
		return ErrJourneyStale
	}
	if goalBookID == bookID {
		return ErrBookIsPrimaryGoal
	}
	if memberIndex >= 0 {
		if err = q.DeleteReadingJourneyMember(ctx, sqlcgen.DeleteReadingJourneyMemberParams{Owner: owner, Language: language, Book: bookID}); err != nil {
			return err
		}
		members = append(members[:memberIndex], members[memberIndex+1:]...)
		if err = synchronizeBookDisposition(ctx, q, owner, bookID, domain.BookDispositionSetAside); err != nil {
			return err
		}
		if err = rewriteReadingJourneyPositions(ctx, q, owner, language, members); err != nil {
			return err
		}
		derived, queryErr := q.DerivedJourneyBooksExist(ctx, sqlcgen.DerivedJourneyBooksExistParams{Owner: owner, Language: language})
		if queryErr != nil {
			return queryErr
		}
		if len(members) == 0 && !derived {
			if err = q.DeleteReadingJourney(ctx, sqlcgen.DeleteReadingJourneyParams{Owner: owner, Language: language}); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		if _, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if cleaned {
		if err = rewriteReadingJourneyPositions(ctx, q, owner, language, members); err != nil {
			return err
		}
		if len(members) == 0 {
			derived, queryErr := q.DerivedJourneyBooksExist(ctx, sqlcgen.DerivedJourneyBooksExistParams{Owner: owner, Language: language})
			if queryErr != nil {
				return queryErr
			}
			if !derived {
				if err = q.DeleteReadingJourney(ctx, sqlcgen.DeleteReadingJourneyParams{Owner: owner, Language: language}); err != nil {
					return err
				}
				if err = synchronizeBookDisposition(ctx, q, owner, bookID, domain.BookDispositionSetAside); err != nil {
					return err
				}
				return tx.Commit(ctx)
			}
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
