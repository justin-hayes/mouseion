package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

type readingJourneyMembership struct {
	bookID    string
	position  int
	createdAt time.Time
}

// GetReadingJourney returns one language's ordered Journey and current revision.
// A language without a Journey header has the ordinary empty Journey state.
func (s *PostgresStore) GetReadingJourney(ctx context.Context, owner, language string) (domain.ReadingJourney, error) {
	language = canonicalization.NormalizeLanguage(language)
	journey := domain.ReadingJourney{OwnerID: owner, Language: language}
	if err := s.pool.QueryRow(ctx, `SELECT revision,updated_at FROM reading_journeys WHERE owner_id=$1 AND language=$2`, owner, language).Scan(&journey.Revision, &journey.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return journey, nil
		}
		return journey, missing(err)
	}

	rows, err := s.pool.Query(ctx, `SELECT m.book_id::text,m.position,m.created_at
		FROM reading_journey_membership m
		JOIN books b ON b.owner_id=m.owner_id AND b.id=m.book_id AND b.language_state='chosen' AND b.language_tag=$2
		WHERE m.owner_id=$1 AND m.language=$2 ORDER BY m.position,m.created_at,m.book_id`, owner, language)
	if err != nil {
		return journey, err
	}
	defer rows.Close()
	for rows.Next() {
		entry := domain.ReadingJourneyEntry{OwnerID: owner, Language: language}
		if err := rows.Scan(&entry.BookID, &entry.Position, &entry.CreatedAt); err != nil {
			return journey, err
		}
		journey.Entries = append(journey.Entries, entry)
	}
	return journey, rows.Err()
}

func (s *PostgresStore) beginReadingJourneyMutation(ctx context.Context, owner, language string, create bool) (pgx.Tx, int64, []readingJourneyMembership, bool, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, 0, nil, false, false, err
	}
	rollback := func(err error) (pgx.Tx, int64, []readingJourneyMembership, bool, bool, error) {
		_ = tx.Rollback(ctx)
		return nil, 0, nil, false, false, err
	}
	if create {
		if _, err = tx.Exec(ctx, `INSERT INTO reading_journeys(owner_id,language) VALUES($1,$2) ON CONFLICT (owner_id,language) DO NOTHING`, owner, language); err != nil {
			return rollback(err)
		}
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM reading_journeys WHERE owner_id=$1 AND language=$2 FOR UPDATE`, owner, language).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		if create {
			return rollback(ErrNotFound)
		}
		return tx, 0, nil, false, false, nil
	}
	if err != nil {
		return rollback(missing(err))
	}
	// Retagged or unknown-language Books cannot remain members of this Journey.
	cleanup, err := tx.Exec(ctx, `DELETE FROM reading_journey_membership m USING books b
		WHERE m.owner_id=$1 AND m.language=$2 AND m.book_id=b.id AND b.owner_id=$1
		AND (b.language_state <> 'chosen' OR b.language_tag <> $2)`, owner, language)
	if err != nil {
		return rollback(err)
	}
	rows, err := tx.Query(ctx, `SELECT m.book_id::text,m.position,m.created_at
		FROM reading_journey_membership m
		JOIN books b ON b.owner_id=m.owner_id AND b.id=m.book_id AND b.language_state='chosen' AND b.language_tag=$2
		WHERE m.owner_id=$1 AND m.language=$2 ORDER BY m.position,m.created_at,m.book_id FOR UPDATE`, owner, language)
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
	return tx, revision, members, true, cleanup.RowsAffected() > 0, nil
}

func rewriteReadingJourneyPositions(ctx context.Context, tx pgx.Tx, owner, language string, members []readingJourneyMembership) error {
	// UPSERT: a member removed from the slice was already deleted by the
	// caller, so it is not re-inserted here; a freshly appended member (Add)
	// is inserted; all other members are re-positioned to stay contiguous.
	for position, member := range members {
		if _, err := tx.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id, language, book_id, position) VALUES($1, $2, $3, $4)
			ON CONFLICT (owner_id, language, book_id) DO UPDATE SET position = EXCLUDED.position`, owner, language, member.bookID, position+1); err != nil {
			return err
		}
	}
	return nil
}

func bumpReadingJourneyRevision(ctx context.Context, tx pgx.Tx, owner, language string) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `UPDATE reading_journeys SET revision=revision+1,updated_at=now() WHERE owner_id=$1 AND language=$2 RETURNING revision`, owner, language).Scan(&revision)
	return revision, err
}

