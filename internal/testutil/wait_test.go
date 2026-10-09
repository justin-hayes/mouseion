package testutil

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

var _ waitT = (*testing.T)(nil)

// waitTestT records a failure and unwinds like testing.T.FailNow, so Eventually
// never continues past its deadline in the fake.
type waitTestT struct {
	failure string
}

type waitTestFailed struct{}

func (t *waitTestT) Helper() {}

func (t *waitTestT) Fatalf(format string, args ...any) {
	t.failure = fmt.Sprintf(format, args...)
	panic(waitTestFailed{})
}

func runEventually(t waitT, timeout time.Duration, what string, condition func() (bool, string)) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(waitTestFailed); !ok {
				panic(recovered)
			}
		}
	}()
	Eventually(t, timeout, what, condition)
}

func TestEventuallyReturnsWhenConditionIsDone(t *testing.T) {
	fake := &waitTestT{}
	calls := 0
	runEventually(fake, time.Second, "job completion", func() (bool, string) {
		calls++
		return calls == 3, fmt.Sprintf("attempt %d", calls)
	})
	if fake.failure != "" {
		t.Fatalf("unexpected failure: %s", fake.failure)
	}
	if calls != 3 {
		t.Fatalf("condition calls = %d, want 3", calls)
	}
}

func TestEventuallyNamesLastPendingConditionAtDeadline(t *testing.T) {
	fake := &waitTestT{}
	start := time.Now()
	runEventually(fake, 50*time.Millisecond, "known vocabulary job 7", func() (bool, string) {
		return false, "state running"
	})
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Eventually ran for %s, want a bounded deadline", elapsed)
	}
	for _, want := range []string{"known vocabulary job 7", "state running", "50ms"} {
		if !strings.Contains(fake.failure, want) {
			t.Fatalf("failure %q does not contain %q", fake.failure, want)
		}
	}
}

func TestStopOnCleanupReportsStopError(t *testing.T) {
	fake := &cleanupTestT{}
	stopped := false
	StopOnCleanup(fake, "river client", func(ctx context.Context) error {
		stopped = true
		if _, ok := ctx.Deadline(); !ok {
			t.Error("stop context has no deadline")
		}
		return context.DeadlineExceeded
	})
	fake.cleanup()
	if !stopped {
		t.Fatal("stop was not called")
	}
	if len(fake.errors) != 1 || !strings.Contains(fake.errors[0], "river client cleanup failed") {
		t.Fatalf("errors = %v, want one river client cleanup failure", fake.errors)
	}
}
