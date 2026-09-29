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

// CurrentReadingFinishResult is the current-reading name for the existing
// reading completion outcome.
type CurrentReadingFinishResult = domain.CurrentReadingFinishResult

func currentReadingFinishResult(result ReadingFinishResult) CurrentReadingFinishResult {
	completion := result.Completion
	return CurrentReadingFinishResult{Completion: domain.CurrentReadingCompletion{
		OwnerID: completion.OwnerID, Language: completion.Language, BookID: completion.BookID,
		CompletedAt: completion.CompletedAt, SnapshotID: completion.GoalSnapshotID,
		SnapshotVocabularyCount:     completion.SnapshotVocabularyCount,
		EligibleVocabularyCount:     completion.EligibleVocabularyCount,
		GraduatedVocabularyCount:    completion.GraduatedVocabularyCount,
		AlreadyKnownVocabularyCount: completion.AlreadyKnownVocabularyCount,
	}}
}

// GetCurrentReading reads one owner's current reading for a study language.
// The implementation deliberately delegates to the existing Goal repository
// until the learner-facing handlers are cut over.
func (s *PostgresStore) GetCurrentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error) {
	return s.GetPrimaryGoal(ctx, owner, language)
}

// StartCurrentReading freezes the same snapshot and reservation currently
// created by starting a Primary Goal.
func (s *PostgresStore) StartCurrentReading(ctx context.Context, owner, language, bookID string) (domain.CurrentReading, error) {
	return s.CreatePrimaryGoal(ctx, owner, language, bookID)
}

// SwitchCurrentReading replaces the current reading only when the caller's
// expected Book still owns the language slot.
func (s *PostgresStore) SwitchCurrentReading(ctx context.Context, owner, language, bookID, expectedBookID, expectedSnapshotID string) (domain.CurrentReading, error) {
	return s.changePrimaryGoal(ctx, owner, language, bookID, expectedBookID, expectedSnapshotID, true)
}

// StopCurrentReading releases the current reading while preserving its
// immutable snapshot and operational provenance.
func (s *PostgresStore) StopCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) error {
	return s.transitionCurrentReading(ctx, owner, language, expectedBookID, expectedSnapshotID, domain.BookDispositionToRead)
}

// SetAsideCurrentReading ends the active reading and moves its Book to Set Aside
// in the same transaction. Replaying the same request after it has committed is
// a no-op.
func (s *PostgresStore) SetAsideCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) error {
	return s.transitionCurrentReading(ctx, owner, language, expectedBookID, expectedSnapshotID, domain.BookDispositionSetAside)
}

func (s *PostgresStore) transitionCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string, disposition domain.BookDisposition) (err error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return err
	}
	if err = lockPrimaryGoalBook(ctx, q, owner, expectedBookID); err != nil {
		return err
	}
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		actual, dispositionErr := q.GetBookDisposition(ctx, sqlcgen.GetBookDispositionParams{OwnerID: owner, BookID: expectedBookID})
		if dispositionErr != nil {
			return dispositionErr
		}
		if domain.BookDisposition(actual) == disposition {
			return tx.Commit(ctx)
		}
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current.GBookID != expectedBookID {
		return ErrGoalStale
	}
	if expectedSnapshotID != "" && current.SnapshotID != expectedSnapshotID {
		return ErrGoalStale
	}
	if err = upsertBookDisposition(ctx, q, owner, expectedBookID, disposition); err != nil {
		return err
	}
	if err = releasePrimaryGoalSnapshot(ctx, q, owner, current.SnapshotID); err != nil {
		return err
	}
	if err = q.DeletePrimaryGoal(ctx, sqlcgen.DeletePrimaryGoalParams{Owner: owner, Language: language}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FinishCurrentReading accepts the frozen snapshot and records the same
// idempotent completion outcome as the existing Goal lifecycle.
func (s *PostgresStore) FinishCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) (CurrentReadingFinishResult, error) {
	result, err := s.RecordReadingFinishedPrimaryGoal(ctx, owner, language, expectedBookID, expectedSnapshotID)
	if err != nil {
		return CurrentReadingFinishResult{}, err
	}
	return currentReadingFinishResult(result), nil
}

// CountCurrentReadingVocabularyToAccept reports the frozen identities that
// would currently become Known vocabulary on completion.
func (s *PostgresStore) CountCurrentReadingVocabularyToAccept(ctx context.Context, owner, language string) (int, error) {
	return s.CountPrimaryGoalVocabularyToGraduate(ctx, owner, language)
}
