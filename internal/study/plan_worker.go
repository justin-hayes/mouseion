package study

import (
	"context"
	"errors"

	"github.com/riverqueue/river"
)

// Planner plans one learner's Study preparation.
type Planner interface {
	PlanPreparation(ctx context.Context, owner, language string) error
}

// PlanWorker reads the Study stream and queues the per-word preparation jobs.
// The River client the planner enqueues with exists only after the workers are
// registered, so Planner is attached afterwards. The worker calls no provider.
type PlanWorker struct {
	river.WorkerDefaults[PlanPreparationArgs]
	Planner Planner
}

// AddPlanWorker registers the planning worker and returns it so the planner can
// be attached once the River client exists.
func AddPlanWorker(workers *river.Workers) *PlanWorker {
	worker := &PlanWorker{}
	river.AddWorker(workers, worker)
	return worker
}

// Work plans the learner's preparation. An error is retried by River up to the
// job's bounded attempts; the next page load queues a fresh plan after that.
func (w *PlanWorker) Work(ctx context.Context, job *river.Job[PlanPreparationArgs]) error {
	if job == nil || w.Planner == nil {
		return errors.New("study: plan worker is not configured")
	}
	return w.Planner.PlanPreparation(ctx, job.Args.OwnerID, job.Args.Language)
}
