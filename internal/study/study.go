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

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
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

// EnqueueStudyPreparation brings the learner's Study material up to date with the
// stream: the first domain.StudyPreparationWindow words in stream order that
// are not in study and not unpreparable each get a pending record and a durable job. A word
// whose material is ready is left alone, and one whose job is still live is not
// enqueued twice. It is safe to call whenever Today is opened, a batch is taken
// or the Study day rolls over.
func (s *Service) EnqueueStudyPreparation(ctx context.Context, owner, language string) error {
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
	current := make(map[domain.StudyIdentity]domain.StudyMaterial, len(material))
	states := make(map[domain.StudyIdentity]domain.StudyMaterialState, len(material))
	for _, item := range material {
		current[item.StudyIdentity] = item
		states[item.StudyIdentity] = item.State
	}
	for _, target := range domain.StudyPreparationTargets(domain.OrderStudyStream(domain.WithoutStudyProgress(candidates, progress)), states) {
		if existing, ok := current[target.StudyIdentity]; ok && existing.State == domain.StudyMaterialReady {
			continue
		}
		if err := s.enqueueWord(ctx, owner, target.StudyIdentity, current[target.StudyIdentity].ID); err != nil {
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
