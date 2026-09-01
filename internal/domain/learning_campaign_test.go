package domain

import "testing"

func TestDeriveCampaignStatus(t *testing.T) {
	tests := []struct {
		book BookProgress
		deck DeckProgress
		want CampaignStatus
	}{
		{BookQueued, DeckQueued, CampaignQueued},
		{BookQueued, DeckStudying, CampaignActive},
		{BookQueued, DeckReviewed, CampaignActive},
		{BookReading, DeckQueued, CampaignActive},
		{BookReading, DeckStudying, CampaignActive},
		{BookFinished, DeckStudying, CampaignActive},
		{BookFinished, DeckQueued, CampaignActive},
		{BookReading, DeckReviewed, CampaignActive},
		{BookFinished, DeckReviewed, CampaignComplete},
		{BookAbandoned, DeckReviewed, CampaignAbandoned},
		{BookAbandoned, DeckQueued, CampaignAbandoned},
		{BookFinished, DeckAbandoned, CampaignAbandoned},
		{BookQueued, DeckAbandoned, CampaignAbandoned},
	}
	for _, tt := range tests {
		if got := DeriveCampaignStatus(tt.book, tt.deck); got != tt.want {
			t.Errorf("DeriveCampaignStatus(%q, %q) = %q, want %q", tt.book, tt.deck, got, tt.want)
		}
	}
}

func TestCampaignProgressTransitions(t *testing.T) {
	bookTransitions := map[BookProgress][]BookProgress{
		BookQueued:    {BookQueued, BookReading, BookFinished, BookAbandoned},
		BookReading:   {BookReading, BookFinished, BookAbandoned},
		BookFinished:  {BookFinished},
		BookAbandoned: {BookAbandoned},
	}
	for current, allowed := range bookTransitions {
		for _, next := range []BookProgress{BookQueued, BookReading, BookFinished, BookAbandoned} {
			want := containsBookProgress(allowed, next)
			if got := current.CanTransitionTo(next); got != want {
				t.Errorf("book transition %q -> %q = %t, want %t", current, next, got, want)
			}
		}
	}

	deckTransitions := map[DeckProgress][]DeckProgress{
		DeckQueued:    {DeckQueued, DeckStudying, DeckReviewed, DeckAbandoned},
		DeckStudying:  {DeckStudying, DeckReviewed, DeckAbandoned},
		DeckReviewed:  {DeckReviewed},
		DeckAbandoned: {DeckAbandoned},
	}
	for current, allowed := range deckTransitions {
		for _, next := range []DeckProgress{DeckQueued, DeckStudying, DeckReviewed, DeckAbandoned} {
			want := containsDeckProgress(allowed, next)
			if got := current.CanTransitionTo(next); got != want {
				t.Errorf("deck transition %q -> %q = %t, want %t", current, next, got, want)
			}
		}
	}
}

func containsBookProgress(values []BookProgress, wanted BookProgress) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsDeckProgress(values []DeckProgress, wanted DeckProgress) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
