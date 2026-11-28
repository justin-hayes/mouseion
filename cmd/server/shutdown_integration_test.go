//go:build integration

package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"
)

type blockingArgs struct{}

func (blockingArgs) Kind() string { return "shutdown_blocking_test" }

// blockingWorker runs until its context is cancelled, like a job still
// working when the server is told to stop.
type blockingWorker struct {
	river.WorkerDefaults[blockingArgs]
	started chan struct{}
}

func (w *blockingWorker) Work(ctx context.Context, _ *river.Job[blockingArgs]) error {
	close(w.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestShutdownLeavesNoJobRunning(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	worker := &blockingWorker{started: make(chan struct{})}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	inserted, err := client.Insert(ctx, blockingArgs{}, nil)
	require.NoError(t, err)
	select {
	case <-worker.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the job never started")
	}

	signalCtx, signal := context.WithCancel(ctx)
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		done <- serveUntilShutdown(signalCtx, newHTTPServer(listener.Addr().String(), http.NotFoundHandler()), listener, client)
	}()
	signal()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(httpShutdownTimeout + riverSoftStopTimeout + riverHardStopTimeout + 2*time.Second):
		t.Fatal("shutdown overran its deadlines")
	}

	var state string
	require.NoError(t, pool.QueryRow(ctx, `SELECT state::text FROM river_job WHERE id=$1`, inserted.Job.ID).Scan(&state))
	require.NotEqual(t, "running", state)
	require.Equal(t, "available", state, "a cancelled job runs again promptly")
}
