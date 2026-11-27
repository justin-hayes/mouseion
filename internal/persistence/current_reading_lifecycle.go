package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

func currentReadingFromValues(ownerID, language, bookID string, createdAt, updatedAt time.Time) domain.CurrentReading {
	return domain.CurrentReading{OwnerID: ownerID, Language: language, BookID: bookID, CreatedAt: createdAt, UpdatedAt: updatedAt}
}

func currentReadingFromRow(row sqlcgen.GetCurrentReadingRow) domain.CurrentReading {
	return currentReadingFromValues(row.GOwnerID, row.Language, row.GBookID, row.CreatedAt, row.UpdatedAt)
}

type currentReadingVocabularyIdentity struct {
	lemma         string
	upos          string
	discriminator string
}

// currentAcrossBookCounts totals each identity's projected count over the
// learner's currently analyzed Books in a language, optionally leaving one Book
// out. Counts and readiness share one SQL statement snapshot; the totals are
// withheld with a pending or unavailable error while any such Book lacks a
// ready projection. The identity includes the lexical discriminator, so source
// homographs are totalled separately.
func currentAcrossBookCounts(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, owner, language, excludeBook string, identities []currentReadingVocabularyIdentity) (map[currentReadingVocabularyIdentity]int64, error) {
	acrossBooks := make(map[currentReadingVocabularyIdentity]int64, len(identities))
	if len(identities) == 0 {
		return acrossBooks, nil
	}
	lemmas, upos := make([]string, 0, len(identities)), make([]string, 0, len(identities))
	discriminators := make([]string, 0, len(identities))
	for _, identity := range identities {
		lemmas, upos = append(lemmas, identity.lemma), append(upos, identity.upos)
		discriminators = append(discriminators, identity.discriminator)
	}
	rows, err := q.Query(ctx, `
WITH requested AS (
 SELECT lemma,upos,discriminator FROM unnest($3::text[],$4::text[],$5::text[]) AS r(lemma,upos,discriminator)
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
     AND r.language=cb.language AND r.builder_version=5
   WHERE r.owner_id IS NULL
 ) AS ready,
 EXISTS (
   SELECT 1 FROM current_books cb
   LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=cb.owner_id AND r.book_id=cb.book_id
     AND r.analysis_run_id=cb.analysis_run_id AND r.corpus_id=cb.corpus_id
     AND r.language=cb.language AND r.builder_version=5
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
 SELECT c.canonical_lemma,c.upos,c.lexical_discriminator,sum(c.occurrence_count)::bigint AS occurrences
 FROM current_books cb
 JOIN vocabulary_browse_count_readiness r ON r.owner_id=cb.owner_id AND r.book_id=cb.book_id
   AND r.analysis_run_id=cb.analysis_run_id AND r.corpus_id=cb.corpus_id
   AND r.language=cb.language AND r.builder_version=5
 JOIN vocabulary_browse_counts c ON c.owner_id=cb.owner_id AND c.book_id=cb.book_id
   AND c.analysis_run_id=cb.analysis_run_id AND c.corpus_id=cb.corpus_id AND c.language=cb.language
 JOIN requested i ON i.lemma=c.canonical_lemma AND i.upos=c.upos AND i.discriminator=c.lexical_discriminator
 WHERE $6::text='' OR c.book_id::text<>$6
 GROUP BY c.canonical_lemma,c.upos,c.lexical_discriminator
)
SELECT i.lemma,i.upos,i.discriminator,COALESCE(c.occurrences,0)::bigint,s.ready,s.unavailable
FROM requested i CROSS JOIN projection_status s
LEFT JOIN counts c ON c.canonical_lemma=i.lemma AND c.upos=i.upos AND c.lexical_discriminator=i.discriminator`, owner, language, lemmas, upos, discriminators, excludeBook)
	if err != nil {
		return nil, fmt.Errorf("load current across-Book vocabulary counts: %w", err)
	}
	ready, unavailable := true, false
	for rows.Next() {
		var lemma, pos, discriminator string
		var count int64
		if err := rows.Scan(&lemma, &pos, &discriminator, &count, &ready, &unavailable); err != nil {
			rows.Close()
			return nil, err
		}
		acrossBooks[currentReadingVocabularyIdentity{lemma: lemma, upos: pos, discriminator: discriminator}] = count
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
	return acrossBooks, nil
}

func releaseCurrentReadingSnapshot(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) error {
	if snapshotID == "" {
		return nil
	}
	if _, err := q.LockCurrentReadingSnapshot(ctx, sqlcgen.LockCurrentReadingSnapshotParams{Owner: owner, Snapshot: snapshotID}); err != nil {
		return err
	}
	return q.ReleaseCurrentReadingSnapshot(ctx, sqlcgen.ReleaseCurrentReadingSnapshotParams{Owner: owner, Snapshot: snapshotID})
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

// GetCurrentReading returns the owner's current reading in one study language.
// No row is the ordinary absent current-reading state.
func (s *PostgresStore) GetCurrentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error) {
	language = canonicalization.NormalizeLanguage(language)
	if language == "" {
		return domain.CurrentReading{}, nil
	}
	row, err := s.queries().GetCurrentReading(ctx, sqlcgen.GetCurrentReadingParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, nil
	}
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromRow(row), nil
}

// StartResult is domain.StartResult.
type StartResult = domain.StartResult

// StartCurrentReading is Read next: the Book becomes the owner's current reading
// in language, replacing any other current reading there.
func (s *PostgresStore) StartCurrentReading(ctx context.Context, owner, language, bookID string) (domain.CurrentReading, error) {
	start, err := s.StartCurrentReadingResult(ctx, owner, language, bookID)
	return start.Reading, err
}

// StartCurrentReadingResult is StartCurrentReading that also reports whether
// the Book was already the current reading, so nothing was written. It needs no
// confirmation: it is refused only when the Book is not To Read, not analysed,
// or has no Book language for the study language (ADR 0136 §4).
func (s *PostgresStore) StartCurrentReadingResult(ctx context.Context, owner, language, bookID string) (start StartResult, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if err := validateCurrentReadingStart(owner, language, bookID); err != nil {
		return StartResult{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return StartResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err := lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return StartResult{}, err
	}
	if err := lockBookForUpdate(ctx, q, owner, bookID); err != nil {
		return StartResult{}, err
	}
	facts, current, err := loadStartFacts(ctx, q, owner, language, bookID)
	if err != nil {
		return StartResult{}, err
	}
	decision := domain.DecideStart(facts)
	if err := domain.StartDecisionError(decision); err != nil {
		return StartResult{}, err
	}
	if decision.Outcome == domain.StartReplayed {
		reading, readErr := readCurrentReadingLocked(ctx, q, owner, language)
		if readErr != nil {
			return StartResult{}, readErr
		}
		if err := tx.Commit(ctx); err != nil {
			return StartResult{}, err
		}
		return StartResult{Reading: reading, Replayed: true}, nil
	}
	reading, err := applyReadNext(ctx, q, owner, language, bookID, current)
	if err != nil {
		return StartResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StartResult{}, err
	}
	return StartResult{Reading: reading}, nil
}

// applyReadNext writes the Book as the language's current reading: a change
// when another Book is current, an insert otherwise. A reading started before
// the vocabulary snapshot was retired has its snapshot released first.
func applyReadNext(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID string, current *sqlcgen.GetCurrentReadingForUpdateRow) (domain.CurrentReading, error) {
	if current == nil {
		row, err := q.InsertCurrentReading(ctx, sqlcgen.InsertCurrentReadingParams{Owner: owner, Language: language, Book: bookID})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CurrentReading{}, ErrCurrentReadingExists
		}
		if err != nil {
			return domain.CurrentReading{}, err
		}
		return currentReadingFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt), nil
	}
	if err := releaseCurrentReadingSnapshot(ctx, q, owner, current.SnapshotID); err != nil {
		return domain.CurrentReading{}, err
	}
	row, err := q.ChangeCurrentReadingBook(ctx, sqlcgen.ChangeCurrentReadingBookParams{Owner: owner, Language: language, Book: bookID})
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromValues(row.OwnerID, row.Language, row.BookID, row.CreatedAt, row.UpdatedAt), nil
}

