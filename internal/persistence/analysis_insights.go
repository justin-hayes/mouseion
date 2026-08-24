package persistence

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// GetAnalysisCorpusVocabulary resolves the corpus through its owner and groups
// morphology-specific artifact rows into lemma-and-UPOS occurrence identities.
func (s *PostgresStore) GetAnalysisCorpusVocabulary(ctx context.Context, owner, corpusID string) (value domain.AnalysisCorpusVocabulary, err error) {
	var analyzableTokenCount, distinctLemmaCount *int64
	err = s.pool.QueryRow(ctx, `SELECT id::text,source_material_id::text,analyzable_token_count,distinct_lemma_count FROM corpora WHERE owner_id=$1 AND id=$2`, owner, corpusID).Scan(&value.CorpusID, &value.SourceMaterialID, &analyzableTokenCount, &distinctLemmaCount)
	if err = missing(err); err != nil {
		return value, err
	}
	if analyzableTokenCount != nil && distinctLemmaCount != nil {
		value.Statistics = &domain.AnalysisStatistics{AnalyzableTokenCount: *analyzableTokenCount, DistinctLemmaCount: *distinctLemmaCount}
	}
	rows, err := s.pool.Query(ctx, `SELECT sl.language,sl.canonical_lemma,sl.upos,SUM(sl.frequency)::bigint FROM corpora co JOIN shared_lemmas sl ON sl.content_hash=co.artifact_hash WHERE co.owner_id=$1 AND co.id=$2 GROUP BY sl.language,sl.canonical_lemma,sl.upos ORDER BY sl.language,sl.canonical_lemma,sl.upos`, owner, corpusID)
	if err != nil {
		return value, err
	}
	defer rows.Close()
	for rows.Next() {
		var lemma domain.LemmaOccurrence
		if err = rows.Scan(&lemma.Language, &lemma.CanonicalLemma, &lemma.UPOS, &lemma.OccurrenceCount); err != nil {
			return value, err
		}
		value.Lemmas = append(value.Lemmas, lemma)
	}
	return value, rows.Err()
}
