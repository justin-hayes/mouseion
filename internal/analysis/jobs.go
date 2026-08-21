// Package analysis implements the durable, owner-scoped analysis job API.
package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/ranking"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

const (
	Queue             = "analysis"
	jobTimeoutEnv     = "MOUSEION_ANALYSIS_JOB_TIMEOUT"
	defaultJobTimeout = 30 * time.Minute
)

var ErrNotFound = errors.New("analysis job: not found")

type JobArgs struct {
	OwnerID          string `json:"owner_id" river:"unique"`
	SourceMaterialID string `json:"source_material_id"`
	ContentHash      string `json:"content_hash" river:"unique"`
	Language         string `json:"language"`
	Text             string `json:"text"`
	SourceIdentifier string `json:"source_identifier"`
	Title            string `json:"title"`
}

func (JobArgs) Kind() string { return "analyze_corpus" }

type Handle struct{ ID int64 }

type Status struct {
	ID          int64
	State       rivertype.JobState
	Progress    int
	Error       string
	CorpusID    string
	Attempt     int
	CreatedAt   time.Time
	FinalizedAt *time.Time
}

type Service struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
}

func NewService(pool *pgxpool.Pool, client *river.Client[pgx.Tx]) *Service {
	return &Service{pool: pool, client: client}
}

// SubmitAnalysis atomically records an owner-scoped handle and inserts its
// River job. River uniqueness is based on owner + source content hash.
func (s *Service) SubmitAnalysis(ctx context.Context, owner, sourceID string) (Handle, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("begin analysis submission: %w", err)
	}
	defer tx.Rollback(ctx)
	var args JobArgs
	err = tx.QueryRow(ctx, `SELECT owner_id,id,content_hash,language,full_text,source_identifier,title FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, sourceID).
		Scan(&args.OwnerID, &args.SourceMaterialID, &args.ContentHash, &args.Language, &args.Text, &args.SourceIdentifier, &args.Title)
	if errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, ErrNotFound
	}
	if err != nil {
		return Handle{}, fmt.Errorf("load source for analysis: %w", err)
	}
	var existingID int64
	err = tx.QueryRow(ctx, `SELECT river_job_id FROM analysis_jobs WHERE owner_id=$1 AND content_hash=$2`, owner, args.ContentHash).Scan(&existingID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return Handle{}, fmt.Errorf("commit duplicate analysis lookup: %w", err)
		}
		return Handle{ID: existingID}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, fmt.Errorf("check duplicate analysis: %w", err)
	}
	inserted, err := s.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: Queue, MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue analysis: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,content_hash) DO UPDATE SET updated_at=now()`, inserted.Job.ID, owner, sourceID, args.ContentHash)
	if err != nil {
		return Handle{}, fmt.Errorf("record analysis job: %w", err)
	}
	details, _ := json.Marshal(map[string]any{"river_job_id": inserted.Job.ID, "content_hash": args.ContentHash})
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details) VALUES($1,'analysis','queued',$2)`, owner, details); err != nil {
		return Handle{}, fmt.Errorf("record queued analysis history: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Handle{}, fmt.Errorf("commit analysis submission: %w", err)
	}
	return Handle{ID: inserted.Job.ID}, nil
}

func (s *Service) Get(ctx context.Context, owner string, id int64) (Status, error) {
	var status Status
	err := s.pool.QueryRow(ctx, `SELECT river_job_id,progress,error,COALESCE(corpus_id::text,''),created_at FROM analysis_jobs WHERE owner_id=$1 AND river_job_id=$2`, owner, id).
		Scan(&status.ID, &status.Progress, &status.Error, &status.CorpusID, &status.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, ErrNotFound
	}
	if err != nil {
		return Status{}, fmt.Errorf("get analysis handle: %w", err)
	}
	row, err := s.client.JobGet(ctx, id)
	if err != nil {
		return Status{}, fmt.Errorf("get River job: %w", err)
	}
	status.State, status.Attempt, status.FinalizedAt = row.State, row.Attempt, row.FinalizedAt
	if status.Error == "" && len(row.Errors) > 0 {
		status.Error = row.Errors[len(row.Errors)-1].Error
	}
	return status, nil
}

func (s *Service) Result(ctx context.Context, owner string, id int64) (domain.Corpus, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return domain.Corpus{}, err
	}
	if status.State != rivertype.JobStateCompleted || status.CorpusID == "" {
		return domain.Corpus{}, fmt.Errorf("analysis job %d has no result", id)
	}
	var corpus domain.Corpus
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,source_material_id,artifact_hash,status,created_at FROM corpora WHERE owner_id=$1 AND id=$2`, owner, status.CorpusID).
		Scan(&corpus.ID, &corpus.OwnerID, &corpus.SourceMaterialID, &corpus.ArtifactHash, &corpus.Status, &corpus.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Corpus{}, ErrNotFound
	}
	return corpus, err
}

