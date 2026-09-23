package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// RecordBookCoverAdvertisement applies one Catalog entry's advertised cover
// state to the owner-scoped cover row and reports whether retrieval work is
// needed. The row is locked for the duration so concurrent syncs of the same
// Book cannot interleave source selection. Only the currently selected Catalog
// entry may replace or remove a retained image; another alias can only become
// the candidate while no image is selected.
func (s *PostgresStore) RecordBookCoverAdvertisement(ctx context.Context, owner, bookID, connectionID, sourceIdentifier string, advertised bool) (retrieve bool, advertisedAt time.Time, err error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" || strings.TrimSpace(connectionID) == "" || strings.TrimSpace(sourceIdentifier) == "" {
		return false, time.Time{}, errors.New("persistence: book cover advertisement identity is incomplete")
	}
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		current, err := q.GetBookCoverForUpdate(ctx, sqlcgen.GetBookCoverForUpdateParams{Owner: owner, Book: bookID})
		if errors.Is(err, pgx.ErrNoRows) {
			if !advertised {
				// Absence without a known source is not provenance worth
				// persisting; leave the Book without a cover row.
				retrieve = false
				return nil
			}
			inserted, insertErr := q.InsertBookCoverCandidate(ctx, sqlcgen.InsertBookCoverCandidateParams{
				Owner: owner, Book: bookID, State: domain.BookCoverPending, Advertised: advertised,
				Connection: connectionID, SourceIdentifier: sourceIdentifier,
			})
			if insertErr != nil {
				return insertErr
			}
			retrieve = inserted > 0
			err = nil
		} else {
			if err != nil {
				return err
			}
			sameSource := current.SelectedConnectionID == connectionID && current.SelectedSourceIdentifier == sourceIdentifier
			switch {
			case current.State == domain.BookCoverAvailable && sameSource && !advertised:
				retrieve = false
				err = q.ClearBookCover(ctx, sqlcgen.ClearBookCoverParams{Owner: owner, Book: bookID})
			case current.State == domain.BookCoverAvailable && sameSource:
				retrieve = true
				err = q.RefreshBookCoverAdvertisement(ctx, sqlcgen.RefreshBookCoverAdvertisementParams{Owner: owner, Book: bookID})
			case current.State == domain.BookCoverAvailable:
				retrieve = false
			case current.State == domain.BookCoverPending && advertised:
				// Keep one pending generation for concurrent initial candidates;
				// their first validated completion will select the source atomically.
				retrieve = true
			case sameSource && !advertised:
				retrieve = false
				err = q.ClearBookCover(ctx, sqlcgen.ClearBookCoverParams{Owner: owner, Book: bookID})
			case advertised:
				retrieve = true
				err = q.SetBookCoverPending(ctx, sqlcgen.SetBookCoverPendingParams{
					Owner: owner, Book: bookID, Connection: uuidArg(connectionID), SourceIdentifier: textArg(sourceIdentifier),
				})
			}
		}
		if err != nil {
			return err
		}
		if retrieve {
			current, err = q.GetBookCoverForUpdate(ctx, sqlcgen.GetBookCoverForUpdateParams{Owner: owner, Book: bookID})
			if err != nil {
				return err
			}
			advertisedAt = current.AdvertisedAt.Time
		}
		return nil
	})
	if err != nil {
		return false, time.Time{}, err
	}
	return retrieve, advertisedAt, nil
}

// SaveBookCover persists a retrieval only while its advertisement generation is
// current. Pending candidates in that generation race for the first success;
// an available image can only be replaced by its selected source.
func (s *PostgresStore) SaveBookCover(ctx context.Context, owner, bookID, connectionID, sourceIdentifier string, advertisedAt time.Time, mediaType string, width, height int, contentHash string, bytes []byte) error {
	if len(bytes) == 0 || width <= 0 || height <= 0 || mediaType == "" || contentHash == "" {
		return errors.New("persistence: incomplete normalized book cover")
	}
	return s.queries().SaveBookCover(ctx, sqlcgen.SaveBookCoverParams{
		Owner: owner, Book: bookID, Connection: nullableUUIDArg(connectionID), SourceIdentifier: textArg(sourceIdentifier),
		AdvertisedAt: pgtype.Timestamptz{Time: advertisedAt, Valid: !advertisedAt.IsZero()},
		MediaType:    textArg(mediaType), Width: int4Arg(width), Height: int4Arg(height), ContentHash: textArg(contentHash), Bytes: bytes,
	})
}

func (s *PostgresStore) MarkBookCoverUnavailable(ctx context.Context, owner, bookID, connectionID, sourceIdentifier string, advertisedAt time.Time, reason string) error {
	return s.queries().MarkBookCoverUnavailable(ctx, sqlcgen.MarkBookCoverUnavailableParams{
		Owner: owner, Book: bookID, Connection: uuidArg(connectionID), SourceIdentifier: textArg(sourceIdentifier),
		AdvertisedAt: pgtype.Timestamptz{Time: advertisedAt, Valid: !advertisedAt.IsZero()}, FailureReason: textArg(reason),
	})
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

// GetBookCoverForRetrieval returns the owner-scoped selected-source state the
// retrieval worker must reload before fetching anything.
func (s *PostgresStore) GetBookCoverForRetrieval(ctx context.Context, owner, bookID string) (domain.BookCoverRetrieval, error) {
	row, err := s.queries().GetBookCoverForRetrieval(ctx, sqlcgen.GetBookCoverForRetrievalParams{Owner: owner, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BookCoverRetrieval{}, ErrNotFound
	}
	if err != nil {
		return domain.BookCoverRetrieval{}, err
	}
	return domain.BookCoverRetrieval{
		State:                    row.State,
		SelectedConnectionID:     row.SelectedConnectionID,
		SelectedSourceIdentifier: row.SelectedSourceIdentifier,
		FailureReason:            row.FailureReason,
		AdvertisedAt:             row.AdvertisedAt.Time,
	}, nil
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
