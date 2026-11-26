package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// CreateStudyReport records a Study report of the word's current Study
// material (ADR 0135 §1). The word is locked first, as a grade locks it, so a
// card answered since it was shown is domain.ErrStudyStale. Reporting the same
// material again while its report is open returns that report. The word's
// scheduling state is never touched. ErrNotFound means the word is not the
// learner's.
func (s *PostgresStore) CreateStudyReport(ctx context.Context, owner string, record domain.StudyReportRecord) (domain.StudyReport, error) {
	if _, ok := domain.ParseStudyReportKind(string(record.Kind)); !ok {
		return domain.StudyReport{}, fmt.Errorf("create study report: unknown kind %q", record.Kind)
	}
	note, ok := domain.CleanStudyReportNote(record.Note)
	if !ok {
		return domain.StudyReport{}, fmt.Errorf("create study report: note is longer than %d characters", domain.StudyReportNoteMax)
	}
	if !validUUID(record.ProgressID) {
		return domain.StudyReport{}, ErrNotFound
	}
	var report domain.StudyReport
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		report, err = createStudyReport(ctx, sqlcgen.New(tx), owner, record, note)
		return err
	})
	if err != nil {
		return domain.StudyReport{}, err
	}
	return report, nil
}

// createStudyReport runs CreateStudyReport inside its transaction, with the
// word locked.
func createStudyReport(ctx context.Context, q *sqlcgen.Queries, owner string, record domain.StudyReportRecord, note string) (domain.StudyReport, error) {
	row, err := q.LockStudyProgress(ctx, sqlcgen.LockStudyProgressParams{Owner: owner, ID: record.ProgressID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyReport{}, ErrNotFound
	}
	if err != nil {
		return domain.StudyReport{}, fmt.Errorf("lock study progress: %w", err)
	}
	progress := studyProgressFromRow(row)
	existing, found, err := openStudyReportOf(ctx, q, owner, progress, record.Kind)
	if err != nil || found {
		return existing, err
	}
	if progress.Withdrawn || progress.Memory.Reps != record.ExpectedReps {
		return domain.StudyReport{}, domain.ErrStudyStale
	}
	created, err := q.CreateStudyReport(ctx, sqlcgen.CreateStudyReportParams{
		Owner: owner, Progress: record.ProgressID, Kind: string(record.Kind), Note: note, CreatedAt: record.CreatedAt.UTC(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyReport{}, domain.ErrStudyStale
	}
	if err != nil {
		return domain.StudyReport{}, fmt.Errorf("create study report: %w", err)
	}
	if record.Kind == domain.StudyReportUnsuitableContext {
		// The word is re-introduced: its reported material is replaced, so new
		// material is prepared from another context (ADR 0135 §3). Its FSRS state
		// is untouched.
		if _, err := q.SupersedeStudyMaterial(ctx, sqlcgen.SupersedeStudyMaterialParams{Owner: owner, ID: created.MaterialID}); err != nil {
			return domain.StudyReport{}, fmt.Errorf("supersede reported study material: %w", err)
		}
	}
	return studyReportFromRow(created), nil
}

// openStudyReportOf finds the open report that a new report of the kind would
// repeat: a Wrong-word report of the word, or an Unsuitable-context report of
// its current material.
func openStudyReportOf(ctx context.Context, q *sqlcgen.Queries, owner string, progress domain.StudyProgress, kind domain.StudyReportKind) (report domain.StudyReport, found bool, err error) {
	var row sqlcgen.StudyReport
	if kind == domain.StudyReportWrongWord {
		row, err = q.GetOpenStudyReport(ctx, sqlcgen.GetOpenStudyReportParams{Owner: owner, Language: progress.Language, Progress: uuidArg(progress.ID), Kind: string(kind)})
	} else {
		row, err = q.GetOpenStudyReportOfMaterial(ctx, sqlcgen.GetOpenStudyReportOfMaterialParams{Owner: owner, Material: progress.MaterialID, Kind: string(kind)})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyReport{}, false, nil
	}
	if err != nil {
		return domain.StudyReport{}, false, fmt.Errorf("get open study report: %w", err)
	}
	return studyReportFromRow(row), true, nil
}

// GetStudyReport returns one report, open or ended. A missing or foreign
// report is ErrNotFound.
func (s *PostgresStore) GetStudyReport(ctx context.Context, owner, id string) (domain.StudyReport, error) {
	if !validUUID(id) {
		return domain.StudyReport{}, ErrNotFound
	}
	row, err := sqlcgen.New(s.pool).GetStudyReport(ctx, sqlcgen.GetStudyReportParams{Owner: owner, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyReport{}, ErrNotFound
	}
	if err != nil {
		return domain.StudyReport{}, fmt.Errorf("get study report: %w", err)
	}
	return studyReportFromRow(row), nil
}

// GetOpenStudyReport returns the word's open report of the kind, or
// ErrNotFound.
func (s *PostgresStore) GetOpenStudyReport(ctx context.Context, owner, language, progressID string, kind domain.StudyReportKind) (domain.StudyReport, error) {
	if !validUUID(progressID) {
		return domain.StudyReport{}, ErrNotFound
	}
	row, err := sqlcgen.New(s.pool).GetOpenStudyReport(ctx, sqlcgen.GetOpenStudyReportParams{
		Owner: owner, Language: canonicalization.NormalizeLanguage(language), Progress: uuidArg(progressID), Kind: string(kind),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyReport{}, ErrNotFound
	}
	if err != nil {
		return domain.StudyReport{}, fmt.Errorf("get open study report: %w", err)
	}
	return studyReportFromRow(row), nil
}

// ListOpenStudyReports returns the learner's open reports of the kind in the
// study language, oldest first.
func (s *PostgresStore) ListOpenStudyReports(ctx context.Context, owner, language string, kind domain.StudyReportKind) ([]domain.StudyReport, error) {
	rows, err := sqlcgen.New(s.pool).ListOpenStudyReports(ctx, sqlcgen.ListOpenStudyReportsParams{
		Owner: owner, Language: canonicalization.NormalizeLanguage(language), Kind: string(kind),
	})
	if err != nil {
		return nil, fmt.Errorf("list open study reports: %w", err)
	}
	return studyReportsFromRows(rows), nil
}

// ListOpenWrongWordReportsForBook returns the learner's open Wrong-word reports
// whose reported occurrence is in the Book, oldest first.
func (s *PostgresStore) ListOpenWrongWordReportsForBook(ctx context.Context, owner, bookID string) ([]domain.StudyReport, error) {
	if !validUUID(bookID) {
		return nil, nil
	}
	rows, err := sqlcgen.New(s.pool).ListOpenStudyReportsForBook(ctx, sqlcgen.ListOpenStudyReportsForBookParams{
		Owner: owner, Book: bookID, Kind: string(domain.StudyReportWrongWord),
	})
	if err != nil {
		return nil, fmt.Errorf("list open study reports for book: %w", err)
	}
	return studyReportsFromRows(rows), nil
}

// ListRetiredStudyOccurrences returns the occurrences retired as the word's
// context by its open Unsuitable-context reports, each with its locator so it
// is found again after an Analysis update (domain.RetiredStudyOccurrence).
func (s *PostgresStore) ListRetiredStudyOccurrences(ctx context.Context, owner, progressID string) ([]domain.StudyOccurrenceRef, error) {
	if !validUUID(progressID) {
		return nil, nil
	}
	rows, err := sqlcgen.New(s.pool).ListOpenStudyReportsOfWord(ctx, sqlcgen.ListOpenStudyReportsOfWordParams{
		Owner: owner, Progress: uuidArg(progressID), Kind: string(domain.StudyReportUnsuitableContext),
	})
	if err != nil {
		return nil, fmt.Errorf("list retired study occurrences: %w", err)
	}
	return s.locatedReportSources(ctx, owner, studyReportsFromRows(rows)), nil
}

// ListRetiredStudyOccurrencesOfWord is ListRetiredStudyOccurrences by the word's
// vocabulary identity, for material preparation, which knows no progress row.
func (s *PostgresStore) ListRetiredStudyOccurrencesOfWord(ctx context.Context, owner string, identity domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error) {
	identity = normalizedStudyIdentity(identity)
	rows, err := sqlcgen.New(s.pool).ListOpenStudyReportsOfIdentity(ctx, sqlcgen.ListOpenStudyReportsOfIdentityParams{
		Owner: owner, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, Upos: identity.UPOS,
		LexicalDiscriminator: identity.LexicalDiscriminator, Kind: string(domain.StudyReportUnsuitableContext),
	})
	if err != nil {
		return nil, fmt.Errorf("list retired study occurrences of word: %w", err)
	}
	return s.locatedReportSources(ctx, owner, studyReportsFromRows(rows)), nil
}

// locatedReportSources completes each report's occurrence with its locator, so
// a retired occurrence is matched across Analysis runs the way a Lemma
// correction relocates an occurrence. An occurrence whose token is gone keeps
// its run-specific key.
func (s *PostgresStore) locatedReportSources(ctx context.Context, owner string, reports []domain.StudyReport) []domain.StudyOccurrenceRef {
	refs := make([]domain.StudyOccurrenceRef, 0, len(reports))
	for _, report := range reports {
		refs = append(refs, s.locatedOccurrence(ctx, owner, report.Source))
	}
	return refs
}

// locatedOccurrence is the occurrence with its locator, or as it was when the
// token cannot be found.
func (s *PostgresStore) locatedOccurrence(ctx context.Context, owner string, ref domain.StudyOccurrenceRef) domain.StudyOccurrenceRef {
	if located, err := s.StudyOccurrenceLocator(ctx, owner, ref); err == nil {
		return located
	}
	return ref
}

// EndStudyReport ends an open report because its occurrence was repaired or is
// gone (domain.StudyReportEnd.Repair). It never changes the word's schedule.
// A report that is missing, foreign or already ended is ErrNotFound.
func (s *PostgresStore) EndStudyReport(ctx context.Context, owner, id string, reason domain.StudyReportEnd, now time.Time) error {
	if !reason.Repair() {
		return fmt.Errorf("end study report: %q is not a repair", reason)
	}
	if !validUUID(id) {
		return ErrNotFound
	}
	ended, err := sqlcgen.New(s.pool).EndStudyReport(ctx, sqlcgen.EndStudyReportParams{Owner: owner, ID: id, Now: now.UTC(), Reason: string(reason)})
	if err != nil {
		return fmt.Errorf("end study report: %w", err)
	}
	if ended == 0 {
		return ErrNotFound
	}
	return nil
}

// DismissStudyReport ends an open Wrong-word report without a repair. The
// word's due time moves later by the time it was held, so the report never
// makes it overdue (ADR 0135 §2). A report that is missing, foreign, ended or
// of another kind is ErrNotFound.
func (s *PostgresStore) DismissStudyReport(ctx context.Context, owner, id string, now time.Time) error {
	return s.changeStudyReport(ctx, owner, id, func(ctx context.Context, q *sqlcgen.Queries, report domain.StudyReport, progress domain.StudyProgress) error {
		if report.Kind != domain.StudyReportWrongWord {
			return ErrNotFound
		}
		if err := endStudyReport(ctx, q, owner, report.ID, domain.StudyReportEndDismissed, now); err != nil {
			return err
		}
		if progress.ID == "" {
			return nil
		}
		due := domain.StudyHeldDue(progress.Memory.Due, report.CreatedAt, now)
		if err := q.UpdateStudyProgressDue(ctx, sqlcgen.UpdateStudyProgressDueParams{Owner: owner, ID: progress.ID, Due: due.UTC()}); err != nil {
			return fmt.Errorf("move study due: %w", err)
		}
		return nil
	})
}

// UndoStudyReport ends an open Unsuitable-context report as undone and returns
// the word to the material, learned meaning and correction time it had when it
// was reported. It is refused with domain.ErrStudyStale once any other Study
// action of the learner in the language followed the report: a graded review, a
// batch taken or declined, a Fluency response or another report. A report that
// is missing, foreign, ended or of another kind is ErrNotFound.
func (s *PostgresStore) UndoStudyReport(ctx context.Context, owner, id string, now time.Time) error {
	return s.changeStudyReport(ctx, owner, id, func(ctx context.Context, q *sqlcgen.Queries, report domain.StudyReport, progress domain.StudyProgress) error {
		if report.Kind != domain.StudyReportUnsuitableContext {
			return ErrNotFound
		}
		locked, err := q.LockStudyReport(ctx, sqlcgen.LockStudyReportParams{Owner: owner, ID: id})
		if err != nil {
			return fmt.Errorf("lock study report: %w", err)
		}
		acted, err := q.StudyActionSince(ctx, sqlcgen.StudyActionSinceParams{Owner: owner, Language: report.Language, Since: locked.RecordedAt, Report: id})
		if err != nil {
			return fmt.Errorf("study action since: %w", err)
		}
		if acted {
			return domain.ErrStudyStale
		}
		if err := endStudyReport(ctx, q, owner, id, domain.StudyReportEndUndone, now); err != nil {
			return err
		}
		return restoreReportedMaterial(ctx, q, owner, report, progress)
	})
}

// restoreReportedMaterial returns the word to the material it was reported in:
// any replacement is itself replaced, the reported record is current again and
// the word's learned meaning and correction time are as they were.
func restoreReportedMaterial(ctx context.Context, q *sqlcgen.Queries, owner string, report domain.StudyReport, progress domain.StudyProgress) error {
	if report.PreviousMaterialID == "" || progress.ID == "" {
		return nil
	}
	if _, err := q.SupersedeOtherCurrentStudyMaterialOfWord(ctx, sqlcgen.SupersedeOtherCurrentStudyMaterialOfWordParams{
		Owner: owner, Language: progress.Language, CanonicalLemma: progress.CanonicalLemma, Upos: progress.UPOS,
		LexicalDiscriminator: progress.LexicalDiscriminator, Keep: report.PreviousMaterialID,
	}); err != nil {
		return fmt.Errorf("supersede replacement material: %w", err)
	}
	if _, err := q.UnsupersedeStudyMaterial(ctx, sqlcgen.UnsupersedeStudyMaterialParams{Owner: owner, ID: report.PreviousMaterialID}); err != nil {
		return fmt.Errorf("restore study material: %w", err)
	}
	if err := q.RestoreStudyProgressMaterial(ctx, sqlcgen.RestoreStudyProgressMaterialParams{
		Owner: owner, ID: progress.ID, Material: report.PreviousMaterialID, LearnedMeaning: report.PreviousLearnedMeaning,
		CorrectedAt: pgtype.Timestamptz{Time: report.PreviousCorrectedAt.UTC(), Valid: !report.PreviousCorrectedAt.IsZero()},
	}); err != nil {
		return fmt.Errorf("restore study progress: %w", err)
	}
	return nil
}

// changeStudyReport runs a change to an open report and its word in one
// transaction, locking the word before the report as a grade does.
func (s *PostgresStore) changeStudyReport(ctx context.Context, owner, id string, change func(context.Context, *sqlcgen.Queries, domain.StudyReport, domain.StudyProgress) error) error {
	if !validUUID(id) {
		return ErrNotFound
	}
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		row, err := q.GetStudyReport(ctx, sqlcgen.GetStudyReportParams{Owner: owner, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("get study report: %w", err)
		}
		// The word's progress row is gone when it was removed after the report.
		var progress domain.StudyProgress
		if row.ProgressID.Valid {
			progressRow, err := q.LockStudyProgress(ctx, sqlcgen.LockStudyProgressParams{Owner: owner, ID: uuidString(row.ProgressID)})
			if err != nil {
				return fmt.Errorf("lock study progress: %w", err)
			}
			progress = studyProgressFromRow(progressRow)
		}
		locked, err := q.LockStudyReport(ctx, sqlcgen.LockStudyReportParams{Owner: owner, ID: id})
		if err != nil {
			return fmt.Errorf("lock study report: %w", err)
		}
		report := studyReportFromRow(locked)
		if !report.Open() {
			return ErrNotFound
		}
		return change(ctx, q, report, progress)
	})
}

func endStudyReport(ctx context.Context, q *sqlcgen.Queries, owner, id string, reason domain.StudyReportEnd, now time.Time) error {
	ended, err := q.EndStudyReport(ctx, sqlcgen.EndStudyReportParams{Owner: owner, ID: id, Now: now.UTC(), Reason: string(reason)})
	if err != nil {
		return fmt.Errorf("end study report: %w", err)
	}
	if ended == 0 {
		return ErrNotFound
	}
	return nil
}

func studyReportsFromRows(rows []sqlcgen.StudyReport) []domain.StudyReport {
	reports := make([]domain.StudyReport, 0, len(rows))
	for _, row := range rows {
		reports = append(reports, studyReportFromRow(row))
	}
	return reports
}

func studyReportFromRow(row sqlcgen.StudyReport) domain.StudyReport {
	report := domain.StudyReport{
		ID: row.ID, OwnerID: row.OwnerID,
		Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, LexicalDiscriminator: row.LexicalDiscriminator,
		ProgressID: uuidString(row.ProgressID), MaterialID: row.MaterialID, Kind: domain.StudyReportKind(row.Kind), Note: row.Note,
		Source: domain.StudyOccurrenceRef{
			BookID: row.BookID, AnalysisRunID: row.AnalysisRunID, CorpusID: row.CorpusID,
			SentenceOrdinal: row.SentenceOrdinal, TokenOrdinal: row.TokenOrdinal,
		},
		Sentence: row.Sentence, TargetWord: row.TargetWord, Gloss: row.Gloss, SentenceTranslation: row.SentenceTranslation,
		CreatedAt: row.CreatedAt,
	}
	if row.PreviousMaterialID.Valid {
		report.PreviousMaterialID = uuidString(row.PreviousMaterialID)
	}
	if row.PreviousCorrectedAt.Valid {
		report.PreviousCorrectedAt = row.PreviousCorrectedAt.Time
	}
	report.PreviousLearnedMeaning = row.PreviousLearnedMeaning.String
	if row.EndedAt.Valid {
		report.EndedAt = row.EndedAt.Time
		report.EndReason = domain.StudyReportEnd(row.EndReason.String)
	}
	return report
}
