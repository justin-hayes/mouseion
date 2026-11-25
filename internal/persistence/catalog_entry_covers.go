package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// Catalog entry cover states (ADR 0130). No row means the entry advertises no
// cover.
const (
	CatalogEntryCoverPending     = "pending"
	CatalogEntryCoverAvailable   = "available"
	CatalogEntryCoverUnavailable = "unavailable"
)

// CatalogEntryCoverFence is the precision a retrieval generation keeps: it is
// the PostgreSQL timestamptz resolution, so the value a sync passes to
// CatalogEntryCoverGeneration equals the stored fence.
const CatalogEntryCoverFence = time.Microsecond

// previousCatalogueCovers reads the advertised cover identity of the connection's
// index before a run replaces it, so reconciliation can tell a changed image
// from an unchanged one.
func previousCatalogueCovers(ctx context.Context, q *sqlcgen.Queries, owner, connectionID string) (map[string]string, error) {
	rows, err := q.ListCatalogueEntries(ctx, sqlcgen.ListCatalogueEntriesParams{OwnerID: uuidArg(owner), ConnectionID: connectionID})
	if err != nil {
		return nil, err
	}
	previous := make(map[string]string, len(rows))
	for _, row := range rows {
		previous[row.EntryIdentifier] = row.CoverHref + "\x00" + row.CoverMediaType
	}
	return previous, nil
}

// reconcileCatalogEntryCovers applies a complete run's cover bookkeeping in the
// index transaction (ADR 0077, ADR 0130). An entry that advertises an image and
// has no retained, unchanged one starts a retrieval generation fenced by
// observedAt; the previous image stays until a replacement validates. An entry
// that left the index or stopped advertising an image loses its cover.
func reconcileCatalogEntryCovers(ctx context.Context, q *sqlcgen.Queries, owner, connectionID string, entries []domain.CatalogueIndexEntry, previous map[string]string, observedAt time.Time) error {
	if _, err := q.DeleteCatalogEntryCoversNotAdvertised(ctx, sqlcgen.DeleteCatalogEntryCoversNotAdvertisedParams{OwnerID: owner, ConnectionID: connectionID}); err != nil {
		return err
	}
	states, err := q.ListCatalogEntryCoverStates(ctx, sqlcgen.ListCatalogEntryCoverStatesParams{OwnerID: owner, ConnectionID: connectionID})
	if err != nil {
		return err
	}
	available := make(map[string]bool, len(states))
	for _, state := range states {
		available[state.EntryIdentifier] = state.State == CatalogEntryCoverAvailable
	}
	fence := observedAt.Truncate(CatalogEntryCoverFence)
	for _, entry := range entries {
		if entry.CoverHref == "" {
			continue
		}
		unchanged := previous[entry.EntryIdentifier] == entry.CoverHref+"\x00"+entry.CoverMediaType
		if available[entry.EntryIdentifier] && unchanged {
			continue
		}
		if err := q.AdvertiseCatalogEntryCover(ctx, sqlcgen.AdvertiseCatalogEntryCoverParams{
			OwnerID: owner, ConnectionID: connectionID, EntryIdentifier: entry.EntryIdentifier, AdvertisedAt: fence,
		}); err != nil {
			return err
		}
	}
	return nil
}

// CatalogEntryCoverGeneration lists the connection's entries whose retrieval
// generation is the one a run observed at observedAt, so the sync can schedule
// retrieval after the index transaction commits.
func (s *PostgresStore) CatalogEntryCoverGeneration(ctx context.Context, owner, connectionID string, observedAt time.Time) ([]string, error) {
	return s.queries().ListCatalogEntryCoverGeneration(ctx, sqlcgen.ListCatalogEntryCoverGenerationParams{
		OwnerID: owner, ConnectionID: connectionID, AdvertisedAt: observedAt.Truncate(CatalogEntryCoverFence),
	})
}

// GetCatalogEntryCoverForRetrieval returns the state the retrieval worker
// reloads before it fetches anything.
func (s *PostgresStore) GetCatalogEntryCoverForRetrieval(ctx context.Context, owner, connectionID, entryIdentifier string) (domain.CatalogEntryCoverRetrieval, error) {
	row, err := s.queries().GetCatalogEntryCoverForRetrieval(ctx, sqlcgen.GetCatalogEntryCoverForRetrievalParams{OwnerID: owner, ConnectionID: connectionID, EntryIdentifier: entryIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CatalogEntryCoverRetrieval{}, ErrNotFound
	}
	if err != nil {
		return domain.CatalogEntryCoverRetrieval{}, err
	}
	return domain.CatalogEntryCoverRetrieval{State: row.State, AdvertisedAt: row.AdvertisedAt}, nil
}

// SaveCatalogEntryCover stores a validated image only while its generation is
// current; a stale completion changes nothing.
func (s *PostgresStore) SaveCatalogEntryCover(ctx context.Context, owner, connectionID, entryIdentifier string, advertisedAt time.Time, mediaType string, width, height int, contentHash string, bytes []byte) error {
	if len(bytes) == 0 || width <= 0 || height <= 0 || mediaType == "" || contentHash == "" {
		return errors.New("persistence: incomplete normalized catalog entry cover")
	}
	_, err := s.queries().SaveCatalogEntryCover(ctx, sqlcgen.SaveCatalogEntryCoverParams{
		OwnerID: owner, ConnectionID: connectionID, EntryIdentifier: entryIdentifier, AdvertisedAt: advertisedAt,
		MediaType: textArg(mediaType), Width: int4Arg(width), Height: int4Arg(height), ContentHash: textArg(contentHash), Bytes: bytes,
	})
	return err
}

// MarkCatalogEntryCoverUnavailable records a failed retrieval for the current
// generation. A retained image is kept.
func (s *PostgresStore) MarkCatalogEntryCoverUnavailable(ctx context.Context, owner, connectionID, entryIdentifier string, advertisedAt time.Time, reason string) error {
	_, err := s.queries().MarkCatalogEntryCoverUnavailable(ctx, sqlcgen.MarkCatalogEntryCoverUnavailableParams{
		OwnerID: owner, ConnectionID: connectionID, EntryIdentifier: entryIdentifier, AdvertisedAt: advertisedAt, FailureReason: textArg(reason),
	})
	return err
}

// GetCatalogEntryCoverResource is the owner-scoped endpoint read: only an
// available image of the owner's own entry is returned, and every other case is
// ErrNotFound so nothing about the entry is revealed.
func (s *PostgresStore) GetCatalogEntryCoverResource(ctx context.Context, owner, connectionID, entryIdentifier string) (domain.CatalogEntryCoverResource, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(connectionID) == "" || strings.TrimSpace(entryIdentifier) == "" {
		return domain.CatalogEntryCoverResource{}, ErrNotFound
	}
	row, err := s.queries().GetCatalogEntryCoverResource(ctx, sqlcgen.GetCatalogEntryCoverResourceParams{OwnerID: owner, ConnectionID: connectionID, EntryIdentifier: entryIdentifier})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && len(row.Bytes) == 0) {
		return domain.CatalogEntryCoverResource{}, ErrNotFound
	}
	if err != nil {
		return domain.CatalogEntryCoverResource{}, err
	}
	return domain.CatalogEntryCoverResource{
		MediaType: pgText(row.MediaType), ContentHash: pgText(row.ContentHash), Bytes: row.Bytes,
		Width: pgInt4(row.Width), Height: pgInt4(row.Height),
	}, nil
}
