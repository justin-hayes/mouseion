package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// EndCurrentReading clears the current role in language. The Book keeps its
// disposition (To Read), visibility, history, and artifacts. It names no
// commitment: whatever reading is current ends, and ErrNotFound reports that
// none was (ADR 0136 §4).
func (s *PostgresStore) EndCurrentReading(ctx context.Context, owner, language string) (err error) {
	language = canonicalization.NormalizeLanguage(language)
	if err := validateCurrentReadingScope(owner, language); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err := lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return err
	}
	if _, err := lockCurrentReading(ctx, q, owner, language); err != nil {
		return err
	}
	if err := clearCurrentReading(ctx, q, owner, language); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FinishCurrentReading appends a Reading completion for the current reading,
// returns its Book to the Inbox, and clears the current role. It has no
// vocabulary effect and names no commitment; ErrNotFound reports that no reading
// was current (ADR 0136 §4).
func (s *PostgresStore) FinishCurrentReading(ctx context.Context, owner, language string) (result domain.CurrentReadingFinishResult, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if err := validateCurrentReadingScope(owner, language); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err := lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	current, err := lockCurrentReading(ctx, q, owner, language)
	if err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	completedAt := time.Now().UTC()
	if err := q.InsertReadingCompletion(ctx, sqlcgen.InsertReadingCompletionParams{
		Owner: owner, Language: language, Book: current.GBookID, CompletedAt: completedAt,
	}); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	if err := upsertBookDisposition(ctx, q, owner, current.GBookID, domain.BookDispositionInbox); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	if err := clearCurrentReading(ctx, q, owner, language); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	return domain.CurrentReadingFinishResult{Completion: domain.CurrentReadingCompletion{
		OwnerID: owner, Language: language, BookID: current.GBookID, CompletedAt: completedAt,
	}}, nil
}

// validateCurrentReadingScope rejects an owner or language Postgres cannot use,
// treating an owner it cannot read as a uuid as the not-found a well-formed
// unknown owner gets.
func validateCurrentReadingScope(owner, language string) error {
	if err := (domain.CurrentReading{OwnerID: owner, Language: language, BookID: "-"}).Validate(); err != nil {
		return err
	}
	if !validUUID(owner) {
		return ErrNotFound
	}
	return nil
}

// lockCurrentReading locks the owner's current reading in language and reports
// ErrNotFound when there is none.
func lockCurrentReading(ctx context.Context, q *sqlcgen.Queries, owner, language string) (sqlcgen.GetCurrentReadingForUpdateRow, error) {
	current, err := q.GetCurrentReadingForUpdate(ctx, sqlcgen.GetCurrentReadingForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return current, ErrNotFound
	}
	return current, err
}

// clearCurrentReading deletes the current role.
func clearCurrentReading(ctx context.Context, q *sqlcgen.Queries, owner, language string) error {
	return q.DeleteCurrentReading(ctx, sqlcgen.DeleteCurrentReadingParams{Owner: owner, Language: language})
}
