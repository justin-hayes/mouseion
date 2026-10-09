package testutil

import (
	"context"
	"time"
)

// DefaultWait bounds asynchronous integration conditions that should settle
// within a few seconds once their dependencies are running.
const DefaultWait = 30 * time.Second

// shutdownTimeout bounds how long a background client may take to stop at test
// cleanup. A stuck client must fail the test rather than hang the package.
const shutdownTimeout = 10 * time.Second

type waitT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Eventually polls condition until it reports done or timeout elapses. condition
// returns a short description of what is still pending; on timeout the test
// fails naming what, along with that last description, so a missed condition is
// diagnosable without reading the test's context.
func Eventually(t waitT, timeout time.Duration, what string, condition func() (done bool, pending string)) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var last string
	for {
		done, pending := condition()
		if done {
			return
		}
		last = pending
		select {
		case <-deadline.C:
			t.Fatalf("timed out after %s waiting for %s: still pending: %s", timeout, what, last)
		case <-ticker.C:
		}
	}
}

// StopOnCleanup stops a background client when the test ends. The stop is
// bounded so a client that cannot shut down reports an error instead of
// blocking the package; River's notifier retries listener errors indefinitely
// after its context is cancelled, so waiting without a deadline can hang.
func StopOnCleanup(t cleanupT, name string, stop func(context.Context) error) {
	t.Helper()
	Cleanup(t, name, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return stop(ctx)
	})
}
