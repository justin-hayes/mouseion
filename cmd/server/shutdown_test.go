package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeStopper struct {
	softErr  error
	calls    []string
	hardErr  error
	softLeft time.Duration
}

func (f *fakeStopper) Stop(ctx context.Context) error {
	f.calls = append(f.calls, "stop")
	if deadline, ok := ctx.Deadline(); ok {
		f.softLeft = time.Until(deadline)
	}
	return f.softErr
}

func (f *fakeStopper) StopAndCancel(context.Context) error {
	f.calls = append(f.calls, "stopAndCancel")
	return f.hardErr
}

func probe(url string) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}

func startTestServer(t *testing.T, ctx context.Context, workers workerStopper) (url string, done <-chan error) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := newHTTPServer(listener.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	result := make(chan error, 1)
	go func() { result <- serveUntilShutdown(ctx, srv, listener, workers) }()
	return "http://" + listener.Addr().String(), result
}

func TestServeUntilShutdownStopsServerAndWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workers := &fakeStopper{}
	url, done := startTestServer(t, ctx, workers)
	require.Eventually(t, func() bool {
		status, err := probe(url)
		return err == nil && status == http.StatusNoContent
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err, "a signal-initiated shutdown is not an error")
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	require.Equal(t, []string{"stop"}, workers.calls)
	require.LessOrEqual(t, workers.softLeft, riverSoftStopTimeout)
	_, err := probe(url)
	require.Error(t, err)
}

func TestServeUntilShutdownCancelsJobsThatOutlastSoftStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	workers := &fakeStopper{softErr: context.DeadlineExceeded}
	_, done := startTestServer(t, ctx, workers)
	require.NoError(t, <-done)
	require.Equal(t, []string{"stop", "stopAndCancel"}, workers.calls)
}

func TestServeUntilShutdownReportsHardStopFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	workers := &fakeStopper{softErr: context.DeadlineExceeded, hardErr: errors.New("stuck")}
	_, done := startTestServer(t, ctx, workers)
	require.ErrorContains(t, <-done, "stuck")
}

func TestShutdownDeadlinesFitComposeGracePeriod(t *testing.T) {
	total := httpShutdownTimeout + riverSoftStopTimeout + riverHardStopTimeout
	require.LessOrEqual(t, total, 8*time.Second, "leave room for closes inside Compose's 10 s stop grace period")
}
