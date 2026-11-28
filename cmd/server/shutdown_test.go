package main

import (
	"context"
	"errors"
	"fmt"
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
	status := resp.StatusCode
	return status, resp.Body.Close()
}

func startTestServer(t *testing.T, ctx context.Context) (url string, done <-chan error) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := newHTTPServer(listener.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	result := make(chan error, 1)
	go func() { result <- serveUntilShutdown(ctx, srv, listener) }()
	return "http://" + listener.Addr().String(), result
}

func TestServeUntilShutdownDrainsTheServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url, done := startTestServer(t, ctx)
	require.Eventually(t, func() bool {
		status, err := probe(url)
		return err == nil && status == http.StatusNoContent
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	var err error
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	t.Run("a signal-initiated shutdown is not an error", func(t *testing.T) {
		require.NoError(t, err)
	})
	t.Run("the listener is closed", func(t *testing.T) {
		_, probeErr := probe(url)
		require.Error(t, probeErr)
	})
}

func TestServeUntilShutdownReportsAServeFailure(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, listener.Close())
	err = serveUntilShutdown(context.Background(), newHTTPServer(listener.Addr().String(), http.NotFoundHandler()), listener)
	require.ErrorIs(t, err, net.ErrClosed)
}

func TestStopRiverClientStopsInStages(t *testing.T) {
	t.Run("jobs that finish in time are only stopped", func(t *testing.T) {
		workers := &fakeStopper{}
		var err error
		stopRiverClient(workers, &err)
		require.NoError(t, err)
		require.Equal(t, []string{"stop"}, workers.calls)
		require.LessOrEqual(t, workers.softLeft, riverSoftStopTimeout)
	})
	t.Run("jobs that outlast the soft stop are cancelled", func(t *testing.T) {
		workers := &fakeStopper{softErr: context.DeadlineExceeded}
		var err error
		stopRiverClient(workers, &err)
		require.NoError(t, err)
		require.Equal(t, []string{"stop", "stopAndCancel"}, workers.calls)
	})
	t.Run("a hard stop failure is reported", func(t *testing.T) {
		workers := &fakeStopper{softErr: context.DeadlineExceeded, hardErr: errors.New("stuck")}
		var err error
		stopRiverClient(workers, &err)
		require.ErrorContains(t, err, "stuck")
	})
	t.Run("a stop failure does not mask a startup failure", func(t *testing.T) {
		startupErr := errors.New("startup failed")
		workers := &fakeStopper{softErr: context.DeadlineExceeded, hardErr: errors.New("stuck")}
		err := startupErr
		stopRiverClient(workers, &err)
		require.ErrorIs(t, err, startupErr)
		require.ErrorContains(t, err, "stuck")
	})
}

func TestShutdownDeadlinesFitComposeGracePeriod(t *testing.T) {
	total := httpShutdownTimeout + riverSoftStopTimeout + riverHardStopTimeout
	require.LessOrEqual(t, total, 8*time.Second, "leave room for closes inside Compose's 10 s stop grace period")
}

func TestStartupResult(t *testing.T) {
	signalled, signal := context.WithCancel(context.Background())
	signal()
	unsignalled := context.Background()
	failure := errors.New("migrate failed")
	tests := []struct {
		name      string
		signalCtx context.Context
		err       error
		want      error
	}{
		{"a step cancelled by a signal is a clean stop", signalled, fmt.Errorf("reconcile: %w", context.Canceled), nil},
		{"a cancellation with no signal is a failure", unsignalled, context.Canceled, context.Canceled},
		{"another failure after a signal is still a failure", signalled, failure, failure},
		{"success stays success", unsignalled, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := startupResult(tc.signalCtx, tc.err)
			if tc.want == nil {
				require.NoError(t, got)
				return
			}
			require.ErrorIs(t, got, tc.want)
		})
	}
}
