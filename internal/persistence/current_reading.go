package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

func currentReadingCompletionFromValues(ownerID, language, bookID string, completedAt time.Time, snapshotID string, snapshotCount, eligibleCount, graduatedCount, alreadyKnownCount int) domain.CurrentReadingCompletion {
	return domain.CurrentReadingCompletion{
		OwnerID: ownerID, Language: language, BookID: bookID, CompletedAt: completedAt,
		SnapshotID: snapshotID, SnapshotVocabularyCount: snapshotCount,
		EligibleVocabularyCount: eligibleCount, GraduatedVocabularyCount: graduatedCount,
		AlreadyKnownVocabularyCount: alreadyKnownCount,
	}
}

// SwitchCurrentReading replaces the current reading only when the caller's
// expected commitment still names the language's current reading. A replay is
// accepted only when durable facts prove that this exact switch already ran.
func (s *PostgresStore) SwitchCurrentReading(ctx context.Context, owner, language, bookID, expectedBookID, expectedSnapshotID string) (result domain.CurrentReading, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if err := (domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID}).Validate(); err != nil {
		return domain.CurrentReading{}, err
	}
	if !exactCommitment(expectedBookID, expectedSnapshotID) {
		return domain.CurrentReading{}, ErrCurrentReadingStale
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return domain.CurrentReading{}, err
	}
	if err = lockCurrentReadingBook(ctx, q, owner, bookID); err != nil {
		return domain.CurrentReading{}, err
	}
	current, err := q.GetCurrentReadingForUpdate(ctx, sqlcgen.GetCurrentReadingForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, ErrNotFound
	}
	if err != nil {
		return domain.CurrentReading{}, err
	}
	if current.GBookID != expectedBookID || current.SnapshotID != expectedSnapshotID {
		if current.GBookID == bookID {
			proven, proofErr := switchReplayProven(ctx, q, owner, language, expectedBookID, expectedSnapshotID, current.SnapshotID)
			if proofErr != nil {
				return domain.CurrentReading{}, proofErr
			}
			if proven {
				snapshotSize, sizeErr := snapshotSizeForGoal(ctx, q, current.SnapshotID, owner)
				if sizeErr != nil {
					return domain.CurrentReading{}, sizeErr
				}
				if err = tx.Commit(ctx); err != nil {
					return domain.CurrentReading{}, err
				}
				return currentReadingFromValues(current.GOwnerID, current.Language, current.GBookID, current.SnapshotID, current.SourceMaterialID, current.AnalysisRunID, current.ContentRevisionID, current.ContentSnapshotID, current.CorpusID, snapshotSize, current.CreatedAt, current.UpdatedAt), nil
			}
		}
		return domain.CurrentReading{}, ErrCurrentReadingStale
	}
	if err = ensureCurrentReadingCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.CurrentReading{}, err
	}
	if err = releaseCurrentReadingSnapshot(ctx, q, owner, current.SnapshotID); err != nil {
		return domain.CurrentReading{}, err
	}
	identity, err := q.GetCurrentReadingCandidateIdentity(ctx, sqlcgen.GetCurrentReadingCandidateIdentityParams{Owner: owner, Language: language, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, ErrCurrentReadingIneligible
	}
	if err != nil {
		return domain.CurrentReading{}, err
	}
	blocked, err := unresolvedLemmaReviewFlags(ctx, tx, owner, bookID, identity.CaAnalysisRunID)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	if blocked {
		return domain.CurrentReading{}, ErrUnresolvedLemmaReviewFlags
	}
	snapshot, candidates, err := createCurrentReadingSnapshot(ctx, tx, q, owner, language, bookID, identity)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	row, err := q.ChangeCurrentReadingBook(ctx, sqlcgen.ChangeCurrentReadingBookParams{Owner: owner, Language: language, Book: bookID, Snapshot: uuidArg(snapshot.ID)})
	if err != nil {
		return domain.CurrentReading{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.CurrentReading{}, err
	}
	result = currentReadingFromValues(row.OwnerID, row.Language, row.BookID, row.SnapshotID, identity.CaSourceMaterialID, identity.CaAnalysisRunID, identity.CaContentRevisionID, identity.CaSnapshotID, identity.CaCorpusID, len(candidates), row.CreatedAt, row.UpdatedAt)
	return result, nil
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
	current, err := q.GetCurrentReadingForUpdate(ctx, sqlcgen.GetCurrentReadingForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		lifecycle, lifecycleErr := q.GetCurrentReadingSnapshotLifecycle(ctx, sqlcgen.GetCurrentReadingSnapshotLifecycleParams{Owner: owner, Language: language, Snapshot: expectedSnapshotID})
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
	if err = q.DeleteCurrentReading(ctx, sqlcgen.DeleteCurrentReadingParams{Owner: owner, Language: language}); err != nil {
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

// FinishCurrentReading accepts the frozen snapshot into modeled Known vocabulary,
// records the completion fact, returns the Book to Inbox, and ends the current
// reading. The snapshot is the request identity, so retries replay the same
// outcome even after the Book is completed again in the future.
func (s *PostgresStore) FinishCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) (result domain.CurrentReadingFinishResult, err error) {
	language = canonicalization.NormalizeLanguage(language)
	// A legacy reading that never received a snapshot has no snapshot identity to
	// name; the empty expectation still has to match the current row exactly.
	if _, parseErr := uuid.Parse(expectedBookID); parseErr != nil {
		return domain.CurrentReadingFinishResult{}, ErrCurrentReadingStale
	}
	if _, parseErr := uuid.Parse(expectedSnapshotID); parseErr != nil && expectedSnapshotID != "" {
		return domain.CurrentReadingFinishResult{}, ErrCurrentReadingStale
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	current, err := q.GetCurrentReadingForUpdate(ctx, sqlcgen.GetCurrentReadingForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		row, completionErr := getReadingCompletion(ctx, q, owner, language, expectedBookID, expectedSnapshotID)
		if errors.Is(completionErr, pgx.ErrNoRows) {
			return domain.CurrentReadingFinishResult{}, ErrNotFound
		}
		if completionErr != nil {
			return domain.CurrentReadingFinishResult{}, completionErr
		}
		if row.BookID != expectedBookID {
			return domain.CurrentReadingFinishResult{}, ErrCurrentReadingStale
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.CurrentReadingFinishResult{}, err
		}
		return domain.CurrentReadingFinishResult{Completion: currentReadingCompletionFromValues(row.OwnerID, row.Language, row.BookID, row.CompletedAt, row.GoalSnapshotID, row.SnapshotVocabularyCount, row.EligibleVocabularyCount, row.GraduatedVocabularyCount, row.AlreadyKnownVocabularyCount)}, nil
	}
	if err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	if current.GBookID != expectedBookID || current.SnapshotID != expectedSnapshotID {
		return domain.CurrentReadingFinishResult{}, ErrCurrentReadingStale
	}
	counts := sqlcgen.CountCurrentReadingSnapshotVocabularyRow{}
	if current.SnapshotID != "" {
		counts, err = q.CountCurrentReadingSnapshotVocabulary(ctx, sqlcgen.CountCurrentReadingSnapshotVocabularyParams{Owner: owner, Snapshot: current.SnapshotID})
		if err != nil {
			return domain.CurrentReadingFinishResult{}, err
		}
	}
	completedAt := time.Now().UTC()
	completionRow, insertErr := q.InsertReadingCompletion(ctx, sqlcgen.InsertReadingCompletionParams{
		Owner: owner, Language: language, Book: expectedBookID, CompletedAt: completedAt,
		Snapshot: expectedSnapshotID, SnapshotVocabularyCount: counts.SnapshotCount,
		EligibleVocabularyCount: counts.EligibleCount, GraduatedVocabularyCount: 0,
		AlreadyKnownVocabularyCount: counts.SnapshotCount - counts.EligibleCount,
	})
	inserted := insertErr == nil
	if errors.Is(insertErr, pgx.ErrNoRows) {
		existing, getErr := getReadingCompletion(ctx, q, owner, language, expectedBookID, expectedSnapshotID)
		if getErr != nil {
			return domain.CurrentReadingFinishResult{}, getErr
		}
		completionRow = sqlcgen.InsertReadingCompletionRow{
			OwnerID: existing.OwnerID, Language: existing.Language, BookID: existing.BookID, CompletedAt: existing.CompletedAt,
			GoalSnapshotID: existing.GoalSnapshotID, SnapshotVocabularyCount: existing.SnapshotVocabularyCount,
			EligibleVocabularyCount: existing.EligibleVocabularyCount, GraduatedVocabularyCount: existing.GraduatedVocabularyCount,
			AlreadyKnownVocabularyCount: existing.AlreadyKnownVocabularyCount,
		}
	}
	if insertErr != nil {
		return domain.CurrentReadingFinishResult{}, insertErr
	}
	graduatedCount := completionRow.GraduatedVocabularyCount
	alreadyKnownCount := completionRow.AlreadyKnownVocabularyCount
	if inserted && current.SnapshotID != "" {
		graduatedCount, err = q.GraduateCurrentReadingSnapshotVocabulary(ctx, sqlcgen.GraduateCurrentReadingSnapshotVocabularyParams{
			Owner: owner, Snapshot: current.SnapshotID, CompletedAt: completedAt,
		})
		if err != nil {
			return domain.CurrentReadingFinishResult{}, err
		}
		if err = q.UpdateReadingCompletionOutcome(ctx, sqlcgen.UpdateReadingCompletionOutcomeParams{
			Owner: owner, Language: language, Snapshot: expectedSnapshotID,
			GraduatedVocabularyCount: graduatedCount, AlreadyKnownVocabularyCount: counts.SnapshotCount - graduatedCount,
		}); err != nil {
			return domain.CurrentReadingFinishResult{}, err
		}
		alreadyKnownCount = counts.SnapshotCount - graduatedCount
	}
	if err = upsertBookDisposition(ctx, q, owner, expectedBookID, domain.BookDispositionInbox); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	if err = releaseCurrentReadingSnapshot(ctx, q, owner, current.SnapshotID); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	if err = q.DeleteCurrentReading(ctx, sqlcgen.DeleteCurrentReadingParams{Owner: owner, Language: language}); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	return domain.CurrentReadingFinishResult{Completion: currentReadingCompletionFromValues(completionRow.OwnerID, completionRow.Language, completionRow.BookID, completionRow.CompletedAt, completionRow.GoalSnapshotID, completionRow.SnapshotVocabularyCount, completionRow.EligibleVocabularyCount, graduatedCount, alreadyKnownCount)}, nil
}
