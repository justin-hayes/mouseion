package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
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
	row, err := s.queries().GetReadingJourney(ctx, sqlcgen.GetReadingJourneyParams{Owner: owner, Language: language})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return journey, nil
		}
		return journey, err
	}
	journey.Revision = row.Revision
	journey.UpdatedAt = row.UpdatedAt

	members, err := s.queries().ListReadingJourneyMembers(ctx, sqlcgen.ListReadingJourneyMembersParams{Owner: owner, Language: language})
	if err != nil {
		return journey, err
	}
	for _, member := range members {
		entry := domain.ReadingJourneyEntry{OwnerID: owner, Language: language}
		entry.BookID = member.BookID
		entry.Position = member.Position
		entry.CreatedAt = member.CreatedAt
		journey.Entries = append(journey.Entries, entry)
	}
	return journey, nil
}

func (s *PostgresStore) beginReadingJourneyMutation(ctx context.Context, owner, language string, create bool) (tx pgx.Tx, revision int64, members []readingJourneyMembership, exists, cleaned bool, err error) {
	tx, err = s.pool.Begin(ctx)
	if err != nil {
		return nil, 0, nil, false, false, err
	}
	rollback := func(cause error) (pgx.Tx, int64, []readingJourneyMembership, bool, bool, error) {
		return nil, 0, nil, false, false, errors.Join(cause, txcleanup.Rollback(ctx, tx))
	}
	if create {
		if err = sqlcgen.New(tx).InsertReadingJourneyIfAbsent(ctx, sqlcgen.InsertReadingJourneyIfAbsentParams{Owner: owner, Language: language}); err != nil {
			return rollback(err)
		}
	}
	revision, err = sqlcgen.New(tx).GetReadingJourneyRevisionForUpdate(ctx, sqlcgen.GetReadingJourneyRevisionForUpdateParams{Owner: owner, Language: language})
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
	cleanupRows, err := sqlcgen.New(tx).DeleteNonChosenJourneyMembers(ctx, sqlcgen.DeleteNonChosenJourneyMembersParams{Owner: owner, Language: language})
	if err != nil {
		return rollback(err)
	}
	for _, bookID := range cleanupRows {
		if err = synchronizeBookDisposition(ctx, sqlcgen.New(tx), owner, bookID, domain.BookDispositionSetAside); err != nil {
			return rollback(err)
		}
	}
	rows, err := sqlcgen.New(tx).ListReadingJourneyMembersForUpdate(ctx, sqlcgen.ListReadingJourneyMembersForUpdateParams{Owner: owner, Language: language})
	if err != nil {
		return rollback(err)
	}
	for _, row := range rows {
		members = append(members, readingJourneyMembership{bookID: row.BookID, position: row.Position, createdAt: row.CreatedAt})
	}
	return tx, revision, members, true, len(cleanupRows) > 0, nil
}

func rewriteReadingJourneyPositions(ctx context.Context, q interface {
	UpsertReadingJourneyMemberPosition(context.Context, sqlcgen.UpsertReadingJourneyMemberPositionParams) error
}, owner, language string, members []readingJourneyMembership) error {
	// UPSERT: a member removed from the slice was already deleted by the
	// caller, so it is not re-inserted here; a freshly appended member (Add)
	// is inserted; all other members are re-positioned to stay contiguous.
	for position, member := range members {
		if err := q.UpsertReadingJourneyMemberPosition(ctx, sqlcgen.UpsertReadingJourneyMemberPositionParams{
			Owner: owner, Language: language, Book: member.bookID, Position: position + 1,
		}); err != nil {
			return err
		}
	}
	return nil
}

func bumpReadingJourneyRevision(ctx context.Context, tx pgx.Tx, owner, language string) (int64, error) {
	return sqlcgen.New(tx).BumpReadingJourneyRevision(ctx, sqlcgen.BumpReadingJourneyRevisionParams{Owner: owner, Language: language})
}

func validateReadingJourneyMove(exists bool, revision, expectedRevision int64, members []readingJourneyMembership, bookID string) (int, error) {
	if !exists {
		if expectedRevision != 0 {
			return -1, ErrJourneyStale
		}
		return -1, ErrNotFound
	}
	if expectedRevision != revision {
		return -1, ErrJourneyStale
	}
	for index, member := range members {
		if member.bookID == bookID {
			return index, nil
		}
	}
	return -1, ErrNotFound
}

