package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListStudyProgress returns every word the learner has in study in the study
// language, oldest introduction first.
func (s *PostgresStore) ListStudyProgress(ctx context.Context, owner, language string) ([]domain.StudyProgress, error) {
	rows, err := sqlcgen.New(s.pool).ListStudyProgress(ctx, sqlcgen.ListStudyProgressParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, fmt.Errorf("list study progress: %w", err)
	}
	progress := make([]domain.StudyProgress, 0, len(rows))
	for _, row := range rows {
		progress = append(progress, studyProgressFromRow(row))
	}
	return progress, nil
}

// StudyBatchLedger reports the New-word batches taken and declined on a Study
// day. A day with none is the zero ledger.
func (s *PostgresStore) StudyBatchLedger(ctx context.Context, owner, language, studyDay string) (domain.StudyBatchLedger, error) {
	day, err := studyDayDate(studyDay)
	if err != nil {
		return domain.StudyBatchLedger{}, err
	}
	row, err := sqlcgen.New(s.pool).GetStudyBatchLedger(ctx, sqlcgen.GetStudyBatchLedgerParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language), StudyDay: day})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyBatchLedger{StudyDay: studyDay}, nil
	}
	if err != nil {
		return domain.StudyBatchLedger{}, fmt.Errorf("get study batch ledger: %w", err)
	}
	return domain.StudyBatchLedger{StudyDay: studyDay, Taken: row.BatchesTaken, Declined: row.BatchesDeclined}, nil
}

// TakeStudyBatch takes one New-word batch on the Study day: it records the
// batch in the day's ledger and introduces each word from its ready Study
// material, with the material's Gloss as the learned meaning, due now. It
// returns the words introduced. A word already in study, or whose material is
// no longer ready, is skipped; a batch that introduces nothing is taken by
// nobody and returns ErrNotFound. domain.ErrStudyBatchLimit means the day's
// batches are all answered.
func (s *PostgresStore) TakeStudyBatch(ctx context.Context, owner, language, studyDay string, now time.Time, batchLimit int, words []domain.StudyBatchWord) ([]domain.StudyProgress, error) {
	day, err := studyDayDate(studyDay)
	if err != nil {
		return nil, err
	}
	language = canonicalization.NormalizeLanguage(language)
	var introduced []domain.StudyProgress
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if _, err := q.TakeStudyBatchSlot(ctx, sqlcgen.TakeStudyBatchSlotParams{Owner: owner, Language: language, StudyDay: day, BatchLimit: batchLimit}); err != nil {
			return mapBatchSlotError(err)
		}
		var err error
		introduced, err = introduceStudyWords(ctx, q, owner, language, now, words)
		if err != nil {
			return err
		}
		if len(introduced) == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return introduced, nil
}

// mapBatchSlotError turns the ledger's refusal, an upsert that returns no row,
// into domain.ErrStudyBatchLimit.
func mapBatchSlotError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrStudyBatchLimit
	}
	return fmt.Errorf("study batch ledger: %w", err)
}

func introduceStudyWords(ctx context.Context, q *sqlcgen.Queries, owner, language string, now time.Time, words []domain.StudyBatchWord) ([]domain.StudyProgress, error) {
	var introduced []domain.StudyProgress
	for _, word := range words {
		if !validUUID(word.MaterialID) {
			continue
		}
		row, err := q.IntroduceStudyWord(ctx, sqlcgen.IntroduceStudyWordParams{Now: now.UTC(), Owner: owner, Material: word.MaterialID, Language: language})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("introduce study word: %w", err)
		}
		introduced = append(introduced, studyProgressFromRow(row))
	}
	return introduced, nil
}

// DeclineStudyBatch declines one New-word batch on the Study day. It
// introduces no word. domain.ErrStudyBatchLimit means the day's batches are
// all answered.
func (s *PostgresStore) DeclineStudyBatch(ctx context.Context, owner, language, studyDay string, batchLimit int) error {
	day, err := studyDayDate(studyDay)
	if err != nil {
		return err
	}
	if _, err = sqlcgen.New(s.pool).DeclineStudyBatchSlot(ctx, sqlcgen.DeclineStudyBatchSlotParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language), StudyDay: day, BatchLimit: batchLimit}); err != nil {
		return mapBatchSlotError(err)
	}
	return nil
}

// NextStudyCard returns the word to show next, with the Study material it is
// reviewed in. What is due is decided by the domain (Today's due set and the
// same-day repeats), so both adapters agree. ErrNotFound means nothing is due
// at now.
func (s *PostgresStore) NextStudyCard(ctx context.Context, owner, language string, now time.Time) (domain.StudyCard, error) {
	all, err := s.ListStudyProgress(ctx, owner, language)
	if err != nil {
		return domain.StudyCard{}, err
	}
	progress, ok := domain.NextStudyReview(all, now)
	if !ok {
		return domain.StudyCard{}, ErrNotFound
	}
	material, err := s.GetStudyMaterial(ctx, owner, progress.MaterialID)
	if err != nil {
		return domain.StudyCard{}, err
	}
	return domain.StudyCard{Progress: progress, Material: material}, nil
}

