package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"context"
	"fmt"
	"strconv"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/riverqueue/river/rivertype"
)

func (s *Store) ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error) {
	return append([]domain.AnalysisJob(nil), s.jobs...), nil
}

func (s *Store) analysisJobState(id int64) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.jobs {
		if job.ID == id {
			return job.AnalysisState, true
		}
	}
	return "", false
}

// cancelFixtureAnalysisJob reports whether a running fixture job was cancelled.
func (s *Store) cancelFixtureAnalysisJob(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID == id && s.jobs[i].AnalysisState == "running" {
			s.jobs[i].AnalysisState = "cancelled"
			return true
		}
	}
	return false
}

// fixtureCancellableJobID is the only fixture analysis job that starts running,
// so the browser smoke can cancel it without changing the fixed jobs.
const fixtureCancellableJobID int64 = 59

func fixtureJobs() []domain.AnalysisJob {
	jobs := []domain.AnalysisJob{{ID: 42, DisplayNumber: 1, OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", AnalysisState: "completed", Progress: 100}, {ID: 43, DisplayNumber: 2, OwnerID: OwnerID, SourceMaterialID: "fixture-failed", AnalysisState: "failed", Error: "The analyzer stopped after the normalized corpus could not be read.\nRetry the analysis when you are ready.", Progress: 42}}
	for i := int64(3); i <= 18; i++ {
		jobs = append(jobs, domain.AnalysisJob{ID: 40 + i, DisplayNumber: i, OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: "fixture-history-" + strconv.FormatInt(i, 10), CorpusID: "fixture-corpus", AnalysisState: "completed", Progress: 100})
	}
	return append(jobs, domain.AnalysisJob{ID: fixtureCancellableJobID, DisplayNumber: 19, OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: "fixture-cancellable-run", CorpusID: "fixture-corpus", AnalysisState: "running", Progress: 35})
}

// Analysis serves the fixed analysis projections. Only the running fixture job
// in fixtureCancellableJobID changes state, and only in Store memory.
type Analysis struct{ Store *Store }

func (Analysis) SubmitToReadBookAnalysis(context.Context, string, string, string) (analysis.Handle, error) {
	return analysis.Handle{ID: 42, DisplayNumber: 1, RunID: ResultRunID}, nil
}

func (a Analysis) Get(ctx context.Context, owner string, id int64) (analysis.Status, error) {
	if state, ok := a.cancellableJobState(id); ok {
		return analysis.Status{ID: id, DisplayNumber: 19, State: rivertype.JobState(state), Progress: 35, SourceMaterialID: SourceID, RunID: "fixture-cancellable-run", LogicalState: state}, nil
	}
	if id == 43 {
		return analysis.Status{ID: 43, DisplayNumber: 2, State: rivertype.JobStateDiscarded, SourceMaterialID: "fixture-failed", Error: "The analyzer stopped after the normalized corpus could not be read.\nRetry the analysis when you are ready.", LogicalState: "failed", Progress: 42}, nil
	}
	return analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, Progress: 100, SourceMaterialID: SourceID, CorpusID: "fixture-corpus", RunID: ResultRunID, LogicalState: "completed"}, nil
}

func (Analysis) Retry(context.Context, string, int64) (analysis.Handle, error) {
	return analysis.Handle{ID: 43, DisplayNumber: 2}, nil
}

// Reconcile has nothing to reconcile without River; it reports the fixed status.
func (a Analysis) Reconcile(ctx context.Context, owner string, id int64) (analysis.Status, error) {
	return a.Get(ctx, owner, id)
}

// Cancel moves the running fixture job to cancelled in memory. Nothing is queued
// or stopped, and every other fixture job stays in its fixed state.
func (a Analysis) Cancel(ctx context.Context, owner string, id int64) (analysis.Status, error) {
	if a.Store == nil || !a.Store.cancelFixtureAnalysisJob(id) {
		return analysis.Status{}, analysis.ErrNotFound
	}
	return a.Get(ctx, owner, id)
}

// EnqueueBrowseCountRebuild is a deliberate no-op: the fixture has no count projection.
func (Analysis) EnqueueBrowseCountRebuild(context.Context, string, string) error { return nil }

func (a Analysis) cancellableJobState(id int64) (string, bool) {
	if a.Store == nil || id != fixtureCancellableJobID {
		return "", false
	}
	return a.Store.analysisJobState(id)
}

func (Analysis) GetCompletedAnalysis(_ context.Context, _ string, sourceMaterialID, runID string) (analysis.CompletedAnalysis, error) {
	result := analysis.CompletedAnalysis{RunID: ResultRunID, OwnerID: OwnerID, SourceMaterialID: SourceID, SnapshotID: "fixture-snapshot", JobID: 42, DisplayNumber: 1, Source: domain.SourceMaterial{ID: SourceID, OwnerID: OwnerID, Language: "de", Title: "Der lange Weg nach Hause"}, Corpus: domain.Corpus{ID: "fixture-corpus", OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 123456, DistinctLemmaCount: 45678, TextProfile: &domain.TextProfile{SentenceCount: 2048, NormalizedTokenCount: 130000, EmptySentenceCount: 3, MedianSentenceTokenCount: 12.5, P90SentenceTokenCount: 38, LongSentenceCount: 117}}}}
	if sourceMaterialID == routeMatchBookID && runID == "fixture-route-match-run" {
		result.RunID = runID
		result.SourceMaterialID = sourceMaterialID
		result.Source = domain.SourceMaterial{ID: sourceMaterialID, OwnerID: OwnerID, Language: "de", Title: "Route match: familiar German"}
		result.Corpus = domain.Corpus{ID: "fixture-route-match-corpus", OwnerID: OwnerID, SourceMaterialID: sourceMaterialID, AnalysisRunID: runID, Statistics: result.Corpus.Statistics}
	}
	return result, nil
}