func readingJourneyGoalBookID(ctx context.Context, tx pgx.Tx, owner, language string) (string, error) {
	goalBookID, err := sqlcgen.New(tx).GetPrimaryGoalBookID(ctx, sqlcgen.GetPrimaryGoalBookIDParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return goalBookID, err
}

func clampReadingJourneyPosition(position, length int) int {
	if position < 1 {
		return 1
	}
	if position > length {
		return length
	}
	return position
}

func moveReadingJourneySlice(members []readingJourneyMembership, memberIndex, newPosition int) []readingJourneyMembership {
	member := members[memberIndex]
	members = append(members[:memberIndex], members[memberIndex+1:]...)
	members = append(members, readingJourneyMembership{})
	copy(members[newPosition:], members[newPosition-1:])
	members[newPosition-1] = member
	return members
}

func visibleReadingJourneyMembers(members []readingJourneyMembership, memberIndex int, goalBookID string) ([]readingJourneyMembership, int, int) {
	goalIndex := -1
	visibleIndex := -1
	visible := make([]readingJourneyMembership, 0, len(members)-1)
	for index, member := range members {
		if member.bookID == goalBookID {
			goalIndex = index
			continue
		}
		if index == memberIndex {
			visibleIndex = len(visible)
		}
		visible = append(visible, member)
	}
	return visible, goalIndex, visibleIndex
}

func restoreReadingJourneyGoal(visible []readingJourneyMembership, goalIndex int, goalBookID string) []readingJourneyMembership {
	members := make([]readingJourneyMembership, 0, len(visible)+1)
	visibleIndex := 0
	for index := range len(visible) + 1 {
		if index == goalIndex {
			members = append(members, readingJourneyMembership{bookID: goalBookID})
			continue
		}
		members = append(members, visible[visibleIndex])
		visibleIndex++
	}
	return members
}

func moveReadingJourneyMembers(members []readingJourneyMembership, memberIndex int, bookID, goalBookID string, newPosition int) ([]readingJourneyMembership, bool) {
	if goalBookID != "" && goalBookID != bookID {
		visible, goalIndex, visibleIndex := visibleReadingJourneyMembers(members, memberIndex, goalBookID)
		if goalIndex >= 0 {
			newPosition = clampReadingJourneyPosition(newPosition, len(visible))
			if visibleIndex == newPosition-1 {
				return members, false
			}
			visible = moveReadingJourneySlice(visible, visibleIndex, newPosition)
			return restoreReadingJourneyGoal(visible, goalIndex, goalBookID), true
		}
	}

	newPosition = clampReadingJourneyPosition(newPosition, len(members))
	if memberIndex == newPosition-1 {
		return members, false
	}
	return moveReadingJourneySlice(members, memberIndex, newPosition), true
}

func (s *PostgresStore) commitReadingJourneyMove(ctx context.Context, tx pgx.Tx, owner, language string, members []readingJourneyMembership) (int64, error) {
	if err := rewriteReadingJourneyPositions(ctx, sqlcgen.New(tx), owner, language, members); err != nil {
		return 0, err
	}
	revision, err := bumpReadingJourneyRevision(ctx, tx, owner, language)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return revision, nil
}

// AddToReadingJourney appends a known owner book to the Journey.
func (s *PostgresStore) AddToReadingJourney(ctx context.Context, owner, language, bookID string, expectedRevision int64) (result int64, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if language == "" {
		return 0, ErrJourneyLanguageRequired
	}
	tx, revision, members, _, cleaned, err := s.beginReadingJourneyMutation(ctx, owner, language, true)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if expectedRevision != revision {
		return 0, ErrJourneyStale
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return 0, err
	}
	book, err := sqlcgen.New(tx).GetBookLanguageState(ctx, sqlcgen.GetBookLanguageStateParams{Owner: owner, Book: bookID})
	if err != nil {
		return 0, err
	}
	if book.LanguageState != domain.LanguageChosen || book.LanguageTag != language {
		return 0, ErrBookLanguageRequired
	}
	for _, member := range members {
		if member.bookID == bookID {
			if err = synchronizeBookDisposition(ctx, sqlcgen.New(tx), owner, bookID, domain.BookDispositionToRead); err != nil {
				return 0, err
			}
			if cleaned {
				if err = rewriteReadingJourneyPositions(ctx, sqlcgen.New(tx), owner, language, members); err != nil {
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
	if err = synchronizeBookDisposition(ctx, sqlcgen.New(tx), owner, bookID, domain.BookDispositionToRead); err != nil {
		return 0, err
	}
	members = append(members, readingJourneyMembership{bookID: bookID})
	if err = rewriteReadingJourneyPositions(ctx, sqlcgen.New(tx), owner, language, members); err != nil {
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
	exists, err := s.queries().BookExists(ctx, sqlcgen.BookExistsParams{Owner: owner, Book: id})
	if err != nil {
		return "", false, err
	}
	if exists {
		return id, true, nil
	}
	linked, err := s.queries().ResolveJourneyLinkedBook(ctx, sqlcgen.ResolveJourneyLinkedBookParams{
		Owner: owner, Source: id, Namespace: domain.NamespaceSourceIdentifier,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if linked == "" {
		return "", false, nil
	}
	return linked, true, nil
}

// RemoveFromReadingJourney removes a Journey member and compacts positions.
func (s *PostgresStore) RemoveFromReadingJourney(ctx context.Context, owner, language, bookID string, expectedRevision int64) (result int64, err error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, revision, members, exists, cleaned, err := s.beginReadingJourneyMutation(ctx, owner, language, false)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
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
			derived, queryErr := sqlcgen.New(tx).DerivedJourneyBooksExist(ctx, sqlcgen.DerivedJourneyBooksExistParams{Owner: owner, Language: language})
			if queryErr != nil {
				return 0, queryErr
			}
			if !derived {
				if err = sqlcgen.New(tx).DeleteReadingJourney(ctx, sqlcgen.DeleteReadingJourneyParams{Owner: owner, Language: language}); err != nil {
					return 0, err
				}
				if err = tx.Commit(ctx); err != nil {
					return 0, err
				}
				return 0, nil
			}
		}
		if cleaned {
			if err = rewriteReadingJourneyPositions(ctx, sqlcgen.New(tx), owner, language, members); err != nil {
				return 0, err
			}
			if revision, err = bumpReadingJourneyRevision(ctx, tx, owner, language); err != nil {
				return 0, err
			}
			if err = tx.Commit(ctx); err != nil {
				return 0, err
			}
		}
		return revision, nil
	}
	q := sqlcgen.New(tx)
	if err = q.DeleteReadingJourneyMember(ctx, sqlcgen.DeleteReadingJourneyMemberParams{Owner: owner, Language: language, Book: bookID}); err != nil {
		return 0, err
	}
	snapshotIDs, err := q.LockPrimaryGoalsForBook(ctx, sqlcgen.LockPrimaryGoalsForBookParams{Owner: owner, Language: language, Book: bookID})
	if err != nil {
		return 0, err
	}
	if err = releasePrimaryGoalSnapshots(ctx, q, owner, snapshotIDs); err != nil {
		return 0, err
	}
	if err = q.DeletePrimaryGoalForBook(ctx, sqlcgen.DeletePrimaryGoalForBookParams{Owner: owner, Language: language, Book: bookID}); err != nil {
		return 0, err
	}
	if err = synchronizeBookDisposition(ctx, q, owner, bookID, domain.BookDispositionSetAside); err != nil {
		return 0, err
	}
	members = append(members[:memberIndex], members[memberIndex+1:]...)
	if err = rewriteReadingJourneyPositions(ctx, sqlcgen.New(tx), owner, language, members); err != nil {
		return 0, err
	}
	derived, err := q.DerivedJourneyBooksExist(ctx, sqlcgen.DerivedJourneyBooksExistParams{Owner: owner, Language: language})
	if err != nil {
		return 0, err
	}
	if len(members) == 0 && !derived {
		if err = q.DeleteReadingJourney(ctx, sqlcgen.DeleteReadingJourneyParams{Owner: owner, Language: language}); err != nil {
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
func (s *PostgresStore) MoveReadingJourneyEntry(ctx context.Context, owner, language, bookID string, newPosition int, expectedRevision int64) (result int64, err error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, revision, members, exists, cleaned, err := s.beginReadingJourneyMutation(ctx, owner, language, false)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	memberIndex, err := validateReadingJourneyMove(exists, revision, expectedRevision, members, bookID)
	if err != nil {
		if cleaned && errors.Is(err, ErrNotFound) {
			if _, cleanupErr := s.commitReadingJourneyMove(ctx, tx, owner, language, members); cleanupErr != nil {
				return 0, cleanupErr
			}
		}
		return 0, err
	}
	goalBookID, err := readingJourneyGoalBookID(ctx, tx, owner, language)
	if err != nil {
		return 0, err
	}
	members, changed := moveReadingJourneyMembers(members, memberIndex, bookID, goalBookID, newPosition)
	if !changed {
		return revision, nil
	}
	return s.commitReadingJourneyMove(ctx, tx, owner, language, members)
}
