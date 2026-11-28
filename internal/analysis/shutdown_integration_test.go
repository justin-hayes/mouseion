//go:build integration

package analysis

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/jobstop"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
)

// shutdownFixture is the store, River schema, owner and source the analysis
// shutdown tests share.
type shutdownFixture struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	owner  domain.User
	source domain.SourceMaterial
}

func newShutdownFixture(t *testing.T, name string) shutdownFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, name+"-alice", false)
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, name+"-source", name, "Hallo Berlin.", "sha256:"+name)
	require.NoError(t, err)
	return shutdownFixture{ctx: ctx, pool: pool, owner: owner, source: source}
}

func (f shutdownFixture) runAndAttemptState(t *testing.T, runID string) (runState, attemptState string) {
	t.Helper()
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT r.state,a.state FROM analysis_runs r JOIN analysis_run_attempts a ON a.run_id=r.id WHERE r.id=$1`, runID).Scan(&runState, &attemptState))
	return runState, attemptState
}

func (f shutdownFixture) analysisFailures(t *testing.T) int {
	t.Helper()
	var failures int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='analysis' AND status='failed'`, f.owner.ID).Scan(&failures))
	return failures
}

// TestAnalysisJobStoppedByShutdownRunsAgainAndCompletes proves a single-attempt
// analysis job (MaxAttempts 1) cancelled by StopAndCancel is made available
// with its attempt refunded, not discarded, and that a fresh client resumes its
// run and completes it.
func TestAnalysisJobStoppedByShutdownRunsAgainAndCompletes(t *testing.T) {
	f := newShutdownFixture(t, "shutdown")
	ctx := f.ctx
	started := make(chan struct{}, 1)
	blocking := &analyzertest.Fake{AnalyzeFunc: func(ctx context.Context, _ analyzer.AnalyzeRequest) (analyzer.Result, error) {
		started <- struct{}{}
		<-ctx.Done()
		// The production analyzer reports a cancelled call as a gRPC status
		// error, not context.Canceled.
		return analyzer.Result{}, fmt.Errorf("call NLP analyzer: %w", status.Error(codes.Canceled, "context canceled"))
	}}
	first, err := NewClient(f.pool, blocking, analyzertest.ReadyDepparseCapabilityProvider())
	require.NoError(t, err)
	require.NoError(t, first.Start(ctx))
	handle, err := NewService(f.pool, first).SubmitAnalysis(ctx, f.owner.ID, f.source.ID)
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("the analysis never reached the analyzer")
	}
	require.NoError(t, first.StopAndCancel(ctx))

	t.Run("the River job is available with its attempt refunded", func(t *testing.T) {
		job, err := first.JobGet(ctx, handle.JobID)
		require.NoError(t, err)
		require.Equal(t, rivertype.JobStateAvailable, job.State, "a stopped job is not discarded")
		require.Equal(t, 0, job.Attempt, "the attempt is refunded")
	})
	t.Run("the run and its attempt stay resumable", func(t *testing.T) {
		runState, attemptState := f.runAndAttemptState(t, handle.RunID)
		require.Equal(t, "running", runState, "the run is resumable, not failed")
		require.Equal(t, "running", attemptState)
	})
	t.Run("no analysis failure is recorded", func(t *testing.T) {
		require.Zero(t, f.analysisFailures(t), "a shutdown is not recorded as an analysis failure")
	})

	resumed := &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{
			SchemaVersion: "1.0.0",
			Language:      req.Language,
			Sentences: []analyzer.Sentence{{
				Text:     "Hallo Berlin.",
				Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, StartOffset: 0, EndOffset: 13},
				Tokens: []analyzer.Token{{Surface: "Hallo", RawLemma: "hallo", CanonicalLemma: "hallo", UPOS: "INTJ", Dependency: "root", Head: 0,
					Morphology: map[string]string{}, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, StartOffset: 0, EndOffset: 5}}},
			}},
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fixture", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "german-standard-post-1996", Version: "6"},
		}, nil
	}}
	second, err := NewClient(f.pool, resumed, analyzertest.ReadyDepparseCapabilityProvider())
	require.NoError(t, err)
	require.NoError(t, second.Start(ctx))
	testutil.StopOnCleanup(t, "resumed analysis client", second.Stop)
	status, err := NewService(f.pool, second).Wait(ctx, f.owner.ID, handle.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateCompleted, status.State)
	runState, _ := f.runAndAttemptState(t, handle.RunID)
	require.Equal(t, "completed", runState)
}

// singleAttemptArgs is a non-analysis job kind on the shared client with one
// attempt, like Classical preparation and EPUB acquisition.
type singleAttemptArgs struct{}

func (singleAttemptArgs) Kind() string { return "shutdown_single_attempt_test" }

func (singleAttemptArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: ClassicalPreparationQueue, MaxAttempts: 1}
}

// interruptedDownloadWorker reports its cancellation as its own error, as a
// download or store call does, not as context.Canceled.
type interruptedDownloadWorker struct {
	river.WorkerDefaults[singleAttemptArgs]
	started chan struct{}
}

func (w *interruptedDownloadWorker) Work(ctx context.Context, _ *river.Job[singleAttemptArgs]) error {
	close(w.started)
	<-ctx.Done()
	return errors.New("download interrupted")
}

