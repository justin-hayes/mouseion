package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListVocabularyBrowsePage projects current analyzer tokens through the
// owner's occurrence decisions. It intentionally reads the live corpus, not
// selection_candidates (which are a prepared-deck artifact).
func (s *PostgresStore) ListVocabularyBrowsePage(ctx context.Context, owner, language, prefix string, page int) (domain.VocabularyBrowsePage, error) {
	if page < 1 {
		page = 1
	}
	const query = `
WITH eligible_books AS (
  SELECT b.id::text AS id
  FROM books b
  WHERE b.owner_id = $1 AND b.language_state = 'chosen' AND b.language_tag = $2
), evidence AS (
  SELECT o.book_id, o.upos,
         COALESCE(d.canonical_lemma, o.canonical_lemma) AS lemma,
         (d.canonical_lemma IS NOT NULL) AS corrected
  FROM concordance_occurrences o
  JOIN eligible_books b ON b.id = o.book_id
  LEFT JOIN occurrence_lemma_corrections d
    ON d.owner_id = o.owner_id AND d.book_id::text = o.book_id
   AND d.corpus_id::text = o.corpus_id AND d.analysis_run_id::text = o.analysis_run_id
   AND d.source_document_id = o.unit_id
 AND d.start_offset = o.unit_start_offset AND d.end_offset = o.unit_end_offset
  WHERE o.owner_id = $1 AND o.language = $2 AND o.upos IN ('NOUN','VERB','ADJ','ADV')
    AND o.dependency <> 'compound:prt' AND COALESCE(d.excluded, false) = false
), eligible_evidence AS (
  SELECT * FROM evidence WHERE lemma ~ '[[:alpha:]]'
), grouped AS (
  SELECT lemma, upos, count(*)::bigint AS occurrences,
         count(DISTINCT book_id)::bigint AS books, bool_or(corrected) AS corrected
  FROM eligible_evidence
  GROUP BY lemma, upos
), annotated AS (
  SELECT g.*,
    EXISTS (SELECT 1 FROM known_vocabulary k WHERE k.owner_id = $1 AND k.language = $2 AND k.canonical_lemma = g.lemma AND k.upos = g.upos) AS known,
    EXISTS (SELECT 1 FROM primary_goal_snapshots ps
      JOIN primary_goals pg ON pg.owner_id = ps.owner_id AND pg.snapshot_id = ps.id
      JOIN primary_goal_snapshot_vocabulary pv ON pv.owner_id = ps.owner_id AND pv.snapshot_id = ps.id
      WHERE ps.owner_id = $1 AND ps.language = $2 AND pv.language = $2
        AND pv.canonical_lemma = g.lemma AND pv.upos = g.upos AND ps.released_at IS NULL) AS reserved,
    EXISTS (SELECT 1 FROM generated_vocabulary v WHERE v.owner_id = $1 AND v.language = $2 AND v.canonical_lemma = g.lemma AND v.upos = g.upos) AS generated
  FROM grouped g
 ), filtered AS (
  SELECT * FROM annotated
  WHERE $3 = '' OR left(lower(lemma), length(lower($3))) = lower($3)
 ), page_rows AS (
  SELECT * FROM filtered
  ORDER BY lemma, upos
  LIMIT 25 OFFSET (($4::bigint - 1) * 25)
), summary AS (
  SELECT (SELECT count(*)::bigint FROM filtered) AS total,
         (SELECT count(*)::bigint FROM annotated) AS inventory_total
), analyzed_books AS (
  SELECT cai.book_id::text AS id
  FROM current_analysis_identity cai
  JOIN books b ON b.owner_id = cai.owner_id AND b.id = cai.book_id
  WHERE cai.owner_id = $1 AND b.language_state = 'chosen' AND b.language_tag = $2
), coverage AS (
  SELECT (SELECT count(*)::bigint FROM analyzed_books) AS analyzed,
         (SELECT count(DISTINCT book_id)::bigint FROM eligible_evidence) AS contributing
), book_status AS (
  SELECT b.title,
         EXISTS (SELECT 1 FROM analyzed_books a WHERE a.id = eb.id) AS analyzed,
         EXISTS (SELECT 1 FROM eligible_evidence e WHERE e.book_id = eb.id) AS contributing
  FROM eligible_books eb JOIN books b ON b.id::text = eb.id AND b.owner_id = $1
)
SELECT p.lemma, p.upos, p.occurrences, p.books, p.corrected, p.known, p.reserved, p.generated,
       summary.total, summary.inventory_total, coverage.analyzed, coverage.contributing,
       (SELECT count(*) FROM eligible_books)::bigint - coverage.contributing AS noncontributing,
       (SELECT count(*) FROM eligible_books)::bigint - coverage.analyzed AS without_current_analysis,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('title', title, 'has_current_analysis', analyzed,
         'has_vocabulary_evidence', contributing) ORDER BY title) FROM book_status), '[]'::jsonb) AS book_status
FROM summary CROSS JOIN coverage LEFT JOIN page_rows p ON true
ORDER BY p.lemma, p.upos`
	rows, err := s.pool.Query(ctx, query, owner, language, strings.TrimSpace(prefix), page)
	if err != nil {
		return domain.VocabularyBrowsePage{}, fmt.Errorf("query effective vocabulary browse: %w", err)
	}
	defer rows.Close()
	result := domain.VocabularyBrowsePage{Page: page}
	var bookStatus []byte
	for rows.Next() {
		var row domain.VocabularyBrowseRow
		var lemma, upos *string
		var occurrences, books *int64
		var corrected, known, reserved, generated *bool
		if err := rows.Scan(&lemma, &upos, &occurrences, &books, &corrected, &known, &reserved, &generated, &result.Total, &result.InventoryTotal, &result.AnalyzedBooks, &result.ContributingBooks, &result.NoncontributingBooks, &result.BooksWithoutCurrentAnalysis, &bookStatus); err != nil {
			return domain.VocabularyBrowsePage{}, err
		}
		if lemma != nil {
			row.CanonicalLemma = *lemma
			row.UPOS = *upos
			row.OccurrenceCount = *occurrences
			row.BookCount = *books
			row.Corrected = *corrected
			row.Known = *known
			row.Reserved = *reserved
			row.Generated = *generated
			result.Rows = append(result.Rows, row)
		}
	}
	if err := json.Unmarshal(bookStatus, &result.Books); err != nil {
		return domain.VocabularyBrowsePage{}, fmt.Errorf("decode vocabulary Book status: %w", err)
	}
	if err := rows.Err(); err != nil {
		return domain.VocabularyBrowsePage{}, err
	}
	lastPage := int((result.Total + 24) / 25)
	if lastPage > 0 && page > lastPage {
		return s.ListVocabularyBrowsePage(ctx, owner, language, prefix, lastPage)
	}
	return result, nil
}
