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
// owner's occurrence decisions and annotates each identity with its Study state
// (ADR 0136 §5): Mature, in study, or not yet introduced.
// A page past the last page returns no rows with its exact Total; the caller
// decides how to explain it rather than silently serving a different page.
func (s *PostgresStore) ListVocabularyBrowsePage(ctx context.Context, owner, language string, queryParams domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	queryParams = normalizeVocabularyBrowseQuery(queryParams)
	rows, err := s.pool.Query(ctx, vocabularyBrowseSQL, vocabularyBrowseArgs(owner, language, queryParams)...)
	if err != nil {
		return domain.VocabularyBrowsePage{}, fmt.Errorf("query effective vocabulary browse: %w", err)
	}
	defer rows.Close()
	result, err := readVocabularyBrowseRows(rows, queryParams)
	if err != nil {
		return domain.VocabularyBrowsePage{}, err
	}
	return result, nil
}

func normalizeVocabularyBrowseQuery(queryParams domain.VocabularyBrowseQuery) domain.VocabularyBrowseQuery {
	if queryParams.Page < 1 {
		queryParams.Page = 1
	}
	queryParams.BookIDs = uniqueStrings(queryParams.BookIDs)
	if queryParams.CurrentBookID != "" {
		// A Current-reading request is one-Book by construction. Ignore any
		// legacy Book filters instead of allowing them to alter or widen scope.
		queryParams.BookIDs = nil
	}
	queryParams.UPOS = vocabularyBrowseUPOS(queryParams.UPOS)
	if queryParams.Sort != "occurrences" && queryParams.Sort != "books" {
		queryParams.Sort = "lemma"
	}
	return queryParams
}

// vocabularyBrowseArgs binds a normalized query to vocabularyBrowseSQL's
// parameters. The Browse scale test explains the same text with these
// arguments, so it inspects the plan a learner's request executes.
func vocabularyBrowseArgs(owner, language string, queryParams domain.VocabularyBrowseQuery) []any {
	return []any{owner, language, strings.TrimSpace(queryParams.Prefix), queryParams.BookIDs, queryParams.UPOS, queryParams.Sort, queryParams.Page, queryParams.CurrentBookID, int64(domain.FluencyMaturityDays)}
}

