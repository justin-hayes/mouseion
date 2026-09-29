package persistence

import (
	"context"
	"errors"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// GetAnalysisCorpusVocabulary resolves the corpus through its owner and groups
// morphology-specific artifact rows into lemma-and-UPOS occurrence identities.
func (s *PostgresStore) GetAnalysisCorpusVocabulary(ctx context.Context, owner, corpusID string) (value domain.AnalysisCorpusVocabulary, err error) {
	corpus, err := s.queries().GetCorpus(ctx, sqlcgen.GetCorpusParams{OwnerID: owner, ID: corpusID})
	if err != nil {
		return value, missing(err)
	}
	value.CorpusID, value.SourceMaterialID, value.AnalysisRunID = corpus.ID, corpus.SourceMaterialID, corpus.AnalysisRunID
	value.Statistics = corpusFromRow(corpus).Statistics
	rows, err := s.queries().ListAnalysisCorpusVocabulary(ctx, sqlcgen.ListAnalysisCorpusVocabularyParams{OwnerID: owner, ID: corpusID})
	if err != nil {
		return value, err
	}
	for _, row := range rows {
		lemma := domain.LemmaOccurrence{Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, OccurrenceCount: row.OccurrenceCount}
		if !lexical.IsLemma(lemma.CanonicalLemma) {
			if value.Statistics != nil {
				value.Statistics.AnalyzableTokenCount = max(value.Statistics.AnalyzableTokenCount-lemma.OccurrenceCount, 0)
				value.Statistics.DistinctLemmaCount = max(value.Statistics.DistinctLemmaCount-1, 0)
			}
			continue
		}
		value.Lemmas = append(value.Lemmas, lemma)
	}
	// Occurrence decisions belong to the exact current Book analysis. Rebuild
	// only the owner-facing identity list; persisted analyzer statistics remain
	// the authoritative coverage denominator.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return value, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	var bookID string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(book_id::text, '') FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, corpus.SourceMaterialID).Scan(&bookID); err != nil {
		return value, err
	}
	if bookID == "" {
		return value, nil
	}
	hasCorrections, err := sqlcgen.New(tx).HasCurrentLemmaCorrectionsForAnalysis(ctx, sqlcgen.HasCurrentLemmaCorrectionsForAnalysisParams{
		Owner: owner, Book: bookID, Corpus: corpusID, AnalysisRun: corpus.AnalysisRunID,
	})
	if err != nil {
		return value, err
	}
	if !hasCorrections {
		return value, nil
	}
	analysis, decisions, err := loadAnalysisProjectionFactsTx(ctx, tx, sqlcgen.New(tx), domain.DeckPreparation{
		OwnerID: owner, SourceMaterialID: corpus.SourceMaterialID, AnalysisRunID: corpus.AnalysisRunID,
	}, corpusID)
	if err != nil {
		return value, err
	}
	projectionDecisions := make([]selection.OccurrenceDecision, 0, len(decisions))
	for _, correction := range decisions {
		start, startErr := checked.Uint64FromInt64(correction.StartOffset)
		end, endErr := checked.Uint64FromInt64(correction.EndOffset)
		if startErr != nil || endErr != nil {
			return value, errors.New("persisted lemma correction contains invalid offsets")
		}
		projectionDecisions = append(projectionDecisions, selection.OccurrenceDecision{
			Occurrence: selection.OccurrenceIdentity{SourceDocumentID: correction.SourceDocumentID, StartOffset: start, EndOffset: end},
			Lemma:      correction.CanonicalLemma,
			Excluded:   correction.Excluded,
		})
	}
	projected, err := selection.Project(analysis, selection.DefaultConfig(corpusID), projectionDecisions)
	if err != nil {
		return value, err
	}
	value.Lemmas = value.Lemmas[:0]
	for _, candidate := range projected {
		if !lexical.IsLemma(candidate.Identity.CanonicalLemma) {
			continue
		}
		value.Lemmas = append(value.Lemmas, domain.LemmaOccurrence{
			Language: candidate.Identity.Language, CanonicalLemma: candidate.Identity.CanonicalLemma,
			UPOS: candidate.Identity.UPOS, OccurrenceCount: int64(candidate.OccurrenceCount),
		})
	}
	if value.Statistics != nil {
		value.Statistics.DistinctLemmaCount = int64(len(value.Lemmas))
	}
	return value, nil
}
