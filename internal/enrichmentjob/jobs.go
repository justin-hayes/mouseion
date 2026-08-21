// Package enrichmentjob runs bulk external translation as durable River work.
package enrichmentjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

const Queue = "enrichment"

var ErrNotFound = errors.New("enrichment job: not found")
var ErrMixedLanguages = errors.New("enrichment job: candidates must share one language")

type Item struct {
	CanonicalLemma  string `json:"canonical_lemma"`
	UPOS            string `json:"upos"`
	ExampleSentence string `json:"example_sentence,omitempty"`
}

type JobArgs struct {
	OwnerID  string `json:"owner_id"`
	Language string `json:"language"`
	Items    []Item `json:"items"`
}

func (JobArgs) Kind() string { return "enrich_external_translation" }

type Handle struct{ ID int64 }

type Status struct {
	ID, Completed, Total int64
	State                rivertype.JobState
	Attempt              int
	Error                string
	CreatedAt            time.Time
	FinalizedAt          *time.Time
}

type Service struct {
	pool       *pgxpool.Pool
	client     *river.Client[pgx.Tx]
	enrichment *enrichment.Service
}

func NewService(pool *pgxpool.Pool, client *river.Client[pgx.Tx], enrich *enrichment.Service) *Service {
	return &Service{pool: pool, client: client, enrichment: enrich}
}

// SubmitEnrichment atomically inserts owner-scoped external translation work.
// A zero handle means external translation is disabled or no work was supplied.
func (s *Service) SubmitEnrichment(ctx context.Context, owner string, candidates []enrichment.Candidate) (Handle, error) {
	if s.enrichment == nil || !s.enrichment.ExternalConfigured() || len(candidates) == 0 {
		return Handle{}, nil
	}
	language := candidates[0].Language
	items := make([]Item, len(candidates))
	for i, candidate := range candidates {
		if candidate.Language != language {
			return Handle{}, ErrMixedLanguages
		}
		items[i] = Item{CanonicalLemma: candidate.CanonicalLemma, UPOS: strings.ToUpper(candidate.UPOS), ExampleSentence: candidate.ExampleSentence}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("begin enrichment submission: %w", err)
	}
	defer tx.Rollback(ctx)
	inserted, err := s.client.InsertTx(ctx, tx, JobArgs{OwnerID: owner, Language: language, Items: items}, &river.InsertOpts{Queue: Queue, MaxAttempts: 3, Metadata: []byte(fmt.Sprintf(`{"completed":0,"total":%d}`, len(items)))})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue enrichment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Handle{}, fmt.Errorf("commit enrichment submission: %w", err)
	}
	return Handle{ID: inserted.Job.ID}, nil
}

func (s *Service) Get(ctx context.Context, owner string, id int64) (Status, error) {
	row, err := s.client.JobGet(ctx, id)
	if errors.Is(err, river.ErrNotFound) {
		return Status{}, ErrNotFound
	}
	if err != nil {
		return Status{}, fmt.Errorf("get River enrichment job: %w", err)
	}
	var args JobArgs
	if row.Kind != (JobArgs{}).Kind() || json.Unmarshal(row.EncodedArgs, &args) != nil || args.OwnerID != owner {
		return Status{}, ErrNotFound
	}
	var progress struct{ Completed, Total int64 }
	_ = json.Unmarshal(row.Metadata, &progress)
	status := Status{ID: row.ID, Completed: progress.Completed, Total: progress.Total, State: row.State, Attempt: row.Attempt, CreatedAt: row.CreatedAt, FinalizedAt: row.FinalizedAt}
	if len(row.Errors) > 0 {
		status.Error = row.Errors[len(row.Errors)-1].Error
	}
	return status, nil
}

func (s *Service) Cancel(ctx context.Context, owner string, id int64) (Status, error) {
	if _, err := s.Get(ctx, owner, id); err != nil {
		return Status{}, err
	}
	if _, err := s.client.JobCancel(ctx, id); err != nil {
		return Status{}, fmt.Errorf("cancel enrichment job: %w", err)
	}
	return s.Get(ctx, owner, id)
}

func (s *Service) Wait(ctx context.Context, owner string, id int64) (Status, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := s.Get(ctx, owner, id)
		if err != nil {
			return Status{}, err
		}
		switch status.State {
		case rivertype.JobStateCompleted, rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
			return status, nil
		}
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

type Worker struct {
	river.WorkerDefaults[JobArgs]
	Pool       *pgxpool.Pool
	Enrichment *enrichment.Service
	Progress   func(context.Context, int64, int, int) error
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	progress := w.Progress
	if progress == nil {
		progress = w.updateProgress
	}
	for i, item := range job.Args.Items {
		candidate := enrichment.Candidate{Identity: enrichment.Identity{Language: job.Args.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS}, ExampleSentence: item.ExampleSentence}
		if _, err := w.Enrichment.EnrichExternal(ctx, candidate); err != nil {
			return fmt.Errorf("translate %s/%s: %w", item.CanonicalLemma, item.UPOS, err)
		}
		if err := progress(ctx, job.ID, i+1, len(job.Args.Items)); err != nil {
			return fmt.Errorf("record enrichment progress: %w", err)
		}
	}
	return nil
}

func (w *Worker) updateProgress(ctx context.Context, id int64, completed, total int) error {
	_, err := w.Pool.Exec(ctx, `UPDATE river_job SET metadata=jsonb_set(jsonb_set(metadata,'{completed}',to_jsonb($2::int),true),'{total}',to_jsonb($3::int),true) WHERE id=$1`, id, completed, total)
	return err
}

func NewClient(pool *pgxpool.Pool, enrich *enrichment.Service) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &Worker{Pool: pool, Enrichment: enrich})
	return river.NewClient(riverpgxv5.New(pool), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
}
