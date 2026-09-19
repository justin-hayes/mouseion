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

func primaryGoalFromValues(ownerID, language, bookID string, createdAt, updatedAt time.Time) domain.PrimaryGoal {
	return domain.PrimaryGoal{
		OwnerID: ownerID, Language: language, BookID: bookID,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

// ReadingFinishResult is the webapp-facing result of recording reading.
type ReadingFinishResult struct {
	Completion domain.ReadingCompletion
}

func readingCompletionFromValues(ownerID, language, bookID string, completedAt time.Time) domain.ReadingCompletion {
	return domain.ReadingCompletion{OwnerID: ownerID, Language: language, BookID: bookID, CompletedAt: completedAt}
}

// GetPrimaryGoal returns the owner's current Goal in one study language. No
// row is the ordinary absent Goal state.
func (s *PostgresStore) GetPrimaryGoal(ctx context.Context, owner, language string) (domain.PrimaryGoal, error) {
	language = canonicalization.NormalizeLanguage(language)
	if language == "" {
		return domain.PrimaryGoal{}, nil
	}
	row, err := s.queries().GetPrimaryGoal(ctx, sqlcgen.GetPrimaryGoalParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, nil
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	return primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt), nil
}

// CreatePrimaryGoal creates the owner's current Goal for an analyzed Reading
// Journey member in language.
func (s *PostgresStore) CreatePrimaryGoal(ctx context.Context, owner, language, bookID string) (goal domain.PrimaryGoal, err error) {
	language = canonicalization.NormalizeLanguage(language)
	goalInput := domain.PrimaryGoal{OwnerID: owner, Language: language, BookID: bookID}
	if err := goalInput.Validate(); err != nil {
		return domain.PrimaryGoal{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	_, err = q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if err == nil {
		return domain.PrimaryGoal{}, ErrGoalExists
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, err
	}
	if err = ensurePrimaryGoalCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	goal, err = insertPrimaryGoal(ctx, q, owner, language, bookID)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	return goal, nil
}

func ensurePrimaryGoalCandidate(ctx context.Context, tx pgx.Tx, owner, language, bookID string) error {
	eligible, err := sqlcgen.New(tx).PrimaryGoalCandidateEligible(ctx, sqlcgen.PrimaryGoalCandidateEligibleParams{
		Owner: owner, Language: language, Book: bookID,
	})
	if err != nil {
		return err
	}
	if !eligible {
		return ErrGoalIneligible
	}
	return nil
}

func insertPrimaryGoal(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID string) (domain.PrimaryGoal, error) {
	row, err := q.InsertPrimaryGoal(ctx, sqlcgen.InsertPrimaryGoalParams{Owner: owner, Language: language, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrGoalExists
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	return primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt), nil
}

// ChangePrimaryGoal changes the language's Goal only when expectedBookID still
// names the current Goal, protecting callers from overwriting stale state.
func (s *PostgresStore) ChangePrimaryGoal(ctx context.Context, owner, language, bookID, expectedBookID string) (result domain.PrimaryGoal, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if err := (domain.PrimaryGoal{OwnerID: owner, Language: language, BookID: bookID}).Validate(); err != nil {
		return domain.PrimaryGoal{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrNotFound
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if current.BookID != expectedBookID {
		return domain.PrimaryGoal{}, ErrGoalStale
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = ensurePrimaryGoalCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	row, err := q.ChangePrimaryGoalBook(ctx, sqlcgen.ChangePrimaryGoalBookParams{Owner: owner, Language: language, Book: bookID})
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	return primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt), nil
}

// RecordReadingFinishedPrimaryGoal atomically appends the reading completion,
// removes the Book from the active Journey, and clears the current Goal.
// Vocabulary graduation is not part of this reading transition.
func (s *PostgresStore) RecordReadingFinishedPrimaryGoal(ctx context.Context, owner, language, expectedBookID string) (result ReadingFinishResult, err error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReadingFinishResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockReadingJourneyForCompletion(ctx, q, owner, language); err != nil {
		return ReadingFinishResult{}, err
	}

	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		row, completionErr := q.GetReadingCompletion(ctx, sqlcgen.GetReadingCompletionParams{Owner: owner, Language: language, Book: expectedBookID})
		if errors.Is(completionErr, pgx.ErrNoRows) {
			return ReadingFinishResult{}, ErrNotFound
		}
		if completionErr != nil {
			return ReadingFinishResult{}, completionErr
		}
		if err = tx.Commit(ctx); err != nil {
			return ReadingFinishResult{}, err
		}
		return ReadingFinishResult{Completion: readingCompletionFromValues(row.OwnerID, row.Language, row.BookID, row.CompletedAt)}, nil
	}
	if err != nil {
		return ReadingFinishResult{}, err
	}
	if current.BookID != expectedBookID {
		return ReadingFinishResult{}, ErrGoalStale
	}
	completedAt := time.Now().UTC()
	completionRow, insertErr := q.InsertReadingCompletion(ctx, sqlcgen.InsertReadingCompletionParams{
		Owner: owner, Language: language, Book: expectedBookID, CompletedAt: completedAt,
	})
	if errors.Is(insertErr, pgx.ErrNoRows) {
		existing, getErr := q.GetReadingCompletion(ctx, sqlcgen.GetReadingCompletionParams{Owner: owner, Language: language, Book: expectedBookID})
		if getErr != nil {
			return ReadingFinishResult{}, getErr
		}
		completionRow = sqlcgen.InsertReadingCompletionRow{
			OwnerID: existing.OwnerID, Language: existing.Language, BookID: existing.BookID, CompletedAt: existing.CompletedAt,
		}
	}
	if insertErr != nil {
		return ReadingFinishResult{}, insertErr
	}
	if err = removeCompletedGoalFromJourney(ctx, q, owner, language, expectedBookID); err != nil {
		return ReadingFinishResult{}, err
	}
	if err = q.DeletePrimaryGoal(ctx, sqlcgen.DeletePrimaryGoalParams{Owner: owner, Language: language}); err != nil {
		return ReadingFinishResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ReadingFinishResult{}, err
	}
	return ReadingFinishResult{Completion: readingCompletionFromValues(completionRow.OwnerID, completionRow.Language, completionRow.BookID, completionRow.CompletedAt)}, nil
}

func lockReadingJourneyForCompletion(ctx context.Context, q *sqlcgen.Queries, owner, language string) error {
	_, err := q.GetReadingJourneyRevisionForUpdate(ctx, sqlcgen.GetReadingJourneyRevisionForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func removeCompletedGoalFromJourney(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID string) error {
	_, err := q.GetReadingJourneyRevisionForUpdate(ctx, sqlcgen.GetReadingJourneyRevisionForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	members, err := q.ListAllReadingJourneyMembersForUpdate(ctx, sqlcgen.ListAllReadingJourneyMembersForUpdateParams{Owner: owner, Language: language})
	if err != nil {
		return err
	}
	memberIndex := -1
	journeyMembers := make([]readingJourneyMembership, 0, len(members))
	for _, member := range members {
		journeyMembers = append(journeyMembers, readingJourneyMembership{bookID: member.BookID, position: member.Position, createdAt: member.CreatedAt})
		if member.BookID == bookID {
			memberIndex = len(journeyMembers) - 1
		}
	}
	if memberIndex < 0 {
		return nil
	}
	if err = q.DeleteReadingJourneyMember(ctx, sqlcgen.DeleteReadingJourneyMemberParams{Owner: owner, Language: language, Book: bookID}); err != nil {
		return err
	}
	journeyMembers = append(journeyMembers[:memberIndex], journeyMembers[memberIndex+1:]...)
	if err = rewriteReadingJourneyPositions(ctx, q, owner, language, journeyMembers); err != nil {
		return err
	}
	derived, err := q.DerivedJourneyBooksExist(ctx, sqlcgen.DerivedJourneyBooksExistParams{Owner: owner, Language: language})
	if err != nil {
		return err
	}
	if len(journeyMembers) == 0 && !derived {
		return q.DeleteReadingJourney(ctx, sqlcgen.DeleteReadingJourneyParams{Owner: owner, Language: language})
	}
	_, err = q.BumpReadingJourneyRevision(ctx, sqlcgen.BumpReadingJourneyRevisionParams{Owner: owner, Language: language})
	return err
}

// ClearPrimaryGoal removes the language's Goal only when expectedBookID still
// names the current Goal.
func (s *PostgresStore) ClearPrimaryGoal(ctx context.Context, owner, language, expectedBookID string) (err error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current.BookID != expectedBookID {
		return ErrGoalStale
	}
	if err = q.DeletePrimaryGoal(ctx, sqlcgen.DeletePrimaryGoalParams{Owner: owner, Language: language}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
