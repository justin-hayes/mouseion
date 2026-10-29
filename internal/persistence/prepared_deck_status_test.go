package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{name: "terminal chunks do not override active run", chunks: []domain.PreparedDeckBatchChunk{{State: domain.PreparedDeckBatchCompleted}}, want: "translating"},
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

func TestAppendMeaningOmissionsSeparatesLemmaMismatchFromAmbiguous(t *testing.T) {
	preparation := domain.DeckPreparation{}
	items := []cardexport.ManifestItem{
		{Ordinal: 0, Entry: cardexport.Entry{TargetWord: "sieht"}},
		{Ordinal: 1, Entry: cardexport.Entry{TargetWord: "Bank"}},
		{Ordinal: 2, Entry: cardexport.Entry{TargetWord: "λόγος"}},
	}
	outcomes := []domain.PreparedDeckTranslationOutcome{
		{Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted, OmissionReason: domain.DeckPreparationLemmaMismatchOmissionReason("The sentence uses fernsehen, not sehen.")},
		{Ordinal: 1, State: domain.PreparedDeckOutcomeCompleted, OmissionReason: "The sentence does not distinguish the meanings."},
		{Ordinal: 2, State: domain.PreparedDeckOutcomeCompleted, OmissionReason: domain.DeckPreparationEvidenceAssistedPassAbstentionReason},
	}

	appendMeaningOmissions(&preparation, outcomes, items)

	t.Run("counts only the lemma mismatch subset separately", func(t *testing.T) {
		require.Equal(t, 1, preparation.LemmaMismatchOmissions)
	})
	t.Run("labels the lemma mismatch and keeps the ambiguous reason", func(t *testing.T) {
		require.Equal(t, []domain.DeckPreparationMeaningOmission{
			{TargetWord: "sieht", Reason: domain.DeckPreparationLemmaMismatchLabel},
			{TargetWord: "Bank", Reason: "The sentence does not distinguish the meanings."},
		}, preparation.MeaningOmissions)
	})
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
