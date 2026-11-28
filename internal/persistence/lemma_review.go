package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

// listEligibleLemmaReviewRows fetches candidate occurrences and keeps only
// those selection counts as vocabulary (ADR 0087).
func listEligibleLemmaReviewRows(ctx context.Context, q *sqlcgen.Queries, owner, bookID, surface string) ([]sqlcgen.ListLemmaReviewOccurrencesRow, error) {
	rows, err := q.ListLemmaReviewOccurrences(ctx, sqlcgen.ListLemmaReviewOccurrencesParams{Owner: owner, Book: bookID, Surface: surface})
	if err != nil {
		return nil, err
	}
	cfg := selection.DefaultConfig("")
	eligible := rows[:0]
	for _, row := range rows {
		if selection.OccurrenceEligible(cfg, row.Language, row.CanonicalLemma, row.Upos, row.Dependency) {
			eligible = append(eligible, row)
		}
	}
	return eligible, nil
}

// ListLemmaReviewOccurrences resolves an exact observed form only in the
// owner's current completed analysis for the given Book.
func (s *PostgresStore) ListLemmaReviewOccurrences(ctx context.Context, owner, bookID, surface string) ([]domain.LemmaReviewOccurrence, error) {
	return listLemmaReviewOccurrences(ctx, s.queries(), owner, bookID, surface)
}

func listLemmaReviewOccurrences(ctx context.Context, q *sqlcgen.Queries, owner, bookID, surface string) ([]domain.LemmaReviewOccurrence, error) {
	rows, err := listEligibleLemmaReviewRows(ctx, q, owner, bookID, surface)
	if err != nil {
		return nil, err
	}
	result := make([]domain.LemmaReviewOccurrence, 0, len(rows))
	for _, row := range rows {
		occurrence := domain.LemmaReviewOccurrence{
			OwnerID: owner, BookID: bookID, CorpusID: row.CorpusID, AnalysisRunID: row.AnalysisRunID,
			SourceDocumentID: row.UnitID, StartOffset: row.StartOffset, EndOffset: row.EndOffset,
			SentenceOrdinal: row.SentenceOrdinal, TokenOrdinal: row.TokenOrdinal,
			Surface: row.Surface, RawLemma: row.RawLemma, CanonicalLemma: row.CanonicalLemma,
			UPOS: row.Upos, Dependency: row.Dependency, SentenceText: row.SentenceText, CorrectedLemma: row.CorrectedLemma, Excluded: row.Excluded,
			LexicalDiscriminator: row.LexicalDiscriminator,
			NormalizationProfile: row.NormalizationProfile, NormalizationVersion: row.NormalizationVersion,
		}
		result = append(result, occurrence)
	}
	return result, nil
}

// ListCorpusLemmaReviewOccurrences resolves one observed form with one analyzer
// lemma in the current completed analysis of every Book in the owner's My
// Books of the language, with each occurrence's learner decision
// applied. Selection decides which occurrences count as vocabulary, as in a
// Book's own review.
func (s *PostgresStore) ListCorpusLemmaReviewOccurrences(ctx context.Context, owner, language, surface, lemma string) ([]domain.LemmaReviewOccurrence, error) {
	rows, err := s.queries().ListCorpusLemmaReviewOccurrences(ctx, sqlcgen.ListCorpusLemmaReviewOccurrencesParams{Owner: owner, Language: language, Surface: surface, Lemma: lemma})
	if err != nil {
		return nil, err
	}
	cfg := selection.DefaultConfig("")
	result := make([]domain.LemmaReviewOccurrence, 0, len(rows))
	for _, row := range rows {
		if !selection.OccurrenceEligible(cfg, row.Language, row.CanonicalLemma, row.Upos, row.Dependency) {
			continue
		}
		occurrence := domain.LemmaReviewOccurrence{
			OwnerID: owner, BookID: row.BookID, CorpusID: row.CorpusID, AnalysisRunID: row.AnalysisRunID,
			SourceDocumentID: row.UnitID, StartOffset: row.StartOffset, EndOffset: row.EndOffset,
			SentenceOrdinal: row.SentenceOrdinal, TokenOrdinal: row.TokenOrdinal,
			Surface: row.Surface, RawLemma: row.RawLemma, CanonicalLemma: row.CanonicalLemma,
			UPOS: row.Upos, Dependency: row.Dependency, SentenceText: row.SentenceText, CorrectedLemma: row.CorrectedLemma, Excluded: row.Excluded,
			LexicalDiscriminator: row.LexicalDiscriminator,
			NormalizationProfile: row.NormalizationProfile, NormalizationVersion: row.NormalizationVersion,
		}
		result = append(result, occurrence)
	}
	return result, nil
}

// PutLemmaDecisions applies an explicitly reviewed set atomically. Every row
// retains its own expected prior state, and one stale member rolls back the set.
// Learner decisions reach it only through PutLemmaDecisionProposal; the set form
// remains for integration-test setup.
func (s *PostgresStore) PutLemmaDecisions(ctx context.Context, decisions []domain.LemmaReviewDecision) error {
	if len(decisions) == 0 {
		return nil
	}
	owner, book := decisions[0].Occurrence.OwnerID, decisions[0].Occurrence.BookID
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
			return err
		}
		if err := lockBookForUpdate(ctx, sqlcgen.New(tx), owner, book); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, owner+":"+book); err != nil {
			return err
		}
		return applyLemmaDecisionsTx(ctx, tx, owner, book, decisions)
	})
}

