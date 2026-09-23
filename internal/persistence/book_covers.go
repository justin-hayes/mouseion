package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// RecordBookCoverAdvertisement records the latest truthful state without
// replacing an already selected available image while retrieval is pending.
func (s *PostgresStore) RecordBookCoverAdvertisement(ctx context.Context, owner, bookID, connectionID, sourceIdentifier string, advertised bool) error {
	state := domain.BookCoverNone
	if advertised {
		state = domain.BookCoverPending
	}
	return s.queries().RecordBookCoverAdvertisement(ctx, sqlcgen.RecordBookCoverAdvertisementParams{
		Owner: owner, Book: bookID, State: state,
		Connection: nullableUUIDArg(connectionID), SourceIdentifier: textArg(sourceIdentifier),
	})
}

// SaveBookCover atomically selects the first successful source, or replaces an
// image only when it belongs to the currently selected source.
func (s *PostgresStore) SaveBookCover(ctx context.Context, owner, bookID, connectionID, sourceIdentifier, mediaType string, width, height int, contentHash string, bytes []byte) error {
	if len(bytes) == 0 || width <= 0 || height <= 0 || mediaType == "" || contentHash == "" {
		return errors.New("persistence: incomplete normalized book cover")
	}
	return s.queries().SaveBookCover(ctx, sqlcgen.SaveBookCoverParams{
		Owner: owner, Book: bookID, Connection: nullableUUIDArg(connectionID), SourceIdentifier: textArg(sourceIdentifier),
		MediaType: textArg(mediaType), Width: int4Arg(width), Height: int4Arg(height), ContentHash: textArg(contentHash), Bytes: bytes,
	})
}

func (s *PostgresStore) MarkBookCoverUnavailable(ctx context.Context, owner, bookID, reason string) error {
	return s.queries().MarkBookCoverUnavailable(ctx, sqlcgen.MarkBookCoverUnavailableParams{Owner: owner, Book: bookID, FailureReason: textArg(reason)})
}

// GetBookCover returns only byte-free metadata for a collection projection.
func (s *PostgresStore) GetBookCover(ctx context.Context, owner, bookID string) (domain.BookCover, error) {
	row, err := s.queries().GetBookCover(ctx, sqlcgen.GetBookCoverParams{Owner: owner, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BookCover{}, ErrNotFound
	}
	if err != nil {
		return domain.BookCover{}, err
	}
	return domain.BookCover{State: row.State, Width: pgInt4(row.Width), Height: pgInt4(row.Height)}, nil
}

// GetBookCoverResource is used only by the authenticated image endpoint.
func (s *PostgresStore) GetBookCoverResource(ctx context.Context, owner, bookID string) (domain.BookCoverResource, error) {
	row, err := s.queries().GetBookCover(ctx, sqlcgen.GetBookCoverParams{Owner: owner, Book: bookID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.BookCoverResource{}, ErrNotFound
		}
		return domain.BookCoverResource{}, err
	}
	if row.State != domain.BookCoverAvailable || len(row.Bytes) == 0 {
		return domain.BookCoverResource{}, ErrNotFound
	}
	return domain.BookCoverResource{
		OwnerID: row.OwnerID, BookID: row.BookID, MediaType: pgText(row.MediaType), ContentHash: pgText(row.ContentHash),
		Bytes: row.Bytes, Width: pgInt4(row.Width), Height: pgInt4(row.Height),
	}, nil
}

func int4Arg(value int) pgtype.Int4 {
	if value < 0 || value > 1<<31-1 {
		panic("book cover dimension exceeds PostgreSQL integer range")
	}
	return pgtype.Int4{Int32: int32(value), Valid: true}
}
