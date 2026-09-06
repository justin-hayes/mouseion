package persistence

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListLanguageCorpusEvidence uses two SQL queries per call: one for the
// owner/language book and current-analysis rows, and one for all lemma rows
// belonging to those current corpora. The second query is deliberately batched
// with ANY rather than loaded per book. Neither query selects source text or
// sentence data.
func (s *PostgresStore) ListLanguageCorpusEvidence(ctx context.Context, owner, language string) ([]domain.LanguageCorpusBookEvidence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.pool.Query(ctx, currentAnalysisCTE+`
		SELECT b.id::text,b.owner_id::text,b.title,b.metadata_provenance,b.language_state,COALESCE(b.language_tag,''),b.created_at,b.updated_at,
		       COALESCE(s.id::text,''),COALESCE(s.language,''),
		       CASE WHEN s.id IS NULL THEN 'not_acquired'
		            WHEN s.current_content_revision_id IS NULL OR s.current_snapshot_id IS NULL THEN 'unavailable'
		            WHEN p.analysis_run_id IS NOT NULL AND ca.analysis_run_id IS NULL THEN 'stale'
		            WHEN ca.analysis_run_id IS NOT NULL THEN 'analyzed'
		            ELSE 'acquired_unassessed' END,
		       COALESCE(p.source_material_id::text,''),COALESCE(p.analysis_run_id::text,''),
		       COALESCE(ca.source_material_id::text,''),COALESCE(ca.analysis_run_id::text,''),COALESCE(ca.corpus_id::text,''),
		       c.analyzable_token_count,c.distinct_lemma_count
		FROM books b
		JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active'
		LEFT JOIN book_current_analyses p ON p.owner_id=b.owner_id AND p.book_id=b.id
		LEFT JOIN current_analysis ca ON ca.owner_id=b.owner_id AND ca.book_id=b.id
		LEFT JOIN corpora c ON c.owner_id=ca.owner_id AND c.id=ca.corpus_id
		LEFT JOIN LATERAL (SELECT s.id,s.language,s.current_content_revision_id,s.current_snapshot_id
			FROM source_materials s
			WHERE s.owner_id=b.owner_id AND s.book_id=b.id
			ORDER BY CASE WHEN p.source_material_id IS NOT NULL AND s.id=p.source_material_id THEN 0 ELSE 1 END,s.created_at DESC,s.id DESC LIMIT 1) s ON true
		WHERE b.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2
		ORDER BY lower(b.title),b.title,b.id`, owner, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.LanguageCorpusBookEvidence
	var corpusIDs []string
	byCorpus := make(map[string]int)
	for rows.Next() {
		var item domain.LanguageCorpusBookEvidence
		var statisticsAnalyzable, statisticsDistinct *int64
		if err := rows.Scan(&item.Book.ID, &item.Book.OwnerID, &item.Book.Title, &item.Book.MetadataProvenance, &item.Book.LanguageState, &item.Book.LanguageTag, &item.Book.CreatedAt, &item.Book.UpdatedAt,
			&item.SourceMaterialID, &item.SourceLanguage, &item.EvidenceState,
			&item.CurrentSourceMaterialID, &item.CurrentAnalysisRunID, &item.AnalysisSourceMaterialID, &item.AnalysisRunID, &item.CorpusID,
			&statisticsAnalyzable, &statisticsDistinct); err != nil {
			return nil, err
		}
		if statisticsAnalyzable != nil && statisticsDistinct != nil {
			item.Statistics = &domain.AnalysisStatistics{AnalyzableTokenCount: *statisticsAnalyzable, DistinctLemmaCount: *statisticsDistinct}
		}
		if item.CorpusID != "" {
			byCorpus[item.CorpusID] = len(result)
			corpusIDs = append(corpusIDs, item.CorpusID)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(corpusIDs) == 0 {
		return result, nil
	}

	lemmaRows, err := s.pool.Query(ctx, `SELECT co.id::text,sl.language,sl.canonical_lemma,sl.upos,SUM(sl.frequency)::bigint
		FROM corpora co
		JOIN shared_lemmas sl ON sl.content_hash=co.artifact_hash
		WHERE co.owner_id=$1 AND co.id::text=ANY($2::text[]) AND upper(sl.upos) IN ('NOUN','VERB','ADJ','ADV') AND btrim(sl.canonical_lemma)<>''
		GROUP BY co.id,sl.language,sl.canonical_lemma,sl.upos
		ORDER BY co.id,sl.language,sl.canonical_lemma,sl.upos`, owner, corpusIDs)
	if err != nil {
		return nil, err
	}
	defer lemmaRows.Close()
	for lemmaRows.Next() {
		var corpusID string
		var lemma domain.LemmaOccurrence
		if err := lemmaRows.Scan(&corpusID, &lemma.Language, &lemma.CanonicalLemma, &lemma.UPOS, &lemma.OccurrenceCount); err != nil {
			return nil, err
		}
		if index, ok := byCorpus[corpusID]; ok {
			result[index].Lemmas = append(result[index].Lemmas, lemma)
		}
	}
	if err := lemmaRows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
