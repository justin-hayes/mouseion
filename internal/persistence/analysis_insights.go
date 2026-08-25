package persistence

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
)

// GetAnalysisCorpusVocabulary resolves the corpus through its owner and groups
// morphology-specific artifact rows into lemma-and-UPOS occurrence identities.
func (s *PostgresStore) GetAnalysisCorpusVocabulary(ctx context.Context, owner, corpusID string) (value domain.AnalysisCorpusVocabulary, err error) {
	var analyzableTokenCount, distinctLemmaCount, sentenceCount, normalizedTokenCount, emptySentenceCount, p90SentenceTokenCount, longSentenceCount *int64
	var medianSentenceTokenCount *float64
	err = s.pool.QueryRow(ctx, `SELECT id::text,source_material_id::text,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count FROM corpora WHERE owner_id=$1 AND id=$2`, owner, corpusID).Scan(&value.CorpusID, &value.SourceMaterialID, &analyzableTokenCount, &distinctLemmaCount, &sentenceCount, &normalizedTokenCount, &emptySentenceCount, &medianSentenceTokenCount, &p90SentenceTokenCount, &longSentenceCount)
	if err = missing(err); err != nil {
		return value, err
	}
	if analyzableTokenCount != nil && distinctLemmaCount != nil {
		value.Statistics = &domain.AnalysisStatistics{AnalyzableTokenCount: *analyzableTokenCount, DistinctLemmaCount: *distinctLemmaCount}
		if sentenceCount != nil && normalizedTokenCount != nil && emptySentenceCount != nil && medianSentenceTokenCount != nil && p90SentenceTokenCount != nil && longSentenceCount != nil {
			value.Statistics.TextProfile = &domain.TextProfile{SentenceCount: *sentenceCount, NormalizedTokenCount: *normalizedTokenCount, EmptySentenceCount: *emptySentenceCount, MedianSentenceTokenCount: *medianSentenceTokenCount, P90SentenceTokenCount: *p90SentenceTokenCount, LongSentenceCount: *longSentenceCount}
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT sl.language,sl.canonical_lemma,sl.upos,SUM(sl.frequency)::bigint FROM corpora co JOIN shared_lemmas sl ON sl.content_hash=co.artifact_hash WHERE co.owner_id=$1 AND co.id=$2 AND upper(sl.upos) IN ('NOUN','VERB','ADJ','ADV') AND btrim(sl.canonical_lemma)<>'' GROUP BY sl.language,sl.canonical_lemma,sl.upos ORDER BY sl.language,sl.canonical_lemma,sl.upos`, owner, corpusID)
	if err != nil {
		return value, err
	}
	defer rows.Close()
	for rows.Next() {
		var lemma domain.LemmaOccurrence
		if err = rows.Scan(&lemma.Language, &lemma.CanonicalLemma, &lemma.UPOS, &lemma.OccurrenceCount); err != nil {
			return value, err
		}
		if !lexical.IsLemma(lemma.CanonicalLemma) {
			if value.Statistics != nil {
				value.Statistics.AnalyzableTokenCount = max(value.Statistics.AnalyzableTokenCount-lemma.OccurrenceCount, 0)
				value.Statistics.DistinctLemmaCount = max(value.Statistics.DistinctLemmaCount-1, 0)
			}
			continue
		}
		value.Lemmas = append(value.Lemmas, lemma)
	}
	return value, rows.Err()
}
