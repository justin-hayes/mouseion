package fixtures

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestStoreMoveReadingJourneyEntryMutatesAndProtectsRevision(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.MoveReadingJourneyEntry(ctx, OwnerID, edgeBookID, 1, journey.Revision)
	if err != nil || revision != journey.Revision+1 {
		t.Fatalf("move revision=%d err=%v", revision, err)
	}
	journey, _ = store.GetReadingJourney(ctx, OwnerID)
	if journey.Entries[0].BookID != edgeBookID || journey.Entries[0].Position != 1 || journey.Entries[1].Position != 2 {
		t.Fatalf("reordered journey=%+v", journey.Entries)
	}
	if _, err = store.MoveReadingJourneyEntry(ctx, OwnerID, "fixture-empty", 1, revision-1); !errors.Is(err, persistence.ErrJourneyStale) {
		t.Fatalf("stale move error=%v", err)
	}
	if unchanged, err := store.MoveReadingJourneyEntry(ctx, OwnerID, edgeBookID, 0, revision); err != nil || unchanged != revision {
		t.Fatalf("clamped move revision=%d err=%v", unchanged, err)
	}
}

func TestStoreConcurrentJourneyMovesAcceptOnlyOneRevision(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	wait.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wait.Done()
			_, moveErr := store.MoveReadingJourneyEntry(ctx, OwnerID, edgeBookID, 1, journey.Revision)
			results <- moveErr
		}()
	}
	wait.Wait()
	close(results)
	var successes, stale int
	for moveErr := range results {
		switch {
		case moveErr == nil:
			successes++
		case errors.Is(moveErr, persistence.ErrJourneyStale):
			stale++
		default:
			t.Fatalf("concurrent move error=%v", moveErr)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent move results successes=%d stale=%d", successes, stale)
	}
}
