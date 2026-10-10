package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

func currentReadingCompletionFromValues(ownerID, language, bookID string, completedAt time.Time, snapshotID string, snapshotCount, eligibleCount, graduatedCount, alreadyKnownCount int) domain.CurrentReadingCompletion {
	return domain.CurrentReadingCompletion{
		OwnerID: ownerID, Language: language, BookID: bookID, CompletedAt: completedAt,
		SnapshotID: snapshotID, SnapshotVocabularyCount: snapshotCount,
		EligibleVocabularyCount: eligibleCount, GraduatedVocabularyCount: graduatedCount,
		AlreadyKnownVocabularyCount: alreadyKnownCount,
	}
}

// SwitchCurrentReading replaces the current reading only when the caller's
// expected commitment still names the language's current reading. Postgres
// loads the facts under the learner-state and Book locks;
// domain.DecideSwitchCurrentReading accepts, rejects, or replays, and this
// method applies the plan.
func (s *PostgresStore) SwitchCurrentReading(ctx context.Context, owner, language, bookID, expectedBookID, expectedSnapshotID string) (result domain.CurrentReading, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if err := (domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID}).Validate(); err != nil {
		return domain.CurrentReading{}, err
	}
	if !exactCommitment(expectedBookID, expectedSnapshotID) {
		return domain.CurrentReading{}, ErrCurrentReadingStale
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
	facts, identity, err := loadSwitchFacts(ctx, tx, q, owner, language, bookID, expectedBookID, expectedSnapshotID)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	decision := domain.DecideSwitchCurrentReading(facts)
	if err = currentReadingRejection(decision, ErrNotFound); err != nil {
		return domain.CurrentReading{}, err
	}
	if decision.Verdict == domain.CurrentReadingReplay {
		current := facts.Current
		snapshotSize, sizeErr := snapshotSizeForGoal(ctx, q, current.SnapshotID, owner)
		if sizeErr != nil {
			return domain.CurrentReading{}, sizeErr
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.CurrentReading{}, err
		}
		current.SnapshotSize = snapshotSize
		return current, nil
	}
	if err = releaseCurrentReadingSnapshot(ctx, q, owner, decision.ReleaseSnapshotID); err != nil {
		return domain.CurrentReading{}, err
	}
	snapshot, candidates, err := createCurrentReadingSnapshot(ctx, tx, q, owner, language, bookID, identity)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	row, err := q.ChangeCurrentReadingBook(ctx, sqlcgen.ChangeCurrentReadingBookParams{Owner: owner, Language: language, Book: bookID, Snapshot: uuidArg(snapshot.ID)})
	if err != nil {
		return domain.CurrentReading{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.CurrentReading{}, err
	}
	result = currentReadingFromValues(row.OwnerID, row.Language, row.BookID, row.SnapshotID, identity.CaSourceMaterialID, identity.CaAnalysisRunID, identity.CaContentRevisionID, identity.CaSnapshotID, identity.CaCorpusID, len(candidates), row.CreatedAt, row.UpdatedAt)
	return result, nil
}

// loadSwitchFacts reads everything DecideSwitchCurrentReading decides over. The
// target's candidate identity and flags are read only when the classifier makes
// it eligible, since an ineligible target has no published identity to read.
func loadSwitchFacts(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language, bookID, expectedBookID, expectedSnapshotID string) (domain.SwitchCurrentReadingFacts, sqlcgen.GetCurrentReadingCandidateIdentityRow, error) {
	var identity sqlcgen.GetCurrentReadingCandidateIdentityRow
	facts := domain.SwitchCurrentReadingFacts{
		TargetBookID: bookID,
		Expected:     domain.CurrentReadingCommitment{BookID: expectedBookID, SnapshotID: expectedSnapshotID},
	}
	current, err := loadCurrentReadingForUpdate(ctx, q, owner, language)
	if err != nil {
		return facts, identity, err
	}
	facts.Current = current
	if err = loadSnapshotLifecycle(ctx, q, owner, language, expectedSnapshotID, &facts.ExpectedSnapshot); err != nil {
		return facts, identity, err
	}
	if err = loadSnapshotLifecycle(ctx, q, owner, language, current.SnapshotID, &facts.CurrentSnapshot); err != nil {
		return facts, identity, err
	}
	if facts.TargetEligibility, err = currentReadingEligibility(ctx, q, owner, language, bookID); err != nil {
		return facts, identity, err
	}
	if facts.TargetEligibility != domain.CurrentReadingEligible {
		return facts, identity, nil
	}
	identity, err = q.GetCurrentReadingCandidateIdentity(ctx, sqlcgen.GetCurrentReadingCandidateIdentityParams{Owner: owner, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		// The classifier and the identity view disagree: report the missing
		// completed analysis.
		facts.TargetEligibility = domain.CurrentReadingNoCompletedAnalysis
		return facts, identity, nil
	}
	if err != nil {
		return facts, identity, err
	}
	facts.UnresolvedFlags, err = unresolvedLemmaReviewFlags(ctx, tx, owner, bookID, identity.CaAnalysisRunID)
	return facts, identity, err
}

// currentReadingRejection translates a rejecting decision to the sentinel
// error the webapp store interface exposes, and accepted or replayed decisions
// to nil. noCurrent is the error for a transition that needs a current reading
// and found none.
func currentReadingRejection(decision domain.CurrentReadingDecision, noCurrent error) error {
	switch decision.Verdict {
	case domain.CurrentReadingApply, domain.CurrentReadingReplay:
		return nil
	case domain.CurrentReadingRejectNoCurrent:
		return noCurrent
	case domain.CurrentReadingRejectIneligible:
		return CurrentReadingIneligibleError{Reason: decision.Ineligible}
	case domain.CurrentReadingRejectUnresolvedFlags:
		return ErrUnresolvedLemmaReviewFlags
	case domain.CurrentReadingRejectStale:
		return ErrCurrentReadingStale
	}
	return ErrCurrentReadingStale
}

// EndCurrentReading releases exactly the expected commitment's Reserved
// vocabulary and clears the current role without recording completion or Known
// acceptance. The Book keeps its disposition (To Read), visibility, snapshot,
// history, and artifacts. domain.DecideEndCurrentReading accepts, rejects, or
// proves a replay from the loaded facts; this method applies the plan.
func (s *PostgresStore) EndCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) (err error) {
	language = canonicalization.NormalizeLanguage(language)
	if !exactCommitment(expectedBookID, expectedSnapshotID) {
		return ErrCurrentReadingStale
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return err
	}
	if err = lockCurrentReadingBook(ctx, q, owner, expectedBookID); err != nil {
		return err
	}
	facts := domain.EndCurrentReadingFacts{Expected: domain.CurrentReadingCommitment{BookID: expectedBookID, SnapshotID: expectedSnapshotID}}
	if facts.Current, err = loadCurrentReadingForUpdate(ctx, q, owner, language); err != nil {
		return err
	}
	if err = loadSnapshotLifecycle(ctx, q, owner, language, expectedSnapshotID, &facts.Snapshot); err != nil {
		return err
	}
	decision := domain.DecideEndCurrentReading(facts)
	if err = currentReadingRejection(decision, ErrCurrentReadingStale); err != nil {
		return err
	}
	if decision.Verdict == domain.CurrentReadingReplay {
		return tx.Commit(ctx)
	}
	if err = releaseCurrentReadingSnapshot(ctx, q, owner, decision.ReleaseSnapshotID); err != nil {
		return err
	}
	if err = q.DeleteCurrentReading(ctx, sqlcgen.DeleteCurrentReadingParams{Owner: owner, Language: language}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// exactCommitment reports whether both identities are present and well formed,
// so malformed or missing expectations are rejected as stale instead of
// reaching SQL.
func exactCommitment(bookID, snapshotID string) bool {
	if _, err := uuid.Parse(bookID); err != nil {
		return false
	}
	_, err := uuid.Parse(snapshotID)
	return err == nil
}

// FinishCurrentReading accepts the frozen snapshot into modeled Known vocabulary,
// records the completion fact, returns the Book to Inbox, and ends the current
// reading. The snapshot is the request identity, so retries replay the same
// outcome even after the Book is completed again in the future.
//
// The adapter only loads facts under the learner-state and current-reading
// locks and applies domain.PlanCurrentReadingFinish: SQL neither chooses the
// identities that become Known nor counts them.
func (s *PostgresStore) FinishCurrentReading(ctx context.Context, owner, language, expectedBookID, expectedSnapshotID string) (result domain.CurrentReadingFinishResult, err error) {
	language = canonicalization.NormalizeLanguage(language)
	// A legacy reading that never received a snapshot has no snapshot identity to
	// name; the empty expectation still has to match the current row exactly.
	if _, parseErr := uuid.Parse(expectedBookID); parseErr != nil {
		return domain.CurrentReadingFinishResult{}, ErrCurrentReadingStale
	}
	if _, parseErr := uuid.Parse(expectedSnapshotID); parseErr != nil && expectedSnapshotID != "" {
		return domain.CurrentReadingFinishResult{}, ErrCurrentReadingStale
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err = lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	facts := domain.CurrentReadingFinishFacts{ExpectedBookID: expectedBookID, ExpectedSnapshotID: expectedSnapshotID}
	current, err := q.GetCurrentReadingForUpdate(ctx, sqlcgen.GetCurrentReadingForUpdateParams{Owner: owner, Language: language})
	if err == nil {
		facts.Current = domain.CurrentReading{OwnerID: owner, Language: language, BookID: current.GBookID, SnapshotID: current.SnapshotID}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReadingFinishResult{}, err
	}
	row, err := getReadingCompletion(ctx, q, owner, language, expectedBookID, expectedSnapshotID)
	if err == nil {
		completion := currentReadingCompletionFromValues(row.OwnerID, row.Language, row.BookID, row.CompletedAt, row.GoalSnapshotID, row.SnapshotVocabularyCount, row.EligibleVocabularyCount, row.GraduatedVocabularyCount, row.AlreadyKnownVocabularyCount)
		facts.Completion = &completion
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReadingFinishResult{}, err
	}
	if facts.Current.IsActive() && facts.Completion == nil && facts.Current.BookID == expectedBookID && facts.Current.SnapshotID == expectedSnapshotID && expectedSnapshotID != "" {
		facts.Snapshot, facts.Known, err = loadCurrentReadingSnapshotFacts(ctx, q, owner, expectedSnapshotID)
		if err != nil {
			return domain.CurrentReadingFinishResult{}, err
		}
	}

	decision := domain.PlanCurrentReadingFinish(facts)
	switch decision.Outcome {
	case domain.CurrentReadingFinishRejected:
		if decision.Rejection == domain.CurrentReadingFinishUnknown {
			return domain.CurrentReadingFinishResult{}, ErrNotFound
		}
		return domain.CurrentReadingFinishResult{}, ErrCurrentReadingStale
	case domain.CurrentReadingFinishReplayed:
		result = domain.CurrentReadingFinishResult{Completion: decision.Replay}
	case domain.CurrentReadingFinishPlanned:
		plan := decision.Plan
		completedAt := time.Now().UTC()
		completionRow, insertErr := q.InsertReadingCompletion(ctx, sqlcgen.InsertReadingCompletionParams{
			Owner: owner, Language: language, Book: expectedBookID, CompletedAt: completedAt,
			Snapshot: expectedSnapshotID, SnapshotVocabularyCount: plan.SnapshotCount,
			EligibleVocabularyCount: plan.EligibleCount, GraduatedVocabularyCount: plan.NewlyKnownCount,
			AlreadyKnownVocabularyCount: plan.AlreadyKnownCount,
		})
		if insertErr != nil {
			return domain.CurrentReadingFinishResult{}, insertErr
		}
		if len(plan.Accept) > 0 {
			inserted, acceptErr := q.InsertCurrentReadingKnownVocabulary(ctx, knownVocabularyInsertParams(owner, facts.Current.SnapshotID, completedAt, plan.Accept))
			if acceptErr != nil {
				return domain.CurrentReadingFinishResult{}, acceptErr
			}
			if inserted != len(plan.Accept) {
				return domain.CurrentReadingFinishResult{}, fmt.Errorf("persistence: finish planned %d Known identities but inserted %d", len(plan.Accept), inserted)
			}
		}
		result = domain.CurrentReadingFinishResult{Completion: currentReadingCompletionFromValues(completionRow.OwnerID, completionRow.Language, completionRow.BookID, completionRow.CompletedAt, completionRow.GoalSnapshotID, plan.SnapshotCount, plan.EligibleCount, plan.NewlyKnownCount, plan.AlreadyKnownCount)}
	}
	if facts.Current.IsActive() {
		if err = upsertBookDisposition(ctx, q, owner, expectedBookID, domain.BookDispositionInbox); err != nil {
			return domain.CurrentReadingFinishResult{}, err
		}
		if err = releaseCurrentReadingSnapshot(ctx, q, owner, facts.Current.SnapshotID); err != nil {
			return domain.CurrentReadingFinishResult{}, err
		}
		if err = q.DeleteCurrentReading(ctx, sqlcgen.DeleteCurrentReadingParams{Owner: owner, Language: language}); err != nil {
			return domain.CurrentReadingFinishResult{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.CurrentReadingFinishResult{}, err
	}
	return result, nil
}

// loadCurrentReadingSnapshotFacts reads the frozen identities of a snapshot and
// the Known rows that can cover them. It decides nothing.
func loadCurrentReadingSnapshotFacts(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) ([]domain.SnapshotIdentity, []domain.KnownVocabulary, error) {
	rows, err := q.ListCurrentReadingSnapshotIdentities(ctx, sqlcgen.ListCurrentReadingSnapshotIdentitiesParams{Owner: owner, Snapshot: snapshotID})
	if err != nil {
		return nil, nil, err
	}
	snapshot := make([]domain.SnapshotIdentity, 0, len(rows))
	for _, row := range rows {
		snapshot = append(snapshot, domain.SnapshotIdentity{Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos})
	}
	knownRows, err := q.ListKnownVocabularyForCurrentReadingSnapshot(ctx, sqlcgen.ListKnownVocabularyForCurrentReadingSnapshotParams{Owner: owner, Snapshot: snapshotID})
	if err != nil {
		return nil, nil, err
	}
	known := make([]domain.KnownVocabulary, 0, len(knownRows))
	for _, row := range knownRows {
		known = append(known, domain.KnownVocabulary{OwnerID: owner, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos})
	}
	return snapshot, known, nil
}

func knownVocabularyInsertParams(owner, snapshotID string, completedAt time.Time, accept []domain.SnapshotIdentity) sqlcgen.InsertCurrentReadingKnownVocabularyParams {
	params := sqlcgen.InsertCurrentReadingKnownVocabularyParams{Owner: owner, Snapshot: snapshotID, CompletedAt: completedAt}
	for _, identity := range accept {
		params.Languages = append(params.Languages, identity.Language)
		params.Lemmas = append(params.Lemmas, identity.CanonicalLemma)
		params.Uposes = append(params.Uposes, identity.UPOS)
	}
	return params
}
