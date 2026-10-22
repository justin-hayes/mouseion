package sourcecheck

import (
	"context"
	"errors"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// Store is the persistence seam for checks. Reads resolve through the exact
// owner/source identity, and only a successful outcome is ever recorded.
type Store interface {
	GetSourceMaterial(ctx context.Context, owner, id string) (domain.SourceMaterial, error)
	GetCurrentExtractedSnapshot(ctx context.Context, owner, sourceID string) (domain.ExtractedSnapshot, error)
	GetSourceStructureCheck(ctx context.Context, key domain.SourceStructureCheckKey) (domain.SourceStructureCheck, bool, error)
	RecordSourceStructureCheck(ctx context.Context, check domain.SourceStructureCheck) error
	ListSourceStructureCheckCandidates(ctx context.Context, targetContractVersion int, conventionSet, afterSourceID string, limit int) ([]domain.SourceStructureCheckKey, error)
}

// JobInserter is the River insertion seam. River owns queued/running/retry/
// failure; this service only decides which exact keys deserve work.
type JobInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Service enqueues checks from the bounded startup scan and from relevant
// source-evidence changes. It never runs extraction itself and never touches a
// snapshot.
type Service struct {
	store    Store
	client   JobInserter
	target   Target
	pageSize int
}

// NewService builds a check scheduler for one configured target contract.
func NewService(store Store, client JobInserter, target Target) *Service {
	return &Service{store: store, client: client, target: target, pageSize: startupScanPageSize}
}

// EnqueueSourceStructureCheck enqueues one check after a relevant source
// evidence change, if the current snapshot is older or different enough from
// the configured target. This is also the explicit retry path after River
// exhausts its automatic attempts: a discarded job has no success record, while
// a successful exact-key check is reused and never re-run.
func (s *Service) EnqueueSourceStructureCheck(ctx context.Context, ownerID, sourceMaterialID string) error {
	if s == nil || !s.target.Enabled() {
		return nil // inert until the v2 extraction contract is configured
	}
	if s.store == nil || s.client == nil {
		return errors.New("sourcecheck: service is not configured")
	}
	_, current, ok, err := loadEvidence(ctx, s.store, ownerID, sourceMaterialID)
	if err != nil || !ok || !s.target.Needs(current) {
		return err
	}
	key := domain.SourceStructureCheckKey{
		OwnerID: ownerID, SourceMaterialID: sourceMaterialID,
		ContentRevisionID: current.ContentRevisionID, SnapshotID: current.SnapshotID,
		TargetContractVersion: s.target.ContractVersion, ConventionSet: s.target.ConventionSet,
	}
	return s.enqueueKey(ctx, key)
}

// EnqueueStartupChecks walks current EPUB Book sources once, in bounded pages,
// and enqueues every exact key without a successful check. Completed exact keys
// are filtered out, so restarting does not rescan them.
func (s *Service) EnqueueStartupChecks(ctx context.Context) error {
	if s == nil || !s.target.Enabled() {
		return nil
	}
	if s.store == nil || s.client == nil {
		return errors.New("sourcecheck: service is not configured")
	}
	pageSize := s.pageSize
	if pageSize < 1 {
		pageSize = startupScanPageSize
	}
	after := "00000000-0000-0000-0000-000000000000"
	for {
		keys, err := s.store.ListSourceStructureCheckCandidates(ctx, s.target.ContractVersion, s.target.ConventionSet, after, pageSize)
		if err != nil {
			return fmt.Errorf("scan source structure check candidates: %w", err)
		}
		for _, key := range keys {
			if err := s.enqueueKey(ctx, key); err != nil {
				return err
			}
		}
		if len(keys) < pageSize {
			return nil
		}
		after = keys[len(keys)-1].SourceMaterialID
	}
}

// enqueueKey inserts one exact-key job. River's live-state uniqueness prevents
// duplicate work while a check is queued or running; after a success the durable
// record prevents another scan from offering the same key again.
func (s *Service) enqueueKey(ctx context.Context, key domain.SourceStructureCheckKey) error {
	_, found, err := s.store.GetSourceStructureCheck(ctx, key)
	if err != nil {
		return fmt.Errorf("load existing source structure check: %w", err)
	}
	if found {
		return nil
	}
	args := CheckArgs{
		OwnerID: key.OwnerID, SourceMaterialID: key.SourceMaterialID,
		ContentRevisionID: key.ContentRevisionID, SnapshotID: key.SnapshotID,
		TargetContractVersion: key.TargetContractVersion, ConventionSet: key.ConventionSet,
	}
	if _, err = s.client.Insert(ctx, args, insertOpts()); err != nil {
		return fmt.Errorf("enqueue source structure check for source %s: %w", key.SourceMaterialID, err)
	}
	return nil
}

func insertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue:       Queue,
		MaxAttempts: maxAttempts,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByState: liveCheckStates,
		},
	}
}

// liveCheckStates bound uniqueness to work that can still run. Completed and
// discarded jobs do not block an explicit retry after River exhausts attempts.
var liveCheckStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateRetryable,
	rivertype.JobStateScheduled,
}