// RecordStudyGrade stores one graded review: the review-log row with the state
// before and after, and the word's new scheduling state. The word is locked
// first, so a card graded twice is graded once: the second grade returns
// domain.ErrStudyStale. ErrNotFound means the word is not the learner's.
func (s *PostgresStore) RecordStudyGrade(ctx context.Context, owner string, record domain.StudyGradeRecord) error {
	if !validUUID(record.ProgressID) {
		return ErrNotFound
	}
	day, err := studyDayDate(record.StudyDay)
	if err != nil {
		return err
	}
	if _, ok := domain.ParseStudyGrade(string(record.Grade)); !ok {
		return fmt.Errorf("record study grade: unknown grade %q", record.Grade)
	}
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		row, err := q.LockStudyProgress(ctx, sqlcgen.LockStudyProgressParams{Owner: owner, ID: record.ProgressID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock study progress: %w", err)
		}
		before := studyProgressFromRow(row)
		if before.Withdrawn || before.Memory.Reps != record.ExpectedReps {
			return domain.ErrStudyStale
		}
		if err := appendStudyReview(ctx, q, owner, before, record, day); err != nil {
			return err
		}
		after := record.After
		if err := q.UpdateStudyProgressMemory(ctx, sqlcgen.UpdateStudyProgressMemoryParams{
			State: string(after.State), Due: after.Due.UTC(), Stability: after.Stability, Difficulty: after.Difficulty,
			ScheduledDays: after.ScheduledDays, Reps: after.Reps, Lapses: after.Lapses, RemainingSteps: after.RemainingSteps,
			LastReview: pgtype.Timestamptz{Time: after.LastReview.UTC(), Valid: !after.LastReview.IsZero()},
			Owner:      owner, ID: record.ProgressID,
		}); err != nil {
			return fmt.Errorf("update study progress: %w", err)
		}
		return nil
	})
}

func appendStudyReview(ctx context.Context, q *sqlcgen.Queries, owner string, before domain.StudyProgress, record domain.StudyGradeRecord, day pgtype.Date) error {
	encodedBefore, err := json.Marshal(before.Memory)
	if err != nil {
		return fmt.Errorf("encode study state: %w", err)
	}
	encodedAfter, err := json.Marshal(record.After)
	if err != nil {
		return fmt.Errorf("encode study state: %w", err)
	}
	if err := q.AppendStudyReviewLog(ctx, sqlcgen.AppendStudyReviewLogParams{
		Owner: owner, Progress: record.ProgressID, Material: before.MaterialID, Grade: string(record.Grade),
		ReviewedAt: record.ReviewedAt.UTC(), StudyDay: day, StateBefore: encodedBefore, StateAfter: encodedAfter,
	}); err != nil {
		return fmt.Errorf("append study review log: %w", err)
	}
	return nil
}

// ListStudyReviewLog returns a word's review log, oldest first.
func (s *PostgresStore) ListStudyReviewLog(ctx context.Context, owner, progressID string) ([]domain.StudyReviewLog, error) {
	if !validUUID(progressID) {
		return nil, nil
	}
	rows, err := sqlcgen.New(s.pool).ListStudyReviewLog(ctx, sqlcgen.ListStudyReviewLogParams{Owner: owner, Progress: progressID})
	if err != nil {
		return nil, fmt.Errorf("list study review log: %w", err)
	}
	logs := make([]domain.StudyReviewLog, 0, len(rows))
	for _, row := range rows {
		entry := domain.StudyReviewLog{
			ID: row.ID, OwnerID: row.OwnerID, ProgressID: row.ProgressID, MaterialID: row.MaterialID,
			Grade: domain.StudyGrade(row.Grade), ReviewedAt: row.ReviewedAt, StudyDay: row.StudyDay.Time.Format(time.DateOnly),
		}
		if err := json.Unmarshal(row.StateBefore, &entry.Before); err != nil {
			return nil, fmt.Errorf("decode study state: %w", err)
		}
		if err := json.Unmarshal(row.StateAfter, &entry.After); err != nil {
			return nil, fmt.Errorf("decode study state: %w", err)
		}
		logs = append(logs, entry)
	}
	return logs, nil
}

func studyDayDate(studyDay string) (pgtype.Date, error) {
	parsed, err := time.Parse(time.DateOnly, studyDay)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("study day %q: %w", studyDay, err)
	}
	return pgtype.Date{Time: parsed, Valid: true}, nil
}

func studyProgressFromRow(row sqlcgen.StudyProgress) domain.StudyProgress {
	memory := domain.StudyMemory{
		State: domain.StudyCardState(row.State), Due: row.Due, Stability: row.Stability, Difficulty: row.Difficulty,
		ScheduledDays: row.ScheduledDays, Reps: row.Reps, Lapses: row.Lapses, RemainingSteps: row.RemainingSteps,
	}
	if row.LastReview.Valid {
		memory.LastReview = row.LastReview.Time
	}
	return domain.StudyProgress{
		ID: row.ID, OwnerID: row.OwnerID,
		Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, LexicalDiscriminator: row.LexicalDiscriminator,
		MaterialID: row.MaterialID, IntroducedAt: row.IntroducedAt, LearnedMeaning: row.LearnedMeaning,
		Memory: memory, Withdrawn: row.WithdrawnAt.Valid, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
