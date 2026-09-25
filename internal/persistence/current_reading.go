package persistence

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
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
func (s *PostgresStore) SwitchCurrentReading(ctx context.Context, owner, language, bookID, expectedBookID string) (domain.CurrentReading, error) {
	return s.ChangePrimaryGoal(ctx, owner, language, bookID, expectedBookID)
}

// StopCurrentReading releases the current reading while preserving its
// immutable snapshot and operational provenance.
func (s *PostgresStore) StopCurrentReading(ctx context.Context, owner, language, expectedBookID string) error {
	return s.ClearPrimaryGoal(ctx, owner, language, expectedBookID)
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
