package domain

import (
	"testing"
	"time"
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
		if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
			t.Errorf("%s -> %s = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestVocabularyStudyStatusPrioritizesTerminalFacts(t *testing.T) {
	if got := (DeckPreparation{}).VocabularyStudyStatus(); got != VocabularyStudyNotStarted {
		t.Fatalf("empty study status=%q", got)
	}
	if got := (DeckPreparation{ReleasedAt: timePtr()}).VocabularyStudyStatus(); got != VocabularyStudyReleased {
		t.Fatalf("released study status=%q", got)
	}
	if got := (DeckPreparation{StudyingAt: timePtr()}).VocabularyStudyStatus(); got != VocabularyStudyStudying {
		t.Fatalf("studying status=%q", got)
	}
	if got := (DeckPreparation{StudyingAt: timePtr(), GraduatedAt: timePtr()}).VocabularyStudyStatus(); got != VocabularyStudyReviewed {
		t.Fatalf("graduated status=%q", got)
	}
}

func timePtr() *time.Time {
	now := time.Now()
	return &now
}
