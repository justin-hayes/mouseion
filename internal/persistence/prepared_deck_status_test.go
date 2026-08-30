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

func TestPreparationPhasesExposeStandardTranslationAndAssembly(t *testing.T) {
	standard := domain.PreparedDeckRun{ExecutionMode: domain.PreparedDeckExecutionStandard, State: domain.PreparedDeckRunTranslating}
	if got := preparationPhase(domain.DeckPreparationPreparing, standard, domain.PreparedDeckRunProgress{}, nil); got != "translating" {
		t.Fatalf("standard translating phase=%q", got)
	}
	standard.State = domain.PreparedDeckRunFinalizing
	if got := preparationPhase(domain.DeckPreparationPreparing, standard, domain.PreparedDeckRunProgress{}, nil); got != "assembling" {
		t.Fatalf("standard assembling phase=%q", got)
	}
}

func TestPreparedDeckFailureClassUsesFailedChunkDiagnostic(t *testing.T) {
	run := domain.PreparedDeckRun{ErrorClass: "reconciliation"}
	chunks := []domain.PreparedDeckBatchChunk{
		{State: domain.PreparedDeckBatchCompleted, ErrorClass: "expired"},
		{State: domain.PreparedDeckBatchFailed, ErrorClass: "provider"},
	}
	if got := preparedDeckFailureClass(run, chunks); got != "provider" {
		t.Fatalf("failure class=%q want provider", got)
	}
	run.ErrorClass = "configuration"
	if got := preparedDeckFailureClass(run, chunks); got != "configuration" {
		t.Fatalf("specific run failure class was replaced: %q", got)
	}
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
		if got := preparedDeckRunReconciliationErrorClass(chunkClass); got != want {
			t.Errorf("chunk class %q mapped to %q want %q", chunkClass, got, want)
		}
	}
}
