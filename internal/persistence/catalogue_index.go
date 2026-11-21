package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ReconcileCatalogueIndex replaces one connection's Catalog index with a
// complete sync run. It upserts every observed entry and deletes entries the
// run did not see, in one transaction, so a failed or partial traversal can
// never delete entries. The connection lock scopes ownership and serializes the
// run; a connection deleted mid-run leaves the index gone and changes nothing.
func (s *PostgresStore) ReconcileCatalogueIndex(ctx context.Context, owner, connectionID string, entries []domain.CatalogueIndexEntry, observedAt time.Time) error {
	owner, connectionID = strings.TrimSpace(owner), strings.TrimSpace(connectionID)
	if owner == "" || connectionID == "" || observedAt.IsZero() {
		return errors.New("persistence: catalog index identity is incomplete")
	}
	entries, err := normalizedCatalogueIndexEntries(entries)
	if err != nil {
		return err
	}
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		return reconcileCatalogueIndex(ctx, tx, owner, connectionID, entries, observedAt)
	})
}

// normalizedCatalogueIndexEntries trims the fields a sync supplies and rejects
// an entry that cannot represent a Catalog offering.
func normalizedCatalogueIndexEntries(entries []domain.CatalogueIndexEntry) ([]domain.CatalogueIndexEntry, error) {
	normalized := make([]domain.CatalogueIndexEntry, len(entries))
	for i, entry := range entries {
		entry.EntryIdentifier = strings.TrimSpace(entry.EntryIdentifier)
		entry.Title = strings.TrimSpace(entry.Title)
		entry.Author = strings.TrimSpace(entry.Author)
		entry.LanguageTag = strings.TrimSpace(entry.LanguageTag)
		entry.CoverHref = strings.TrimSpace(entry.CoverHref)
		entry.CoverMediaType = strings.TrimSpace(entry.CoverMediaType)
		entry.EPUBHref = strings.TrimSpace(entry.EPUBHref)
		if entry.EntryIdentifier == "" || entry.Title == "" || entry.LanguageTag == "" || entry.EPUBHref == "" {
			return nil, fmt.Errorf("persistence: catalog index entry %d is incomplete", i)
		}
		normalized[i] = entry
	}
	return normalized, nil
}

// reconcileCatalogueIndex writes one complete run under the connection lock
// and deletes the connection's entries the run did not see.
func reconcileCatalogueIndex(ctx context.Context, tx pgx.Tx, owner, connectionID string, entries []domain.CatalogueIndexEntry, observedAt time.Time) error {
	q := sqlcgen.New(tx)
	if _, err := q.LockCatalogueIndexConnection(ctx, sqlcgen.LockCatalogueIndexConnectionParams{OwnerID: uuidArg(owner), ConnectionID: connectionID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if err := q.UpsertCatalogueEntry(ctx, sqlcgen.UpsertCatalogueEntryParams{
			ConnectionID:    connectionID,
			EntryIdentifier: entry.EntryIdentifier,
			Title:           entry.Title,
			Author:          entry.Author,
			LanguageTag:     entry.LanguageTag,
			CoverHref:       entry.CoverHref,
			CoverMediaType:  entry.CoverMediaType,
			EpubHref:        entry.EPUBHref,
			LastSeenSync:    observedAt,
		}); err != nil {
			return err
		}
	}
	_, err := q.DeleteCatalogueEntriesNotSeen(ctx, sqlcgen.DeleteCatalogueEntriesNotSeenParams{
		OwnerID: uuidArg(owner), ConnectionID: connectionID, LastSeenSync: observedAt,
	})
	return err
}

// ListCatalogueEntries returns one connection's Catalog index in the order the
// Catalog browses it: title, then author, then identity.
func (s *PostgresStore) ListCatalogueEntries(ctx context.Context, owner, connectionID string) ([]domain.CatalogueIndexEntry, error) {
	rows, err := s.queries().ListCatalogueEntries(ctx, sqlcgen.ListCatalogueEntriesParams{OwnerID: uuidArg(owner), ConnectionID: connectionID})
	if err != nil {
		return nil, err
	}
	entries := make([]domain.CatalogueIndexEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, catalogueIndexEntry(row.ConnectionID, row.EntryIdentifier, row.Title, row.Author, row.LanguageTag, row.CoverHref, row.CoverMediaType, row.EpubHref, row.LastSeenSync))
	}
	return entries, nil
}

// GetCatalogueEntry returns one owner-scoped Catalog entry by its
// connection-scoped identity.
func (s *PostgresStore) GetCatalogueEntry(ctx context.Context, owner, connectionID, entryIdentifier string) (domain.CatalogueIndexEntry, error) {
	row, err := s.queries().GetCatalogueEntry(ctx, sqlcgen.GetCatalogueEntryParams{OwnerID: uuidArg(owner), ConnectionID: connectionID, EntryIdentifier: entryIdentifier})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CatalogueIndexEntry{}, ErrNotFound
		}
		return domain.CatalogueIndexEntry{}, err
	}
	return catalogueIndexEntry(row.ConnectionID, row.EntryIdentifier, row.Title, row.Author, row.LanguageTag, row.CoverHref, row.CoverMediaType, row.EpubHref, row.LastSeenSync), nil
}

func catalogueIndexEntry(connectionID, entryIdentifier, title, author, languageTag, coverHref, coverMediaType, epubHref string, lastSeenSync time.Time) domain.CatalogueIndexEntry {
	return domain.CatalogueIndexEntry{
		ConnectionID:    connectionID,
		EntryIdentifier: entryIdentifier,
		Title:           title,
		Author:          author,
		LanguageTag:     languageTag,
		CoverHref:       coverHref,
		CoverMediaType:  coverMediaType,
		EPUBHref:        epubHref,
		LastSeenSync:    lastSeenSync,
	}
}
