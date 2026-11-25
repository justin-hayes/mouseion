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

// catalogCoverAdvert is the advertised cover identity of an index entry; a
// change in either field means the upstream image changed.
type catalogCoverAdvert struct{ href, mediaType string }

// catalogCoverReconciler holds the pre-run advertised covers of one connection
// so reconciliation can tell a changed image from an unchanged one. Its writes
// run in a savepoint: a failure rolls back only cover bookkeeping.
type catalogCoverReconciler struct {
	tx                  pgx.Tx
	owner, connectionID string
	previous            map[string]catalogCoverAdvert
}

// newCatalogCoverReconciler reads the connection's index before a run replaces
// it.
func newCatalogCoverReconciler(ctx context.Context, tx pgx.Tx, owner, connectionID string) (*catalogCoverReconciler, error) {
	r := &catalogCoverReconciler{tx: tx, owner: owner, connectionID: connectionID}
	err := pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error {
		rows, err := sqlcgen.New(sp).ListCatalogueEntries(ctx, sqlcgen.ListCatalogueEntriesParams{OwnerID: uuidArg(owner), ConnectionID: connectionID})
		if err != nil {
			return err
		}
		r.previous = make(map[string]catalogCoverAdvert, len(rows))
		for _, row := range rows {
			r.previous[row.EntryIdentifier] = catalogCoverAdvert{row.CoverHref, row.CoverMediaType}
		}
		return nil
	})
	return r, err
}

// reconcile applies a complete run's cover bookkeeping (ADR 0077, ADR 0130). An
// entry that advertises an image and has no retained, unchanged one starts a
// retrieval generation fenced by observedAt; the previous image stays until a
// replacement validates. An entry that left the index or stopped advertising an
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
		states, err := q.ListCatalogEntryCoverStates(ctx, sqlcgen.ListCatalogEntryCoverStatesParams{OwnerID: r.owner, ConnectionID: r.connectionID})
		if err != nil {
			return err
		}
		settled := make(map[string]bool, len(states))
		for _, state := range states {
			// An image whose last refresh failed is retried even if its URL is
			// unchanged, so a failed replacement never sticks.
			settled[state.EntryIdentifier] = state.State == CatalogEntryCoverAvailable && !state.Failed
		}
		return r.advertise(ctx, q, entries, settled, observedAt.Truncate(CatalogEntryCoverFence))
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
// image and has no settled, unchanged retained one.
func (r *catalogCoverReconciler) advertise(ctx context.Context, q *sqlcgen.Queries, entries []domain.CatalogueIndexEntry, settled map[string]bool, fence time.Time) error {
	for _, entry := range entries {
		if entry.CoverHref == "" {
			continue
		}
		unchanged := r.previous[entry.EntryIdentifier] == catalogCoverAdvert{entry.CoverHref, entry.CoverMediaType}
		if settled[entry.EntryIdentifier] && unchanged {
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
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		saved, err := q.SaveCatalogEntryCover(ctx, sqlcgen.SaveCatalogEntryCoverParams{
			OwnerID: owner, ConnectionID: connectionID, EntryIdentifier: entryIdentifier, AdvertisedAt: advertisedAt,
			MediaType: textArg(mediaType), Width: int4Arg(width), Height: int4Arg(height), ContentHash: textArg(contentHash), Bytes: bytes,
		})
		if err != nil || saved == 0 {
			return err
		}
		// First success selects: Books aliasing this entry that have no
		// available image take it (ADR 0077).
		return selectBookCoversFromEntry(ctx, q, owner, connectionID, entryIdentifier)
	})
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
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(connectionID) == "" || strings.TrimSpace(entryIdentifier) == "" || !validUUID(owner) || !validUUID(connectionID) {
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
