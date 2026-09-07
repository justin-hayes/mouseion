package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

type readingJourneyMembership struct {
	bookID    string
	position  int
	createdAt time.Time
}

// GetReadingJourney returns the owner's ordered Journey and current revision.
// An owner without a Journey header has the ordinary empty Journey state.
func (s *PostgresStore) GetReadingJourney(ctx context.Context, owner string) (domain.ReadingJourney, error) {
	journey := domain.ReadingJourney{OwnerID: owner}
	if err := s.pool.QueryRow(ctx, `SELECT revision,updated_at FROM reading_journeys WHERE owner_id=$1`, owner).Scan(&journey.Revision, &journey.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return journey, nil
		}
		return journey, missing(err)
	}

	rows, err := s.pool.Query(ctx, `SELECT book_id::text,position,created_at FROM reading_journey_membership WHERE owner_id=$1 ORDER BY position,created_at,book_id`, owner)
	if err != nil {
		return journey, err
	}
	defer rows.Close()
	for rows.Next() {
		entry := domain.ReadingJourneyEntry{OwnerID: owner}
		if err := rows.Scan(&entry.BookID, &entry.Position, &entry.CreatedAt); err != nil {
			return journey, err
		}
		journey.Entries = append(journey.Entries, entry)
	}
	return journey, rows.Err()
}

func (s *PostgresStore) beginReadingJourneyMutation(ctx context.Context, owner string) (pgx.Tx, int64, []readingJourneyMembership, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	rollback := func(err error) (pgx.Tx, int64, []readingJourneyMembership, error) {
		_ = tx.Rollback(ctx)
		return nil, 0, nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO reading_journeys(owner_id) VALUES($1) ON CONFLICT (owner_id) DO NOTHING`, owner); err != nil {
		return rollback(err)
	}
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM reading_journeys WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&revision); err != nil {
		return rollback(missing(err))
	}
	rows, err := tx.Query(ctx, `SELECT book_id::text,position,created_at FROM reading_journey_membership WHERE owner_id=$1 ORDER BY position,created_at,book_id FOR UPDATE`, owner)
	if err != nil {
		return rollback(err)
	}
	defer rows.Close()
	var members []readingJourneyMembership
	for rows.Next() {
		var member readingJourneyMembership
		if err := rows.Scan(&member.bookID, &member.position, &member.createdAt); err != nil {
			return rollback(err)
		}
		members = append(members, member)
	}
	if err = rows.Err(); err != nil {
		return rollback(err)
	}
	return tx, revision, members, nil
}

func rewriteReadingJourneyPositions(ctx context.Context, tx pgx.Tx, owner string, members []readingJourneyMembership) error {
	// UPSERT: a member removed from the slice was already deleted by the
	// caller, so it is not re-inserted here; a freshly appended member (Add)
	// is inserted; all other members are re-positioned to stay contiguous.
	for position, member := range members {
		if _, err := tx.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id, book_id, position) VALUES($1, $2, $3)
			ON CONFLICT (owner_id, book_id) DO UPDATE SET position = EXCLUDED.position`, owner, member.bookID, position+1); err != nil {
			return err
		}
	}
	return nil
}

func bumpReadingJourneyRevision(ctx context.Context, tx pgx.Tx, owner string) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `UPDATE reading_journeys SET revision=revision+1,updated_at=now() WHERE owner_id=$1 RETURNING revision`, owner).Scan(&revision)
	return revision, err
}

// AddToReadingJourney appends a known owner book to the Journey.
func (s *PostgresStore) AddToReadingJourney(ctx context.Context, owner, bookID string, expectedRevision int64) (int64, error) {
	tx, revision, members, err := s.beginReadingJourneyMutation(ctx, owner)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if expectedRevision != revision {
		return 0, ErrJourneyStale
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return 0, err
	}
	for _, member := range members {
		if member.bookID == bookID {
			return revision, nil
		}
	}
	members = append(members, readingJourneyMembership{bookID: bookID})
	if err = rewriteReadingJourneyPositions(ctx, tx, owner, members); err != nil {
		return 0, err
	}
	if revision, err = bumpReadingJourneyRevision(ctx, tx, owner); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return revision, nil
}

// ResolveJourneyBookID maps the identity used by a deck-journey action surface
// to the canonical books.id that Reading Journey membership stores. Deck
// preparation surfaces are keyed by source_materials.id while membership and
// the Goal are keyed by books.id, so an id that is already a book passes
// through unchanged and a source material is resolved through its linked book
// (source_materials.book_id), falling back to the source-identifier alias for
// legacy sources where the link column is unset. The second result reports
// whether a book identity exists for the owner.
func (s *PostgresStore) ResolveJourneyBookID(ctx context.Context, owner, id string) (string, bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE owner_id=$1 AND id=$2)`, owner, id).Scan(&exists); err != nil {
		return "", false, err
	}
	if exists {
		return id, true, nil
	}
	var linked *string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sm.book_id::text,b.id::text)
		FROM source_materials sm
		LEFT JOIN book_aliases a ON a.owner_id=sm.owner_id AND a.namespace=$3 AND a.value=sm.source_identifier
		LEFT JOIN books b ON b.owner_id=a.owner_id AND b.id=a.book_id
		WHERE sm.owner_id=$1 AND sm.id=$2`, owner, id, domain.NamespaceSourceIdentifier).Scan(&linked)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if linked == nil {
		return "", false, nil
	}
	return *linked, true, nil
}

