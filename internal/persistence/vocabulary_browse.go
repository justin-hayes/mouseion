package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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
), current_evidence AS (
  SELECT cai.book_id::text AS book_id, t.upos,
         COALESCE(d.canonical_lemma,t.canonical_lemma) AS lemma,
         d.canonical_lemma IS NOT NULL AS corrected
  FROM corpus_tokens t
  JOIN current_analysis_identity cai
    ON cai.owner_id=t.owner_id AND cai.analysis_run_id=t.analysis_run_id AND cai.corpus_id=t.corpus_id
  JOIN all_books b ON b.id=cai.book_id::text
  JOIN corpus_sentences s
    ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id AND s.corpus_id=t.corpus_id
   AND s.sentence_ordinal=t.sentence_ordinal
  JOIN source_material_units u
    ON u.owner_id=cai.owner_id AND u.source_material_id=cai.source_material_id
   AND u.snapshot_id=cai.snapshot_id AND u.unit_id=s.unit_id
  LEFT JOIN occurrence_lemma_corrections d
    ON d.owner_id=t.owner_id AND d.book_id=cai.book_id
   AND d.corpus_id=t.corpus_id AND d.analysis_run_id=t.analysis_run_id
   AND d.source_document_id=s.unit_id
   AND d.start_offset=t.start_offset AND d.end_offset=t.end_offset
  WHERE t.owner_id=$1 AND t.language=$2 AND t.upos IN ('NOUN','VERB','ADJ','ADV')
    AND t.dependency <> 'compound:prt' AND COALESCE(d.excluded,false)=false
    AND COALESCE(d.canonical_lemma,t.canonical_lemma) ~ '[[:alpha:]]'
), book_identities AS MATERIALIZED (
  SELECT book_id,lemma,upos,count(*)::bigint AS occurrences,bool_or(corrected) AS corrected
  FROM current_evidence GROUP BY book_id,lemma,upos
), grouped AS (
  SELECT lemma,upos,sum(occurrences)::bigint AS occurrences,count(*)::bigint AS books,bool_or(corrected) AS corrected
  FROM book_identities GROUP BY lemma,upos
), scoped_grouped AS (
  SELECT lemma,upos,sum(occurrences)::bigint AS occurrences,count(*)::bigint AS books,bool_or(corrected) AS corrected
  FROM book_identities
  WHERE (COALESCE(cardinality($4::text[]),0)=0 OR book_id=ANY($4::text[]))
    AND (COALESCE(cardinality($5::text[]),0)=0 OR upos=ANY($5::text[]))
  GROUP BY lemma,upos
), annotated AS (
  -- Every scoped identity belongs to grouped; avoid a second aggregate join.
  SELECT g.*,
    EXISTS (SELECT 1 FROM known_vocabulary k WHERE k.owner_id=$1 AND k.language=$2 AND k.canonical_lemma=g.lemma AND k.upos=g.upos) AS known,
    EXISTS (SELECT 1 FROM primary_goal_snapshots ps
      JOIN primary_goals pg ON pg.owner_id=ps.owner_id AND pg.snapshot_id=ps.id
      JOIN primary_goal_snapshot_vocabulary pv ON pv.owner_id=ps.owner_id AND pv.snapshot_id=ps.id
      WHERE ps.owner_id=$1 AND ps.language=$2 AND pv.language=$2
        AND pv.canonical_lemma=g.lemma AND pv.upos=g.upos AND ps.released_at IS NULL) AS reserved,
    EXISTS (SELECT 1 FROM generated_vocabulary v WHERE v.owner_id=$1 AND v.language=$2 AND v.canonical_lemma=g.lemma AND v.upos=g.upos) AS generated
  FROM scoped_grouped g
), eligible AS (
  SELECT a.lemma,a.upos,a.occurrences,a.books,a.corrected,a.known,a.reserved,a.generated
  FROM annotated a
  WHERE ($3='' OR left(lower(lemma),length(lower($3)))=lower($3))
	    AND ($6='any' OR ($6='known' AND a.known) OR ($6='not-known' AND NOT a.known) OR $6='not-known-or-reserved')
	    AND ($7='any' OR ($7='reserved' AND a.reserved) OR ($7='not-reserved' AND NOT a.reserved))
	    AND ($6<>'not-known-or-reserved' OR (NOT known AND NOT reserved))
), page_rows AS (
  SELECT * FROM eligible
  ORDER BY CASE WHEN $8='occurrences' THEN occurrences END DESC,
	           CASE WHEN $8='books' THEN books END DESC, lemma, upos
	  LIMIT 25 OFFSET (($9::bigint-1)*25)
), summary AS (
  SELECT (SELECT count(*)::bigint FROM eligible) AS total,
         (SELECT count(*)::bigint FROM scoped_grouped) AS scoped_total,
         (SELECT count(*)::bigint FROM grouped) AS inventory_total
), coverage AS (
  SELECT count(*)::bigint AS total,
    count(*) FILTER (WHERE analyzed)::bigint AS analyzed,
    count(*) FILTER (WHERE has_evidence)::bigint AS contributing
  FROM (
    SELECT b.*, EXISTS (SELECT 1 FROM book_identities e WHERE e.book_id=b.id) AS has_evidence
    FROM scope_books b
  ) scoped
), book_status AS (
  SELECT b.id,b.title,b.analyzed,
    EXISTS (SELECT 1 FROM book_identities e WHERE e.book_id=b.id) AS contributing
  FROM all_books b
), revision AS (
  SELECT md5(
    COALESCE((SELECT string_agg(b.id || ':' || COALESCE(cai.analysis_run_id::text,'') || ':' || COALESCE(cai.corpus_id::text,''), ',' ORDER BY b.id)
      FROM all_books b LEFT JOIN current_analysis_identity cai ON cai.owner_id=$1 AND cai.book_id::text=b.id),'')
    || '|' ||
    COALESCE((SELECT string_agg(d.book_id::text || ':' || d.corpus_id::text || ':' || d.analysis_run_id::text || ':' || d.source_document_id || ':' || d.start_offset::text || ':' || d.end_offset::text || ':' || COALESCE(d.canonical_lemma,'') || ':' || COALESCE(d.excluded,false)::text, ','
      ORDER BY d.book_id,d.corpus_id,d.analysis_run_id,d.source_document_id,d.start_offset,d.end_offset)
      FROM occurrence_lemma_corrections d JOIN all_books b ON b.id=d.book_id::text
      JOIN current_analysis_identity cai ON cai.owner_id=d.owner_id AND cai.book_id=d.book_id AND cai.corpus_id=d.corpus_id AND cai.analysis_run_id=d.analysis_run_id
      WHERE d.owner_id=$1),'')
    || '|' || COALESCE((SELECT string_agg(k.canonical_lemma || ':' || k.upos, ',' ORDER BY k.canonical_lemma,k.upos)
      FROM known_vocabulary k WHERE k.owner_id=$1 AND k.language=$2),'')
    || '|' || COALESCE((SELECT string_agg(reserved.canonical_lemma || ':' || reserved.upos, ',' ORDER BY reserved.canonical_lemma,reserved.upos)
      FROM (SELECT DISTINCT pv.canonical_lemma,pv.upos FROM primary_goal_snapshots ps
        JOIN primary_goals pg ON pg.owner_id=ps.owner_id AND pg.snapshot_id=ps.id
        JOIN primary_goal_snapshot_vocabulary pv ON pv.owner_id=ps.owner_id AND pv.snapshot_id=ps.id
        WHERE ps.owner_id=$1 AND ps.language=$2 AND pv.language=$2 AND ps.released_at IS NULL) reserved),'')
  ) AS value
)
SELECT p.lemma,p.upos,p.occurrences,p.books,p.corrected,p.known,p.reserved,p.generated,
  summary.total,summary.scoped_total,summary.inventory_total,coverage.analyzed,coverage.contributing,
  coverage.total-coverage.contributing,coverage.total-coverage.analyzed,
  COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'title',title,'has_current_analysis',analyzed,
    'has_vocabulary_evidence',contributing) ORDER BY title,id) FROM book_status),'[]'::jsonb),revision.value
