package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func primaryGoalFromValues(ownerID, language, bookID string, createdAt, updatedAt time.Time, readingFinishedAt pgtype.Timestamptz) domain.PrimaryGoal {
	return domain.PrimaryGoal{
		OwnerID: ownerID, Language: language, BookID: bookID,
		CreatedAt: createdAt, UpdatedAt: updatedAt, ReadingFinishedAt: pgTimePtr(readingFinishedAt),
	}
}

// ReadingFinishResult is the webapp-facing result of recording reading
type ReadingFinishResult struct {
	Goal domain.PrimaryGoal
}

// GetPrimaryGoal returns the owner's current Goal in one study language. No
// row is the ordinary absent Goal state.
func (s *PostgresStore) GetPrimaryGoal(ctx context.Context, owner, language string) (domain.PrimaryGoal, error) {
	language = canonicalization.NormalizeLanguage(language)
	if language == "" {
		return domain.PrimaryGoal{}, nil
	}
	row, err := s.queries().GetPrimaryGoal(ctx, sqlcgen.GetPrimaryGoalParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, nil
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	return primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt, row.ReadingFinishedAt), nil
}

// CreatePrimaryGoal creates the owner's current Goal for an analyzed Reading
// Journey member in language.
func (s *PostgresStore) CreatePrimaryGoal(ctx context.Context, owner, language, bookID string) (domain.PrimaryGoal, error) {
	language = canonicalization.NormalizeLanguage(language)
	goal := domain.PrimaryGoal{OwnerID: owner, Language: language, BookID: bookID}
	if err := goal.Validate(); err != nil {
		return domain.PrimaryGoal{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	defer tx.Rollback(ctx)
	q := sqlcgen.New(tx)
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if err == nil && !current.ReadingFinishedAt.Valid {
		return domain.PrimaryGoal{}, ErrGoalExists
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, err
	}
	if err = ensurePrimaryGoalCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	goal, err = insertPrimaryGoal(ctx, q, owner, language, bookID)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	return goal, nil
}

func ensurePrimaryGoalCandidate(ctx context.Context, tx pgx.Tx, owner, language, bookID string) error {
	eligible, err := sqlcgen.New(tx).PrimaryGoalCandidateEligible(ctx, sqlcgen.PrimaryGoalCandidateEligibleParams{
		Owner: owner, Language: language, Book: bookID,
	})
	if err != nil {
		return err
	}
	if !eligible {
		return ErrGoalIneligible
	}
	return nil
}

func insertPrimaryGoal(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID string) (domain.PrimaryGoal, error) {
	row, err := q.InsertPrimaryGoal(ctx, sqlcgen.InsertPrimaryGoalParams{Owner: owner, Language: language, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		reactivated, err := q.ReactivatePrimaryGoal(ctx, sqlcgen.ReactivatePrimaryGoalParams{Owner: owner, Language: language, Book: bookID})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PrimaryGoal{}, ErrGoalExists
		}
		if err != nil {
			return domain.PrimaryGoal{}, err
		}
		return primaryGoalFromValues(reactivated.OwnerID, reactivated.Language, reactivated.BookID, reactivated.CreatedAt, reactivated.UpdatedAt, reactivated.ReadingFinishedAt), nil
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	return primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt, row.ReadingFinishedAt), nil
}

// ChangePrimaryGoal changes the language's Goal only when expectedBookID still
// names the current Goal, protecting callers from overwriting stale state.
func (s *PostgresStore) ChangePrimaryGoal(ctx context.Context, owner, language, bookID, expectedBookID string) (domain.PrimaryGoal, error) {
	language = canonicalization.NormalizeLanguage(language)
	if err := (domain.PrimaryGoal{OwnerID: owner, Language: language, BookID: bookID}).Validate(); err != nil {
		return domain.PrimaryGoal{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	defer tx.Rollback(ctx)
	q := sqlcgen.New(tx)
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrNotFound
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if current.BookID != expectedBookID {
		return domain.PrimaryGoal{}, ErrGoalStale
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = ensurePrimaryGoalCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	row, err := q.ChangePrimaryGoalBook(ctx, sqlcgen.ChangePrimaryGoalBookParams{Owner: owner, Language: language, Book: bookID})
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	return primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt, row.ReadingFinishedAt), nil
}

// RecordReadingFinishedPrimaryGoal records only the reading-finished fact for
// the current Goal. Vocabulary graduation is not part of this reading
// transition.
func (s *PostgresStore) RecordReadingFinishedPrimaryGoal(ctx context.Context, owner, language, expectedBookID string) (ReadingFinishResult, error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReadingFinishResult{}, err
	}
	defer tx.Rollback(ctx)
	q := sqlcgen.New(tx)

	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReadingFinishResult{}, ErrNotFound
	}
	if err != nil {
		return ReadingFinishResult{}, err
	}
	goal := primaryGoalFromValues(current.OwnerID, current.Language, current.BookID, current.CreatedAt, current.UpdatedAt, current.ReadingFinishedAt)
	if goal.BookID != expectedBookID {
		return ReadingFinishResult{}, ErrGoalStale
	}
	if goal.ReadingFinishedAt == nil {
		row, finishErr := q.FinishPrimaryGoalReading(ctx, sqlcgen.FinishPrimaryGoalReadingParams{Owner: owner, Language: language})
		if finishErr != nil {
			return ReadingFinishResult{}, finishErr
		}
		goal = primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt, row.ReadingFinishedAt)
	}
	if err = tx.Commit(ctx); err != nil {
		return ReadingFinishResult{}, err
	}
	return ReadingFinishResult{Goal: goal}, nil
}

// ClearPrimaryGoal removes the language's Goal only when expectedBookID still
// names the current Goal.
func (s *PostgresStore) ClearPrimaryGoal(ctx context.Context, owner, language, expectedBookID string) error {
	language = canonicalization.NormalizeLanguage(language)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := sqlcgen.New(tx)
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current.BookID != expectedBookID {
		return ErrGoalStale
	}
	if current.ReadingFinishedAt.Valid {
		return ErrNotFound
	}
	if err = q.DeletePrimaryGoal(ctx, sqlcgen.DeletePrimaryGoalParams{Owner: owner, Language: language}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
