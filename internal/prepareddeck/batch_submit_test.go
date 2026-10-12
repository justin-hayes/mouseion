package prepareddeck

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBatchSubmitRejectsInvalidInput pins the input guard that runs before any
// read: a nil worker or any missing worker dependency, identity, or generation
// must return ErrInvalidInput instead of touching the store.
func TestBatchSubmitRejectsInvalidInput(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	valid := BatchSubmitJobArgs{OwnerID: "owner", PreparationID: "preparation", RunID: "run", ChunkID: "chunk", Generation: 1}
	worker := &BatchSubmitWorker{Store: &persistence.PostgresStore{}, Provider: &batchListProviderStub{}, Codec: codec}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "nil worker", run: func() error {
			var nilWorker *BatchSubmitWorker
			return nilWorker.Submit(context.Background(), valid)
		}},
		{name: "missing store", run: func() error {
			return (&BatchSubmitWorker{Provider: &batchListProviderStub{}, Codec: codec}).Submit(context.Background(), valid)
		}},
		{name: "missing provider", run: func() error {
			return (&BatchSubmitWorker{Store: worker.Store, Codec: codec}).Submit(context.Background(), valid)
		}},
		{name: "missing codec", run: func() error {
			return (&BatchSubmitWorker{Store: worker.Store, Provider: &batchListProviderStub{}}).Submit(context.Background(), valid)
		}},
		{name: "missing owner", run: func() error {
			args := valid
			args.OwnerID = " "
			return worker.Submit(context.Background(), args)
		}},
		{name: "missing preparation", run: func() error {
			args := valid
			args.PreparationID = ""
			return worker.Submit(context.Background(), args)
		}},
		{name: "missing run", run: func() error {
			args := valid
			args.RunID = ""
			return worker.Submit(context.Background(), args)
		}},
		{name: "missing chunk", run: func() error {
			args := valid
			args.ChunkID = ""
			return worker.Submit(context.Background(), args)
		}},
		{name: "missing generation", run: func() error {
			args := valid
			args.Generation = 0
			return worker.Submit(context.Background(), args)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.ErrorIs(t, test.run(), ErrInvalidInput)
		})
	}
}
