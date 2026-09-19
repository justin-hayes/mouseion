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

func primaryGoalFromValues(ownerID, language, bookID, snapshotID, sourceMaterialID, analysisRunID, contentRevisionID, contentSnapshotID, corpusID string, snapshotSize int, createdAt, updatedAt time.Time) domain.PrimaryGoal {
	return domain.PrimaryGoal{
		OwnerID: ownerID, Language: language, BookID: bookID, SnapshotID: snapshotID,
		SourceMaterialID: sourceMaterialID, AnalysisRunID: analysisRunID,
		ContentRevisionID: contentRevisionID, ContentSnapshotID: contentSnapshotID,
		CorpusID: corpusID, SnapshotSize: snapshotSize,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

func primaryGoalFromRow(row sqlcgen.GetPrimaryGoalRow, snapshotSize int) domain.PrimaryGoal {
	return primaryGoalFromValues(row.GOwnerID, row.Language, row.GBookID, row.SnapshotID, row.SourceMaterialID, row.AnalysisRunID, row.ContentRevisionID, row.ContentSnapshotID, row.CorpusID, snapshotSize, row.CreatedAt, row.UpdatedAt)
}

func listPrimaryGoalSnapshotVocabulary(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
	rows, err := q.ListPrimaryGoalSnapshotVocabulary(ctx, sqlcgen.ListPrimaryGoalSnapshotVocabularyParams{Owner: owner, Snapshot: snapshotID})
	if err != nil {
		return nil, err
	}
	result := make([]domain.SelectionCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, selectionCandidateFromFields(row.OwnerID, row.CorpusID, row.Language, row.CanonicalLemma, row.Upos, row.OccurrenceCount, row.ObservedForms, row.EligibleSentenceRefs, row.Provenance, row.SelectedAt, row.FirstEncounter))
	}
	return result, nil
}

func createPrimaryGoalSnapshot(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID string, identity sqlcgen.GetPrimaryGoalCandidateIdentityRow) (sqlcgen.CreatePrimaryGoalSnapshotRow, []domain.SelectionCandidate, error) {
	snapshot, err := q.CreatePrimaryGoalSnapshot(ctx, sqlcgen.CreatePrimaryGoalSnapshotParams{
		Owner: owner, Language: language, Book: bookID, SourceMaterial: identity.CaSourceMaterialID,
		AnalysisRun: identity.CaAnalysisRunID, ContentRevision: identity.CaContentRevisionID,
		ContentSnapshot: identity.CaSnapshotID, Corpus: identity.CaCorpusID,
	})
	if err != nil {
		return sqlcgen.CreatePrimaryGoalSnapshotRow{}, nil, err
	}
	rows, err := q.ListPrimaryGoalSnapshotCandidates(ctx, sqlcgen.ListPrimaryGoalSnapshotCandidatesParams{Owner: owner, Language: language, Corpus: identity.CaCorpusID})
	if err != nil {
		return sqlcgen.CreatePrimaryGoalSnapshotRow{}, nil, err
	}
	candidates := make([]domain.SelectionCandidate, 0, len(rows))
	for _, row := range rows {
		candidate := selectionCandidateFromFields(row.OwnerID, row.CorpusID, row.Language, row.CanonicalLemma, row.Upos, row.OccurrenceCount, row.ObservedForms, row.EligibleSentenceRefs, row.Provenance, row.SelectedAt, row.FirstEncounter)
		candidates = append(candidates, candidate)
		if err := q.InsertPrimaryGoalSnapshotVocabulary(ctx, sqlcgen.InsertPrimaryGoalSnapshotVocabularyParams{
			Owner: owner, Snapshot: snapshot.ID, Corpus: candidate.CorpusID, Language: candidate.Language,
			CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS, OccurrenceCount: candidate.OccurrenceCount,
			ObservedForms: candidate.ObservedForms, EligibleSentenceRefs: candidate.SentenceReferences,
			Provenance: candidate.Provenance, FirstEncounter: candidate.FirstEncounter, SelectedAt: candidate.SelectedAt,
		}); err != nil {
			return sqlcgen.CreatePrimaryGoalSnapshotRow{}, nil, err
		}
	}
	return snapshot, candidates, nil
}

func releasePrimaryGoalSnapshot(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) error {
	if snapshotID == "" {
		return nil
	}
	return q.ReleasePrimaryGoalSnapshot(ctx, sqlcgen.ReleasePrimaryGoalSnapshotParams{Owner: owner, Snapshot: snapshotID})
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
	snapshotSize, err := snapshotSizeForGoal(ctx, s.queries(), row.SnapshotID, row.GOwnerID)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	return primaryGoalFromRow(row, snapshotSize), nil
}

func snapshotSizeForGoal(ctx context.Context, q *sqlcgen.Queries, snapshotID, owner string) (int, error) {
	if snapshotID == "" {
		return 0, nil
	}
	rows, err := q.ListPrimaryGoalSnapshotVocabulary(ctx, sqlcgen.ListPrimaryGoalSnapshotVocabularyParams{Owner: owner, Snapshot: snapshotID})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
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
	identity, identityErr := q.GetPrimaryGoalCandidateIdentity(ctx, sqlcgen.GetPrimaryGoalCandidateIdentityParams{Owner: owner, Language: language, Book: bookID})
	if errors.Is(identityErr, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrGoalIneligible
	}
	if identityErr != nil {
		return domain.PrimaryGoal{}, identityErr
	}
	snapshot, candidates, snapshotErr := createPrimaryGoalSnapshot(ctx, q, owner, language, bookID, identity)
	if snapshotErr != nil {
		return domain.PrimaryGoal{}, snapshotErr
	}
	goal, err = insertPrimaryGoal(ctx, q, owner, language, bookID, snapshot.ID)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	goal.SourceMaterialID = identity.CaSourceMaterialID
	goal.AnalysisRunID = identity.CaAnalysisRunID
	goal.ContentRevisionID = identity.CaContentRevisionID
	goal.ContentSnapshotID = identity.CaSnapshotID
	goal.CorpusID = identity.CaCorpusID
	goal.SnapshotSize = len(candidates)
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

func insertPrimaryGoal(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID, snapshotID string) (domain.PrimaryGoal, error) {
	row, err := q.InsertPrimaryGoal(ctx, sqlcgen.InsertPrimaryGoalParams{Owner: owner, Language: language, Book: bookID, Snapshot: uuidArg(snapshotID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrGoalExists
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	return primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.SnapshotID, "", "", "", "", "", 0, row.CreatedAt, row.UpdatedAt), nil
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
	if current.GBookID != expectedBookID {
		return domain.PrimaryGoal{}, ErrGoalStale
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = ensurePrimaryGoalCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = releasePrimaryGoalSnapshot(ctx, q, owner, current.SnapshotID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	identity, err := q.GetPrimaryGoalCandidateIdentity(ctx, sqlcgen.GetPrimaryGoalCandidateIdentityParams{Owner: owner, Language: language, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrGoalIneligible
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	snapshot, candidates, err := createPrimaryGoalSnapshot(ctx, q, owner, language, bookID, identity)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	row, err := q.ChangePrimaryGoalBook(ctx, sqlcgen.ChangePrimaryGoalBookParams{Owner: owner, Language: language, Book: bookID, Snapshot: uuidArg(snapshot.ID)})
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	result = primaryGoalFromValues(row.OwnerID, row.Language, row.BookID, row.SnapshotID, identity.CaSourceMaterialID, identity.CaAnalysisRunID, identity.CaContentRevisionID, identity.CaSnapshotID, identity.CaCorpusID, len(candidates), row.CreatedAt, row.UpdatedAt)
	return result, nil
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
	if current.GBookID != expectedBookID {
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
	if err = releasePrimaryGoalSnapshot(ctx, q, owner, current.SnapshotID); err != nil {
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
	if current.GBookID != expectedBookID {
		return ErrGoalStale
	}
	if err = releasePrimaryGoalSnapshot(ctx, q, owner, current.SnapshotID); err != nil {
		return err
	}
	if err = q.DeletePrimaryGoal(ctx, sqlcgen.DeletePrimaryGoalParams{Owner: owner, Language: language}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
