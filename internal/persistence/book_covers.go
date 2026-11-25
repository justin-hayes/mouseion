package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// A Book's cover resolves through the Catalog entry it selected (ADR 0130).
// book_covers holds only that selection; the retained image and its retrieval
// state live in catalog_entry_covers (catalog_entry_covers.go).

// GetBookCover returns only byte-free metadata for a collection projection.
func (s *PostgresStore) GetBookCover(ctx context.Context, owner, bookID string) (domain.BookCover, error) {
	row, err := s.queries().GetBookCover(ctx, sqlcgen.GetBookCoverParams{Owner: owner, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BookCover{}, ErrNotFound
	}
	if err != nil {
		return domain.BookCover{}, err
	}
	return domain.BookCover{State: row.State, Width: row.Width, Height: row.Height}, nil
}

// GetBookCoverResource reads retained bytes for internal persistence checks.
func (s *PostgresStore) GetBookCoverResource(ctx context.Context, owner, bookID string) (domain.BookCoverResource, error) {
	row, err := s.queries().GetBookCoverResource(ctx, sqlcgen.GetBookCoverResourceParams{Owner: owner, Book: bookID})
	return bookCoverResource(err, row.OwnerID, row.BookID, row.MediaType, row.ContentHash, row.Width, row.Height, row.Bytes)
}

// GetActiveBookCoverResource is the authenticated image endpoint read. The
// active-membership predicate is part of the same query as the bytes so a
// removal cannot race authorization and still expose a retained cover.
func (s *PostgresStore) GetActiveBookCoverResource(ctx context.Context, owner, bookID string) (domain.BookCoverResource, error) {
	row, err := s.queries().GetActiveBookCoverResource(ctx, sqlcgen.GetActiveBookCoverResourceParams{Owner: owner, Book: bookID})
	return bookCoverResource(err, row.OwnerID, row.BookID, row.MediaType, row.ContentHash, row.Width, row.Height, row.Bytes)
}

func bookCoverResource(err error, ownerID, bookID string, mediaType, contentHash pgtype.Text, width, height pgtype.Int4, bytes []byte) (domain.BookCoverResource, error) {
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.BookCoverResource{}, ErrNotFound
		}
		return domain.BookCoverResource{}, err
	}
	if len(bytes) == 0 {
		return domain.BookCoverResource{}, ErrNotFound
	}
	return domain.BookCoverResource{
		OwnerID: ownerID, BookID: bookID, MediaType: pgText(mediaType), ContentHash: pgText(contentHash),
		Bytes: bytes, Width: pgInt4(width), Height: pgInt4(height),
	}, nil
}

// selectBookCoversFromEntry makes every Book aliasing the entry select its
// image once that is available, unless the Book already selects an available
// one (first success stays, ADR 0077).
func selectBookCoversFromEntry(ctx context.Context, q *sqlcgen.Queries, owner, connectionID, entryIdentifier string) error {
	return q.SelectBookCoversFromEntry(ctx, sqlcgen.SelectBookCoversFromEntryParams{
		Owner: owner, Connection: uuidArg(connectionID), EntryIdentifier: entryIdentifier,
	})
}

func int4Arg(value int) pgtype.Int4 {
	if value < 0 || value > 1<<31-1 {
		panic("book cover dimension exceeds PostgreSQL integer range")
	}
	return pgtype.Int4{Int32: int32(value), Valid: true}
}
