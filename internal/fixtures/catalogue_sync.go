package fixtures

import (
	"context"
	"time"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
)

var fixtureCatalogueSyncJobIDs = map[string]int64{
	"fixture-connection":              101,
	"fixture-failed-connection":       102,
	"fixture-syncing-connection":      103,
	"fixture-never-synced-connection": 104,
}

// CatalogueSync is a small in-memory implementation of the wider sync seam
// used by the webapp. It gives the browser fixture deterministic connection
// states and exercises the same handler contracts as the River-backed service.
type CatalogueSync struct {
	Store *Store
}

func NewCatalogueSync(store *Store) *CatalogueSync { return &CatalogueSync{Store: store} }

func (s *CatalogueSync) RegisterConnection(context.Context, string, string) error { return nil }
func (s *CatalogueSync) UnregisterConnection(string, string) error                { return nil }

func (s *CatalogueSync) Enqueue(ctx context.Context, owner, connectionID string) (cataloguesync.Handle, error) {
	if _, err := s.Store.GetOpdsConnection(ctx, owner, connectionID); err != nil {
		return cataloguesync.Handle{}, cataloguesync.ErrNotFound
	}
	s.Store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner, ConnectionID: connectionID, State: domain.CatalogueSyncSyncing, UpdatedAt: fixtureJourneyTime})
	return cataloguesync.Handle{ID: fixtureCatalogueSyncJobIDs[connectionID], DisplayNumber: fixtureCatalogueSyncJobIDs[connectionID]}, nil
}

func (s *CatalogueSync) FindAcquisitionTarget(ctx context.Context, owner, bookID string) (cataloguesync.AcquisitionTarget, error) {
	if owner != OwnerID || bookID != "fixture-metadata-only" {
		return cataloguesync.AcquisitionTarget{}, cataloguesync.ErrNotFound
	}
	return cataloguesync.AcquisitionTarget{ConnectionID: "fixture-connection", Language: "de", Entry: opds.Entry{ID: "fixture-entry", Title: "Metadata-only migration book"}, Href: "https://fixture.invalid/book.epub"}, nil
}

func (s *CatalogueSync) List(ctx context.Context, owner string) ([]cataloguesync.Status, error) {
	statuses, err := s.Store.ListCatalogueSyncStatuses(ctx, owner)
	if err != nil {
		return nil, err
	}
	result := make([]cataloguesync.Status, 0, len(statuses))
	for _, durable := range statuses {
		result = append(result, s.jobStatus(ctx, durable))
	}
	return result, nil
}

func (s *CatalogueSync) Get(ctx context.Context, owner string, id int64) (cataloguesync.Status, error) {
	statuses, err := s.List(ctx, owner)
	if err != nil {
		return cataloguesync.Status{}, err
	}
	for _, status := range statuses {
		if status.ID == id {
			return status, nil
		}
	}
	return cataloguesync.Status{}, cataloguesync.ErrNotFound
}

func (s *CatalogueSync) Retry(ctx context.Context, owner string, id int64) (cataloguesync.Handle, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return cataloguesync.Handle{}, err
	}
	return s.Enqueue(ctx, owner, status.ConnectionID)
}

func (s *CatalogueSync) Cancel(ctx context.Context, owner string, id int64) (cataloguesync.Status, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return cataloguesync.Status{}, err
	}
	if status.LogicalState == "running" || status.LogicalState == "queued" {
		if err := s.Store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner, ConnectionID: status.ConnectionID, State: domain.CatalogueSyncFailed, LastError: "Catalogue sync cancelled before completion. Retry when ready.", UpdatedAt: fixtureJourneyTime}); err != nil {
			return cataloguesync.Status{}, err
		}
	}
	return s.Get(ctx, owner, id)
}

func (s *CatalogueSync) jobStatus(ctx context.Context, durable domain.CatalogueSyncStatus) cataloguesync.Status {
	connection, _ := s.Store.GetOpdsConnection(ctx, durable.OwnerID, durable.ConnectionID)
	jobID := fixtureCatalogueSyncJobIDs[durable.ConnectionID]
	createdAt := fixtureJourneyTime
	updatedAt := durable.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	status := cataloguesync.Status{ID: jobID, Attempt: 1, AttemptCount: 3, OwnerID: durable.OwnerID, ConnectionID: durable.ConnectionID, ConnectionName: connection.Name, CreatedAt: createdAt, UpdatedAt: updatedAt}
	switch durable.State {
	case domain.CatalogueSyncSynced:
		status.State, status.LogicalState, status.Progress = "completed", "completed", 100
		status.FinalizedAt = timePtr(updatedAt)
	case domain.CatalogueSyncFailed:
		status.State, status.LogicalState, status.Progress, status.Error = "discarded", "failed", 42, durable.LastError
		status.FinalizedAt = timePtr(updatedAt)
	default:
		status.State, status.LogicalState, status.Progress = "running", "running", 35
	}
	return status
}

func timePtr(value time.Time) *time.Time { return &value }