// validateCurrentReadingStart rejects a Start whose reading identity is
// incomplete and treats a Book identity Postgres cannot read as a uuid as the
// same not-found a well-formed unknown Book gets.
func validateCurrentReadingStart(owner, language, bookID string) error {
	if err := (domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID}).Validate(); err != nil {
		return err
	}
	if !validUUID(owner) || !validUUID(bookID) {
		return ErrNotFound
	}
	return nil
}

// readCurrentReadingLocked reads the owner's current reading in language inside
// a transaction whose learner-state lock is already held.
func readCurrentReadingLocked(ctx context.Context, q *sqlcgen.Queries, owner, language string) (domain.CurrentReading, error) {
	row, err := q.GetCurrentReading(ctx, sqlcgen.GetCurrentReadingParams{Owner: owner, Language: language})
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromRow(row), nil
}

// CurrentReadingIneligibleError is domain.CurrentReadingIneligibleError.
type CurrentReadingIneligibleError = domain.CurrentReadingIneligibleError

// loadStartFacts reads the facts domain.DecideStart decides over. The caller
// holds the learner-state and Book locks, so the facts cannot change before the
// transition commits. current is nil when no reading is current in language.
func loadStartFacts(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID string) (domain.StartFacts, *sqlcgen.GetCurrentReadingForUpdateRow, error) {
	facts := domain.StartFacts{Language: language}
	var current *sqlcgen.GetCurrentReadingForUpdateRow
	row, err := q.GetCurrentReadingForUpdate(ctx, sqlcgen.GetCurrentReadingForUpdateParams{Owner: owner, Language: language})
	switch {
	case err == nil:
		current = &row
		facts.CurrentIsBook = strings.EqualFold(row.GBookID, bookID)
	case !errors.Is(err, pgx.ErrNoRows):
		return facts, nil, err
	}
	if err := loadStartEvidence(ctx, q, owner, bookID, &facts); err != nil {
		return facts, nil, err
	}
	_, err = q.GetCurrentReadingCandidateIdentity(ctx, sqlcgen.GetCurrentReadingCandidateIdentityParams{Owner: owner, Book: bookID})
	switch {
	case err == nil:
		facts.IdentityPublished = true
	case !errors.Is(err, pgx.ErrNoRows):
		return facts, nil, err
	}
	return facts, current, nil
}

