package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestPreparationPhasesExposeDurableBatchLifecycle(t *testing.T) {
	run := domain.PreparedDeckRun{State: domain.PreparedDeckRunTranslating}
	for _, test := range []struct {
		name     string
		progress domain.PreparedDeckRunProgress
		chunks   []domain.PreparedDeckBatchChunk
		want     string
	}{
		{name: "submission", chunks: []domain.PreparedDeckBatchChunk{{State: domain.PreparedDeckBatchPending}}, want: "submitting"},
		{name: "waiting", chunks: []domain.PreparedDeckBatchChunk{{State: domain.PreparedDeckBatchPolling}}, want: "waiting"},
		{name: "reconciling", chunks: []domain.PreparedDeckBatchChunk{{State: domain.PreparedDeckBatchReconciling}}, want: "reconciling"},
		{name: "retrying", progress: domain.PreparedDeckRunProgress{RetryingCount: 1}, want: "retrying"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := preparationPhase(domain.DeckPreparationPreparing, run, test.progress, test.chunks); got != test.want {
				t.Fatalf("phase=%q want %q", got, test.want)
			}
		})
	}
}
