package prepareddeck

import (
	"context"
	"errors"

	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
)

// CustomDeckPreparationJobArgs stays registered so jobs submitted before Custom
// decks were retired (ADR 0085) are still recognized by River.
type CustomDeckPreparationJobArgs struct {
	OwnerID, PreparationID string
}

func (CustomDeckPreparationJobArgs) Kind() string { return "custom_deck_preparation" }

const customDeckRetiredMessage = "Custom decks were retired; this preparation was not generated."

// CustomDeckPreparationWorker drains preparations submitted before the cutover.
// It loads no evidence and generates no cards.
type CustomDeckPreparationWorker struct {
	river.WorkerDefaults[CustomDeckPreparationJobArgs]
	Store *persistence.PostgresStore
}

func (w *CustomDeckPreparationWorker) Work(ctx context.Context, job *river.Job[CustomDeckPreparationJobArgs]) error {
	if w == nil || job == nil || w.Store == nil {
		return ErrInvalidInput
	}
	err := w.Store.FailCustomDeckPreparation(ctx, job.Args.OwnerID, job.Args.PreparationID, customDeckRetiredMessage)
	if errors.Is(err, persistence.ErrInvalidTransition) {
		// The preparation already reached a terminal state, or no longer exists.
		return nil
	}
	return err
}

func AddCustomDeckPreparationWorker(workers *river.Workers, store *persistence.PostgresStore) {
	river.AddWorker(workers, &CustomDeckPreparationWorker{Store: store})
}