// AddToReadingJourney appends a known owner book to the Journey.
func (s *PostgresStore) AddToReadingJourney(ctx context.Context, owner, language, bookID string, expectedRevision int64) (int64, error) {
	language = canonicalization.NormalizeLanguage(language)
	if language == "" {
		return 0, ErrJourneyLanguageRequired
	}
	tx, revision, members, _, cleaned, err := s.beginReadingJourneyMutation(ctx, owner, language, true)
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
	var languageState, bookLanguage string
	if err = tx.QueryRow(ctx, `SELECT language_state,COALESCE(language_tag,'') FROM books WHERE owner_id=$1 AND id=$2`, owner, bookID).Scan(&languageState, &bookLanguage); err != nil {
		return 0, err
	}
	if languageState != domain.LanguageChosen || bookLanguage != language {
		return 0, ErrBookLanguageRequired
	}
	for _, member := range members {
		if member.bookID == bookID {
			if cleaned {
				if err = rewriteReadingJourneyPositions(ctx, tx, owner, language, members); err != nil {
					return 0, err
				}
				if revision, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
					return 0, err
				}
				if err = tx.Commit(ctx); err != nil {
					return 0, err
				}
				return revision, nil
			}
			return revision, nil
		}
	}
	members = append(members, readingJourneyMembership{bookID: bookID})
	if err = rewriteReadingJourneyPositions(ctx, tx, owner, language, members); err != nil {
		return 0, err
	}
	if revision, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
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
func (s *PostgresStore) RemoveFromReadingJourney(ctx context.Context, owner, language, bookID string, expectedRevision int64) (int64, error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, revision, members, exists, cleaned, err := s.beginReadingJourneyMutation(ctx, owner, language, false)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if !exists {
		if expectedRevision != 0 {
			return 0, ErrJourneyStale
		}
		return 0, nil
	}
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
		if len(members) == 0 {
			var derived bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM books b
			JOIN book_membership bm ON bm.owner_id=b.owner_id AND bm.book_id=b.id AND bm.state='active'
			WHERE b.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2
		)`, owner, language).Scan(&derived); err != nil {
				return 0, err
			}
			if !derived {
				if _, err = tx.Exec(ctx, `DELETE FROM reading_journeys WHERE owner_id=$1 AND language=$2`, owner, language); err != nil {
					return 0, err
				}
				if err = tx.Commit(ctx); err != nil {
					return 0, err
				}
				return 0, nil
			}
			if cleaned {
				if err = rewriteReadingJourneyPositions(ctx, tx, owner, language, members); err != nil {
					return 0, err
				}
				if revision, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
					return 0, err
				}
				if err = tx.Commit(ctx); err != nil {
					return 0, err
				}
			}
		}
		return revision, nil
	}
	if _, err = tx.Exec(ctx, `DELETE FROM reading_journey_membership WHERE owner_id=$1 AND language=$2 AND book_id=$3`, owner, language, bookID); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM primary_goals WHERE owner_id=$1 AND book_id=$2`, owner, bookID); err != nil {
		return 0, err
	}
	members = append(members[:memberIndex], members[memberIndex+1:]...)
	if err = rewriteReadingJourneyPositions(ctx, tx, owner, language, members); err != nil {
		return 0, err
	}
	var derived bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM books b
		JOIN book_membership bm ON bm.owner_id=b.owner_id AND bm.book_id=b.id AND bm.state='active'
		WHERE b.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2
	)`, owner, language).Scan(&derived); err != nil {
		return 0, err
	}
	if len(members) == 0 && !derived {
		if _, err = tx.Exec(ctx, `DELETE FROM reading_journeys WHERE owner_id=$1 AND language=$2`, owner, language); err != nil {
			return 0, err
		}
		if err = tx.Commit(ctx); err != nil {
			return 0, err
		}
		return 0, nil
	}
	if revision, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
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
func (s *PostgresStore) MoveReadingJourneyEntry(ctx context.Context, owner, language, bookID string, newPosition int, expectedRevision int64) (int64, error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, revision, members, exists, cleaned, err := s.beginReadingJourneyMutation(ctx, owner, language, false)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if !exists {
		if expectedRevision != 0 {
			return 0, ErrJourneyStale
		}
		return 0, ErrNotFound
	}
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
		if cleaned {
			if err = rewriteReadingJourneyPositions(ctx, tx, owner, language, members); err != nil {
				return 0, err
			}
			if revision, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
				return 0, err
			}
			if err = tx.Commit(ctx); err != nil {
				return 0, err
			}
		}
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
	if err = rewriteReadingJourneyPositions(ctx, tx, owner, language, members); err != nil {
		return 0, err
	}
	if revision, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return revision, nil
}
