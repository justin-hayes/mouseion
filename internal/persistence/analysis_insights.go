package persistence

import (
	"context"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
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
	return value, nil
}
