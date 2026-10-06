package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
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

func createPrimaryGoalSnapshot(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language, bookID string, identity sqlcgen.GetPrimaryGoalCandidateIdentityRow) (sqlcgen.CreatePrimaryGoalSnapshotRow, []domain.SelectionCandidate, error) {
	snapshot, err := q.CreatePrimaryGoalSnapshot(ctx, sqlcgen.CreatePrimaryGoalSnapshotParams{
		Owner: owner, Language: language, Book: bookID, SourceMaterial: identity.CaSourceMaterialID,
		AnalysisRun: identity.CaAnalysisRunID, ContentRevision: identity.CaContentRevisionID,
		ContentSnapshot: identity.CaSnapshotID, Corpus: identity.CaCorpusID,
	})
	if err != nil {
		return sqlcgen.CreatePrimaryGoalSnapshotRow{}, nil, err
	}
	hasCorrections, err := q.HasCurrentLemmaCorrections(ctx, sqlcgen.HasCurrentLemmaCorrectionsParams{Owner: owner, Book: bookID})
	if err != nil {
		return sqlcgen.CreatePrimaryGoalSnapshotRow{}, nil, err
	}
	var candidates []domain.SelectionCandidate
	if hasCorrections {
		candidates, err = correctedPrimaryGoalCandidates(ctx, tx, q, owner, bookID, language, identity)
	} else {
		var rows []sqlcgen.ListPrimaryGoalSnapshotCandidatesRow
		rows, err = q.ListPrimaryGoalSnapshotCandidates(ctx, sqlcgen.ListPrimaryGoalSnapshotCandidatesParams{Owner: owner, Language: language, Corpus: identity.CaCorpusID})
		if err == nil {
			candidates = make([]domain.SelectionCandidate, 0, len(rows))
			for _, row := range rows {
				candidates = append(candidates, selectionCandidateFromFields(row.OwnerID, row.CorpusID, row.Language, row.CanonicalLemma, row.Upos, row.OccurrenceCount, row.ObservedForms, row.EligibleSentenceRefs, row.Provenance, row.SelectedAt, row.FirstEncounter))
			}
		}
	}
	if err != nil {
		return sqlcgen.CreatePrimaryGoalSnapshotRow{}, nil, err
	}
	for _, candidate := range candidates {
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

// correctedPrimaryGoalCandidates rebuilds the current Book's candidate set
// from immutable analyzer evidence plus exact-occurrence learner corrections.
// It runs in the same transaction as the freeze, after the Book row is locked.
func correctedPrimaryGoalCandidates(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, bookID, language string, identity sqlcgen.GetPrimaryGoalCandidateIdentityRow) ([]domain.SelectionCandidate, error) {
	analysis, corrections, err := loadAnalysisProjectionFactsTx(ctx, tx, q, domain.DeckPreparation{
		OwnerID: owner, SourceMaterialID: identity.CaSourceMaterialID, AnalysisRunID: identity.CaAnalysisRunID,
	}, identity.CaCorpusID)
	if err != nil {
		return nil, err
	}
	decisions := make([]selection.OccurrenceDecision, 0, len(corrections))
	for _, correction := range corrections {
		start, startErr := checked.Uint64FromInt64(correction.StartOffset)
		end, endErr := checked.Uint64FromInt64(correction.EndOffset)
		if startErr != nil || endErr != nil {
			return nil, errors.New("persisted lemma correction contains invalid offsets")
		}
		decisions = append(decisions, selection.OccurrenceDecision{
			Occurrence: selection.OccurrenceIdentity{SourceDocumentID: correction.SourceDocumentID, StartOffset: start, EndOffset: end},
			Lemma:      correction.CanonicalLemma,
			Excluded:   correction.Excluded,
		})
	}
	projected, err := selection.Project(analysis, selection.DefaultConfig(identity.CaCorpusID), decisions)
	if err != nil {
		return nil, err
	}
	reservedVocabulary, err := listReservedVocabulary(ctx, tx, owner, language)
	if err != nil {
		return nil, err
	}
	candidates := make([]domain.SelectionCandidate, 0, len(projected))
	for _, candidate := range projected {
		if candidate.OccurrenceCount < 3 {
			continue
		}
		known, err := q.IsKnownVocabularyIdentity(ctx, sqlcgen.IsKnownVocabularyIdentityParams{
			OwnerID: owner, Language: language, CanonicalLemma: candidate.Identity.CanonicalLemma, Upos: candidate.Identity.UPOS,
		})
		if err != nil {
			return nil, err
		}
		if known {
			continue
		}
		reservedByDeck := false
		for _, reserved := range reservedVocabulary {
			if reserved.Language == language && reserved.CanonicalLemma == candidate.Identity.CanonicalLemma && (reserved.UPOS == candidate.Identity.UPOS || reserved.UPOS == "") {
				reservedByDeck = true
				break
			}
		}
		if reservedByDeck {
			continue
		}
		reserved, err := q.IsCurrentReadingVocabularyReserved(ctx, sqlcgen.IsCurrentReadingVocabularyReservedParams{
			Owner: owner, Language: language, CanonicalLemma: candidate.Identity.CanonicalLemma, Upos: candidate.Identity.UPOS,
		})
		if err != nil {
			return nil, err
		}
		if reserved {
			continue
		}
		forms, err := json.Marshal(candidate.ObservedForms)
		if err != nil {
			return nil, err
		}
		refs, err := json.Marshal(candidate.SentenceReferences)
		if err != nil {
			return nil, err
		}
		provenance, err := json.Marshal(candidate.Provenance)
		if err != nil {
			return nil, err
		}
		first := int64(^uint64(0) >> 1)
		for _, ref := range candidate.SentenceReferences {
			encounter, convertErr := checked.Int64FromUint64(ref.Location.StartOffset)
			if convertErr != nil {
				return nil, errors.New("projected occurrence offset exceeds persisted range")
			}
			if encounter < first {
				first = encounter
			}
		}
		candidates = append(candidates, domain.SelectionCandidate{
			OwnerID: owner, CorpusID: identity.CaCorpusID, Language: language,
			CanonicalLemma: candidate.Identity.CanonicalLemma, UPOS: candidate.Identity.UPOS,
			OccurrenceCount: candidate.OccurrenceCount, FirstEncounter: first,
			ObservedForms: forms, SentenceReferences: refs, Provenance: provenance, SelectedAt: time.Now().UTC(),
		})
	}
	return candidates, nil
}

func releasePrimaryGoalSnapshot(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) error {
	if snapshotID == "" {
		return nil
	}
	if _, err := q.LockPrimaryGoalSnapshot(ctx, sqlcgen.LockPrimaryGoalSnapshotParams{Owner: owner, Snapshot: snapshotID}); err != nil {
		return err
	}
	return q.ReleasePrimaryGoalSnapshot(ctx, sqlcgen.ReleasePrimaryGoalSnapshotParams{Owner: owner, Snapshot: snapshotID})
}

func releasePrimaryGoalSnapshots(ctx context.Context, q *sqlcgen.Queries, owner string, snapshotIDs []string) error {
	for _, snapshotID := range snapshotIDs {
		if err := releasePrimaryGoalSnapshot(ctx, q, owner, snapshotID); err != nil {
			return err
		}
	}
	return nil
}

// ReadingFinishResult is retained for the existing Goal handlers.
type ReadingFinishResult struct {
	Completion domain.ReadingCompletion
}

func readingCompletionFromValues(ownerID, language, bookID string, completedAt time.Time, snapshotID string, snapshotCount, eligibleCount, graduatedCount, alreadyKnownCount int) domain.ReadingCompletion {
	return domain.ReadingCompletion{
		OwnerID: ownerID, Language: language, BookID: bookID, CompletedAt: completedAt,
		GoalSnapshotID: snapshotID, SnapshotVocabularyCount: snapshotCount,
		EligibleVocabularyCount: eligibleCount, GraduatedVocabularyCount: graduatedCount,
		AlreadyKnownVocabularyCount: alreadyKnownCount,
		Source:                      domain.ReadingCompletionPrimaryGoal,
	}
}

// ImportPreviouslyRead records one owner-and-Book-scoped completion without
// creating vocabulary evidence. The partial unique index makes concurrent
// retries converge on the same imported fact.
func (s *PostgresStore) ImportPreviouslyRead(ctx context.Context, owner, bookID string) (domain.ReadingCompletion, error) {
	row, err := s.queries().InsertPreviouslyReadImport(ctx, sqlcgen.InsertPreviouslyReadImportParams{
		Owner: owner, Book: bookID, CompletedAt: time.Now().UTC(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := s.queries().GetPreviouslyReadImport(ctx, sqlcgen.GetPreviouslyReadImportParams{Owner: owner, Book: bookID})
		if getErr != nil {
			return domain.ReadingCompletion{}, missing(getErr)
		}
		return importedReadingCompletion(existing.OwnerID, existing.Language, existing.BookID, existing.CompletedAt, existing.GoalSnapshotID, existing.SnapshotVocabularyCount, existing.EligibleVocabularyCount, existing.GraduatedVocabularyCount, existing.AlreadyKnownVocabularyCount), nil
	}
	if err != nil {
		return domain.ReadingCompletion{}, missing(err)
	}
	return importedReadingCompletion(row.OwnerID, row.Language, row.BookID, row.CompletedAt, row.GoalSnapshotID, row.SnapshotVocabularyCount, row.EligibleVocabularyCount, row.GraduatedVocabularyCount, row.AlreadyKnownVocabularyCount), nil
}

func importedReadingCompletion(owner, language, book string, completedAt time.Time, snapshotID string, snapshotCount, eligibleCount, graduatedCount, knownCount int) domain.ReadingCompletion {
	return domain.ReadingCompletion{
		OwnerID: owner, Language: language, BookID: book, CompletedAt: completedAt,
		GoalSnapshotID: snapshotID, SnapshotVocabularyCount: snapshotCount,
		EligibleVocabularyCount: eligibleCount, GraduatedVocabularyCount: graduatedCount,
		AlreadyKnownVocabularyCount: knownCount, Source: domain.ReadingCompletionPreviouslyRead,
	}
}

func getReadingCompletion(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID, snapshotID string) (sqlcgen.GetReadingCompletionRow, error) {
	return q.GetReadingCompletion(ctx, sqlcgen.GetReadingCompletionParams{Owner: owner, Language: language, Book: bookID, GoalSnapshot: snapshotID})
}

// CountPrimaryGoalVocabularyToGraduate reports the currently eligible frozen
// identities. It is deliberately read-only so the confirmation can state the
// exact modeled consequence before the learner accepts completion.
func (s *PostgresStore) CountPrimaryGoalVocabularyToGraduate(ctx context.Context, owner, language string) (int, error) {
	goal, err := s.GetPrimaryGoal(ctx, owner, language)
	if err != nil {
		return 0, err
	}
	if !goal.IsActive() || goal.SnapshotID == "" {
		return 0, nil
	}
	counts, err := s.queries().CountPrimaryGoalSnapshotVocabulary(ctx, sqlcgen.CountPrimaryGoalSnapshotVocabularyParams{Owner: owner, Snapshot: goal.SnapshotID})
	if err != nil {
		return 0, err
	}
	return counts.EligibleCount, nil
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

// ListPrimaryGoalSnapshotVocabulary reads one immutable Goal snapshot without
// changing any learner state.
func (s *PostgresStore) ListPrimaryGoalSnapshotVocabulary(ctx context.Context, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
	if snapshotID == "" {
		return nil, nil
	}
	return listPrimaryGoalSnapshotVocabulary(ctx, s.queries(), owner, snapshotID)
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

// CreatePrimaryGoal creates the owner's current reading for an eligible To Read
// Book in language.
func (s *PostgresStore) CreatePrimaryGoal(ctx context.Context, owner, language, bookID string) (goal domain.PrimaryGoal, err error) {
	return s.CreatePrimaryGoalWith(ctx, owner, language, bookID, nil)
}

// CreatePrimaryGoalWith creates the current reading and runs beforeCommit in
// the same transaction after its immutable snapshot has been populated. The
// callback can atomically attach durable work that depends on that snapshot.
func (s *PostgresStore) CreatePrimaryGoalWith(ctx context.Context, owner, language, bookID string, beforeCommit func(context.Context, pgx.Tx, domain.PrimaryGoal) error) (goal domain.PrimaryGoal, err error) {
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
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = lockPrimaryGoalBook(ctx, q, owner, bookID); err != nil {
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
	blocked, err := unresolvedLemmaReviewFlags(ctx, tx, owner, bookID, identity.CaAnalysisRunID)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if blocked {
		return domain.PrimaryGoal{}, ErrUnresolvedLemmaReviewFlags
	}
	snapshot, candidates, snapshotErr := createPrimaryGoalSnapshot(ctx, tx, q, owner, language, bookID, identity)
	if snapshotErr != nil {
		return domain.PrimaryGoal{}, snapshotErr
	}
	goal, err = insertPrimaryGoal(ctx, q, owner, language, bookID, snapshot.ID)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	goal.SourceMaterialID = identity.CaSourceMaterialID
	goal.AnalysisRunID = identity.CaAnalysisRunID
	goal.ContentRevisionID = identity.CaContentRevisionID
	goal.ContentSnapshotID = identity.CaSnapshotID
	goal.CorpusID = identity.CaCorpusID
	goal.SnapshotSize = len(candidates)
	if beforeCommit != nil {
		if err = beforeCommit(ctx, tx, goal); err != nil {
			return domain.PrimaryGoal{}, err
		}
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

func lockPrimaryGoalBook(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) error {
	_, err := q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
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
	return s.changePrimaryGoal(ctx, owner, language, bookID, expectedBookID, "", false)
}

func (s *PostgresStore) changePrimaryGoal(ctx context.Context, owner, language, bookID, expectedBookID, expectedSnapshotID string, idempotent bool) (result domain.PrimaryGoal, err error) {
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
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = lockPrimaryGoalBook(ctx, q, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrNotFound
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if idempotent && current.GBookID == bookID && current.GBookID != expectedBookID {
		snapshotSize, sizeErr := snapshotSizeForGoal(ctx, q, current.SnapshotID, owner)
		if sizeErr != nil {
			return domain.PrimaryGoal{}, sizeErr
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.PrimaryGoal{}, err
		}
		return primaryGoalFromValues(current.GOwnerID, current.Language, current.GBookID, current.SnapshotID, current.SourceMaterialID, current.AnalysisRunID, current.ContentRevisionID, current.ContentSnapshotID, current.CorpusID, snapshotSize, current.CreatedAt, current.UpdatedAt), nil
	}
	if current.GBookID != expectedBookID {
		return domain.PrimaryGoal{}, ErrGoalStale
	}
	if expectedSnapshotID != "" && current.SnapshotID != expectedSnapshotID {
		return domain.PrimaryGoal{}, ErrGoalStale
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
	blocked, err := unresolvedLemmaReviewFlags(ctx, tx, owner, bookID, identity.CaAnalysisRunID)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if blocked {
		return domain.PrimaryGoal{}, ErrUnresolvedLemmaReviewFlags
	}
	snapshot, candidates, err := createPrimaryGoalSnapshot(ctx, tx, q, owner, language, bookID, identity)
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

// RecordReadingFinishedPrimaryGoal atomically records completion, graduates the
// frozen snapshot, sets the Book aside, and clears the current reading. The
// snapshot is the completion request identity, so retries remain idempotent
// even after the Book is completed again in the future.
func (s *PostgresStore) RecordReadingFinishedPrimaryGoal(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) (result ReadingFinishResult, err error) {
	language = canonicalization.NormalizeLanguage(language)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReadingFinishResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return ReadingFinishResult{}, err
	}
	current, err := q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		row, completionErr := getReadingCompletion(ctx, q, owner, language, expectedBookID, expectedSnapshotID)
		if errors.Is(completionErr, pgx.ErrNoRows) {
			return ReadingFinishResult{}, ErrNotFound
		}
		if completionErr != nil {
			return ReadingFinishResult{}, completionErr
		}
		if row.BookID != expectedBookID {
			return ReadingFinishResult{}, ErrGoalStale
		}
		if err = tx.Commit(ctx); err != nil {
			return ReadingFinishResult{}, err
		}
		return ReadingFinishResult{Completion: readingCompletionFromValues(row.OwnerID, row.Language, row.BookID, row.CompletedAt, row.GoalSnapshotID, row.SnapshotVocabularyCount, row.EligibleVocabularyCount, row.GraduatedVocabularyCount, row.AlreadyKnownVocabularyCount)}, nil
	}
	if err != nil {
		return ReadingFinishResult{}, err
	}
	if current.GBookID != expectedBookID || current.SnapshotID != expectedSnapshotID {
		return ReadingFinishResult{}, ErrGoalStale
	}
	counts := sqlcgen.CountPrimaryGoalSnapshotVocabularyRow{}
	if current.SnapshotID != "" {
		counts, err = q.CountPrimaryGoalSnapshotVocabulary(ctx, sqlcgen.CountPrimaryGoalSnapshotVocabularyParams{Owner: owner, Snapshot: current.SnapshotID})
		if err != nil {
			return ReadingFinishResult{}, err
		}
	}
	completedAt := time.Now().UTC()
	completionRow, insertErr := q.InsertReadingCompletion(ctx, sqlcgen.InsertReadingCompletionParams{
		Owner: owner, Language: language, Book: expectedBookID, CompletedAt: completedAt,
		GoalSnapshot: expectedSnapshotID, SnapshotVocabularyCount: counts.SnapshotCount,
		EligibleVocabularyCount: counts.EligibleCount, GraduatedVocabularyCount: 0,
		AlreadyKnownVocabularyCount: counts.SnapshotCount - counts.EligibleCount,
	})
	inserted := insertErr == nil
	if errors.Is(insertErr, pgx.ErrNoRows) {
		existing, getErr := getReadingCompletion(ctx, q, owner, language, expectedBookID, expectedSnapshotID)
		if getErr != nil {
			return ReadingFinishResult{}, getErr
		}
		completionRow = sqlcgen.InsertReadingCompletionRow{
			OwnerID: existing.OwnerID, Language: existing.Language, BookID: existing.BookID, CompletedAt: existing.CompletedAt,
			GoalSnapshotID: existing.GoalSnapshotID, SnapshotVocabularyCount: existing.SnapshotVocabularyCount,
			EligibleVocabularyCount: existing.EligibleVocabularyCount, GraduatedVocabularyCount: existing.GraduatedVocabularyCount,
			AlreadyKnownVocabularyCount: existing.AlreadyKnownVocabularyCount,
		}
	}
	if insertErr != nil {
		return ReadingFinishResult{}, insertErr
	}
	graduatedCount := completionRow.GraduatedVocabularyCount
	alreadyKnownCount := completionRow.AlreadyKnownVocabularyCount
	if inserted && current.SnapshotID != "" {
		graduatedCount, err = q.GraduatePrimaryGoalSnapshotVocabulary(ctx, sqlcgen.GraduatePrimaryGoalSnapshotVocabularyParams{
			Owner: owner, Snapshot: current.SnapshotID, CompletedAt: completedAt,
		})
		if err != nil {
			return ReadingFinishResult{}, err
		}
		if err = q.UpdateReadingCompletionOutcome(ctx, sqlcgen.UpdateReadingCompletionOutcomeParams{
			Owner: owner, Language: language, GoalSnapshot: expectedSnapshotID,
			GraduatedVocabularyCount: graduatedCount, AlreadyKnownVocabularyCount: counts.SnapshotCount - graduatedCount,
		}); err != nil {
			return ReadingFinishResult{}, err
		}
		alreadyKnownCount = counts.SnapshotCount - graduatedCount
	}
	if err = upsertBookDisposition(ctx, q, owner, expectedBookID, domain.BookDispositionSetAside); err != nil {
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
	return ReadingFinishResult{Completion: readingCompletionFromValues(completionRow.OwnerID, completionRow.Language, completionRow.BookID, completionRow.CompletedAt, completionRow.GoalSnapshotID, completionRow.SnapshotVocabularyCount, completionRow.EligibleVocabularyCount, graduatedCount, alreadyKnownCount)}, nil
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
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return err
	}
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
