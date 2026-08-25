// Package analysis implements the durable, owner-scoped analysis job API.
package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
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
	ReviewedScopeID  string `json:"reviewed_scope_id,omitempty" river:"unique"`
}

func (JobArgs) Kind() string { return "analyze_corpus" }

type Handle struct {
	ID, DisplayNumber int64
}

type Status struct {
	ID            int64
	DisplayNumber int64
	State         rivertype.JobState
	Progress      int
	Error         string
	CorpusID      string
	Attempt       int
	CreatedAt     time.Time
	FinalizedAt   *time.Time
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
	return s.submitAnalysis(ctx, owner, sourceID, "")
}

// SubmitScopedAnalysis queues an immutable reviewed EPUB scope. Unit text is
// deliberately absent from job arguments and is reloaded and validated by the worker.
func (s *Service) SubmitScopedAnalysis(ctx context.Context, owner, sourceID, scopeID string) (Handle, error) {
	if scopeID == "" {
		return Handle{}, fmt.Errorf("reviewed scope id is required")
	}
	return s.submitAnalysis(ctx, owner, sourceID, scopeID)
}

func (s *Service) submitAnalysis(ctx context.Context, owner, sourceID, scopeID string) (Handle, error) {
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
	args.ReviewedScopeID = scopeID
	if scopeID != "" {
		if _, err = loadScopedUnits(ctx, tx, owner, sourceID, scopeID); err != nil {
			return Handle{}, err
		}
		args.Text = ""
	}
	// Serialize submissions per owner so display numbers remain gap-free and
	// unique without exposing River's global sequence.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner); err != nil {
		return Handle{}, fmt.Errorf("lock analysis submissions: %w", err)
	}
	var existingID, existingDisplayNumber int64
	err = tx.QueryRow(ctx, `SELECT river_job_id,display_number FROM analysis_jobs WHERE owner_id=$1 AND (($3='' AND reviewed_scope_id IS NULL AND content_hash=$2) OR reviewed_scope_id::text=$3)`, owner, args.ContentHash, scopeID).Scan(&existingID, &existingDisplayNumber)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return Handle{}, fmt.Errorf("commit duplicate analysis lookup: %w", err)
		}
		return Handle{ID: existingID, DisplayNumber: existingDisplayNumber}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, fmt.Errorf("check duplicate analysis: %w", err)
	}
	inserted, err := s.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: Queue, MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue analysis: %w", err)
	}
	var displayNumber int64
	err = tx.QueryRow(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,reviewed_scope_id,display_number)
		VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,(SELECT COALESCE(MAX(display_number),0)+1 FROM analysis_jobs WHERE owner_id=$2))
		RETURNING display_number`, inserted.Job.ID, owner, sourceID, args.ContentHash, scopeID).Scan(&displayNumber)
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
	return Handle{ID: inserted.Job.ID, DisplayNumber: displayNumber}, nil
}

func (s *Service) Get(ctx context.Context, owner string, id int64) (Status, error) {
	var status Status
	err := s.pool.QueryRow(ctx, `SELECT river_job_id,display_number,progress,error,COALESCE(corpus_id::text,''),created_at FROM analysis_jobs WHERE owner_id=$1 AND river_job_id=$2`, owner, id).
		Scan(&status.ID, &status.DisplayNumber, &status.Progress, &status.Error, &status.CorpusID, &status.CreatedAt)
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
	var analyzableTokenCount, distinctLemmaCount, sentenceCount, normalizedTokenCount, emptySentenceCount, p90SentenceTokenCount, longSentenceCount *int64
	var medianSentenceTokenCount *float64
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,source_material_id,artifact_hash,COALESCE(reviewed_scope_id::text,''),status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count,created_at FROM corpora WHERE owner_id=$1 AND id=$2`, owner, status.CorpusID).
		Scan(&corpus.ID, &corpus.OwnerID, &corpus.SourceMaterialID, &corpus.ArtifactHash, &corpus.ReviewedScopeID, &corpus.Status, &analyzableTokenCount, &distinctLemmaCount, &sentenceCount, &normalizedTokenCount, &emptySentenceCount, &medianSentenceTokenCount, &p90SentenceTokenCount, &longSentenceCount, &corpus.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Corpus{}, ErrNotFound
	}
	if err == nil && analyzableTokenCount != nil && distinctLemmaCount != nil {
		corpus.Statistics = &domain.AnalysisStatistics{AnalyzableTokenCount: *analyzableTokenCount, DistinctLemmaCount: *distinctLemmaCount}
		if sentenceCount != nil && normalizedTokenCount != nil && emptySentenceCount != nil && medianSentenceTokenCount != nil && p90SentenceTokenCount != nil && longSentenceCount != nil {
			corpus.Statistics.TextProfile = &domain.TextProfile{SentenceCount: *sentenceCount, NormalizedTokenCount: *normalizedTokenCount, EmptySentenceCount: *emptySentenceCount, MedianSentenceTokenCount: *medianSentenceTokenCount, P90SentenceTokenCount: *p90SentenceTokenCount, LongSentenceCount: *longSentenceCount}
		}
	}
	if err == nil && corpus.ReviewedScopeID != "" {
		rows, queryErr := s.pool.Query(ctx, `SELECT unit_id,unit_order,source_href,resolved_href,title,start_offset,end_offset FROM corpus_selected_units WHERE owner_id=$1 AND corpus_id=$2 ORDER BY unit_order`, owner, corpus.ID)
		if queryErr != nil {
			return domain.Corpus{}, queryErr
		}
		defer rows.Close()
		for rows.Next() {
			var unit domain.CorpusSelectedUnit
			if queryErr = rows.Scan(&unit.UnitID, &unit.Order, &unit.SourceHref, &unit.ResolvedHref, &unit.Title, &unit.StartOffset, &unit.EndOffset); queryErr != nil {
				return domain.Corpus{}, queryErr
			}
			corpus.SelectedUnits = append(corpus.SelectedUnits, unit)
		}
		if queryErr = rows.Err(); queryErr != nil {
			return domain.Corpus{}, queryErr
		}
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
	MaxChunkChars int
}

