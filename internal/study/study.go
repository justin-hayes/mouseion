// Package study runs the durable Study jobs: it keeps Study material prepared
// a short way ahead of the new words Today will offer, one word at a time
// (ADR 0134 §8). It is a job package in the sense of ADR 0091 §3: a job is
// enqueued in the same transaction as the pending record it prepares
// (ADR 0010), so the package holds pgx.
package study

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

// Queue is the River queue Study material preparation runs on. The River
// client's queue map in internal/analysis must name it.
const Queue = "study"

// maxPreparationAttempts bounds River's retries of one preparation job. A word
// left pending when the job is discarded is enqueued again the next time the
// Study page is opened.
const maxPreparationAttempts = 8

// PrepareMaterialArgs identifies one pending Study material record. It carries
// no source text, sentence or Gloss.
type PrepareMaterialArgs struct {
	OwnerID    string `json:"owner_id"`
	MaterialID string `json:"material_id" river:"unique"`
}

// Kind names the job for River.
func (PrepareMaterialArgs) Kind() string { return "study_prepare_material" }

type riverClient interface {
	Insert(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Service enqueues Study material preparation. The web app reaches it only
// through the seam it defines, so no page request calls a provider.
type Service struct {
	store  *persistence.PostgresStore
	client riverClient
}

// NewService returns the preparation enqueuer over the store and River client.
func NewService(store *persistence.PostgresStore, client riverClient) *Service {
	return &Service{store: store, client: client}
}

// PlanPreparationArgs identifies the learner and Study language whose
// preparation is to be planned. It carries no source text.
type PlanPreparationArgs struct {
	OwnerID  string `json:"owner_id" river:"unique"`
	Language string `json:"language" river:"unique"`
	// Trigger is empty for a page trigger, so waiting and running plans absorb
	// it. A publication sets a value no other plan has: a running plan may
	// already have read the stream before that analysis was published, so it
	// must never absorb the publication's plan.
	Trigger string `json:"trigger,omitempty" river:"unique"`
}

// Kind names the job for River.
func (PlanPreparationArgs) Kind() string { return "study_plan_preparation" }

// planUniqueStates are River's default unique states without completed: a plan
// that has finished must not stop the next trigger from queueing another.
var planUniqueStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
	rivertype.JobStateRetryable, rivertype.JobStateScheduled,
}

// EnqueueStudyPreparation asks for the learner's Study material to be brought
// up to date with the stream. It inserts one planning job, which River does not
// queue twice while one is waiting, and reads nothing else: the planning runs in
// PlanWorker. It is safe to call whenever Today is opened or a batch is taken;
// the Study-day rollover is seen by the next page load, as before.
func (s *Service) EnqueueStudyPreparation(ctx context.Context, owner, language string) error {
	if _, err := s.client.Insert(ctx, PlanPreparationArgs{OwnerID: owner, Language: language}, planInsertOpts()); err != nil {
		return fmt.Errorf("enqueue study preparation planning: %w", err)
	}
	return nil
}

func planInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue: Queue, MaxAttempts: maxPreparationAttempts,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: planUniqueStates},
	}
}

// planInserter is the insert-only River client that queues planning from inside
// another package's transaction. It has no workers and no queues and never
// connects: InsertTx writes through the caller's transaction.
var planInserter = sync.OnceValues(func() (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(nil), &river.Config{})
})

// EnqueuePlanningTx queues Study preparation planning for the learner and
// language in the caller's transaction, so planning is durable exactly when the
// analysis it follows is published (ADR 0010). It queues nothing for a language
// Study does not serve, and River does not queue a second plan while one waits.
func EnqueuePlanningTx(ctx context.Context, tx pgx.Tx, owner, language string) error {
	if language == "" || !Available(language) {
		return nil
	}
	trigger := strconv.FormatInt(time.Now().UnixNano(), 10)
	client, err := planInserter()
	if err != nil {
		return fmt.Errorf("create study planning inserter: %w", err)
	}
	if _, err := client.InsertTx(ctx, tx, PlanPreparationArgs{OwnerID: owner, Language: language, Trigger: trigger}, planInsertOpts()); err != nil {
		return fmt.Errorf("enqueue study preparation planning: %w", err)
	}
	return nil
}

// PlanPreparation brings the learner's Study material up to date with the
// stream: the first domain.StudyPreparationWindow words in stream order that
// are not in study and not unpreparable each get a pending record and a durable job. A word
// whose material is ready is left alone, and one whose job is still live is not
// enqueued twice.
func (s *Service) PlanPreparation(ctx context.Context, owner, language string) error {
	// A publication may have repaired a reported word's occurrence; a held word
	// is never due, so Today alone would not notice.
	if err := ReconcileReports(ctx, s.store, owner, language, time.Now()); err != nil {
		return err
	}
	candidates, err := s.store.ListStudyCandidates(ctx, owner, language)
	if err != nil {
		return err
	}
	progress, err := s.store.ListStudyProgress(ctx, owner, language)
	if err != nil {
		return err
	}
	material, err := s.store.ListCurrentStudyMaterial(ctx, owner, language)
	if err != nil {
		return err
	}
	// A held word is not prepared ahead: it would crowd the window.
	reports, err := s.store.ListOpenStudyReports(ctx, owner, language, domain.StudyReportWrongWord)
	if err != nil {
		return err
	}
	current := make(map[domain.StudyIdentity]domain.StudyMaterial, len(material))
	states := make(map[domain.StudyIdentity]domain.StudyMaterialState, len(material))
	for _, item := range material {
		current[item.StudyIdentity] = item
		states[item.StudyIdentity] = item.State
	}
	for _, target := range domain.StudyPreparationTargets(domain.OrderStudyStream(domain.WithoutStudyProgress(candidates, progress, domain.StudyHeldIdentities(reports))), states) {
		if existing, ok := current[target.StudyIdentity]; ok && existing.State == domain.StudyMaterialReady {
			continue
		}
		if err := s.enqueueWord(ctx, owner, target.StudyIdentity, current[target.StudyIdentity].ID); err != nil {
			return err
		}
	}
	return s.enqueueReintroductions(ctx, owner, progress, current)
}

// enqueueReintroductions prepares new material for the words in study whose
// material a correction replaced, so a due review is re-introduced in a sentence
// that belongs to the word. A word withdrawn by a correction waits.
func (s *Service) enqueueReintroductions(ctx context.Context, owner string, progress []domain.StudyProgress, current map[domain.StudyIdentity]domain.StudyMaterial) error {
	for _, item := range progress {
		existing, ok := current[item.StudyIdentity]
		if item.Withdrawn || (ok && existing.State != domain.StudyMaterialPending) {
			continue
		}
		if err := s.enqueueWord(ctx, owner, item.StudyIdentity, existing.ID); err != nil {
			return err
		}
	}
	return nil
}

// enqueueWord reserves the word's pending record, unless existingID names one,
// and inserts its preparation job in the same transaction.
func (s *Service) enqueueWord(ctx context.Context, owner string, identity domain.StudyIdentity, existingID string) (err error) {
	tx, err := s.store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	id := existingID
	if id == "" {
		reserved, created, err := s.store.ReserveStudyMaterialTx(ctx, tx, owner, identity)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		id = reserved
	}
	if _, err := s.client.InsertTx(ctx, tx, PrepareMaterialArgs{OwnerID: owner, MaterialID: id}, &river.InsertOpts{
		Queue: Queue, MaxAttempts: maxPreparationAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true},
	}); err != nil {
		return fmt.Errorf("enqueue study material preparation: %w", err)
	}
	return tx.Commit(ctx)
}
