package fixtures

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

var fixtureCatalogueSyncJobIDs = map[string]int64{
	"fixture-connection":              101,
	"fixture-failed-connection":       102,
	"fixture-syncing-connection":      103,
	"fixture-never-synced-connection": 104,
	"fixture-browser-sync-connection": 105,
}

var fixtureCatalogueEntries = map[string]opds.Entry{
	"fixture-connection": {ID: "fixture-entry", Title: "Metadata-only migration book"},
}

const fixtureCatalogueLanguage = "de"

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
	status := domain.CatalogueSyncStatus{OwnerID: owner, ConnectionID: connectionID, State: domain.CatalogueSyncSyncing, UpdatedAt: fixtureJourneyTime}
	if connectionID == "fixture-connection" {
		s.Store.arriveNextFixtureStudyLanguage()
		status.State = domain.CatalogueSyncSynced
		status.LastSyncedAt = timePtr(fixtureJourneyTime)
		status.LastUpsertedCount = 3
	}
	if err := s.Store.SetCatalogueSyncStatus(ctx, status); err != nil {
		return cataloguesync.Handle{}, err
	}
	s.Store.admitFixtureCatalogueLanguage(owner, connectionID)
	return cataloguesync.Handle{ID: fixtureCatalogueSyncJobIDs[connectionID], DisplayNumber: fixtureCatalogueSyncJobIDs[connectionID]}, nil
}

func (s *CatalogueSync) FindAcquisitionTarget(ctx context.Context, owner, bookID string) (cataloguesync.AcquisitionTarget, error) {
	book, err := s.Store.GetBook(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return cataloguesync.AcquisitionTarget{}, cataloguesync.ErrNotFound
	}
	if err != nil {
		return cataloguesync.AcquisitionTarget{}, err
	}
	alias, err := s.Store.GetBookCatalogEntryAlias(ctx, owner, book.ID)
	if errors.Is(err, persistence.ErrNotFound) {
		return cataloguesync.AcquisitionTarget{}, cataloguesync.ErrNotFound
	}
	if err != nil || alias.BookID != book.ID || alias.AliasType != domain.AliasCatalogEntry || alias.Namespace != domain.NamespaceSourceIdentifier || strings.TrimSpace(alias.Value) == "" {
		if err != nil {
			return cataloguesync.AcquisitionTarget{}, err
		}
		return cataloguesync.AcquisitionTarget{}, cataloguesync.ErrNotFound
	}
	entry, err := s.entryForAlias(ctx, owner, alias)
	if err != nil {
		return cataloguesync.AcquisitionTarget{}, cataloguesync.ErrNotFound
	}
	return cataloguesync.AcquisitionTarget{ConnectionID: alias.ConnectionID, Language: book.LanguageTag, Entry: entry, Href: "https://fixture.invalid/book.epub"}, nil
}

// RefreshEntry mirrors the fixture catalogue feed without adding a reconcile
// implementation that the fixture server does not otherwise use.
func (s *CatalogueSync) RefreshEntry(ctx context.Context, owner, bookID string) (cataloguesync.RefreshResult, error) {
	book, err := s.Store.GetBook(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return cataloguesync.RefreshResult{}, cataloguesync.ErrNotFound
	}
	if err != nil {
		return cataloguesync.RefreshResult{Failed: true}, err
	}
	alias, err := s.Store.GetBookCatalogEntryAlias(ctx, owner, book.ID)
	if errors.Is(err, persistence.ErrNotFound) {
		return cataloguesync.RefreshResult{}, cataloguesync.ErrNotFound
	}
	if err != nil {
		return cataloguesync.RefreshResult{Book: book, Failed: true}, err
	}
	if alias.ConnectionID == "" || strings.TrimSpace(alias.Value) == "" {
		return cataloguesync.RefreshResult{}, cataloguesync.ErrNotFound
	}
	entry, err := s.entryForAlias(ctx, owner, alias)
	if err != nil {
		if errors.Is(err, cataloguesync.ErrConnectionNotFound) {
			return cataloguesync.RefreshResult{Book: book, Failed: true}, err
		}
		return cataloguesync.RefreshResult{Book: book, Missing: true}, nil
	}
	entryTitle := entry.Title
	if book.Title == entryTitle {
		return cataloguesync.RefreshResult{Book: book}, nil
	}
	updated, err := s.Store.UpdateBookMetadata(ctx, owner, book.ID, entryTitle, book.LanguageState, book.LanguageTag)
	if err != nil {
		return cataloguesync.RefreshResult{Book: book, Failed: true}, err
	}
	return cataloguesync.RefreshResult{Book: updated, Updated: true}, nil
}

func (s *CatalogueSync) entryForAlias(ctx context.Context, owner string, alias domain.BookAlias) (opds.Entry, error) {
	if _, err := s.Store.GetOpdsConnection(ctx, owner, alias.ConnectionID); err != nil {
		return opds.Entry{}, cataloguesync.ErrConnectionNotFound
	}
	entry, ok := fixtureCatalogueEntries[alias.ConnectionID]
	if !ok || entry.ID != alias.Value {
		return opds.Entry{}, cataloguesync.ErrNotFound
	}
	return entry, nil
}

func (s *CatalogueSync) List(ctx context.Context, owner string) ([]cataloguesync.Status, error) {
	statuses, err := s.Store.ListCatalogueSyncStatuses(ctx, owner)
	if err != nil {
		return nil, err
	}
	result := make([]cataloguesync.Status, 0, len(statuses))
	for _, durable := range statuses {
		status, err := s.jobStatus(ctx, durable)
		if err != nil {
			return nil, err
		}
		result = append(result, status)
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
		if err := s.Store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner, ConnectionID: status.ConnectionID, State: domain.CatalogueSyncFailed, LastError: "Catalog sync cancelled before completion. Retry when ready.", UpdatedAt: fixtureJourneyTime}); err != nil {
			return cataloguesync.Status{}, err
		}
	}
	return s.Get(ctx, owner, id)
}

func (s *CatalogueSync) jobStatus(ctx context.Context, durable domain.CatalogueSyncStatus) (cataloguesync.Status, error) {
	connection, err := s.Store.GetOpdsConnection(ctx, durable.OwnerID, durable.ConnectionID)
	if err != nil {
		return cataloguesync.Status{}, err
	}
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
	return status, nil
}

func timePtr(value time.Time) *time.Time { return &value }