func (s *Service) Cancel(ctx context.Context, owner string, id int64) (Status, error) {
	if _, err := s.Get(ctx, owner, id); err != nil {
		return Status{}, err
	}
	if _, err := s.client.JobCancel(ctx, id); err != nil {
		return Status{}, fmt.Errorf("cancel analysis job: %w", err)
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
	Pool          *pgxpool.Pool
	Analyzer      analyzer.Analyzer
	Selection     *selection.Service
	Ranking       *ranking.Service
	MaxChunkChars int
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) (workErr error) {
	a := job.Args
	if _, err := w.Pool.Exec(ctx, `UPDATE analysis_jobs SET progress=10,error='',updated_at=now() WHERE river_job_id=$1 AND owner_id=$2`, job.ID, a.OwnerID); err != nil {
		return err
	}
	defer func() {
		if workErr != nil {
			failureCtx := context.WithoutCancel(ctx)
			_, _ = w.Pool.Exec(failureCtx, `UPDATE analysis_jobs SET error=$2,updated_at=now() WHERE river_job_id=$1`, job.ID, workErr.Error())
			details, _ := json.Marshal(map[string]any{"river_job_id": job.ID, "error": workErr.Error()})
			_, _ = w.Pool.Exec(failureCtx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'analysis','failed',$2,now())`, a.OwnerID, details)
		}
	}()
	var exists bool
	if err := w.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_materials WHERE owner_id=$1 AND id=$2 AND content_hash=$3)`, a.OwnerID, a.SourceMaterialID, a.ContentHash).Scan(&exists); err != nil || !exists {
		if err != nil {
			return fmt.Errorf("verify analysis ownership: %w", err)
		}
		return fmt.Errorf("verify analysis ownership: source not owned or changed")
	}
	// Analyze the source in size-based chunks (ADR 0013). Each chunk is a
	// separate Analyze RPC so no single call exceeds the deadline or the gRPC
	// message-size limit. Results are merged into one corpus.
	chunks := chunkText(a.Text, w.MaxChunkChars)
	var merged analyzer.Result
	var chunkStartOffset uint64
	for i, chunk := range chunks {
		chunkResult, err := w.Analyzer.Analyze(ctx, analyzer.AnalyzeRequest{Language: a.Language, Document: analyzer.SourceDocument{ID: a.SourceMaterialID, SourceIdentifier: a.SourceIdentifier, Title: a.Title, Text: chunk}})
		if err != nil {
			return fmt.Errorf("analyze source chunk %d/%d: %w", i+1, len(chunks), err)
		}
		if i == 0 {
			// Carry the schema/analysis/profile from the first chunk.
			merged = chunkResult
			merged.Sentences = nil
		}
		offsetResultLocations(&chunkResult, chunkStartOffset)
		merged.Sentences = append(merged.Sentences, chunkResult.Sentences...)
		chunkStartOffset += uint64(len([]rune(chunk)))
		// Report per-chunk progress: 10% (started) → 90% (last chunk done).
		progress := 10 + (i+1)*80/len(chunks)
		if _, err := w.Pool.Exec(ctx, `UPDATE analysis_jobs SET progress=$2,updated_at=now() WHERE river_job_id=$1 AND owner_id=$3`, job.ID, progress, a.OwnerID); err != nil {
			return fmt.Errorf("update progress after analysis chunk %d/%d: %w", i+1, len(chunks), err)
		}
	}
	result := merged
	lemmas := aggregateLemmas(a.ContentHash, result)
	artifact := domain.NormalizedArtifact{ContentHash: a.ContentHash, Language: result.Language, SchemaVersion: result.SchemaVersion, NormalizationProfile: result.NormalizationProfile.Name, NormalizationVersion: result.NormalizationProfile.Version, AnalyzerName: result.Analysis.AnalyzerName, AnalyzerVersion: result.Analysis.AnalyzerVersion}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(content_hash) DO NOTHING`, artifact.ContentHash, artifact.Language, artifact.SchemaVersion, artifact.NormalizationProfile, artifact.NormalizationVersion, artifact.AnalyzerName, artifact.AnalyzerVersion)
	if err != nil {
		return err
	}
	for _, lemma := range lemmas {
		if _, err = tx.Exec(ctx, `INSERT INTO shared_lemmas(content_hash,language,canonical_lemma,upos,morphology,frequency) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(content_hash,canonical_lemma,upos,morphology) DO UPDATE SET frequency=excluded.frequency`, lemma.ContentHash, lemma.Language, lemma.CanonicalLemma, lemma.UPOS, lemma.Morphology, lemma.Frequency); err != nil {
			return err
		}
	}
	var corpusID string
	err = tx.QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,status) VALUES($1,$2,$3,'complete') ON CONFLICT(owner_id,source_material_id) DO UPDATE SET artifact_hash=excluded.artifact_hash,status='complete' RETURNING id`, a.OwnerID, a.SourceMaterialID, a.ContentHash).Scan(&corpusID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	details, _ := json.Marshal(map[string]any{"river_job_id": job.ID, "content_hash": a.ContentHash})
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,'analysis','complete',$3,$4)`, a.OwnerID, corpusID, details, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE analysis_jobs SET corpus_id=$2,progress=100,error='',updated_at=now() WHERE river_job_id=$1 AND owner_id=$3`, job.ID, corpusID, a.OwnerID); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}

	// Candidate generation is deliberately best-effort. The corpus and analysis
	// status have already been committed, so a downstream selection or ranking
	// failure is surfaced in processing history without retrying or losing the
	// successful analysis.
	candidates, err := w.Selection.Select(ctx, a.OwnerID, result, selection.DefaultConfig(corpusID))
	if err != nil {
		w.recordCandidateGenerationFailure(ctx, a.OwnerID, corpusID, job.ID, "selection", err)
		return nil
	}
	if _, err = w.Ranking.Rank(ctx, a.OwnerID, candidates, ranking.DefaultConfig(corpusID)); err != nil {
		w.recordCandidateGenerationFailure(ctx, a.OwnerID, corpusID, job.ID, "ranking", err)
	}
	return nil
}

