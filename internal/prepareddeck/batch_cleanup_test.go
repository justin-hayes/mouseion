package prepareddeck

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/require"
)

// TestBatchCleanupRejectsInvalidInput pins the input guard that runs before any
// read: a nil worker or any missing worker dependency or identity must return
// ErrInvalidInput instead of touching the store.
func TestBatchCleanupRejectsInvalidInput(t *testing.T) {
	valid := BatchCleanupJobArgs{OwnerID: "owner", PreparationID: "preparation", RunID: "run", ChunkID: "chunk"}
	complete := func() *BatchCleanupWorker {
		return &BatchCleanupWorker{Store: &persistence.PostgresStore{}, Provider: terminalCleanupProvider{}}
	}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "nil worker", run: func() error {
			var nilWorker *BatchCleanupWorker
			return nilWorker.Cleanup(context.Background(), valid)
		}},
		{name: "missing store", run: func() error {
			return (&BatchCleanupWorker{Provider: terminalCleanupProvider{}}).Cleanup(context.Background(), valid)
		}},
		{name: "missing provider", run: func() error {
			return (&BatchCleanupWorker{Store: &persistence.PostgresStore{}}).Cleanup(context.Background(), valid)
		}},
		{name: "missing owner", run: func() error {
			args := valid
			args.OwnerID = " "
			return complete().Cleanup(context.Background(), args)
		}},
		{name: "missing preparation", run: func() error {
			args := valid
			args.PreparationID = ""
			return complete().Cleanup(context.Background(), args)
		}},
		{name: "missing run", run: func() error {
			args := valid
			args.RunID = ""
			return complete().Cleanup(context.Background(), args)
		}},
		{name: "missing chunk", run: func() error {
			args := valid
			args.ChunkID = ""
			return complete().Cleanup(context.Background(), args)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.run(), ErrInvalidInput)
		})
	}
}