type scopedUnit struct {
	SnapshotID, UnitID, Title, SourceHref, ResolvedHref, Text string
	Order                                                     int
	StartOffset, EndOffset                                    uint64
}

type queryRower interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadScopedUnits(ctx context.Context, q queryRower, owner, sourceID, scopeID string) ([]scopedUnit, error) {
	rows, err := q.Query(ctx, `SELECT s.snapshot_id,u.unit_id,u.unit_order,u.title,u.source_href,u.resolved_href,u.text,u.start_offset,u.end_offset,
		(SELECT count(*) FROM epub_reviewed_scope_units expected WHERE expected.scope_id=s.scope_id)
		FROM epub_reviewed_scopes s
		JOIN source_material_unit_snapshots snap ON snap.owner_id=s.owner_id AND snap.source_material_id=s.source_material_id AND snap.snapshot_id=s.snapshot_id AND snap.schema_version=s.extracted_units_schema_version
		JOIN epub_reviewed_scope_units selected ON selected.scope_id=s.scope_id AND selected.owner_id=s.owner_id AND selected.source_material_id=s.source_material_id
		JOIN source_material_units u ON u.owner_id=selected.owner_id AND u.source_material_id=selected.source_material_id AND u.unit_id=selected.unit_id AND u.unit_order=selected.unit_order
		WHERE s.scope_id=$1 AND s.owner_id=$2 AND s.source_material_id=$3 AND s.schema_version=$4
		ORDER BY selected.unit_order`, scopeID, owner, sourceID, domain.EPUBReviewedScopeSchemaVersion)
	if err != nil {
		return nil, fmt.Errorf("load reviewed scope: %w", err)
	}
	defer rows.Close()
	var units []scopedUnit
	expectedCount := -1
	lastOrder := -1
	for rows.Next() {
		var unit scopedUnit
		var rowExpectedCount int
		if err = rows.Scan(&unit.SnapshotID, &unit.UnitID, &unit.Order, &unit.Title, &unit.SourceHref, &unit.ResolvedHref, &unit.Text, &unit.StartOffset, &unit.EndOffset, &rowExpectedCount); err != nil {
			return nil, fmt.Errorf("load reviewed scope unit: %w", err)
		}
		if expectedCount == -1 {
			expectedCount = rowExpectedCount
		}
		if expectedCount != rowExpectedCount || unit.UnitID == "" || strings.TrimSpace(unit.Text) == "" || unit.Order <= lastOrder || unit.EndOffset < unit.StartOffset {
			return nil, fmt.Errorf("reviewed scope is invalid")
		}
		lastOrder = unit.Order
		units = append(units, unit)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("load reviewed scope units: %w", err)
	}
	if len(units) == 0 || len(units) != expectedCount {
		return nil, fmt.Errorf("reviewed scope is missing, stale, cross-owner, empty, or invalid")
	}
	return units, nil
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
	type analysisInput struct{ id, identifier, title, text string }
	inputs := []analysisInput{{a.SourceMaterialID, a.SourceIdentifier, a.Title, a.Text}}
	var selectedUnits []scopedUnit
	artifactHash := a.ContentHash
	if a.ReviewedScopeID != "" {
		var err error
		selectedUnits, err = loadScopedUnits(ctx, w.Pool, a.OwnerID, a.SourceMaterialID, a.ReviewedScopeID)
		if err != nil {
			return err
		}
		inputs = make([]analysisInput, 0, len(selectedUnits))
		for _, unit := range selectedUnits {
			identifier := unit.ResolvedHref
			if identifier == "" {
				identifier = unit.SourceHref
			}
			inputs = append(inputs, analysisInput{unit.UnitID, identifier, unit.Title, unit.Text})
		}
		artifactHash = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(a.ContentHash+"\x00"+a.ReviewedScopeID)))
	}
	// Analyze each selected unit independently, retaining its unit ID as the
	// source-document identity. Oversized units still use ADR 0013 chunking.
	var merged analyzer.Result
	totalChunks := 0
	for _, input := range inputs {
		totalChunks += len(chunkText(input.text, w.MaxChunkChars))
	}
	completedChunks := 0
	for _, input := range inputs {
		chunks := chunkText(input.text, w.MaxChunkChars)
		var chunkStartOffset uint64
		for i, chunk := range chunks {
			chunkResult, err := w.Analyzer.Analyze(ctx, analyzer.AnalyzeRequest{Language: a.Language, Document: analyzer.SourceDocument{ID: input.id, SourceIdentifier: input.identifier, Title: input.title, Text: chunk}})
			if err != nil {
				return fmt.Errorf("analyze source unit %s chunk %d/%d: %w", input.id, i+1, len(chunks), err)
			}
			if completedChunks == 0 {
				// Carry the schema/analysis/profile from the first chunk.
				merged = chunkResult
				merged.Sentences = nil
				merged.SourceDocuments = nil
			}
			offsetResultLocations(&chunkResult, chunkStartOffset)
			merged.Sentences = append(merged.Sentences, chunkResult.Sentences...)
			if i == 0 {
				merged.SourceDocuments = append(merged.SourceDocuments, chunkResult.SourceDocuments...)
			}
			chunkStartOffset += uint64(len([]rune(chunk)))
			completedChunks++
			// Report per-chunk progress: 10% (started) → 90% (last chunk done).
			progress := 10 + completedChunks*80/totalChunks
			if _, err := w.Pool.Exec(ctx, `UPDATE analysis_jobs SET progress=$2,updated_at=now() WHERE river_job_id=$1 AND owner_id=$3`, job.ID, progress, a.OwnerID); err != nil {
				return fmt.Errorf("update progress after analysis chunk %d/%d: %w", i+1, len(chunks), err)
			}
		}
	}
	result := merged
	lemmas := aggregateLemmas(artifactHash, result)
	selectionConfig := selection.DefaultConfig("")
	statistics := selection.AnalyzableStatistics(result, selectionConfig)
	artifact := domain.NormalizedArtifact{ContentHash: artifactHash, Language: result.Language, SchemaVersion: result.SchemaVersion, NormalizationProfile: result.NormalizationProfile.Name, NormalizationVersion: result.NormalizationProfile.Version, AnalyzerName: result.Analysis.AnalyzerName, AnalyzerVersion: result.Analysis.AnalyzerVersion}
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
	profile := statistics.TextProfile
	conflict := `(owner_id,source_material_id) WHERE reviewed_scope_id IS NULL`
	if a.ReviewedScopeID != "" {
		conflict = `(owner_id,reviewed_scope_id) WHERE reviewed_scope_id IS NOT NULL`
	}
	query := `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,reviewed_scope_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count) VALUES($1,$2,$3,NULLIF($4,'')::uuid,'complete',$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT ` + conflict + ` DO UPDATE SET artifact_hash=excluded.artifact_hash,status='complete',analyzable_token_count=excluded.analyzable_token_count,distinct_lemma_count=excluded.distinct_lemma_count,sentence_count=excluded.sentence_count,normalized_token_count=excluded.normalized_token_count,empty_sentence_count=excluded.empty_sentence_count,median_sentence_token_count=excluded.median_sentence_token_count,p90_sentence_token_count=excluded.p90_sentence_token_count,long_sentence_count=excluded.long_sentence_count RETURNING id`
	err = tx.QueryRow(ctx, query, a.OwnerID, a.SourceMaterialID, artifactHash, a.ReviewedScopeID, statistics.AnalyzableTokenCount, statistics.DistinctLemmaCount, profile.SentenceCount, profile.NormalizedTokenCount, profile.EmptySentenceCount, profile.MedianSentenceTokenCount, profile.P90SentenceTokenCount, profile.LongSentenceCount).Scan(&corpusID)
	if err != nil {
		return err
	}
	if a.ReviewedScopeID != "" {
		if _, err = tx.Exec(ctx, `DELETE FROM corpus_selected_units WHERE owner_id=$1 AND corpus_id=$2`, a.OwnerID, corpusID); err != nil {
			return err
		}
		for _, unit := range selectedUnits {
			if _, err = tx.Exec(ctx, `INSERT INTO corpus_selected_units(corpus_id,owner_id,scope_id,source_material_id,snapshot_id,unit_id,unit_order,source_href,resolved_href,title,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, corpusID, a.OwnerID, a.ReviewedScopeID, a.SourceMaterialID, unit.SnapshotID, unit.UnitID, unit.Order, unit.SourceHref, unit.ResolvedHref, unit.Title, unit.StartOffset, unit.EndOffset); err != nil {
				return err
			}
		}
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
	// status have already been committed, so a downstream selection
	// failure is surfaced in processing history without retrying or losing the
	// successful analysis.
	selectionConfig.CorpusID = corpusID
	_, err = w.Selection.Select(ctx, a.OwnerID, result, selectionConfig)
	if err != nil {
		w.recordCandidateGenerationFailure(ctx, a.OwnerID, corpusID, job.ID, "selection", err)
		return nil
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
			if !lexical.IsLemma(token.CanonicalLemma) {
				continue
			}
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

func NewClient(pool *pgxpool.Pool, a analyzer.Analyzer, selectionService *selection.Service, workerSets ...*river.Workers) (*river.Client[pgx.Tx], error) {
	if pool == nil || a == nil || selectionService == nil {
		return nil, errors.New("analysis client requires pool, analyzer, and selection service")
	}
	jobTimeout, err := configuredJobTimeout()
	if err != nil {
		return nil, err
	}
	workers := river.NewWorkers()
	if len(workerSets) > 0 && workerSets[0] != nil {
		workers = workerSets[0]
	}
	river.AddWorker(workers, &Worker{Pool: pool, Analyzer: a, Selection: selectionService})
	return river.NewClient(riverpgxv5.New(pool), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, "known_vocabulary": {MaxWorkers: 1}, "prepared_decks": {MaxWorkers: 1}}, Workers: workers, JobTimeout: jobTimeout})
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
