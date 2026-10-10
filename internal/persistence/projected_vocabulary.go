package persistence

import (
	"context"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// GetProjectedCorpusVocabulary returns a corpus's effective vocabulary counts
// from the Browse count projection, the same per-Book counts My Books uses.
// Persisted analyzer statistics stay the coverage denominator, so excluded
// occurrences remain in the analyzable-token total. When the Book's current
// analysis has no ready projection, Ready is false and no counts are returned.
func (s *PostgresStore) GetProjectedCorpusVocabulary(ctx context.Context, owner, corpusID string) (domain.ProjectedCorpusVocabulary, error) {
	var value domain.ProjectedCorpusVocabulary
	corpus, err := s.queries().GetCorpus(ctx, sqlcgen.GetCorpusParams{OwnerID: owner, ID: corpusID})
	if err != nil {
		return value, missing(err)
	}
	value.CorpusID, value.SourceMaterialID, value.AnalysisRunID = corpus.ID, corpus.SourceMaterialID, corpus.AnalysisRunID
	value.Statistics = corpusFromRow(corpus).Statistics
	rows, err := s.queries().ListProjectedCorpusVocabulary(ctx, sqlcgen.ListProjectedCorpusVocabularyParams{Owner: owner, Corpus: corpusID})
	if err != nil {
		return value, err
	}
	value.Ready = len(rows) > 0
	for _, row := range rows {
		if row.CanonicalLemma == "" {
			continue
		}
		value.Lemmas = append(value.Lemmas, domain.LemmaOccurrence{Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, OccurrenceCount: row.OccurrenceCount})
	}
	if value.Statistics != nil {
		value.Statistics.DistinctLemmaCount = int64(len(value.Lemmas))
	}
	return value, nil
}
