package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, test.want, preparationPhase(domain.DeckPreparationPreparing, run, test.progress, test.chunks), test.name)
		})
	}
}

func TestPreparationPhasesExposeStandardTranslationAndAssembly(t *testing.T) {
	standard := domain.PreparedDeckRun{ExecutionMode: domain.PreparedDeckExecutionStandard, State: domain.PreparedDeckRunTranslating}
	assert.Equal(t, "translating", preparationPhase(domain.DeckPreparationPreparing, standard, domain.PreparedDeckRunProgress{}, nil))
	standard.State = domain.PreparedDeckRunFinalizing
	assert.Equal(t, "assembling", preparationPhase(domain.DeckPreparationPreparing, standard, domain.PreparedDeckRunProgress{}, nil))
}

func TestPreparedDeckFailureClassUsesFailedChunkDiagnostic(t *testing.T) {
	run := domain.PreparedDeckRun{ErrorClass: "reconciliation"}
	chunks := []domain.PreparedDeckBatchChunk{
		{State: domain.PreparedDeckBatchCompleted, ErrorClass: "expired"},
		{State: domain.PreparedDeckBatchFailed, ErrorClass: "provider"},
	}
	assert.Equal(t, "provider", preparedDeckFailureClass(run, chunks))
	run.ErrorClass = "configuration"
	assert.Equal(t, "configuration", preparedDeckFailureClass(run, chunks))
}

func TestPreparedDeckRunReconciliationErrorClassIsBounded(t *testing.T) {
	for chunkClass, want := range map[string]string{
		"provider":         "provider",
		"cancelled":        "provider",
		"missing_result":   "validation",
		"duplicate_result": "validation",
		"configuration":    "configuration",
		"private detail":   "reconciliation",
	} {
		assert.Equal(t, want, preparedDeckRunReconciliationErrorClass(chunkClass), chunkClass)
	}
}
