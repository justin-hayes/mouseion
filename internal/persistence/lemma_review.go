package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

// SaveLemmaReviewFlags records new detector evidence without reopening a
// learner-resolved flag. The Book advisory lock is shared with decisions and
// both vocabulary freeze paths.
func (s *PostgresStore) SaveLemmaReviewFlags(ctx context.Context, flags []domain.LemmaReviewFlag) error {
	if len(flags) == 0 {
		return nil
	}
	first := flags[0].Occurrence
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := sqlcgen.New(tx).GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: first.OwnerID, ID: first.BookID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, first.OwnerID+":"+first.BookID); err != nil {
			return err
		}
		for _, flag := range flags {
			o := flag.Occurrence
			if o.OwnerID != first.OwnerID || o.BookID != first.BookID || o.CorpusID == "" || o.AnalysisRunID == "" || o.SourceDocumentID == "" || o.EndOffset <= o.StartOffset || strings.TrimSpace(flag.Reason) == "" {
				return ErrNotFound
			}
			var occurrenceExists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(
				SELECT 1 FROM book_current_analyses cai
				JOIN corpora c ON c.owner_id=cai.owner_id AND c.id=$3 AND c.analysis_run_id=cai.analysis_run_id
				JOIN corpus_tokens t ON t.owner_id=c.owner_id AND t.corpus_id=c.id AND t.analysis_run_id=c.analysis_run_id
				JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.corpus_id=t.corpus_id AND s.analysis_run_id=t.analysis_run_id AND s.sentence_ordinal=t.sentence_ordinal
				WHERE cai.owner_id=$1 AND cai.book_id=$2 AND cai.analysis_run_id=$4
				AND t.start_offset=$5 AND t.end_offset=$6 AND s.unit_id=$7)`, o.OwnerID, o.BookID, o.CorpusID, o.AnalysisRunID, o.StartOffset, o.EndOffset, o.SourceDocumentID).Scan(&occurrenceExists); err != nil {
				return err
			}
			if !occurrenceExists {
				return ErrNotFound
			}
			provenanceValue := flag.Provenance
			if provenanceValue == nil {
				provenanceValue = map[string]any{}
			}
			provenance, err := json.Marshal(provenanceValue)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO occurrence_lemma_review_flags(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,reason,evidence_provenance) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb) ON CONFLICT(owner_id,book_id,analysis_run_id,source_document_id,start_offset,end_offset) DO NOTHING`, o.OwnerID, o.BookID, o.CorpusID, o.AnalysisRunID, o.SourceDocumentID, o.StartOffset, o.EndOffset, flag.Reason, string(provenance))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func unresolvedLemmaReviewFlags(ctx context.Context, tx pgx.Tx, owner, book, analysisRun string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM occurrence_lemma_review_flags WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND resolution IS NULL)`, owner, book, analysisRun).Scan(&exists)
	return exists, err
}

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
		if selection.OccurrenceEligible(cfg, row.CanonicalLemma, row.Upos, row.Dependency) {
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
			ReviewFlagReason: row.ReviewFlagReason, ReviewFlagResolution: row.ReviewFlagResolution,
		}
		if err := json.Unmarshal([]byte(row.ReviewFlagProvenance), &occurrence.ReviewFlagProvenance); err != nil {
			return nil, err
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
		if err := lockCurrentReadingBook(ctx, sqlcgen.New(tx), owner, book); err != nil {
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
		if err := lockCurrentReadingBook(ctx, sqlcgen.New(tx), owner, book); err != nil {
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
		if err := resolveLemmaReviewFlag(ctx, tx, decision); err != nil {
			return err
		}
	}
	if len(changed) == 0 {
		return nil
	}
	if countReadiness.ready {
		return applyVocabularyBrowseDecisionDeltasTx(ctx, tx, countReadiness, changed)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner, book); err != nil {
		return err
	}
	return nil
}

func resolveLemmaReviewFlag(ctx context.Context, tx pgx.Tx, decision domain.LemmaReviewDecision) error {
	o := decision.Occurrence
	_, err := tx.Exec(ctx, `UPDATE occurrence_lemma_review_flags SET resolution=$7,resolved_at=now() WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND source_document_id=$4 AND start_offset=$5 AND end_offset=$6`, o.OwnerID, o.BookID, o.AnalysisRunID, o.SourceDocumentID, o.StartOffset, o.EndOffset, decision.Resolution())
	return err
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
	evidence, err := q.GetCurrentAnalysisOccurrenceEvidence(ctx, sqlcgen.GetCurrentAnalysisOccurrenceEvidenceParams{
		Owner: occurrence.OwnerID, Book: occurrence.BookID, AnalysisRun: occurrence.AnalysisRunID,
		SourceDocumentID: occurrence.SourceDocumentID, StartOffset: occurrence.StartOffset, EndOffset: occurrence.EndOffset,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if !selection.OccurrenceEligible(selection.DefaultConfig(""), evidence.CanonicalLemma, evidence.Upos, evidence.Dependency) {
		return false, ErrNotFound
	}
	current, err := q.ListOccurrenceLemmaCorrections(ctx, sqlcgen.ListOccurrenceLemmaCorrectionsParams{
		Owner: occurrence.OwnerID, Book: occurrence.BookID, Corpus: occurrence.CorpusID, AnalysisRun: occurrence.AnalysisRunID,
	})
	if err != nil {
		return false, err
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
		return false, ErrNotFound
	}
	changed := currentLemma != decision.StoredCorrection() || currentExcluded != excluded
	expectedLemma := pgtype.Text{String: occurrence.CorrectedLemma, Valid: occurrence.CorrectedLemma != ""}
	if lemma == occurrence.CanonicalLemma && !excluded {
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
		if deleted == 0 {
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
		return changed, nil
	}
	var correctedLemma, normalizationProfile, normalizationVersion pgtype.Text
	if !excluded {
		correctedLemma = pgtype.Text{String: lemma, Valid: true}
		normalizationProfile = pgtype.Text{String: decision.NormalizationProfile, Valid: true}
		normalizationVersion = pgtype.Text{String: decision.NormalizationVersion, Valid: true}
	}
	_, err = q.PutOccurrenceLemmaCorrection(ctx, sqlcgen.PutOccurrenceLemmaCorrectionParams{
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