func (w *Worker) recordCandidateGenerationFailure(ctx context.Context, owner, corpusID string, jobID int64, stage string, cause error) {
	details, _ := json.Marshal(map[string]any{"river_job_id": jobID, "stage": stage, "error": cause.Error()})
	failureCtx := context.WithoutCancel(ctx)
	_, _ = w.Pool.Exec(failureCtx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,'candidate_generation','failed',$3,now())`, owner, corpusID, details)
}

func offsetResultLocations(result *analyzer.Result, offset uint64) {
	if offset == 0 {
		return
	}
	for i := range result.Sentences {
		result.Sentences[i].Location.StartOffset += offset
		result.Sentences[i].Location.EndOffset += offset
		for j := range result.Sentences[i].Tokens {
			result.Sentences[i].Tokens[j].Location.StartOffset += offset
			result.Sentences[i].Tokens[j].Location.EndOffset += offset
		}
	}
}

func aggregateLemmas(hash string, result analyzer.Result) []domain.SharedLemma {
	type entry struct {
		lemma analyzer.Token
		count int64
	}
	values := map[string]entry{}
	for _, sentence := range result.Sentences {
		for _, token := range sentence.Tokens {
			raw, _ := json.Marshal(token.Morphology)
			key := token.CanonicalLemma + "\x00" + token.UPOS + "\x00" + string(raw)
			e := values[key]
			e.lemma = token
			e.count++
			values[key] = e
		}
	}
	out := make([]domain.SharedLemma, 0, len(values))
	for _, e := range values {
		raw, _ := json.Marshal(e.lemma.Morphology)
		out = append(out, domain.SharedLemma{ContentHash: hash, Language: result.Language, CanonicalLemma: e.lemma.CanonicalLemma, UPOS: e.lemma.UPOS, Morphology: raw, Frequency: e.count})
	}
	return out
}

func NewClient(pool *pgxpool.Pool, a analyzer.Analyzer, selectionService *selection.Service, rankingService *ranking.Service) (*river.Client[pgx.Tx], error) {
	if pool == nil || a == nil || selectionService == nil || rankingService == nil {
		return nil, errors.New("analysis client requires pool, analyzer, selection, and ranking services")
	}
	jobTimeout, err := configuredJobTimeout()
	if err != nil {
		return nil, err
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &Worker{Pool: pool, Analyzer: a, Selection: selectionService, Ranking: rankingService})
	return river.NewClient(riverpgxv5.New(pool), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers, JobTimeout: jobTimeout})
}

func configuredJobTimeout() (time.Duration, error) {
	value, ok := os.LookupEnv(jobTimeoutEnv)
	if !ok {
		return defaultJobTimeout, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration: %q", jobTimeoutEnv, value)
	}
	return timeout, nil
}

func MigrateRiver(ctx context.Context, pool *pgxpool.Pool) error {
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("initialize River migrations: %w", err)
	}
	_, err = m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("apply River migrations: %w", err)
	}
	return nil
}
