package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
			UPOS: row.Upos, SentenceText: row.SentenceText, CorrectedLemma: row.CorrectedLemma, Excluded: row.Excluded,
		})
	}
	return result, nil
}

func (s *PostgresStore) LemmaReviewStateFingerprint(ctx context.Context, owner, bookID, language, surface string, extras []domain.LemmaReviewIdentity) (string, error) {
	occurrences, err := s.ListLemmaReviewOccurrences(ctx, owner, bookID, surface)
	if err != nil {
		return "", err
	}
	var vocabulary domain.AnalysisCorpusVocabulary
	if len(occurrences) > 0 {
		vocabulary, err = s.GetAnalysisCorpusVocabulary(ctx, owner, occurrences[0].CorpusID)
		if err != nil {
			return "", err
		}
	}
	known, err := s.ListKnownVocabulary(ctx, owner, language)
	if err != nil {
		return "", err
	}
	identities := make(map[domain.LemmaReviewIdentity]bool, len(vocabulary.Lemmas)+len(extras))
	for _, item := range vocabulary.Lemmas {
		identities[domain.LemmaReviewIdentity{Language: item.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS}] = true
	}
	for _, item := range extras {
		identities[item] = true
	}
	reserved := make(map[domain.LemmaReviewIdentity]bool, len(identities))
	for identity := range identities {
		isReserved, reserveErr := s.IsReservedVocabulary(ctx, owner, identity.Language, identity.CanonicalLemma, identity.UPOS)
		if reserveErr != nil {
			return "", reserveErr
		}
		reserved[identity] = isReserved
	}
	return domain.LemmaReviewStateFingerprint(occurrences, vocabulary, known, reserved, extras), nil
}

// PutLemmaCorrection changes exactly the occurrence shown to the learner and
// rejects stale spans, altered analyzer evidence, other owners, and old analyses.
func (s *PostgresStore) PutLemmaCorrection(ctx context.Context, occurrence domain.LemmaReviewOccurrence, lemma, profile, version string) error {
	return s.PutLemmaDecision(ctx, occurrence, lemma, false, profile, version)
}

func (s *PostgresStore) PutLemmaDecision(ctx context.Context, occurrence domain.LemmaReviewOccurrence, lemma string, excluded bool, profile, version string) error {
	return s.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrence, CanonicalLemma: lemma, Excluded: excluded, NormalizationProfile: profile, NormalizationVersion: version}})
}

// PutLemmaDecisions applies an explicitly reviewed set atomically. Every row
// retains its own expected prior state, and one stale member rolls back the set.
func (s *PostgresStore) PutLemmaDecisions(ctx context.Context, decisions []domain.LemmaReviewDecision) error {
	if len(decisions) == 0 {
		return nil
	}
	owner, book := decisions[0].Occurrence.OwnerID, decisions[0].Occurrence.BookID
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
			return err
		}
		if err := lockPrimaryGoalBook(ctx, sqlcgen.New(tx), owner, book); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, owner+":"+book); err != nil {
			return err
		}
		q := sqlcgen.New(tx)
		for _, decision := range decisions {
			if decision.Occurrence.OwnerID != owner || decision.Occurrence.BookID != book {
				return ErrNotFound
			}
			if err := putLemmaDecisionTx(ctx, q, decision); err != nil {
				return err
			}
		}
		return nil
	})
}

// PutLemmaDecisionProposal revalidates the preview fingerprint while holding
// the learner-state lock, then commits the complete selected set atomically.
func (s *PostgresStore) PutLemmaDecisionProposal(ctx context.Context, decisions []domain.LemmaReviewDecision, surface, language string, extras []domain.LemmaReviewIdentity, expectedFingerprint string) error {
	if len(decisions) == 0 {
		return ErrNotFound
	}
	owner, book := decisions[0].Occurrence.OwnerID, decisions[0].Occurrence.BookID
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
			return err
		}
		if err := lockPrimaryGoalBook(ctx, sqlcgen.New(tx), owner, book); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, owner+":"+book); err != nil {
			return err
		}
		fingerprint, err := s.LemmaReviewStateFingerprint(ctx, owner, book, language, surface, extras)
		if err != nil {
			return err
		}
		if fingerprint != expectedFingerprint {
			return ErrNotFound
		}
		q := sqlcgen.New(tx)
		for _, decision := range decisions {
			if decision.Occurrence.OwnerID != owner || decision.Occurrence.BookID != book {
				return ErrNotFound
			}
			if err := putLemmaDecisionTx(ctx, q, decision); err != nil {
				return err
			}
		}
		return nil
	})
}

func lockLemmaReviewLearnerState(ctx context.Context, tx pgx.Tx, owner string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 194))`, owner)
	return err
}

func putLemmaDecisionTx(ctx context.Context, q *sqlcgen.Queries, decision domain.LemmaReviewDecision) error {
	occurrence, lemma, excluded := decision.Occurrence, decision.CanonicalLemma, decision.Excluded
	current, err := q.ListOccurrenceLemmaCorrections(ctx, sqlcgen.ListOccurrenceLemmaCorrectionsParams{
		Owner: occurrence.OwnerID, Book: occurrence.BookID, Corpus: occurrence.CorpusID, AnalysisRun: occurrence.AnalysisRunID,
	})
	if err != nil {
		return err
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
		return ErrNotFound
	}
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
			return deleteErr
		}
		if deleted == 0 {
			rows, lookupErr := q.ListLemmaReviewOccurrences(ctx, sqlcgen.ListLemmaReviewOccurrencesParams{Owner: occurrence.OwnerID, Book: occurrence.BookID, Surface: occurrence.Surface})
			if lookupErr != nil {
				return lookupErr
			}
			for _, row := range rows {
				if row.AnalysisRunID == occurrence.AnalysisRunID && row.UnitID == occurrence.SourceDocumentID && row.StartOffset == occurrence.StartOffset && row.EndOffset == occurrence.EndOffset && row.RawLemma == occurrence.RawLemma && row.CanonicalLemma == occurrence.CanonicalLemma && row.Upos == occurrence.UPOS && row.CorrectedLemma == occurrence.CorrectedLemma && row.Excluded == occurrence.Excluded {
					return nil
				}
			}
			return ErrNotFound
		}
		return nil
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
		return ErrNotFound
	}
	return err
}

func (s *PostgresStore) HasCurrentLemmaCorrections(ctx context.Context, owner, bookID string) (bool, error) {
	return s.queries().HasCurrentLemmaCorrections(ctx, sqlcgen.HasCurrentLemmaCorrectionsParams{Owner: owner, Book: bookID})
}