const vocabularyBrowseSQL = `
WITH all_books AS (
  SELECT b.id::text AS id, b.title,
    EXISTS (SELECT 1 FROM current_analysis_identity cai WHERE cai.owner_id=b.owner_id AND cai.book_id=b.id) AS analyzed
  FROM books b WHERE b.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2
), scope_books AS (
  SELECT * FROM all_books WHERE
    ($8<>'' AND id=$8) OR
    ($8='' AND (COALESCE(cardinality($4::text[]),0)=0 OR id=ANY($4::text[])))
 ), projection_scope AS MATERIALIZED (
   SELECT cai.owner_id,cai.book_id,cai.analysis_run_id,cai.corpus_id
   FROM current_analysis_identity cai JOIN all_books b ON b.id=cai.book_id::text
 ), projection_status AS MATERIALIZED (
   SELECT NOT EXISTS (
     SELECT 1 FROM projection_scope s
     LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=s.owner_id AND r.book_id=s.book_id
       AND r.analysis_run_id=s.analysis_run_id AND r.corpus_id=s.corpus_id
       AND r.language=$2 AND r.builder_version=5
     WHERE r.owner_id IS NULL
   ) AS ready,
   EXISTS (
     SELECT 1 FROM projection_scope s
     LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=s.owner_id AND r.book_id=s.book_id
       AND r.analysis_run_id=s.analysis_run_id AND r.corpus_id=s.corpus_id
       AND r.language=$2 AND r.builder_version=5
      JOIN river_job failed ON failed.kind='rebuild_vocabulary_browse_counts' AND failed.state='discarded'
        AND failed.args->>'owner_id'=s.owner_id::text AND failed.args->>'book_id'=s.book_id::text
        AND failed.args->>'run_id'=s.analysis_run_id::text
      WHERE r.owner_id IS NULL
        AND NOT EXISTS (SELECT 1 FROM river_job active
          WHERE active.kind='rebuild_vocabulary_browse_counts'
            AND active.state IN ('available','pending','running','retryable','scheduled')
            AND active.args->>'owner_id'=s.owner_id::text AND active.args->>'book_id'=s.book_id::text
            AND active.args->>'run_id'=s.analysis_run_id::text)
   ) AS failed
 ), projection_ready AS MATERIALIZED (
   SELECT r.owner_id,r.book_id,r.analysis_run_id,r.corpus_id
   FROM projection_scope r CROSS JOIN projection_status status
   WHERE status.ready
     AND EXISTS (SELECT 1 FROM vocabulary_browse_count_readiness ready
       WHERE ready.owner_id=r.owner_id AND ready.book_id=r.book_id
         AND ready.analysis_run_id=r.analysis_run_id AND ready.corpus_id=r.corpus_id
         AND ready.language=$2 AND ready.builder_version=5)
 ), identity_headwords AS MATERIALIZED (
  -- The citation headword is display-only evidence, not part of the identity,
  -- so it is read once from the analysis tokens that carry one (imported
  -- classical analyses) rather than stored on the count. Modern-language tokens
  -- carry an empty headword, so this set is empty for them and Browse is
  -- unchanged. When lowercasing merges spellings of one Latin lemma
  -- (Roma and roma), the lowercase spelling is shown (ADR 0132); every other
  -- language keeps the byte-order minimum.
  SELECT p.book_id::text AS book_id,t.canonical_lemma,t.upos,t.lexical_discriminator,(array_agg(t.headword ORDER BY (t.headword<>lower(t.headword) AND $2='la'),t.headword) FILTER (WHERE t.headword IS NOT NULL AND t.headword<>''))[1] AS headword
  FROM corpus_tokens t JOIN projection_ready p
    ON p.owner_id=t.owner_id AND p.analysis_run_id=t.analysis_run_id AND p.corpus_id=t.corpus_id
  WHERE t.language=$2 AND t.headword <> ''
  GROUP BY p.book_id,t.canonical_lemma,t.upos,t.lexical_discriminator
 ), current_evidence AS MATERIALIZED (
  -- Headwords and numbered-sibling marks ride beside the counts and meet them
  -- in book_identities' aggregate. Joining CTEs relied on row estimates they
  -- cannot give, and a nested loop between them grows with identities times
  -- headwords or numbered forms.
  SELECT p.book_id::text AS book_id,c.upos,c.canonical_lemma AS lemma,c.lexical_discriminator AS discriminator,c.corrected,c.occurrence_count,
    NULL::text AS headword,false AS bare_with_numbered_sibling
  FROM vocabulary_browse_counts c JOIN projection_ready p
    ON p.owner_id=c.owner_id AND p.book_id=c.book_id AND p.analysis_run_id=c.analysis_run_id AND p.corpus_id=c.corpus_id
  UNION ALL
  SELECT h.book_id,h.upos,h.canonical_lemma,h.lexical_discriminator,NULL,NULL,h.headword,false
  FROM identity_headwords h
  UNION ALL
  -- A numbered form marks the bare identity of its lemma in the same Book as
  -- one the source gives no homograph number for.
  SELECT p.book_id::text,c.upos,c.canonical_lemma,'',NULL,NULL,NULL,true
  FROM vocabulary_browse_counts c JOIN projection_ready p
    ON p.owner_id=c.owner_id AND p.book_id=c.book_id AND p.analysis_run_id=c.analysis_run_id AND p.corpus_id=c.corpus_id
  WHERE c.lexical_discriminator <> ''
 ), book_identities AS MATERIALIZED (
   -- Only counted identities are vocabulary; a headword or mark alone adds no row.
   SELECT book_id,lemma,upos,discriminator,sum(occurrence_count)::bigint AS occurrences,bool_or(corrected) AS corrected,
     COALESCE((array_agg(headword ORDER BY (headword<>lower(headword) AND $2='la'),headword) FILTER (WHERE headword IS NOT NULL AND headword<>''))[1],'') AS headword,bool_or(bare_with_numbered_sibling) AS bare_with_numbered_sibling
   FROM current_evidence GROUP BY book_id,lemma,upos,discriminator
   HAVING count(occurrence_count) > 0
), identity_totals AS MATERIALIZED (
  -- One aggregate yields both the all-Book and the scoped figures. Joining two
  -- separate aggregates relied on row estimates the CTEs above cannot give,
  -- and the nested loop that followed exceeded the Browse deadline.
  SELECT lemma,upos,discriminator,
    sum(occurrences)::bigint AS across_books_occurrences,
    bool_or(in_scope) AS in_scope,
    sum(occurrences) FILTER (WHERE in_scope)::bigint AS occurrences,
    count(*) FILTER (WHERE in_scope)::bigint AS books,
    bool_or(corrected) FILTER (WHERE in_scope) AS corrected,
    COALESCE((array_agg(headword ORDER BY (headword<>lower(headword) AND $2='la'),headword) FILTER (WHERE in_scope AND headword IS NOT NULL AND headword<>''))[1],'') AS headword,
    bool_or(bare_with_numbered_sibling) FILTER (WHERE in_scope) AS bare_with_numbered_sibling
  FROM (
    SELECT bi.*, (sb.id IS NOT NULL AND (COALESCE(cardinality($5::text[]),0)=0 OR bi.upos=ANY($5::text[]))) AS in_scope
    FROM book_identities bi LEFT JOIN scope_books sb ON sb.id=bi.book_id
  ) scoped
  GROUP BY lemma,upos,discriminator
 ), annotated AS MATERIALIZED (
  -- The learner state is Study progress (ADR 0136 §5): a word is Mature, in
  -- study, or not yet introduced. Mature means a day-scale schedule of at least
  -- domain.FluencyMaturityDays ($9); a withdrawn word counts as neither. Inlining
  -- repeats correlated lookups and can exceed the Browse deadline.
  SELECT g.lemma,g.upos,g.discriminator,g.headword,g.bare_with_numbered_sibling,g.occurrences,g.books,g.corrected,
    g.across_books_occurrences,
    CASE
      WHEN EXISTS (SELECT 1 FROM study_progress sp
        WHERE sp.owner_id=$1 AND sp.language=$2 AND sp.item_kind='word'
          AND sp.canonical_lemma=btrim(g.lemma) AND sp.upos=upper(btrim(g.upos))
          AND sp.lexical_discriminator=btrim(g.discriminator) AND sp.withdrawn_at IS NULL
          AND sp.state='review' AND sp.scheduled_days>=$9::bigint) THEN 'mature'
      WHEN EXISTS (SELECT 1 FROM study_progress sp
        WHERE sp.owner_id=$1 AND sp.language=$2 AND sp.item_kind='word'
          AND sp.canonical_lemma=btrim(g.lemma) AND sp.upos=upper(btrim(g.upos))
          AND sp.lexical_discriminator=btrim(g.discriminator) AND sp.withdrawn_at IS NULL) THEN 'in-study'
      ELSE 'not-introduced'
    END AS study_state
  FROM identity_totals g WHERE g.in_scope
), eligible AS (
  SELECT a.lemma,a.upos,a.discriminator,a.headword,a.bare_with_numbered_sibling,a.occurrences,a.across_books_occurrences,a.books,a.corrected,a.study_state
  FROM annotated a
  WHERE ($3='' OR left(lower(lemma),length(lower($3)))=lower($3))
), page_rows AS (
  SELECT * FROM eligible
  ORDER BY CASE WHEN $6='occurrences' THEN occurrences END DESC,
             CASE WHEN $8<>'' THEN across_books_occurrences END DESC,
	           CASE WHEN $6='books' THEN books END DESC, lemma, upos, discriminator
	  LIMIT 25 OFFSET (($7::bigint-1)*25)
), summary AS (
   SELECT (SELECT count(*)::bigint FROM eligible) AS total,
          (SELECT count(*)::bigint FROM identity_totals WHERE in_scope) AS scoped_total,
          CASE WHEN $8<>'' THEN (SELECT count(*)::bigint FROM book_identities WHERE book_id=$8)
               ELSE (SELECT count(*)::bigint FROM identity_totals) END AS inventory_total,
          NOT (SELECT ready OR failed FROM projection_status) AS browse_counts_updating,
          (SELECT failed FROM projection_status) AS browse_counts_unavailable
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
   FROM scope_books b
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
    || '|' || COALESCE((SELECT string_agg(sp.canonical_lemma || ':' || sp.upos || ':' || sp.lexical_discriminator || ':' || CASE WHEN sp.state='review' AND sp.scheduled_days>=$9::bigint THEN 'mature' ELSE 'in-study' END, ',' ORDER BY sp.canonical_lemma,sp.upos,sp.lexical_discriminator)
      FROM study_progress sp WHERE sp.owner_id=$1 AND sp.language=$2 AND sp.item_kind='word' AND sp.withdrawn_at IS NULL),'')
  ) AS value
)
SELECT p.lemma,p.upos,p.discriminator,p.headword,p.bare_with_numbered_sibling,p.occurrences,p.across_books_occurrences,p.books,p.corrected,p.study_state,
   summary.total,summary.scoped_total,summary.inventory_total,coverage.analyzed,coverage.contributing,
   coverage.total-coverage.contributing,coverage.total-coverage.analyzed,
   summary.browse_counts_updating,summary.browse_counts_unavailable,
  COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'title',title,'has_current_analysis',analyzed,
    'has_vocabulary_evidence',contributing) ORDER BY title,id) FROM book_status),'[]'::jsonb),revision.value
FROM summary CROSS JOIN coverage CROSS JOIN revision LEFT JOIN page_rows p ON true
ORDER BY CASE WHEN $6='occurrences' THEN p.occurrences END DESC,
             CASE WHEN $8<>'' THEN p.across_books_occurrences END DESC,
	         CASE WHEN $6='books' THEN p.books END DESC,p.lemma,p.upos,p.discriminator`

