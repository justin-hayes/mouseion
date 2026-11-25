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

// catalogCoverReconciler carries one connection's cover bookkeeping for a run.
// Its writes run in a savepoint: a failure rolls back only cover bookkeeping.
type catalogCoverReconciler struct {
	tx                  pgx.Tx
	owner, connectionID string
}

func newCatalogCoverReconciler(tx pgx.Tx, owner, connectionID string) *catalogCoverReconciler {
	return &catalogCoverReconciler{tx: tx, owner: owner, connectionID: connectionID}
}

// reconcile applies a complete run's cover bookkeeping (ADR 0077, ADR 0130).
// Every entry that advertises an image starts a retrieval generation fenced by
// observedAt, so a replaced image at the same URL is refreshed on each sync; the
// previous image stays until a replacement validates. An entry that left the index or stopped advertising an
// image loses its cover.
func (r *catalogCoverReconciler) reconcile(ctx context.Context, entries []domain.CatalogueIndexEntry, observedAt time.Time) error {
	return pgx.BeginFunc(ctx, r.tx, func(sp pgx.Tx) error {
		q := sqlcgen.New(sp)
		if err := demoteAbsentBookCovers(ctx, q, r.owner, r.connectionID); err != nil {
			return err
		}
		if _, err := q.DeleteCatalogEntryCoversNotAdvertised(ctx, sqlcgen.DeleteCatalogEntryCoversNotAdvertisedParams{OwnerID: r.owner, ConnectionID: r.connectionID}); err != nil {
			return err
		}
		return r.advertise(ctx, q, entries, observedAt.Truncate(CatalogEntryCoverFence))
	})
}

// demoteAbsentBookCovers handles explicit upstream absence (ADR 0077): a Book
// that selected an entry which is still in the index but advertises no image
// gives the selection up and falls back to another alias, before the entry's
// row is deleted (the foreign key forbids the reverse order).
func demoteAbsentBookCovers(ctx context.Context, q *sqlcgen.Queries, owner, connectionID string) error {
	books, err := q.DemoteBookCoversForAbsentEntries(ctx, sqlcgen.DemoteBookCoversForAbsentEntriesParams{Owner: owner, Connection: connectionID})
	if err != nil || len(books) == 0 {
		return err
	}
	return q.SelectFallbackBookCovers(ctx, sqlcgen.SelectFallbackBookCoversParams{Owner: owner, BookIds: books})
}

// advertise starts a retrieval generation for every entry that advertises an
// image.
func (r *catalogCoverReconciler) advertise(ctx context.Context, q *sqlcgen.Queries, entries []domain.CatalogueIndexEntry, fence time.Time) error {
	for _, entry := range entries {
		if entry.CoverHref == "" {
			continue
		}
		if err := q.AdvertiseCatalogEntryCover(ctx, sqlcgen.AdvertiseCatalogEntryCoverParams{
			OwnerID: r.owner, ConnectionID: r.connectionID, EntryIdentifier: entry.EntryIdentifier, AdvertisedAt: fence,
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
func (s *PostgresStore) GetCatalogEntryCoverForRetrieval(ctx context.Context, key domain.CatalogEntryKey) (domain.CatalogEntryCoverRetrieval, error) {
	row, err := s.queries().GetCatalogEntryCoverForRetrieval(ctx, sqlcgen.GetCatalogEntryCoverForRetrievalParams{OwnerID: key.OwnerID, ConnectionID: key.ConnectionID, EntryIdentifier: key.EntryIdentifier})
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
func (s *PostgresStore) SaveCatalogEntryCover(ctx context.Context, key domain.CatalogEntryKey, advertisedAt time.Time, mediaType string, width, height int, contentHash string, bytes []byte) error {
	if len(bytes) == 0 || width <= 0 || height <= 0 || mediaType == "" || contentHash == "" {
		return errors.New("persistence: incomplete normalized catalog entry cover")
	}
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		saved, err := q.SaveCatalogEntryCover(ctx, sqlcgen.SaveCatalogEntryCoverParams{
			OwnerID: key.OwnerID, ConnectionID: key.ConnectionID, EntryIdentifier: key.EntryIdentifier, AdvertisedAt: advertisedAt,
			MediaType: textArg(mediaType), Width: int4Arg(width), Height: int4Arg(height), ContentHash: textArg(contentHash), Bytes: bytes,
		})
		if err != nil || saved == 0 {
			return err
		}
		// First success selects: Books aliasing this entry that have no
		// available image take it (ADR 0077).
		return selectBookCoversFromEntry(ctx, q, key.OwnerID, key.ConnectionID, key.EntryIdentifier)
	})
}

// MarkCatalogEntryCoverUnavailable records a failed retrieval for the current
// generation. A retained image is kept.
func (s *PostgresStore) MarkCatalogEntryCoverUnavailable(ctx context.Context, key domain.CatalogEntryKey, advertisedAt time.Time, reason string) error {
	_, err := s.queries().MarkCatalogEntryCoverUnavailable(ctx, sqlcgen.MarkCatalogEntryCoverUnavailableParams{
		OwnerID: key.OwnerID, ConnectionID: key.ConnectionID, EntryIdentifier: key.EntryIdentifier, AdvertisedAt: advertisedAt, FailureReason: textArg(reason),
	})
	return err
}

// GetCatalogEntryCoverResource is the owner-scoped endpoint read: only an
// available image of the owner's own entry is returned, and every other case is
// ErrNotFound so nothing about the entry is revealed.
func (s *PostgresStore) GetCatalogEntryCoverResource(ctx context.Context, key domain.CatalogEntryKey) (domain.CatalogEntryCoverResource, error) {
	if strings.TrimSpace(key.OwnerID) == "" || strings.TrimSpace(key.ConnectionID) == "" || strings.TrimSpace(key.EntryIdentifier) == "" || !validUUID(key.OwnerID) || !validUUID(key.ConnectionID) {
		return domain.CatalogEntryCoverResource{}, ErrNotFound
	}
	row, err := s.queries().GetCatalogEntryCoverResource(ctx, sqlcgen.GetCatalogEntryCoverResourceParams{OwnerID: key.OwnerID, ConnectionID: key.ConnectionID, EntryIdentifier: key.EntryIdentifier})
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
