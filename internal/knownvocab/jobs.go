package knownvocab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
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

func (s *JobService) Submit(ctx context.Context, owner, language, fileContents string) (Handle, error) {
	owner, language = strings.TrimSpace(owner), strings.TrimSpace(language)
	if s == nil || s.pool == nil || s.client == nil || owner == "" || language == "" || fileContents == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("begin known vocabulary submission: %w", err)
	}
	defer tx.Rollback(ctx)
	inserted, err := s.client.InsertTx(ctx, tx, JobArgs{OwnerID: owner, Language: language, FileContents: fileContents}, &river.InsertOpts{Queue: Queue, MaxAttempts: 3})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue known vocabulary import: %w", err)
	}
	details, _ := json.Marshal(map[string]any{"river_job_id": inserted.Job.ID, "language": language, "processed": 0, "total": 0, "imported": 0, "already_known": 0, "rejected": []Rejection{}})
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
	err := s.pool.QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND operation='known_vocabulary.import' AND details->>'river_job_id'=$2 ORDER BY started_at DESC LIMIT 1`, owner, fmt.Sprint(id)).Scan(&details)
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
	if err = w.update(ctx, job.ID, a.OwnerID, "running", map[string]any{"language": language, "processed": len(parsed.Rejected), "total": total, "imported": 0, "already_known": 0, "rejected": parsed.Rejected, "error": ""}, false); err != nil {
		return err
	}
	defer func() {
		if workErr != nil {
			_ = w.update(context.WithoutCancel(ctx), job.ID, a.OwnerID, "failed", map[string]any{"error": workErr.Error()}, true)
		}
	}()
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var libraryLanguage bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM books b
		JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active'
		WHERE b.owner_id=$1 AND b.language_state='chosen'
		  AND b.language_tag=$2
	)`, a.OwnerID, language).Scan(&libraryLanguage); err != nil {
		return err
	}
	if !libraryLanguage {
		return fmt.Errorf("known vocabulary language %q is not present in the library", language)
	}
	result := ImportResult{Rejected: parsed.Rejected}
	for i, entry := range parsed.Entries {
		normalized, normalizeErr := normalizeImportLemma(language, entry.RawLemma)
		if normalizeErr != nil && !errors.Is(normalizeErr, canonicalization.ErrUnsupportedLanguage) {
			return normalizeErr
		}
		if normalizeErr != nil || !lexical.IsLemma(normalized.CanonicalLemma) {
			if normalizeErr == nil {
				normalizeErr = errors.New("canonical lemma must contain at least one letter")
			}
			entry.ErrorTo(&result, normalizeErr)
			continue
		}
		entry.CanonicalLemma, entry.ProfileName, entry.ProfileVersion = normalized.CanonicalLemma, normalized.ProfileName, normalized.ProfileVersion
		var known bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND (upos=$4 OR upos=''))`, a.OwnerID, language, entry.CanonicalLemma, entry.UPOS).Scan(&known); err != nil {
			return fmt.Errorf("check row %d: %w", entry.Row, err)
		}
		if !known {
			if _, err = tx.Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO NOTHING`, a.OwnerID, language, entry.CanonicalLemma, entry.UPOS); err != nil {
				return fmt.Errorf("upsert known vocabulary row %d: %w", entry.Row, err)
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET state=excluded.state,updated_at=now()`, a.OwnerID, language, entry.CanonicalLemma, entry.UPOS, string(vocabulary.Known)); err != nil {
			return fmt.Errorf("set known state row %d: %w", entry.Row, err)
		}
		result.Entries = append(result.Entries, entry)
		if known {
			result.AlreadyKnown++
		} else {
			result.Imported++
		}
		processed := len(parsed.Rejected) + i + 1
		if processed%100 == 0 && processed < total {
			if err = w.update(ctx, job.ID, a.OwnerID, "running", map[string]any{"processed": processed, "total": total, "imported": result.Imported, "already_known": result.AlreadyKnown, "rejected": result.Rejected}, false); err != nil {
				return fmt.Errorf("update import progress: %w", err)
			}
		}
	}
	final := map[string]any{"language": language, "processed": total, "total": total, "imported": result.Imported, "already_known": result.AlreadyKnown, "rejected": result.Rejected, "error": ""}
	if err = w.updateTx(ctx, tx, job.ID, a.OwnerID, "completed", final); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return river.RecordOutput(ctx, result)
}

func normalizeImportLemma(language, rawLemma string) (canonicalization.NormalizedLemma, error) {
	normalized, err := canonicalization.Normalize(language, rawLemma)
	if !errors.Is(err, canonicalization.ErrUnsupportedLanguage) {
		return normalized, err
	}
	return canonicalization.NormalizedLemma{
		RawLemma:       rawLemma,
		CanonicalLemma: canonicalization.Lemma(rawLemma),
		ProfileName:    "language-neutral",
		ProfileVersion: "1",
	}, nil
}

func (w *Worker) update(ctx context.Context, id int64, owner, state string, fields map[string]any, completed bool) error {
	completedSQL := ""
	if completed {
		completedSQL = ",completed_at=now()"
	}
	tag, err := w.Pool.Exec(ctx, `UPDATE processing_history SET status=$3,details=details || $4::jsonb`+completedSQL+` WHERE owner_id=$1 AND operation='known_vocabulary.import' AND details->>'river_job_id'=$2`, owner, fmt.Sprint(id), state, fields)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrJobNotFound
	}
	return err
}
func (w *Worker) updateTx(ctx context.Context, tx pgx.Tx, id int64, owner, state string, fields map[string]any) error {
	tag, err := tx.Exec(ctx, `UPDATE processing_history SET status=$3,details=details || $4::jsonb,completed_at=now() WHERE owner_id=$1 AND operation='known_vocabulary.import' AND details->>'river_job_id'=$2`, owner, fmt.Sprint(id), state, fields)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrJobNotFound
	}
	return err
}