// PutLemmaDecisionProposal is the one stale check of a confirmed proposal. It
// rereads the preview's state while holding the learner-state and Book locks,
// rejects any change since the preview, and otherwise commits the complete
// selected set atomically. The current-reading gate runs first, under the same
// locks Start takes, so a Start that committed after the review page was
// checked is rejected here.
func (s *PostgresStore) PutLemmaDecisionProposal(ctx context.Context, proposal domain.LemmaReviewProposal, expectedFingerprint string) error {
	if len(proposal.Occurrences) == 0 {
		return ErrNotFound
	}
	owner, book := proposal.OwnerID, proposal.BookID
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
			return err
		}
		if err := lockBookForUpdate(ctx, sqlcgen.New(tx), owner, book); err != nil {
			return err
		}
		if err := rejectLemmaDecisionForCurrentReading(ctx, tx, owner, book); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, owner+":"+book); err != nil {
			return err
		}
		preview, err := readLemmaReviewProposalTx(ctx, tx, proposal)
		if err != nil {
			return err
		}
		if preview.Fingerprint != expectedFingerprint {
			return domain.ErrLemmaReviewPreviewStale
		}
		return applyLemmaDecisionsTx(ctx, tx, owner, book, proposal.Decisions())
	})
}

func applyLemmaDecisionsTx(ctx context.Context, tx pgx.Tx, owner, book string, decisions []domain.LemmaReviewDecision) error {
	countReadiness, err := browseCountReadinessForDecision(ctx, tx, owner, book)
	if err != nil {
		return err
	}
	q := sqlcgen.New(tx)
	var changed []domain.LemmaReviewDecision
	for _, decision := range decisions {
		if decision.Occurrence.OwnerID != owner || decision.Occurrence.BookID != book {
			return ErrNotFound
		}
		updated, err := putLemmaDecisionTx(ctx, q, decision)
		if err != nil {
			return err
		}
		if updated {
			changed = append(changed, decision)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	// A changed decision can change an Idiom match, so the Book's Idiom counts
	// stop counting until the rebuild the caller queues after commit.
	if _, err := tx.Exec(ctx, `DELETE FROM idiom_candidate_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner, book); err != nil {
		return err
	}
	if countReadiness.ready {
		return applyVocabularyBrowseDecisionDeltasTx(ctx, tx, countReadiness, changed)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner, book); err != nil {
		return err
	}
	return nil
}

func lockLemmaReviewLearnerState(ctx context.Context, tx pgx.Tx, owner string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 194))`, owner)
	return err
}

// rejectLemmaDecisionForCurrentReading refuses vocabulary decisions on the
// Book that is the owner's current reading. The caller holds the learner-state
// and Book locks, so Start cannot make the Book current before this decision
// commits, and the row read here is the one Start would have written.
func rejectLemmaDecisionForCurrentReading(ctx context.Context, tx pgx.Tx, owner, book string) error {
	var current bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM primary_goals WHERE owner_id=$1 AND book_id=$2)`, owner, book).Scan(&current); err != nil {
		return err
	}
	if current {
		return domain.ErrLemmaDecisionCurrentReading
	}
	return nil
}

func putLemmaDecisionTx(ctx context.Context, q *sqlcgen.Queries, decision domain.LemmaReviewDecision) (bool, error) {
	occurrence, lemma, excluded := decision.Occurrence, decision.CanonicalLemma, decision.Excluded
	changed, expectedLemma, err := validateLemmaDecision(ctx, q, decision)
	if err != nil {
		return false, err
	}
	if lemma == occurrence.CanonicalLemma && !excluded {
		return deleteLemmaCorrection(ctx, q, decision, expectedLemma, changed)
	}
	return putLemmaCorrection(ctx, q, decision, expectedLemma, changed)
}

// validateLemmaDecision rereads the occurrence and its current correction under
// the caller's locks, and reports whether applying decision would change stored
// correction state.
func validateLemmaDecision(ctx context.Context, q *sqlcgen.Queries, decision domain.LemmaReviewDecision) (bool, pgtype.Text, error) {
	occurrence := decision.Occurrence
	evidence, err := q.GetCurrentAnalysisOccurrenceEvidence(ctx, sqlcgen.GetCurrentAnalysisOccurrenceEvidenceParams{
		Owner: occurrence.OwnerID, Book: occurrence.BookID, AnalysisRun: occurrence.AnalysisRunID,
		SourceDocumentID: occurrence.SourceDocumentID, StartOffset: occurrence.StartOffset, EndOffset: occurrence.EndOffset,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, pgtype.Text{}, ErrNotFound
	}
	if err != nil {
		return false, pgtype.Text{}, err
	}
	if !selection.OccurrenceEligible(selection.DefaultConfig(""), evidence.Language, evidence.CanonicalLemma, evidence.Upos, evidence.Dependency) {
		return false, pgtype.Text{}, ErrNotFound
	}
	current, err := q.ListOccurrenceLemmaCorrections(ctx, sqlcgen.ListOccurrenceLemmaCorrectionsParams{
		Owner: occurrence.OwnerID, Book: occurrence.BookID, Corpus: occurrence.CorpusID, AnalysisRun: occurrence.AnalysisRunID,
	})
	if err != nil {
		return false, pgtype.Text{}, err
	}
	currentLemma, currentExcluded := "", false
	for _, row := range current {
		if row.SourceDocumentID == occurrence.SourceDocumentID && row.StartOffset == occurrence.StartOffset && row.EndOffset == occurrence.EndOffset {
			if row.CanonicalLemma.Valid {
				currentLemma = row.CanonicalLemma.String
			}
			currentExcluded = row.Excluded
			break
		}
	}
	if currentLemma != occurrence.CorrectedLemma || currentExcluded != occurrence.Excluded {
		return false, pgtype.Text{}, ErrNotFound
	}
	changed := currentLemma != decision.StoredCorrection() || currentExcluded != decision.Excluded
	expectedLemma := pgtype.Text{String: occurrence.CorrectedLemma, Valid: occurrence.CorrectedLemma != ""}
	return changed, expectedLemma, nil
}

// deleteLemmaCorrection clears the stored correction when the decision keeps the
// analyzed lemma; a missing correction is a replay whose eligible row already
// holds the requested state.
func deleteLemmaCorrection(ctx context.Context, q *sqlcgen.Queries, decision domain.LemmaReviewDecision, expectedLemma pgtype.Text, changed bool) (bool, error) {
	occurrence := decision.Occurrence
	deleted, deleteErr := q.DeleteOccurrenceLemmaCorrection(ctx, sqlcgen.DeleteOccurrenceLemmaCorrectionParams{
		Owner: occurrence.OwnerID, Book: occurrence.BookID, AnalysisRun: occurrence.AnalysisRunID,
		SourceDocumentID: occurrence.SourceDocumentID, StartOffset: occurrence.StartOffset, EndOffset: occurrence.EndOffset,
		ExpectedSurface: occurrence.Surface, ExpectedRawLemma: occurrence.RawLemma,
		ExpectedCanonicalLemma: occurrence.CanonicalLemma, ExpectedUpos: occurrence.UPOS,
		ExpectedCorrectedLemma: expectedLemma,
		ExpectedExcluded:       occurrence.Excluded,
	})
	if deleteErr != nil {
		return false, deleteErr
	}
	if deleted != 0 {
		return changed, nil
	}
	rows, lookupErr := listEligibleLemmaReviewRows(ctx, q, occurrence.OwnerID, occurrence.BookID, occurrence.Surface)
	if lookupErr != nil {
		return false, lookupErr
	}
	for _, row := range rows {
		if row.AnalysisRunID == occurrence.AnalysisRunID && row.UnitID == occurrence.SourceDocumentID && row.StartOffset == occurrence.StartOffset && row.EndOffset == occurrence.EndOffset && row.RawLemma == occurrence.RawLemma && row.CanonicalLemma == occurrence.CanonicalLemma && row.Upos == occurrence.UPOS && row.CorrectedLemma == occurrence.CorrectedLemma && row.Excluded == occurrence.Excluded {
			return false, nil
		}
	}
	return false, ErrNotFound
}

// putLemmaCorrection stores the correction or exclusion when the decision
// changes the analyzed lemma.
func putLemmaCorrection(ctx context.Context, q *sqlcgen.Queries, decision domain.LemmaReviewDecision, expectedLemma pgtype.Text, changed bool) (bool, error) {
	occurrence, lemma, excluded := decision.Occurrence, decision.CanonicalLemma, decision.Excluded
	var correctedLemma, normalizationProfile, normalizationVersion pgtype.Text
	if !excluded {
		correctedLemma = pgtype.Text{String: lemma, Valid: true}
		normalizationProfile = pgtype.Text{String: decision.NormalizationProfile, Valid: true}
		normalizationVersion = pgtype.Text{String: decision.NormalizationVersion, Valid: true}
	}
	_, err := q.PutOccurrenceLemmaCorrection(ctx, sqlcgen.PutOccurrenceLemmaCorrectionParams{
		Owner: occurrence.OwnerID, Book: occurrence.BookID, AnalysisRun: occurrence.AnalysisRunID,
		SourceDocumentID: occurrence.SourceDocumentID, StartOffset: occurrence.StartOffset, EndOffset: occurrence.EndOffset,
		CanonicalLemma: correctedLemma, NormalizationProfile: normalizationProfile, NormalizationVersion: normalizationVersion, Excluded: excluded,
		ExpectedSurface: occurrence.Surface, ExpectedRawLemma: occurrence.RawLemma,
		ExpectedCanonicalLemma: occurrence.CanonicalLemma, ExpectedUpos: occurrence.UPOS,
		ExpectedCorrectedLemma: expectedLemma,
		ExpectedExcluded:       occurrence.Excluded,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return changed && err == nil, err
}
