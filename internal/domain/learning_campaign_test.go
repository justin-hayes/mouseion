package domain

import "testing"

func TestDeriveCampaignStatus(t *testing.T) {
	tests := []struct {
		book BookProgress
		deck DeckProgress
		want CampaignStatus
	}{
		{BookQueued, DeckQueued, CampaignQueued},
		{BookReading, DeckQueued, CampaignActive},
		{BookFinished, DeckStudying, CampaignActive},
		{BookFinished, DeckReviewed, CampaignComplete},
		{BookAbandoned, DeckReviewed, CampaignAbandoned},
		{BookFinished, DeckAbandoned, CampaignAbandoned},
	}
	for _, tt := range tests {
		if got := DeriveCampaignStatus(tt.book, tt.deck); got != tt.want {
			t.Errorf("DeriveCampaignStatus(%q, %q) = %q, want %q", tt.book, tt.deck, got, tt.want)
		}
	}
}

func TestCampaignProgressTransitions(t *testing.T) {
	if !BookQueued.CanTransitionTo(BookReading) || !BookReading.CanTransitionTo(BookFinished) || BookFinished.CanTransitionTo(BookReading) {
		t.Fatal("book transition rules are incorrect")
	}
	if !DeckQueued.CanTransitionTo(DeckStudying) || !DeckStudying.CanTransitionTo(DeckReviewed) || DeckReviewed.CanTransitionTo(DeckStudying) {
		t.Fatal("deck transition rules are incorrect")
	}
}
