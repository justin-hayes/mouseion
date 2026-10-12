package knownvocab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const Queue = "known_vocabulary"

var ErrJobNotFound = errors.New("known vocabulary job: not found")

type JobArgs struct {
	OwnerID      string `json:"owner_id"`
	Language     string `json:"language"`
	FileContents string `json:"file_contents"`
}

func (JobArgs) Kind() string { return "import_known_vocabulary" }

type Handle struct{ ID int64 }
type Status struct {
	ID                                       int64
	Language                                 string
	State                                    rivertype.JobState
	Processed, Total, Imported, AlreadyKnown int
	Rejected                                 []Rejection
	Error                                    string
	Attempt                                  int
	FinalizedAt                              *time.Time
}

type JobService struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
}

func NewJobService(pool *pgxpool.Pool, client *river.Client[pgx.Tx]) *JobService {
	return &JobService{pool: pool, client: client}
}

func (s *JobService) Submit(ctx context.Context, owner, language, fileContents string) (result Handle, err error) {
	owner, language = strings.TrimSpace(owner), strings.TrimSpace(language)
	if s == nil || s.pool == nil || s.client == nil || owner == "" || language == "" || fileContents == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("begin known vocabulary submission: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	inserted, err := s.client.InsertTx(ctx, tx, JobArgs{OwnerID: owner, Language: language, FileContents: fileContents}, &river.InsertOpts{Queue: Queue, MaxAttempts: 3})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue known vocabulary import: %w", err)
	}
	details, err := json.Marshal(map[string]any{"river_job_id": inserted.Job.ID, "language": language, "processed": 0, "total": 0, "imported": 0, "already_known": 0, "rejected": []Rejection{}})
	if err != nil {
		return Handle{}, fmt.Errorf("encode known vocabulary history: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details) VALUES($1,'known_vocabulary.import','queued',$2)`, owner, details); err != nil {
		return Handle{}, fmt.Errorf("record known vocabulary job: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, fmt.Errorf("commit known vocabulary submission: %w", err)
	}
	return Handle{ID: inserted.Job.ID}, nil
}

func (s *JobService) Get(ctx context.Context, owner string, id int64) (Status, error) {
	var details []byte
	err := s.pool.QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND operation='known_vocabulary.import' AND details->>'river_job_id'=$2 ORDER BY started_at DESC LIMIT 1`, owner, strconv.FormatInt(id, 10)).Scan(&details)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, ErrJobNotFound
	}
	if err != nil {
		return Status{}, fmt.Errorf("get known vocabulary job: %w", err)
	}
	var p struct {
		Language     string      `json:"language"`
		Processed    int         `json:"processed"`
		Total        int         `json:"total"`
		Imported     int         `json:"imported"`
		AlreadyKnown int         `json:"already_known"`
		Rejected     []Rejection `json:"rejected"`
		Error        string      `json:"error"`
	}
	if err = json.Unmarshal(details, &p); err != nil {
		return Status{}, fmt.Errorf("decode known vocabulary job: %w", err)
	}
	row, err := s.client.JobGet(ctx, id)
	if err != nil {
		return Status{}, fmt.Errorf("get River known vocabulary job: %w", err)
	}
	status := Status{ID: id, Language: p.Language, State: row.State, Processed: p.Processed, Total: p.Total, Imported: p.Imported, AlreadyKnown: p.AlreadyKnown, Rejected: p.Rejected, Error: p.Error, Attempt: row.Attempt, FinalizedAt: row.FinalizedAt}
	if status.Error == "" && len(row.Errors) > 0 {
		status.Error = row.Errors[len(row.Errors)-1].Error
	}
	return status, nil
}

func AddWorker(workers *river.Workers, pool *pgxpool.Pool) {
	river.AddWorker(workers, &Worker{Pool: pool})
}

type Worker struct {
	river.WorkerDefaults[JobArgs]
	Pool *pgxpool.Pool
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) (workErr error) {
	a := job.Args
	language := canonicalization.NormalizeLanguage(a.Language)
	parsed, err := Parse(strings.NewReader(a.FileContents))
	if err != nil {
		return err
	}
	total := len(parsed.Entries) + len(parsed.Rejected)
	if err := w.update(ctx, job.ID, a.OwnerID, "running", map[string]any{"language": language, "processed": len(parsed.Rejected), "total": total, "imported": 0, "already_known": 0, "rejected": parsed.Rejected, "error": ""}, false); err != nil {
		return err
	}
	defer func() {
		if workErr != nil {
			workErr = w.recordFailure(ctx, job.ID, a.OwnerID, workErr)
		}
	}()
	return w.importEntries(ctx, job.ID, a.OwnerID, language, total, parsed)
}

// importEntries runs the import in one transaction: it checks the target
// library language, applies each parsed entry, reports progress in batches,
// and records completion before committing.
func (w *Worker) importEntries(ctx context.Context, id int64, owner, language string, total int, parsed ParseResult) (err error) {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err := requireLibraryLanguage(ctx, tx, owner, language); err != nil {
		return err
	}
	result := ImportResult{Rejected: parsed.Rejected}
	if err := w.applyParsedEntries(ctx, tx, id, owner, language, total, parsed, &result); err != nil {
		return err
	}
	return w.persistCompletion(ctx, tx, id, owner, language, total, result)
}

// applyParsedEntries upserts each parsed entry and reports progress in batches
// while entries remain.
func (w *Worker) applyParsedEntries(ctx context.Context, tx pgx.Tx, id int64, owner, language string, total int, parsed ParseResult, result *ImportResult) error {
	for i, entry := range parsed.Entries {
		alreadyKnown, accepted, err := applyEntry(ctx, tx, owner, language, entry, result)
		if err != nil {
			return err
		}
		if !accepted {
			continue
		}
		if alreadyKnown {
			result.AlreadyKnown++
		} else {
			result.Imported++
		}
		processed := len(parsed.Rejected) + i + 1
		if processed%100 == 0 && processed < total {
			if err := w.reportProgress(ctx, id, owner, processed, total, result); err != nil {
				return err
			}
		}
	}
	return nil
}

// requireLibraryLanguage rejects an import for a language the owner has no
// active chosen-language Book in. It runs inside the import transaction so the
// check and the writes that follow share one snapshot.
func requireLibraryLanguage(ctx context.Context, tx pgx.Tx, owner, language string) error {
	var present bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM books b
		JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active'
		WHERE b.owner_id=$1 AND b.language_state='chosen'
		  AND b.language_tag=$2
	)`, owner, language).Scan(&present); err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("known vocabulary language %q is not present in the library", language)
	}
	return nil
}

