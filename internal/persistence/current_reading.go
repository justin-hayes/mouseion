package persistence

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

func currentReadingFinishResult(result ReadingFinishResult) domain.CurrentReadingFinishResult {
	completion := result.Completion
	return domain.CurrentReadingFinishResult{Completion: domain.CurrentReadingCompletion{
		OwnerID: completion.OwnerID, Language: completion.Language, BookID: completion.BookID,
		CompletedAt: completion.CompletedAt, SnapshotID: completion.GoalSnapshotID,
		SnapshotVocabularyCount:     completion.SnapshotVocabularyCount,
		EligibleVocabularyCount:     completion.EligibleVocabularyCount,
		GraduatedVocabularyCount:    completion.GraduatedVocabularyCount,
		AlreadyKnownVocabularyCount: completion.AlreadyKnownVocabularyCount,
	}}
}

// SwitchCurrentReading replaces the current reading only when the caller's
// expected Book still owns the language slot.
func (s *PostgresStore) SwitchCurrentReading(ctx context.Context, owner, language, bookID, expectedBookID, expectedSnapshotID string) (domain.CurrentReading, error) {
	return s.changeCurrentReading(ctx, owner, language, bookID, expectedBookID, expectedSnapshotID, true)
}

// EndCurrentReading releases exactly the expected commitment's Reserved
// vocabulary and clears the current role without recording completion or Known
// acceptance. The Book keeps its disposition (To Read), visibility, snapshot,
// history, and artifacts. A replay is accepted only when durable facts prove
// that the expected snapshot was released without completion and no reading is
// current; Book identity or disposition alone is never proof.
func (s *PostgresStore) EndCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) (err error) {
	language = canonicalization.NormalizeLanguage(language)
	if !exactCommitment(expectedBookID, expectedSnapshotID) {
		return ErrCurrentReadingStale
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return err
	}
	if err = lockCurrentReadingBook(ctx, q, owner, expectedBookID); err != nil {
		return err
	}
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		lifecycle, lifecycleErr := q.GetPrimaryGoalSnapshotLifecycle(ctx, sqlcgen.GetPrimaryGoalSnapshotLifecycleParams{Owner: owner, Language: language, Snapshot: expectedSnapshotID})
		if errors.Is(lifecycleErr, pgx.ErrNoRows) {
			return ErrCurrentReadingStale
		}
		if lifecycleErr != nil {
			return lifecycleErr
		}
		if lifecycle.BookID != expectedBookID || !lifecycle.ReleasedAt.Valid || lifecycle.Completed {
			return ErrCurrentReadingStale
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if current.GBookID != expectedBookID || current.SnapshotID != expectedSnapshotID {
		return ErrCurrentReadingStale
	}
	if err = releaseCurrentReadingSnapshot(ctx, q, owner, current.SnapshotID); err != nil {
		return err
	}
	if err = q.DeletePrimaryGoal(ctx, sqlcgen.DeletePrimaryGoalParams{Owner: owner, Language: language}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// exactCommitment reports whether both identities are present and well formed,
// so malformed or missing expectations are rejected as stale instead of
// reaching SQL.
func exactCommitment(bookID, snapshotID string) bool {
	if _, err := uuid.Parse(bookID); err != nil {
		return false
	}
	_, err := uuid.Parse(snapshotID)
	return err == nil
}

// FinishCurrentReading accepts the frozen snapshot and records the same
// idempotent completion outcome as the existing Goal lifecycle.
func (s *PostgresStore) FinishCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) (domain.CurrentReadingFinishResult, error) {
	result, err := s.RecordCurrentReadingFinished(ctx, owner, language, expectedBookID, expectedSnapshotID)
	if err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	return currentReadingFinishResult(result), nil
}
