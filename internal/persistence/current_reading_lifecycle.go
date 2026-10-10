package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

func currentReadingFromValues(ownerID, language, bookID, snapshotID, sourceMaterialID, analysisRunID, contentRevisionID, contentSnapshotID, corpusID string, snapshotSize int, createdAt, updatedAt time.Time) domain.CurrentReading {
	return domain.CurrentReading{
		OwnerID: ownerID, Language: language, BookID: bookID, SnapshotID: snapshotID,
		SourceMaterialID: sourceMaterialID, AnalysisRunID: analysisRunID,
		ContentRevisionID: contentRevisionID, ContentSnapshotID: contentSnapshotID,
		CorpusID: corpusID, SnapshotSize: snapshotSize,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

func currentReadingFromRow(row sqlcgen.GetPrimaryGoalRow, snapshotSize int) domain.CurrentReading {
	return currentReadingFromValues(row.GOwnerID, row.Language, row.GBookID, row.SnapshotID, row.SourceMaterialID, row.AnalysisRunID, row.ContentRevisionID, row.ContentSnapshotID, row.CorpusID, snapshotSize, row.CreatedAt, row.UpdatedAt)
}

func listCurrentReadingSnapshotVocabulary(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
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

func createCurrentReadingSnapshot(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language, bookID string, identity sqlcgen.GetPrimaryGoalCandidateIdentityRow) (sqlcgen.CreatePrimaryGoalSnapshotRow, []domain.SelectionCandidate, error) {
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
		candidates, err = correctedCurrentReadingCandidates(ctx, tx, q, owner, bookID, language, identity)
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
	candidates, err = eligibleCurrentReadingCandidates(ctx, tx, q, owner, language, candidates)
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

type currentReadingVocabularyIdentity struct {
	lemma string
	upos  string
}

// eligibleCurrentReadingCandidates applies the common learner-state exclusions
// and uses only ready current per-Book projections for the two-occurrence
// exception. Readiness and totals share one SQL statement snapshot.
func eligibleCurrentReadingCandidates(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language string, candidates []domain.SelectionCandidate) ([]domain.SelectionCandidate, error) {
	known, err := listKnownVocabulary(ctx, tx, owner, language)
	if err != nil {
		return nil, err
	}
	reserved, err := listReservedVocabulary(ctx, tx, owner, language)
	if err != nil {
		return nil, err
	}
	eligibility := selection.NewEligibility(known, reserved)
	identities := make([]currentReadingVocabularyIdentity, 0)
	seen := make(map[currentReadingVocabularyIdentity]struct{})
	for _, candidate := range candidates {
		if candidate.OccurrenceCount != 2 || eligibility.ExcludesLearnerState(candidate) {
			continue
		}
		identity := currentReadingVocabularyIdentity{lemma: candidate.CanonicalLemma, upos: candidate.UPOS}
		if _, ok := seen[identity]; !ok {
			seen[identity] = struct{}{}
			identities = append(identities, identity)
		}
	}

	acrossBooks := make(map[currentReadingVocabularyIdentity]int64, len(identities))
	if len(identities) > 0 {
		lemmas, upos := make([]string, 0, len(identities)), make([]string, 0, len(identities))
		for _, identity := range identities {
			lemmas, upos = append(lemmas, identity.lemma), append(upos, identity.upos)
		}
		rows, err := tx.Query(ctx, `
WITH requested AS (
 SELECT lemma,upos FROM unnest($3::text[],$4::text[]) AS r(lemma,upos)
), current_books AS MATERIALIZED (
 SELECT cai.owner_id,cai.book_id,cai.analysis_run_id,cai.corpus_id,b.language_tag AS language
 FROM current_analysis_identity cai
 JOIN books b ON b.owner_id=cai.owner_id AND b.id=cai.book_id
 WHERE cai.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2
), projection_status AS (
 SELECT NOT EXISTS (
   SELECT 1 FROM current_books cb
   LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=cb.owner_id AND r.book_id=cb.book_id
     AND r.analysis_run_id=cb.analysis_run_id AND r.corpus_id=cb.corpus_id
     AND r.language=cb.language AND r.builder_version=2
   WHERE r.owner_id IS NULL
 ) AS ready,
 EXISTS (
   SELECT 1 FROM current_books cb
   LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=cb.owner_id AND r.book_id=cb.book_id
     AND r.analysis_run_id=cb.analysis_run_id AND r.corpus_id=cb.corpus_id
     AND r.language=cb.language AND r.builder_version=2
   JOIN river_job failed ON failed.kind='rebuild_vocabulary_browse_counts' AND failed.state='discarded'
     AND failed.args->>'owner_id'=cb.owner_id::text AND failed.args->>'book_id'=cb.book_id::text
     AND failed.args->>'run_id'=cb.analysis_run_id::text
   WHERE r.owner_id IS NULL
     AND NOT EXISTS (SELECT 1 FROM river_job active
       WHERE active.kind='rebuild_vocabulary_browse_counts'
         AND active.state IN ('available','pending','running','retryable','scheduled')
         AND active.args->>'owner_id'=cb.owner_id::text AND active.args->>'book_id'=cb.book_id::text
         AND active.args->>'run_id'=cb.analysis_run_id::text)
 ) AS unavailable
), counts AS MATERIALIZED (
 SELECT c.canonical_lemma,c.upos,sum(c.occurrence_count)::bigint AS occurrences
 FROM current_books cb
 JOIN vocabulary_browse_count_readiness r ON r.owner_id=cb.owner_id AND r.book_id=cb.book_id
   AND r.analysis_run_id=cb.analysis_run_id AND r.corpus_id=cb.corpus_id
   AND r.language=cb.language AND r.builder_version=2
 JOIN vocabulary_browse_counts c ON c.owner_id=cb.owner_id AND c.book_id=cb.book_id
   AND c.analysis_run_id=cb.analysis_run_id AND c.corpus_id=cb.corpus_id AND c.language=cb.language
 JOIN requested i ON i.lemma=c.canonical_lemma AND i.upos=c.upos
 GROUP BY c.canonical_lemma,c.upos
)
SELECT i.lemma,i.upos,COALESCE(c.occurrences,0)::bigint,s.ready,s.unavailable
FROM requested i CROSS JOIN projection_status s
LEFT JOIN counts c ON c.canonical_lemma=i.lemma AND c.upos=i.upos`, owner, language, lemmas, upos)
		if err != nil {
			return nil, fmt.Errorf("load current across-Book vocabulary counts: %w", err)
		}
		ready, unavailable := true, false
		for rows.Next() {
			var lemma, pos string
			var count int64
			if err := rows.Scan(&lemma, &pos, &count, &ready, &unavailable); err != nil {
				rows.Close()
				return nil, err
			}
			acrossBooks[currentReadingVocabularyIdentity{lemma: lemma, upos: pos}] = count
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		if !ready {
			if unavailable {
				return nil, ErrVocabularyBrowseCountsUnavailable
			}
			return nil, ErrVocabularyBrowseCountsPending
		}
	}

	selected := make([]domain.SelectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		frequency := int64(0)
		if candidate.OccurrenceCount == 2 {
			frequency = acrossBooks[currentReadingVocabularyIdentity{lemma: candidate.CanonicalLemma, upos: candidate.UPOS}]
		}
		if !eligibility.AllowsBookDeckCandidate(candidate, frequency) {
			continue
		}
		activeReserved, err := q.IsCurrentReadingVocabularyReserved(ctx, sqlcgen.IsCurrentReadingVocabularyReservedParams{
			Owner: owner, Language: language, CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
		})
		if err != nil {
			return nil, err
		}
		if activeReserved {
			continue
		}
		selected = append(selected, candidate)
	}
	return selected, nil
}

// correctedCurrentReadingCandidates rebuilds the current Book's candidate set
// from immutable analyzer evidence plus exact-occurrence learner corrections.
// It runs in the same transaction as the freeze, after the Book row is locked.
func correctedCurrentReadingCandidates(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, bookID, language string, identity sqlcgen.GetPrimaryGoalCandidateIdentityRow) ([]domain.SelectionCandidate, error) {
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
	candidates := make([]domain.SelectionCandidate, 0, len(projected))
	for _, candidate := range projected {
		if candidate.OccurrenceCount < 2 {
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

func releaseCurrentReadingSnapshot(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) error {
	if snapshotID == "" {
		return nil
	}
	if _, err := q.LockPrimaryGoalSnapshot(ctx, sqlcgen.LockPrimaryGoalSnapshotParams{Owner: owner, Snapshot: snapshotID}); err != nil {
		return err
	}
	return q.ReleasePrimaryGoalSnapshot(ctx, sqlcgen.ReleasePrimaryGoalSnapshotParams{Owner: owner, Snapshot: snapshotID})
}

func releaseCurrentReadingSnapshots(ctx context.Context, q *sqlcgen.Queries, owner string, snapshotIDs []string) error {
	for _, snapshotID := range snapshotIDs {
		if err := releaseCurrentReadingSnapshot(ctx, q, owner, snapshotID); err != nil {
			return err
		}
	}
	return nil
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

// CountCurrentReadingVocabularyToAccept reports the currently eligible frozen
// identities. It is deliberately read-only so the confirmation can state the
// exact modeled consequence before the learner accepts completion.
func (s *PostgresStore) CountCurrentReadingVocabularyToAccept(ctx context.Context, owner, language string) (int, error) {
	goal, err := s.GetCurrentReading(ctx, owner, language)
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

// GetCurrentReading returns the owner's current reading in one study language.
// No row is the ordinary absent current-reading state.
func (s *PostgresStore) GetCurrentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error) {
	language = canonicalization.NormalizeLanguage(language)
	if language == "" {
		return domain.CurrentReading{}, nil
	}
	row, err := s.queries().GetPrimaryGoal(ctx, sqlcgen.GetPrimaryGoalParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, nil
	}
	if err != nil {
		return domain.CurrentReading{}, err
	}
	snapshotSize, err := snapshotSizeForGoal(ctx, s.queries(), row.SnapshotID, row.GOwnerID)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromRow(row, snapshotSize), nil
}

// ListCurrentReadingSnapshotVocabulary reads one immutable snapshot without
// changing any learner state.
func (s *PostgresStore) ListCurrentReadingSnapshotVocabulary(ctx context.Context, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
	if snapshotID == "" {
		return nil, nil
	}
	return listCurrentReadingSnapshotVocabulary(ctx, s.queries(), owner, snapshotID)
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

// StartCurrentReading creates the owner's current reading for an eligible To Read
// Book in language.
func (s *PostgresStore) StartCurrentReading(ctx context.Context, owner, language, bookID string) (goal domain.CurrentReading, err error) {
	return s.StartCurrentReadingWith(ctx, owner, language, bookID, nil)
}

// StartCurrentReadingWith creates the current reading and runs beforeCommit in
// the same transaction after its immutable snapshot has been populated. The
// callback can atomically attach durable work that depends on that snapshot.
func (s *PostgresStore) StartCurrentReadingWith(ctx context.Context, owner, language, bookID string, beforeCommit func(context.Context, pgx.Tx, domain.CurrentReading) error) (goal domain.CurrentReading, err error) {
	language = canonicalization.NormalizeLanguage(language)
	goalInput := domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID}
	if err := goalInput.Validate(); err != nil {
		return domain.CurrentReading{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return domain.CurrentReading{}, err
	}
	if err = lockCurrentReadingBook(ctx, q, owner, bookID); err != nil {
		return domain.CurrentReading{}, err
	}
	_, err = q.GetPrimaryGoalForUpdate(ctx, sqlcgen.GetPrimaryGoalForUpdateParams{Owner: owner, Language: language})
	if err == nil {
		return domain.CurrentReading{}, ErrCurrentReadingExists
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, err
	}
	if err = ensureCurrentReadingCandidate(ctx, tx, owner, language, bookID); err != nil {
		return domain.CurrentReading{}, err
	}
	identity, identityErr := q.GetPrimaryGoalCandidateIdentity(ctx, sqlcgen.GetPrimaryGoalCandidateIdentityParams{Owner: owner, Language: language, Book: bookID})
	if errors.Is(identityErr, pgx.ErrNoRows) {
		return domain.CurrentReading{}, ErrCurrentReadingIneligible
	}
	if identityErr != nil {
		return domain.CurrentReading{}, identityErr
	}
	blocked, err := unresolvedLemmaReviewFlags(ctx, tx, owner, bookID, identity.CaAnalysisRunID)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	if blocked {
		return domain.CurrentReading{}, ErrUnresolvedLemmaReviewFlags
	}
	snapshot, candidates, snapshotErr := createCurrentReadingSnapshot(ctx, tx, q, owner, language, bookID, identity)
	if snapshotErr != nil {
		return domain.CurrentReading{}, snapshotErr
	}
	goal, err = insertCurrentReading(ctx, q, owner, language, bookID, snapshot.ID)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	goal.SourceMaterialID = identity.CaSourceMaterialID
	goal.AnalysisRunID = identity.CaAnalysisRunID
	goal.ContentRevisionID = identity.CaContentRevisionID
	goal.ContentSnapshotID = identity.CaSnapshotID
	goal.CorpusID = identity.CaCorpusID
	goal.SnapshotSize = len(candidates)
	if beforeCommit != nil {
		if err = beforeCommit(ctx, tx, goal); err != nil {
			return domain.CurrentReading{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.CurrentReading{}, err
	}
	return goal, nil
}

func ensureCurrentReadingCandidate(ctx context.Context, tx pgx.Tx, owner, language, bookID string) error {
	eligible, err := sqlcgen.New(tx).PrimaryGoalCandidateEligible(ctx, sqlcgen.PrimaryGoalCandidateEligibleParams{
		Owner: owner, Language: language, Book: bookID,
	})
	if err != nil {
		return err
	}
	if !eligible {
		return ErrCurrentReadingIneligible
	}
	return nil
}

func lockCurrentReadingBook(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) error {
	_, err := q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func insertCurrentReading(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID, snapshotID string) (domain.CurrentReading, error) {
	row, err := q.InsertPrimaryGoal(ctx, sqlcgen.InsertPrimaryGoalParams{Owner: owner, Language: language, Book: bookID, Snapshot: uuidArg(snapshotID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, ErrCurrentReadingExists
	}
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromValues(row.OwnerID, row.Language, row.BookID, row.SnapshotID, "", "", "", "", "", 0, row.CreatedAt, row.UpdatedAt), nil
}

// switchReplayProven reports whether the current reading is the successor that
// an earlier Switch from the expected commitment created. Switch releases the
// old snapshot and freezes the new one in one transaction, so their timestamps
// coincide; an End followed by a later Start does not satisfy that, and neither
// does a completed or still-active expected snapshot.
func switchReplayProven(ctx context.Context, q *sqlcgen.Queries, owner, language, expectedBookID, expectedSnapshotID, currentSnapshotID string) (bool, error) {
	if currentSnapshotID == "" {
		return false, nil
	}
	former, err := q.GetPrimaryGoalSnapshotLifecycle(ctx, sqlcgen.GetPrimaryGoalSnapshotLifecycleParams{Owner: owner, Language: language, Snapshot: expectedSnapshotID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if former.BookID != expectedBookID || !former.ReleasedAt.Valid || former.Completed {
		return false, nil
	}
	successor, err := q.GetPrimaryGoalSnapshotLifecycle(ctx, sqlcgen.GetPrimaryGoalSnapshotLifecycleParams{Owner: owner, Language: language, Snapshot: currentSnapshotID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return successor.CreatedAt.Equal(former.ReleasedAt.Time), nil
}
