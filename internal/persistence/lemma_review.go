package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListLemmaReviewOccurrences resolves an exact observed form only in the
// owner's current completed analysis for the given Book.
func (s *PostgresStore) ListLemmaReviewOccurrences(ctx context.Context, owner, bookID, surface string) ([]domain.LemmaReviewOccurrence, error) {
	rows, err := s.queries().ListLemmaReviewOccurrences(ctx, sqlcgen.ListLemmaReviewOccurrencesParams{Owner: owner, Book: bookID, Surface: surface})
	if err != nil {
		return nil, err
	}
	result := make([]domain.LemmaReviewOccurrence, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.LemmaReviewOccurrence{
			OwnerID: owner, BookID: bookID, CorpusID: row.CorpusID, AnalysisRunID: row.AnalysisRunID,
			SourceDocumentID: row.UnitID, StartOffset: row.StartOffset, EndOffset: row.EndOffset,
			SentenceOrdinal: row.SentenceOrdinal, TokenOrdinal: row.TokenOrdinal,
			Surface: row.Surface, RawLemma: row.RawLemma, CanonicalLemma: row.CanonicalLemma,
			UPOS: row.Upos, SentenceText: row.SentenceText, CorrectedLemma: row.CorrectedLemma,
		})
	}
	return result, nil
}

// PutLemmaCorrection changes exactly the occurrence shown to the learner and
// rejects stale spans, altered analyzer evidence, other owners, and old analyses.
func (s *PostgresStore) PutLemmaCorrection(ctx context.Context, occurrence domain.LemmaReviewOccurrence, lemma, profile, version string) error {
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockPrimaryGoalBook(ctx, sqlcgen.New(tx), occurrence.OwnerID, occurrence.BookID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, occurrence.OwnerID+":"+occurrence.BookID); err != nil {
			return err
		}
		q := sqlcgen.New(tx)
		if lemma == occurrence.CanonicalLemma {
			deleted, deleteErr := q.DeleteOccurrenceLemmaCorrection(ctx, sqlcgen.DeleteOccurrenceLemmaCorrectionParams{
				Owner: occurrence.OwnerID, Book: occurrence.BookID, AnalysisRun: occurrence.AnalysisRunID,
				SourceDocumentID: occurrence.SourceDocumentID, StartOffset: occurrence.StartOffset, EndOffset: occurrence.EndOffset,
				ExpectedSurface: occurrence.Surface, ExpectedRawLemma: occurrence.RawLemma,
				ExpectedCanonicalLemma: occurrence.CanonicalLemma, ExpectedUpos: occurrence.UPOS,
				ExpectedCorrectedLemma: occurrence.CorrectedLemma,
			})
			if deleteErr != nil {
				return deleteErr
			}
			if deleted == 0 {
				rows, lookupErr := q.ListLemmaReviewOccurrences(ctx, sqlcgen.ListLemmaReviewOccurrencesParams{Owner: occurrence.OwnerID, Book: occurrence.BookID, Surface: occurrence.Surface})
				if lookupErr != nil {
					return lookupErr
				}
				for _, row := range rows {
					if row.AnalysisRunID == occurrence.AnalysisRunID && row.UnitID == occurrence.SourceDocumentID && row.StartOffset == occurrence.StartOffset && row.EndOffset == occurrence.EndOffset && row.RawLemma == occurrence.RawLemma && row.CanonicalLemma == occurrence.CanonicalLemma && row.Upos == occurrence.UPOS && row.CorrectedLemma == occurrence.CorrectedLemma {
						return nil
					}
				}
				return ErrNotFound
			}
			return nil
		}
		_, err := q.PutOccurrenceLemmaCorrection(ctx, sqlcgen.PutOccurrenceLemmaCorrectionParams{
			Owner: occurrence.OwnerID, Book: occurrence.BookID, AnalysisRun: occurrence.AnalysisRunID,
			SourceDocumentID: occurrence.SourceDocumentID, StartOffset: occurrence.StartOffset, EndOffset: occurrence.EndOffset,
			CanonicalLemma: lemma, NormalizationProfile: profile, NormalizationVersion: version,
			ExpectedSurface: occurrence.Surface, ExpectedRawLemma: occurrence.RawLemma,
			ExpectedCanonicalLemma: occurrence.CanonicalLemma, ExpectedUpos: occurrence.UPOS,
			ExpectedCorrectedLemma: occurrence.CorrectedLemma,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
}

func (s *PostgresStore) HasCurrentLemmaCorrections(ctx context.Context, owner, bookID string) (bool, error) {
	return s.queries().HasCurrentLemmaCorrections(ctx, sqlcgen.HasCurrentLemmaCorrectionsParams{Owner: owner, Book: bookID})
}
