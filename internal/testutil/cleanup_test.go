package testutil

import (
	"errors"
	"fmt"
	"testing"
)

type cleanupTestT struct {
	cleanup func()
	errors  []string
}

var _ cleanupT = (*testing.T)(nil)

func (t *cleanupTestT) Helper() {}

func (t *cleanupTestT) Cleanup(cleanup func()) {
	t.cleanup = cleanup
}

func (t *cleanupTestT) Errorf(format string, args ...any) {
	t.errors = append(t.errors, fmt.Sprintf(format, args...))
}

func TestCleanupReportsErrorWhenCleanupFails(t *testing.T) {
	fake := &cleanupTestT{}
	want := errors.New("store is still open")

	Cleanup(fake, "store", func() error { return want })
	fake.cleanup()

	if len(fake.errors) != 1 {
		t.Fatalf("cleanup errors = %d, want 1", len(fake.errors))
	}
	if fake.errors[0] != "store cleanup failed: store is still open" {
		t.Fatalf("cleanup error = %q", fake.errors[0])
	}
}

func TestCleanupDoesNotReportSuccessfulCleanup(t *testing.T) {
	fake := &cleanupTestT{}

	Cleanup(fake, "store", func() error { return nil })
	fake.cleanup()

	if len(fake.errors) != 0 {
		t.Fatalf("cleanup errors = %d, want 0", len(fake.errors))
	}
}