func readVocabularyBrowseRows(rows pgx.Rows, queryParams domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	result := domain.VocabularyBrowsePage{Page: queryParams.Page, SelectedBooks: queryParams.BookIDs, SelectedUPOS: queryParams.UPOS, Sort: queryParams.Sort}
	var bookStatus []byte
	var corpusRevision string
	for rows.Next() {
		var row domain.VocabularyBrowseRow
		var lemma, pos, discriminator, headword *string
		var occurrences, acrossBooksOccurrences, books *int64
		var corrected, bareWithNumberedSibling *bool
		var studyState *string
		if err := rows.Scan(&lemma, &pos, &discriminator, &headword, &bareWithNumberedSibling, &occurrences, &acrossBooksOccurrences, &books, &corrected, &studyState,
			&result.Total, &result.ScopedInventoryTotal, &result.InventoryTotal, &result.AnalyzedBooks,
			&result.ContributingBooks, &result.NoncontributingBooks, &result.BooksWithoutCurrentAnalysis, &result.BrowseCountsUpdating, &result.BrowseCountsUnavailable, &bookStatus, &corpusRevision); err != nil {
			return domain.VocabularyBrowsePage{}, err
		}
		result.CorpusRevision = corpusRevision
		if lemma != nil {
			row.CanonicalLemma, row.UPOS = *lemma, *pos
			row.LexicalDiscriminator, row.Headword, row.BareLemmaWithNumberedSibling = *discriminator, *headword, *bareWithNumberedSibling
			row.OccurrenceCount, row.AcrossBooksOccurrenceCount, row.BookCount = *occurrences, *acrossBooksOccurrences, *books
			row.Corrected = *corrected
			row.StudyState = vocabularyStudyState(*studyState)
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

// vocabularyStudyState maps a stored Study-state name to its domain value,
// defaulting to not yet introduced.
func vocabularyStudyState(name string) domain.VocabularyStudyState {
	switch state := domain.VocabularyStudyState(name); state {
	case domain.VocabularyStudyMature, domain.VocabularyStudyInStudy, domain.VocabularyStudyNotIntroduced:
		return state
	default:
		return domain.VocabularyStudyNotIntroduced
	}
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