type Insights struct{}

func (Insights) Coverage(context.Context, string, string) (domain.AnalysisCoverage, error) {
	lemmas := make([]domain.LemmaOccurrence, 0, 18)
	for i := 1; i <= 18; i++ {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: "de", CanonicalLemma: fmt.Sprintf("Randlemma-%02d", i), UPOS: "NOUN", OccurrenceCount: int64(100 - i)})
	}
	projections := make([]domain.CoverageProjection, 0, 8)
	for i := int64(1); i <= 8; i++ {
		projections = append(projections, domain.CoverageProjection{TopLemmaCount: i * 3, SelectedLemmaCount: i * 3, OccurrenceCount: i * 2400, EligibleTokenCount: 80000, ProjectedTokenCount: 50000 + i*7000})
	}
	thresholds := []domain.CoverageThreshold{{TargetPercent: 90, LemmaCount: 120, OccurrenceCount: 90000, EligibleTokenCount: 100000, Reachable: true}, {TargetPercent: 95, LemmaCount: 240, OccurrenceCount: 95000, EligibleTokenCount: 100000, Reachable: true}, {TargetPercent: 97, LemmaCount: 390, OccurrenceCount: 97000, EligibleTokenCount: 100000, Reachable: true}, {TargetPercent: 99, LemmaCount: 999, OccurrenceCount: 0, EligibleTokenCount: 100000, Reachable: false}}
	return domain.AnalysisCoverage{SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, AnalyzableTokenCount: 123456, DistinctLemmaCount: 45678, KnownTokenCount: 45678, KnownLemmaCount: 12000, ReservedTokenCount: 12000, ReservedLemmaCount: 1500, UnknownTokenCount: 77778, UnknownLemmaCount: 33678, TopUnknownLemmas: lemmas, UnknownConcentration: domain.CoverageProjection{TopLemmaCount: 10, OccurrenceCount: 1000, EligibleTokenCount: 5000}, Projections: projections, Thresholds: thresholds, TextProfile: &domain.TextProfile{SentenceCount: 2048, NormalizedTokenCount: 130000, EmptySentenceCount: 3, MedianSentenceTokenCount: 12.5, P90SentenceTokenCount: 38, LongSentenceCount: 117}}, nil
}
