package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func scanPrimaryGoal(row pgx.Row) (goal domain.PrimaryGoal, err error) {
	err = row.Scan(&goal.OwnerID, &goal.Language, &goal.BookID, &goal.CreatedAt, &goal.UpdatedAt, &goal.ReadingFinishedAt)
	return goal, missing(err)
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
	goal, err := scanPrimaryGoal(s.pool.QueryRow(ctx, `SELECT owner_id::text,language,book_id::text,created_at,updated_at,reading_finished_at FROM primary_goals WHERE owner_id=$1 AND language=$2`, owner, language))
	if errors.Is(err, ErrNotFound) {
		return domain.PrimaryGoal{}, nil
	}
	return goal, err
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
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	var currentBookID string
	var readingFinishedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT book_id::text,reading_finished_at FROM primary_goals WHERE owner_id=$1 AND language=$2 FOR UPDATE`, owner, language).Scan(&currentBookID, &readingFinishedAt)
	if err == nil && readingFinishedAt == nil {
		return domain.PrimaryGoal{}, ErrGoalExists
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, err
	}
	if err = ensurePrimaryGoalCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	goal, err = insertPrimaryGoal(ctx, tx, owner, language, bookID)
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
		Owner: uuidArg(owner), Language: language, Book: uuidArg(bookID),
	})
	if err != nil {
		return err
	}
	if !eligible {
		return ErrGoalIneligible
	}
	return nil
}

func insertPrimaryGoal(ctx context.Context, tx pgx.Tx, owner, language, bookID string) (domain.PrimaryGoal, error) {
	goal, err := scanPrimaryGoal(tx.QueryRow(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3) ON CONFLICT (owner_id,language) DO NOTHING RETURNING owner_id::text,language,book_id::text,created_at,updated_at,reading_finished_at`, owner, language, bookID))
	if errors.Is(err, ErrNotFound) {
		goal, err = scanPrimaryGoal(tx.QueryRow(ctx, `UPDATE primary_goals SET book_id=$3,reading_finished_at=NULL,updated_at=now() WHERE owner_id=$1 AND language=$2 AND reading_finished_at IS NOT NULL RETURNING owner_id::text,language,book_id::text,created_at,updated_at,reading_finished_at`, owner, language, bookID))
		if errors.Is(err, ErrNotFound) {
			return domain.PrimaryGoal{}, ErrGoalExists
		}
	}
	return goal, err
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
	var currentBookID string
	var readingFinishedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT book_id::text,reading_finished_at FROM primary_goals WHERE owner_id=$1 AND language=$2 FOR UPDATE`, owner, language).Scan(&currentBookID, &readingFinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrNotFound
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if currentBookID != expectedBookID {
		return domain.PrimaryGoal{}, ErrGoalStale
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = ensurePrimaryGoalCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	goal, err := scanPrimaryGoal(tx.QueryRow(ctx, `UPDATE primary_goals SET book_id=$3,reading_finished_at=NULL,updated_at=now() WHERE owner_id=$1 AND language=$2 RETURNING owner_id::text,language,book_id::text,created_at,updated_at,reading_finished_at`, owner, language, bookID))
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	return goal, nil
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

	goal, err := scanPrimaryGoal(tx.QueryRow(ctx, `SELECT owner_id::text,language,book_id::text,created_at,updated_at,reading_finished_at FROM primary_goals WHERE owner_id=$1 AND language=$2 FOR UPDATE`, owner, language))
	if err != nil {
		return ReadingFinishResult{}, err
	}
	if goal.BookID != expectedBookID {
		return ReadingFinishResult{}, ErrGoalStale
	}
	if goal.ReadingFinishedAt == nil {
		goal, err = scanPrimaryGoal(tx.QueryRow(ctx, `UPDATE primary_goals SET reading_finished_at=now(),updated_at=now() WHERE owner_id=$1 AND language=$2 RETURNING owner_id::text,language,book_id::text,created_at,updated_at,reading_finished_at`, owner, language))
		if err != nil {
			return ReadingFinishResult{}, err
		}
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
	var currentBookID string
	var readingFinishedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT book_id::text,reading_finished_at FROM primary_goals WHERE owner_id=$1 AND language=$2 FOR UPDATE`, owner, language).Scan(&currentBookID, &readingFinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if currentBookID != expectedBookID {
		return ErrGoalStale
	}
	if readingFinishedAt != nil {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, `DELETE FROM primary_goals WHERE owner_id=$1 AND language=$2`, owner, language); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
