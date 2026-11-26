package study

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePlanner struct {
	owner, language string
	err             error
}

func (p *fakePlanner) PlanPreparation(_ context.Context, owner, language string) error {
	p.owner, p.language = owner, language
	return p.err
}

func TestPlanWorkerPlansTheJobsLearnerAndLanguage(t *testing.T) {
	planner := &fakePlanner{}
	worker := &PlanWorker{Planner: planner}

	require.NoError(t, worker.Work(context.Background(), &river.Job[PlanPreparationArgs]{Args: PlanPreparationArgs{OwnerID: "owner", Language: "de"}}))

	t.Run("plans the job's learner", func(t *testing.T) { assert.Equal(t, "owner", planner.owner) })
	t.Run("plans the job's language", func(t *testing.T) { assert.Equal(t, "de", planner.language) })
}

func TestPlanWorkerReturnsThePlannerErrorSoRiverRetries(t *testing.T) {
	failure := errors.New("database down")
	worker := &PlanWorker{Planner: &fakePlanner{err: failure}}

	err := worker.Work(context.Background(), &river.Job[PlanPreparationArgs]{Args: PlanPreparationArgs{OwnerID: "owner", Language: "de"}})

	assert.ErrorIs(t, err, failure)
}

func TestPlanWorkerWithoutAPlannerIsNotConfigured(t *testing.T) {
	err := (&PlanWorker{}).Work(context.Background(), &river.Job[PlanPreparationArgs]{})

	assert.Error(t, err)
}