// RemoveFromReadingJourney removes a Journey member and compacts positions.
func (s *PostgresStore) RemoveFromReadingJourney(ctx context.Context, owner, bookID string, expectedRevision int64) (int64, error) {
	tx, revision, members, err := s.beginReadingJourneyMutation(ctx, owner)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if expectedRevision != revision {
		return 0, ErrJourneyStale
	}
	memberIndex := -1
	for i, member := range members {
		if member.bookID == bookID {
			memberIndex = i
			break
		}
	}
	if memberIndex == -1 {
		return revision, nil
	}
	if _, err = tx.Exec(ctx, `DELETE FROM reading_journey_membership WHERE owner_id=$1 AND book_id=$2`, owner, bookID); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM primary_goals WHERE owner_id=$1 AND book_id=$2`, owner, bookID); err != nil {
		return 0, err
	}
	members = append(members[:memberIndex], members[memberIndex+1:]...)
	if err = rewriteReadingJourneyPositions(ctx, tx, owner, members); err != nil {
		return 0, err
	}
	if revision, err = bumpReadingJourneyRevision(ctx, tx, owner); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return revision, nil
}

// MoveReadingJourneyEntry reorders a Journey member, clamping its destination.
// When the owner has a Primary Goal whose book is also a Journey member, that
// anchored Goal entry is invisible to the provisional order: newPosition is then
// interpreted as a 1-based position within the visible (Goal-excluded) order,
// and the Goal entry itself is never moved. Without a member Goal, newPosition
// is an absolute position in the full membership order.
func (s *PostgresStore) MoveReadingJourneyEntry(ctx context.Context, owner, bookID string, newPosition int, expectedRevision int64) (int64, error) {
	tx, revision, members, err := s.beginReadingJourneyMutation(ctx, owner)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if expectedRevision != revision {
		return 0, ErrJourneyStale
	}
	memberIndex := -1
	for i, member := range members {
		if member.bookID == bookID {
			memberIndex = i
			break
		}
	}
	if memberIndex == -1 {
		return 0, ErrNotFound
	}
	var goalBookID string
	if err = tx.QueryRow(ctx, `SELECT book_id::text FROM primary_goals WHERE owner_id=$1`, owner).Scan(&goalBookID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if goalBookID != "" && goalBookID != bookID {
		goalIndex := -1
		visible := make([]readingJourneyMembership, 0, len(members)-1)
		for index, member := range members {
			if member.bookID == goalBookID {
				goalIndex = index
				continue
			}
			visible = append(visible, member)
		}
		if goalIndex >= 0 {
			visibleIndex := 0
			for index, member := range members {
				if member.bookID == bookID {
					visibleIndex = index
					if index > goalIndex {
						visibleIndex--
					}
					break
				}
			}
			if newPosition < 1 {
				newPosition = 1
			}
			if newPosition > len(visible) {
				newPosition = len(visible)
			}
			if visibleIndex == newPosition-1 {
				return revision, nil
			}
			member := visible[visibleIndex]
			visible = append(visible[:visibleIndex], visible[visibleIndex+1:]...)
			visible = append(visible, readingJourneyMembership{})
			copy(visible[newPosition:], visible[newPosition-1:])
			visible[newPosition-1] = member
			members = make([]readingJourneyMembership, 0, len(visible)+1)
			visibleIndex = 0
			for index := 0; index < len(visible)+1; index++ {
				if index == goalIndex {
					members = append(members, readingJourneyMembership{bookID: goalBookID})
					continue
				}
				members = append(members, visible[visibleIndex])
				visibleIndex++
			}
		} else {
			goalBookID = ""
		}
	}
	if goalBookID == "" {
		if newPosition < 1 {
			newPosition = 1
		}
		if newPosition > len(members) {
			newPosition = len(members)
		}
		if memberIndex == newPosition-1 {
			return revision, nil
		}
		member := members[memberIndex]
		members = append(members[:memberIndex], members[memberIndex+1:]...)
		members = append(members, readingJourneyMembership{})
		copy(members[newPosition:], members[newPosition-1:])
		members[newPosition-1] = member
	}
	if err = rewriteReadingJourneyPositions(ctx, tx, owner, members); err != nil {
		return 0, err
	}
	if revision, err = bumpReadingJourneyRevision(ctx, tx, owner); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return revision, nil
}
