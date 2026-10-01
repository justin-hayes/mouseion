package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListVocabularyBrowsePage projects current analyzer tokens through the
// owner's occurrence decisions. Book and state filters are discovery controls;
// they never affect learner state or prepared-deck candidates.
func (s *PostgresStore) ListVocabularyBrowsePage(ctx context.Context, owner, language string, queryParams domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	if queryParams.Page < 1 {
		queryParams.Page = 1
	}
	queryParams.BookIDs = uniqueStrings(queryParams.BookIDs)
	queryParams.UPOS = vocabularyBrowseUPOS(queryParams.UPOS)
	queryParams.KnownFilter = vocabularyBrowseStateFilter(queryParams.KnownFilter)
	queryParams.ReservedFilter = vocabularyBrowseStateFilter(queryParams.ReservedFilter)
	if queryParams.Sort != "occurrences" && queryParams.Sort != "books" {
		queryParams.Sort = "lemma"
	}
	const query = `
WITH all_books AS (
  SELECT b.id::text AS id, b.title,
    EXISTS (SELECT 1 FROM current_analysis_identity cai WHERE cai.owner_id=b.owner_id AND cai.book_id=b.id) AS analyzed
  FROM books b WHERE b.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2
), scope_books AS (
  SELECT * FROM all_books WHERE COALESCE(cardinality($4::text[]),0)=0 OR id=ANY($4::text[])
), all_evidence AS (
  SELECT o.book_id, o.upos, COALESCE(d.canonical_lemma,o.canonical_lemma) AS lemma
  FROM concordance_occurrences o JOIN all_books b ON b.id=o.book_id
  LEFT JOIN occurrence_lemma_corrections d
    ON d.owner_id=o.owner_id AND d.book_id::text=o.book_id
   AND d.corpus_id::text=o.corpus_id AND d.analysis_run_id::text=o.analysis_run_id
   AND d.source_document_id=o.unit_id AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
  WHERE o.owner_id=$1 AND o.language=$2 AND o.upos IN ('NOUN','VERB','ADJ','ADV')
    AND o.dependency <> 'compound:prt' AND COALESCE(d.excluded,false)=false
    AND COALESCE(d.canonical_lemma,o.canonical_lemma) ~ '[[:alpha:]]'
), evidence AS (
  SELECT o.book_id, o.upos,
         COALESCE(d.canonical_lemma, o.canonical_lemma) AS lemma,
         (d.canonical_lemma IS NOT NULL) AS corrected
  FROM concordance_occurrences o
  JOIN scope_books b ON b.id=o.book_id
  LEFT JOIN occurrence_lemma_corrections d
    ON d.owner_id=o.owner_id AND d.book_id::text=o.book_id
   AND d.corpus_id::text=o.corpus_id AND d.analysis_run_id::text=o.analysis_run_id
   AND d.source_document_id=o.unit_id
   AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
  WHERE o.owner_id=$1 AND o.language=$2 AND o.upos IN ('NOUN','VERB','ADJ','ADV')
    AND o.dependency <> 'compound:prt' AND COALESCE(d.excluded,false)=false
	    AND (COALESCE(cardinality($5::text[]),0)=0 OR o.upos=ANY($5::text[]))
), eligible_evidence AS (
  SELECT * FROM evidence WHERE lemma ~ '[[:alpha:]]'
), grouped AS (
  SELECT lemma, upos, count(*)::bigint AS occurrences,
    count(DISTINCT book_id)::bigint AS books, bool_or(corrected) AS corrected
  FROM eligible_evidence GROUP BY lemma, upos
), annotated AS (
  SELECT g.*,
    EXISTS (SELECT 1 FROM known_vocabulary k WHERE k.owner_id=$1 AND k.language=$2 AND k.canonical_lemma=g.lemma AND k.upos=g.upos) AS known,
    EXISTS (SELECT 1 FROM primary_goal_snapshots ps
      JOIN primary_goals pg ON pg.owner_id=ps.owner_id AND pg.snapshot_id=ps.id
      JOIN primary_goal_snapshot_vocabulary pv ON pv.owner_id=ps.owner_id AND pv.snapshot_id=ps.id
      WHERE ps.owner_id=$1 AND ps.language=$2 AND pv.language=$2
        AND pv.canonical_lemma=g.lemma AND pv.upos=g.upos AND ps.released_at IS NULL) AS reserved,
    EXISTS (SELECT 1 FROM generated_vocabulary v WHERE v.owner_id=$1 AND v.language=$2 AND v.canonical_lemma=g.lemma AND v.upos=g.upos) AS generated
  FROM grouped g
), eligible AS (
  SELECT * FROM annotated
  WHERE ($3='' OR left(lower(lemma),length(lower($3)))=lower($3))
	    AND ($6='any' OR ($6='known' AND known) OR ($6='not-known' AND NOT known) OR $6='not-known-or-reserved')
	    AND ($7='any' OR ($7='reserved' AND reserved) OR ($7='not-reserved' AND NOT reserved))
    AND ($6<>'not-known-or-reserved' OR (NOT known AND NOT reserved))
), page_rows AS (
  SELECT * FROM eligible
  ORDER BY CASE WHEN $8='occurrences' THEN occurrences END DESC,
	           CASE WHEN $8='books' THEN books END DESC, lemma, upos
	  LIMIT 25 OFFSET (($9::bigint-1)*25)
), summary AS (
  SELECT (SELECT count(*)::bigint FROM eligible) AS total,
         (SELECT count(*)::bigint FROM grouped) AS scoped_total,
          (SELECT count(*)::bigint FROM (SELECT DISTINCT lemma,upos FROM all_evidence) all_identities) AS inventory_total
), coverage AS (
  SELECT count(*)::bigint AS total,
    count(*) FILTER (WHERE analyzed)::bigint AS analyzed,
    count(*) FILTER (WHERE has_evidence)::bigint AS contributing
  FROM (
    SELECT b.*, EXISTS (SELECT 1 FROM all_evidence e WHERE e.book_id=b.id) AS has_evidence
    FROM scope_books b
  ) scoped
), book_status AS (
  SELECT b.id,b.title,b.analyzed,
    EXISTS (SELECT 1 FROM all_evidence e WHERE e.book_id=b.id) AS contributing
  FROM all_books b
)
SELECT p.lemma,p.upos,p.occurrences,p.books,p.corrected,p.known,p.reserved,p.generated,
  summary.total,summary.scoped_total,summary.inventory_total,coverage.analyzed,coverage.contributing,
  coverage.total-coverage.contributing,coverage.total-coverage.analyzed,
  COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'title',title,'has_current_analysis',analyzed,
    'has_vocabulary_evidence',contributing) ORDER BY title,id) FROM book_status),'[]'::jsonb)
FROM summary CROSS JOIN coverage LEFT JOIN page_rows p ON true
ORDER BY CASE WHEN $8='occurrences' THEN p.occurrences END DESC,
	         CASE WHEN $8='books' THEN p.books END DESC,p.lemma,p.upos`
	rows, err := s.pool.Query(ctx, query, owner, language, strings.TrimSpace(queryParams.Prefix), queryParams.BookIDs, queryParams.UPOS, queryParams.KnownFilter, queryParams.ReservedFilter, queryParams.Sort, queryParams.Page)
	if err != nil {
		return domain.VocabularyBrowsePage{}, fmt.Errorf("query effective vocabulary browse: %w", err)
	}
	defer rows.Close()
	result := domain.VocabularyBrowsePage{Page: queryParams.Page, SelectedBooks: queryParams.BookIDs, SelectedUPOS: queryParams.UPOS, KnownFilter: queryParams.KnownFilter, ReservedFilter: queryParams.ReservedFilter, Sort: queryParams.Sort}
	var bookStatus []byte
	for rows.Next() {
		var row domain.VocabularyBrowseRow
		var lemma, pos *string
		var occurrences, books *int64
		var corrected, isKnown, isReserved, generated *bool
		if err := rows.Scan(&lemma, &pos, &occurrences, &books, &corrected, &isKnown, &isReserved, &generated,
			&result.Total, &result.ScopedInventoryTotal, &result.InventoryTotal, &result.AnalyzedBooks,
			&result.ContributingBooks, &result.NoncontributingBooks, &result.BooksWithoutCurrentAnalysis, &bookStatus); err != nil {
			return domain.VocabularyBrowsePage{}, err
		}
		if lemma != nil {
			row.CanonicalLemma, row.UPOS = *lemma, *pos
			row.OccurrenceCount, row.BookCount = *occurrences, *books
			row.Corrected, row.Known, row.Reserved, row.Generated = *corrected, *isKnown, *isReserved, *generated
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
	if lastPage > 0 && queryParams.Page > lastPage {
		queryParams.Page = lastPage
		return s.ListVocabularyBrowsePage(ctx, owner, language, queryParams)
	}
	return result, nil
}

func vocabularyBrowseUPOS(values []string) []string {
	allowed := map[string]bool{"NOUN": true, "VERB": true, "ADJ": true, "ADV": true}
	var result []string
	for _, value := range uniqueStrings(values) {
		if allowed[value] {
			result = append(result, value)
		}
	}
	return result
}

func vocabularyBrowseStateFilter(value string) string {
	if value == "known" || value == "not-known" || value == "not-known-or-reserved" || value == "reserved" || value == "not-reserved" {
		return value
	}
	return "any"
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