// loadStartEvidence fills the classifier's inputs from the Book's evidence row.
func loadStartEvidence(ctx context.Context, q *sqlcgen.Queries, owner, bookID string, facts *domain.StartFacts) error {
	evidence, err := q.GetCurrentReadingEvidence(ctx, sqlcgen.GetCurrentReadingEvidenceParams{Owner: owner, Book: bookID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// A Book without an evidence row has no acquired content, no analysis, and
	// no disposition, so the zero row reads as exactly that.
	// Eligibility does not read a failure code: the classifier treats every
	// failed run the same way, and the query selects no display-only column.
	facts.Signals = analysisSignalsFromView(evidence.SourceID, evidence.SourceMediaType, evidence.SourceContentRevisionID, evidence.SourceContentSnapshotID, evidence.AnalysisStatus, evidence.AnalysisState, evidence.AnalysisRunID, "")
	facts.Disposition = domain.BookDisposition(evidence.Disposition)
	facts.BookLanguage = ""
	if evidence.BookLanguageState == domain.LanguageChosen {
		facts.BookLanguage = evidence.BookLanguageTag
	}
	return nil
}

// lockBookForUpdate locks the owner's Book row for update and reports a missing
// Book as ErrNotFound. Callers use it to fence the Book before they lock the
// Current reading or preparation rows that hang off it.
func lockBookForUpdate(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) error {
	_, err := q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
