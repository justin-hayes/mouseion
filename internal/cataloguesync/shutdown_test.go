package cataloguesync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"
)

// stoppedContext is a job context as River leaves it when the client stops:
// cancelled with River's stop cause.
func stoppedContext() context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("stop initiated"))
	return ctx
}

// userCancelledContext is a job context a learner's cancel left behind.
func userCancelledContext() context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(river.ErrJobCancelledRemotely)
	return ctx
}

func failedStatuses(statuses []domain.CatalogueSyncStatus) int {
	failed := 0
	for _, status := range statuses {
		if status.State == domain.CatalogueSyncFailed {
			failed++
		}
	}
	return failed
}

func TestSyncWorkerStoppedForShutdownRecordsNoFailure(t *testing.T) {
	run := func(ctx context.Context) (*refreshStore, error) {
		store := &refreshStore{connections: []domain.OpdsConnection{{ID: "connection-1", OwnerID: "alice"}}}
		reader := &refreshReader{browseErr: errors.New("feed unavailable")}
		capabilities := stubCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
		err := worker.Work(ctx, &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: "alice", ConnectionID: "connection-1"}})
		return store, err
	}
	t.Run("a shutdown writes no failed status", func(t *testing.T) {
		store, err := run(stoppedContext())
		require.Error(t, err)
		require.Zero(t, failedStatuses(store.statuses))
	})
	t.Run("a user cancel still writes a failed status", func(t *testing.T) {
		store, err := run(userCancelledContext())
		require.Error(t, err)
		require.Equal(t, 1, failedStatuses(store.statuses))
	})
}

// coverStore is a refreshStore that also answers the cover retrieval reads
// and records whether a cover was marked unavailable.
type coverStore struct {
	*refreshStore
	advertisedAt time.Time
	unavailable  int
}

func (s *coverStore) CatalogEntryCoverGeneration(context.Context, string, string, time.Time) ([]string, error) {
	return nil, nil
}

func (s *coverStore) GetCatalogEntryCoverForRetrieval(context.Context, domain.CatalogEntryKey) (domain.CatalogEntryCoverRetrieval, error) {
	return domain.CatalogEntryCoverRetrieval{AdvertisedAt: s.advertisedAt}, nil
}

func (s *coverStore) SaveCatalogEntryCover(context.Context, domain.CatalogEntryKey, time.Time, string, int, int, string, []byte) error {
	return errors.New("unexpected cover save")
}

func (s *coverStore) MarkCatalogEntryCoverUnavailable(context.Context, domain.CatalogEntryKey, time.Time, string) error {
	s.unavailable++
	return nil
}

type failingCoverReader struct{ refreshReader }

func (*failingCoverReader) FetchCover(context.Context, string, string, string) ([]byte, string, error) {
	return nil, "", errors.New("cover download cancelled")
}

func TestEntryCoverWorkerStoppedForShutdownKeepsTheCover(t *testing.T) {
	run := func(ctx context.Context) (*coverStore, error) {
		advertisedAt := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		store := &coverStore{
			refreshStore: &refreshStore{catalogueEntry: domain.CatalogueIndexEntry{
				ConnectionID: "connection-1", EntryIdentifier: "entry-1", CoverHref: "https://catalog.example/cover.png",
			}},
			advertisedAt: advertisedAt,
		}
		worker := &EntryCoverWorker{Connections: store, Catalogue: store, Reader: &failingCoverReader{}}
		err := worker.Work(ctx, &river.Job[EntryCoverArgs]{Args: EntryCoverArgs{
			OwnerID: "alice", ConnectionID: "connection-1", SourceIdentifier: "entry-1", AdvertisedAt: advertisedAt,
		}})
		return store, err
	}
	t.Run("a shutdown does not mark the cover unavailable", func(t *testing.T) {
		store, err := run(stoppedContext())
		require.Error(t, err)
		require.Zero(t, store.unavailable)
	})
	t.Run("a user cancel still marks the cover unavailable", func(t *testing.T) {
		store, err := run(userCancelledContext())
		require.Error(t, err)
		require.Equal(t, 1, store.unavailable)
	})
}
