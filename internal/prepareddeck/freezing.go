package prepareddeck

import (
	"context"
	"errors"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
)

type JobArgs struct {
	// PreparationID is the logical idempotency key. The other fields are
	// immutable worker input and must not create competing jobs for the same
	// preparation.
	PreparationID    string `json:"preparation_id" river:"unique"`
	OwnerID          string `json:"owner_id"`
	SourceMaterialID string `json:"source_material_id"`
	ContentHash      string `json:"content_hash"`
	AnalysisRunID    string `json:"analysis_run_id,omitempty"`
}

func (JobArgs) Kind() string { return "prepared_deck" }

// Worker owns only the short freeze operation. Provider calls are performed
// later by the durable Batch workers.
type Worker struct {
	river.WorkerDefaults[JobArgs]
	Coordinator *DurableCoordinator
	Store       *persistence.PostgresStore
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if w == nil || w.Coordinator == nil || w.Store == nil || job == nil {
		return ErrInvalidInput
	}
	a := job.Args
	preparation, err := w.Store.ClaimDeckPreparation(ctx, a.OwnerID, a.PreparationID)
	if errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.AnalysisRunID == "" {
		source, sourceErr := w.Store.GetSourceMaterial(ctx, a.OwnerID, a.SourceMaterialID)
		if sourceErr != nil {
			return w.fail(ctx, a, fmt.Errorf("load source material: %w", sourceErr))
		}
		if source.ContentHash != a.ContentHash {
			return w.fail(ctx, a, errors.New("source material identity changed"))
		}
	}
	if preparation.SourceMaterialID != a.SourceMaterialID || preparation.ContentHash != a.ContentHash || preparation.AnalysisRunID != a.AnalysisRunID {
		return w.fail(ctx, a, errors.New("source material identity changed"))
	}
	_, err = w.Coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: a.OwnerID, PreparationID: a.PreparationID})
	if err == nil || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	return w.fail(ctx, a, err)
}

func (w *Worker) fail(ctx context.Context, args JobArgs, cause error) error {
	_, failErr := w.Store.FailDeckPreparation(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, cause.Error())
	if failErr != nil && !errors.Is(failErr, persistence.ErrInvalidTransition) {
		return fmt.Errorf("prepare durable run: %w; mark preparation failed: %w", cause, failErr)
	}
	return nil
}

func AddPreparedDeckWorker(workers *river.Workers, store *persistence.PostgresStore, presentation *cardexport.Presentation, client riverClient, codec *enrichment.TranslationCodec, batchConfig BatchConfig, preparedConfig PreparedDeckConfig, externalEnabled bool) {
	AddPreparedDeckWorkerWithAssessments(workers, store, presentation, client, codec, batchConfig, preparedConfig, externalEnabled, enrichment.ProviderAssessments())
}

// AddPreparedDeckWorkerWithAssessments registers the freeze worker with an
// explicit set of recorded personal-use payload assessments. Production uses
// the compiled reviewed set through AddPreparedDeckWorker; a caller installs a
// recorded assessment here to exercise a gated payload.
func AddPreparedDeckWorkerWithAssessments(workers *river.Workers, store *persistence.PostgresStore, presentation *cardexport.Presentation, client riverClient, codec *enrichment.TranslationCodec, batchConfig BatchConfig, preparedConfig PreparedDeckConfig, externalEnabled bool, assessments enrichment.ProviderAssessmentSet) {
	planner := NewPreparedDeckPlanner(NewInputAssembler(store), presentation, codec, externalEnabled, batchConfig, preparedConfig)
	if setter, ok := planner.(interface {
		SetAssessments(enrichment.ProviderAssessmentSet)
	}); ok {
		setter.SetAssessments(assessments)
	}
	river.AddWorker(workers, &Worker{Coordinator: NewDurableCoordinator(store, client, planner), Store: store})
}

// SetAssessments installs the recorded personal-use payload assessments the
// assessed payloads are gated against.
func (p *BatchPlanner) SetAssessments(assessments enrichment.ProviderAssessmentSet) {
	if p != nil {
		p.Assessments = assessments
	}
}