// applyEntry normalizes one parsed entry and upserts its known-vocabulary row.
// A rejected entry is recorded on result and reported with accepted false; an
// accepted entry is appended to result and reports whether it was already known.
func applyEntry(ctx context.Context, tx pgx.Tx, owner, language string, entry Entry, result *ImportResult) (alreadyKnown, accepted bool, err error) {
	normalized, normalizeErr := normalizeImportLemma(language, entry.RawLemma)
	if normalizeErr != nil && !errors.Is(normalizeErr, canonicalization.ErrUnsupportedLanguage) {
		return false, false, normalizeErr
	}
	if normalizeErr != nil || !lexical.IsLemma(normalized.CanonicalLemma) {
		if normalizeErr == nil {
			normalizeErr = errors.New("canonical lemma must contain at least one letter")
		}
		entry.ErrorTo(result, normalizeErr)
		return false, false, nil
	}
	entry.CanonicalLemma, entry.ProfileName, entry.ProfileVersion = normalized.CanonicalLemma, normalized.ProfileName, normalized.ProfileVersion
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND (upos=$4 OR upos=''))`, owner, language, entry.CanonicalLemma, entry.UPOS).Scan(&alreadyKnown); err != nil {
		return false, false, fmt.Errorf("check row %d: %w", entry.Row, err)
	}
	if !alreadyKnown {
		if _, err := tx.Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO NOTHING`, owner, language, entry.CanonicalLemma, entry.UPOS); err != nil {
			return false, false, fmt.Errorf("upsert known vocabulary row %d: %w", entry.Row, err)
		}
	}
	result.Entries = append(result.Entries, entry)
	return alreadyKnown, true, nil
}

// reportProgress publishes the periodic running counters while entries remain.
func (w *Worker) reportProgress(ctx context.Context, id int64, owner string, processed, total int, result *ImportResult) error {
	if err := w.update(ctx, id, owner, "running", map[string]any{"processed": processed, "total": total, "imported": result.Imported, "already_known": result.AlreadyKnown, "rejected": result.Rejected}, false); err != nil {
		return fmt.Errorf("update import progress: %w", err)
	}
	return nil
}

// persistCompletion writes the completed status and final counters in the
// import transaction, commits it, and records the River output.
func (w *Worker) persistCompletion(ctx context.Context, tx pgx.Tx, id int64, owner, language string, total int, result ImportResult) error {
	final := map[string]any{"language": language, "processed": total, "total": total, "imported": result.Imported, "already_known": result.AlreadyKnown, "rejected": result.Rejected, "error": ""}
	if err := w.updateTx(ctx, tx, id, owner, "completed", final); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return river.RecordOutput(ctx, result)
}

// recordFailure records one failed import against its processing-history row
// and completed time, joining any recording failure onto the work error.
func (w *Worker) recordFailure(ctx context.Context, id int64, owner string, workErr error) error {
	if updateErr := w.update(context.WithoutCancel(ctx), id, owner, "failed", map[string]any{"error": workErr.Error()}, true); updateErr != nil {
		return errors.Join(workErr, fmt.Errorf("record known vocabulary failure: %w", updateErr))
	}
	return workErr
}

func normalizeImportLemma(language, rawLemma string) (canonicalization.NormalizedLemma, error) {
	return canonicalization.Normalize(language, rawLemma)
}

func (w *Worker) update(ctx context.Context, id int64, owner, state string, fields map[string]any, completed bool) error {
	completedSQL := ""
	if completed {
		completedSQL = ",completed_at=now()"
	}
	tag, err := w.Pool.Exec(ctx, `UPDATE processing_history SET status=$3,details=details || $4::jsonb`+completedSQL+` WHERE owner_id=$1 AND operation='known_vocabulary.import' AND details->>'river_job_id'=$2`, owner, strconv.FormatInt(id, 10), state, fields)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrJobNotFound
	}
	return err
}
func (w *Worker) updateTx(ctx context.Context, tx pgx.Tx, id int64, owner, state string, fields map[string]any) error {
	tag, err := tx.Exec(ctx, `UPDATE processing_history SET status=$3,details=details || $4::jsonb,completed_at=now() WHERE owner_id=$1 AND operation='known_vocabulary.import' AND details->>'river_job_id'=$2`, owner, strconv.FormatInt(id, 10), state, fields)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrJobNotFound
	}
	return err
}