// TestSharedClientRefundsAnyJobKindStoppedByShutdown proves the shared
// client's middleware, not only the analysis worker, keeps a single-attempt
// job that StopAndCancel interrupted available with its attempt refunded.
func TestSharedClientRefundsAnyJobKindStoppedByShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	_, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	require.NoError(t, MigrateRiver(ctx, pool))
	worker := &interruptedDownloadWorker{started: make(chan struct{})}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	client, err := NewClient(pool, &analyzertest.Fake{}, analyzertest.ReadyDepparseCapabilityProvider(), workers)
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	inserted, err := client.Insert(ctx, singleAttemptArgs{}, nil)
	require.NoError(t, err)
	select {
	case <-worker.started:
	case <-time.After(20 * time.Second):
		t.Fatal("the job never started")
	}
	require.NoError(t, client.StopAndCancel(ctx))

	job, err := client.JobGet(ctx, inserted.Job.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateAvailable, job.State, "a stopped single-attempt job is not discarded")
	require.Equal(t, 0, job.Attempt, "the attempt is refunded")
}

type causeArgs struct{}

func (causeArgs) Kind() string { return "job_context_cause_test" }

type causeWorker struct {
	river.WorkerDefaults[causeArgs]
	started chan struct{}
	seen    chan context.Context
}

func (w *causeWorker) Work(ctx context.Context, _ *river.Job[causeArgs]) error {
	close(w.started)
	<-ctx.Done()
	w.seen <- ctx
	return ctx.Err()
}

// TestJobContextCauseOnStopAndCancel pins the cause River sets on a job
// context at StopAndCancel against jobstop's River stop cause, and that a
// user's JobCancel is not mistaken for it.
func TestJobContextCauseOnStopAndCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	// cancelledJobContext starts one job, applies cancelJob to it, and returns
	// the job context the worker saw cancelled.
	cancelledJobContext := func(t *testing.T, cancelJob func(client *river.Client[pgx.Tx], jobID int64)) context.Context {
		t.Helper()
		// Each case gets its own database, so a job left by one is not picked up by the next.
		_, pool := testutil.Postgres(t, ctx, persistence.Migrate)
		require.NoError(t, MigrateRiver(ctx, pool))
		worker := &causeWorker{started: make(chan struct{}), seen: make(chan context.Context, 1)}
		workers := river.NewWorkers()
		river.AddWorker(workers, worker)
		client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
			Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}}, Workers: workers,
		})
		require.NoError(t, err)
		require.NoError(t, client.Start(ctx))
		inserted, err := client.Insert(ctx, causeArgs{}, nil)
		require.NoError(t, err)
		<-worker.started
		cancelJob(client, inserted.Job.ID)
		defer func() { require.NoError(t, client.StopAndCancel(ctx)) }()
		select {
		case seen := <-worker.seen:
			return seen
		case <-time.After(10 * time.Second):
			t.Fatal("the job context was never cancelled")
			return nil
		}
	}
	t.Run("client stop", func(t *testing.T) {
		stopErr := make(chan error, 1)
		seen := cancelledJobContext(t, func(client *river.Client[pgx.Tx], _ int64) {
			go func() { stopErr <- client.StopAndCancel(ctx) }()
		})
		require.NoError(t, <-stopErr)
		require.True(t, jobstop.CancelledByClientStop(seen), "cause = %v", context.Cause(seen))
	})
	t.Run("user cancel", func(t *testing.T) {
		seen := cancelledJobContext(t, func(client *river.Client[pgx.Tx], id int64) {
			_, err := client.JobCancel(ctx, id)
			require.NoError(t, err)
		})
		require.ErrorIs(t, context.Cause(seen), river.ErrJobCancelledRemotely)
		require.False(t, jobstop.CancelledByClientStop(seen))
	})
}

// TestAnalysisCancelledByUserStillRecordsFailure keeps the user-cancel path as
// it was: cancelling a running analysis is not a server stop, so the worker
// still records the run's failure history.
func TestAnalysisCancelledByUserStillRecordsFailure(t *testing.T) {
	f := newShutdownFixture(t, "cancel")
	ctx, pool, owner, source := f.ctx, f.pool, f.owner, f.source
	started := make(chan struct{}, 1)
	blocking := &analyzertest.Fake{AnalyzeFunc: func(ctx context.Context, _ analyzer.AnalyzeRequest) (analyzer.Result, error) {
		started <- struct{}{}
		<-ctx.Done()
		return analyzer.Result{}, ctx.Err()
	}}
	client, err := NewClient(pool, blocking, analyzertest.ReadyDepparseCapabilityProvider())
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	service := NewService(pool, client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	<-started
	_, err = service.Cancel(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	testutil.Eventually(t, 20*time.Second, "the worker records the cancelled run's failure history", func() (bool, string) {
		var failures int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='analysis' AND status='failed'`, owner.ID).Scan(&failures); err != nil {
			return false, err.Error()
		}
		return failures == 1, "no failure history yet"
	})
	var runState string
	require.NoError(t, pool.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE id=$1`, handle.RunID).Scan(&runState))
	require.Equal(t, "cancelled", runState)
}