FROM summary CROSS JOIN coverage CROSS JOIN revision LEFT JOIN page_rows p ON true
ORDER BY CASE WHEN $8='occurrences' THEN p.occurrences END DESC,
	         CASE WHEN $8='books' THEN p.books END DESC,p.lemma,p.upos`
	rows, err := s.pool.Query(ctx, query, owner, language, strings.TrimSpace(queryParams.Prefix), queryParams.BookIDs, queryParams.UPOS, queryParams.KnownFilter, queryParams.ReservedFilter, queryParams.Sort, queryParams.Page)
	if err != nil {
		return domain.VocabularyBrowsePage{}, fmt.Errorf("query effective vocabulary browse: %w", err)
	}
	defer rows.Close()
	result, err := readVocabularyBrowseRows(rows, queryParams)
	if err != nil {
		return domain.VocabularyBrowsePage{}, err
	}
	lastPage := int((result.Total + 24) / 25)
	if lastPage > 0 && queryParams.Page > lastPage {
		queryParams.Page = lastPage
		return s.ListVocabularyBrowsePage(ctx, owner, language, queryParams)
	}
	return result, nil
}

func readVocabularyBrowseRows(rows pgx.Rows, queryParams domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	result := domain.VocabularyBrowsePage{Page: queryParams.Page, SelectedBooks: queryParams.BookIDs, SelectedUPOS: queryParams.UPOS, KnownFilter: queryParams.KnownFilter, ReservedFilter: queryParams.ReservedFilter, Sort: queryParams.Sort}
	var bookStatus []byte
	var corpusRevision string
	for rows.Next() {
		var row domain.VocabularyBrowseRow
		var lemma, pos *string
		var occurrences, books *int64
		var corrected, isKnown, isReserved, generated *bool
		if err := rows.Scan(&lemma, &pos, &occurrences, &books, &corrected, &isKnown, &isReserved, &generated,
			&result.Total, &result.ScopedInventoryTotal, &result.InventoryTotal, &result.AnalyzedBooks,
			&result.ContributingBooks, &result.NoncontributingBooks, &result.BooksWithoutCurrentAnalysis, &bookStatus, &corpusRevision); err != nil {
			return domain.VocabularyBrowsePage{}, err
		}
		result.CorpusRevision = corpusRevision
		if lemma != nil {
			row.CanonicalLemma, row.UPOS = *lemma, *pos
			row.OccurrenceCount, row.BookCount = *occurrences, *books
			row.Corrected, row.Known, row.Reserved, row.Generated = *corrected, *isKnown, *isReserved, *generated
			result.Rows = append(result.Rows, row)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.VocabularyBrowsePage{}, fmt.Errorf("read effective vocabulary browse rows: %w", err)
	}
	if bookStatus == nil {
		return domain.VocabularyBrowsePage{}, errors.New("query effective vocabulary browse: missing summary row")
	}
	if err := json.Unmarshal(bookStatus, &result.Books); err != nil {
		return domain.VocabularyBrowsePage{}, fmt.Errorf("decode vocabulary Book status: %w", err)
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
