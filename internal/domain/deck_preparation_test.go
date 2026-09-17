package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeckPreparationStateTransitions(t *testing.T) {
	tests := []struct {
		from, to DeckPreparationState
		want     bool
	}{
		{DeckPreparationQueued, DeckPreparationPreparing, true},
		{DeckPreparationQueued, DeckPreparationCancelled, true},
		{DeckPreparationPreparing, DeckPreparationReady, true},
		{DeckPreparationPreparing, DeckPreparationFailed, true},
		{DeckPreparationPreparing, DeckPreparationCancelled, true},
		{DeckPreparationFailed, DeckPreparationQueued, true},
		{DeckPreparationCancelled, DeckPreparationQueued, true},
		{DeckPreparationReady, DeckPreparationQueued, false},
		{DeckPreparationFailed, DeckPreparationReady, false},
		{DeckPreparationQueued, DeckPreparationReady, false},
	}
	for _, tt := range tests {
		got := tt.from.CanTransitionTo(tt.to)
		assert.Equal(t, tt.want, got, "%s -> %s = %v, want %v", tt.from, tt.to, got, tt.want)
	}
}

func TestTerminalLifecycleStatesRejectCrossTransitions(t *testing.T) {
	tests := []struct {
		name string
		got  bool
	}{
		{"deck preparation ready", DeckPreparationReady.CanTransitionTo(DeckPreparationQueued)},
		{"run completed", PreparedDeckRunCompleted.CanTransitionTo(PreparedDeckRunFailed)},
		{"run failed", PreparedDeckRunFailed.CanTransitionTo(PreparedDeckRunTranslating)},
		{"run cancelled", PreparedDeckRunCancelled.CanTransitionTo(PreparedDeckRunTranslating)},
		{"translation completed", PreparedDeckTranslationCompleted.CanTransitionTo(PreparedDeckTranslationRunning)},
		{"translation failed", PreparedDeckTranslationFailed.CanTransitionTo(PreparedDeckTranslationPending)},
		{"translation cancelled", PreparedDeckTranslationCancelled.CanTransitionTo(PreparedDeckTranslationPending)},
		{"outcome completed", PreparedDeckOutcomeCompleted.CanTransitionTo(PreparedDeckOutcomeFailed)},
		{"outcome failed", PreparedDeckOutcomeFailed.CanTransitionTo(PreparedDeckOutcomePending)},
		{"outcome cancelled", PreparedDeckOutcomeCancelled.CanTransitionTo(PreparedDeckOutcomePending)},
		{"batch completed", PreparedDeckBatchCompleted.CanTransitionTo(PreparedDeckBatchFailed)},
		{"batch failed", PreparedDeckBatchFailed.CanTransitionTo(PreparedDeckBatchPending)},
		{"batch cancelled", PreparedDeckBatchCancelled.CanTransitionTo(PreparedDeckBatchPending)},
		{"batch ambiguous", PreparedDeckBatchAmbiguous.CanTransitionTo(PreparedDeckBatchPending)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, tt.got, "terminal lifecycle state accepted a cross-transition")
		})
	}
}

func TestVocabularyStudyStatusPrioritizesTerminalFacts(t *testing.T) {
	now := time.Now()
	assert.Equal(t, VocabularyStudyNotStarted, (DeckPreparation{}).VocabularyStudyStatus())
	assert.Equal(t, VocabularyStudyReleased, (DeckPreparation{ReleasedAt: &now}).VocabularyStudyStatus())
	assert.Equal(t, VocabularyStudyStudying, (DeckPreparation{StudyingAt: &now}).VocabularyStudyStatus())
	assert.Equal(t, VocabularyStudyReviewed, (DeckPreparation{StudyingAt: &now, GraduatedAt: &now}).VocabularyStudyStatus())
}
