package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

var ErrVocabularyIdentityNotCurrent = errors.New("vocabulary identity has no current eligible evidence")
var ErrEmptyVocabularySelection = errors.New("cannot create a Custom deck from an empty selection")
var ErrCustomVocabularyDeckNotFound = errors.New("Custom deck not found")
var ErrCustomVocabularyDeckNameInvalid = errors.New("deck name must contain between 1 and 120 characters")

func (s *PostgresStore) SetVocabularyBrowseSelection(ctx context.Context, owner, language, lemma, upos string, selected bool) (err error) {
	lemma = strings.TrimSpace(lemma)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1||':'||$2,0))`, owner, language); err != nil {
		return err
	}
	if !selected {
		if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_selections WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4`, owner, language, lemma, upos); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	const query = `
WITH eligible AS (
 SELECT 1
 FROM concordance_occurrences o
 JOIN current_analysis_identity cai ON cai.owner_id=o.owner_id AND cai.book_id::text=o.book_id
  AND cai.analysis_run_id::text=o.analysis_run_id AND cai.corpus_id::text=o.corpus_id
 LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=o.owner_id AND d.book_id::text=o.book_id
  AND d.corpus_id::text=o.corpus_id AND d.analysis_run_id::text=o.analysis_run_id
  AND d.source_document_id=o.unit_id AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
 WHERE o.owner_id=$1 AND o.language=$2 AND o.upos=$4
  AND COALESCE(d.canonical_lemma,o.canonical_lemma)=$3 AND COALESCE(d.excluded,false)=false
  AND o.upos IN ('NOUN','VERB','ADJ','ADV') AND o.dependency <> 'compound:prt'
  AND COALESCE(d.canonical_lemma,o.canonical_lemma) ~ '[[:alpha:]]'
 LIMIT 1
)
INSERT INTO vocabulary_browse_selections(owner_id,language,canonical_lemma,upos)
SELECT $1,$2,$3,$4 WHERE EXISTS (SELECT 1 FROM eligible)
ON CONFLICT DO NOTHING`
	tag, err := tx.Exec(ctx, query, owner, language, lemma, upos)
	if err != nil {
		return fmt.Errorf("select effective vocabulary identity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vocabulary_browse_selections WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4)`, owner, language, lemma, upos).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrVocabularyIdentityNotCurrent
		}
	}
	return tx.Commit(ctx)
}

// ListVocabularyBrowseSelection uses the current per-Book Browse projection
// when ready. Only Books awaiting a rebuild need to recount raw occurrences;
// otherwise a review of a small saved selection scans the entire corpus.
func (s *PostgresStore) ListVocabularyBrowseSelection(ctx context.Context, owner, language string) ([]domain.VocabularyIdentity, error) {
	const query = `
WITH current_books AS MATERIALIZED (
 SELECT cai.owner_id,cai.book_id,cai.analysis_run_id,cai.corpus_id,cai.source_material_id,cai.snapshot_id,
  r.owner_id IS NOT NULL AS counts_ready
 FROM current_analysis_identity cai
 JOIN books b ON b.owner_id=cai.owner_id AND b.id=cai.book_id
  AND b.language_state='chosen' AND b.language_tag=$2
 LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=cai.owner_id AND r.book_id=cai.book_id
  AND r.analysis_run_id=cai.analysis_run_id AND r.corpus_id=cai.corpus_id
  AND r.language=$2 AND r.builder_version=2
 WHERE cai.owner_id=$1
), evidence AS (
 SELECT c.book_id,c.canonical_lemma AS lemma,c.upos,c.occurrence_count
 FROM current_books b
 JOIN vocabulary_browse_counts c ON c.owner_id=b.owner_id AND c.book_id=b.book_id
  AND c.analysis_run_id=b.analysis_run_id AND c.corpus_id=b.corpus_id AND c.language=$2
 JOIN vocabulary_browse_selections s ON s.owner_id=c.owner_id AND s.language=c.language
  AND s.canonical_lemma=c.canonical_lemma AND s.upos=c.upos
 WHERE b.counts_ready
 UNION ALL
 SELECT b.book_id,COALESCE(d.canonical_lemma,t.canonical_lemma),t.upos,count(*)::bigint
 FROM current_books b
 JOIN corpus_tokens t ON t.owner_id=b.owner_id AND t.analysis_run_id=b.analysis_run_id
  AND t.corpus_id=b.corpus_id AND t.language=$2
 JOIN corpus_sentences sentence ON sentence.owner_id=t.owner_id AND sentence.analysis_run_id=t.analysis_run_id
  AND sentence.corpus_id=t.corpus_id AND sentence.sentence_ordinal=t.sentence_ordinal
 JOIN source_material_units u ON u.owner_id=b.owner_id AND u.source_material_id=b.source_material_id
  AND u.snapshot_id=b.snapshot_id AND u.unit_id=sentence.unit_id
 LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=b.owner_id AND d.book_id=b.book_id
  AND d.corpus_id=t.corpus_id AND d.analysis_run_id=t.analysis_run_id
  AND d.source_document_id=sentence.unit_id AND d.start_offset=t.start_offset AND d.end_offset=t.end_offset
 JOIN vocabulary_browse_selections s ON s.owner_id=t.owner_id AND s.language=t.language
  AND s.canonical_lemma=COALESCE(d.canonical_lemma,t.canonical_lemma) AND s.upos=t.upos
 WHERE NOT b.counts_ready AND t.upos IN ('NOUN','VERB','ADJ','ADV') AND t.dependency <> 'compound:prt'
  AND COALESCE(d.excluded,false)=false AND COALESCE(d.canonical_lemma,t.canonical_lemma) ~ '[[:alpha:]]'
 GROUP BY b.book_id,COALESCE(d.canonical_lemma,t.canonical_lemma),t.upos
), counts AS (
 SELECT lemma,upos,sum(occurrence_count)::bigint AS occurrences,count(*)::bigint AS books
 FROM evidence GROUP BY lemma,upos
)
SELECT s.canonical_lemma,s.upos,COALESCE(c.occurrences,0),COALESCE(c.books,0),c.lemma IS NULL
FROM vocabulary_browse_selections s LEFT JOIN counts c ON c.lemma=s.canonical_lemma AND c.upos=s.upos
WHERE s.owner_id=$1 AND s.language=$2 ORDER BY s.canonical_lemma,s.upos`
	rows, err := s.pool.Query(ctx, query, owner, language)
	if err != nil {
		return nil, fmt.Errorf("list Browse selection: %w", err)
	}
	defer rows.Close()
	var identities []domain.VocabularyIdentity
	for rows.Next() {
		var identity domain.VocabularyIdentity
		if err := rows.Scan(&identity.CanonicalLemma, &identity.UPOS, &identity.OccurrenceCount, &identity.BookCount, &identity.MissingEvidence); err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return identities, rows.Err()
}

// VocabularyBrowseSelectionState reads only the saved-selection count and
// membership for the identities on the current Browse page. Review evidence
// remains the responsibility of ListVocabularyBrowseSelection; joining every
// saved identity to current corpus evidence here makes Browse scale with the
// complete cross-Book corpus for no visible benefit.
func (s *PostgresStore) VocabularyBrowseSelectionState(ctx context.Context, owner, language string, rows []domain.VocabularyBrowseRow) (int, []bool, error) {
	lemmas := make([]string, len(rows))
	upos := make([]string, len(rows))
	for i, row := range rows {
		lemmas[i], upos[i] = row.CanonicalLemma, row.UPOS
	}
	const query = `
WITH visible AS (
 SELECT lemma,upos,ordinality
 FROM unnest($3::text[],$4::text[]) WITH ORDINALITY AS v(lemma,upos,ordinality)
), matched AS (
 SELECT v.ordinality,s.canonical_lemma IS NOT NULL AS selected
 FROM visible v LEFT JOIN vocabulary_browse_selections s
  ON s.owner_id=$1 AND s.language=$2 AND s.canonical_lemma=v.lemma AND s.upos=v.upos
)
SELECT (SELECT count(*)::bigint FROM vocabulary_browse_selections WHERE owner_id=$1 AND language=$2),
 COALESCE(array_agg(selected ORDER BY ordinality),ARRAY[]::boolean[])
FROM matched`
	var count int64
	var selected []bool
	if err := s.pool.QueryRow(ctx, query, owner, language, lemmas, upos).Scan(&count, &selected); err != nil {
		return 0, nil, fmt.Errorf("read Browse selection state: %w", err)
	}
	return int(count), selected, nil
}

func (s *PostgresStore) ClearVocabularyBrowseSelection(ctx context.Context, owner, language string) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1||':'||$2,0))`, owner, language); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_selections WHERE owner_id=$1 AND language=$2`, owner, language); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) CreateCustomVocabularyDeck(ctx context.Context, owner, language, name, creationKey string) (result domain.CustomVocabularyDeck, err error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return domain.CustomVocabularyDeck{}, ErrCustomVocabularyDeckNameInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CustomVocabularyDeck{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1||':'||$2,0))`, owner, language); err != nil {
		return domain.CustomVocabularyDeck{}, err
	}
	var deck domain.CustomVocabularyDeck
	deck.Language, deck.Name = language, name
	err = tx.QueryRow(ctx, `INSERT INTO custom_vocabulary_decks(owner_id,language,name,creation_key)
 VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,creation_key) DO NOTHING RETURNING id::text`, owner, language, name, creationKey).Scan(&deck.ID)
	created := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomVocabularyDeck{}, fmt.Errorf("create Custom deck: %w", err)
	}
	if !created {
		if err := tx.QueryRow(ctx, `SELECT id::text,language,name FROM custom_vocabulary_decks WHERE owner_id=$1 AND language=$2 AND creation_key=$3`, owner, language, creationKey).Scan(&deck.ID, &deck.Language, &deck.Name); err != nil {
			return domain.CustomVocabularyDeck{}, fmt.Errorf("resolve Custom deck retry: %w", err)
		}
	} else {
		var selected int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM vocabulary_browse_selections WHERE owner_id=$1 AND language=$2`, owner, language).Scan(&selected); err != nil {
			return domain.CustomVocabularyDeck{}, err
		}
		if selected == 0 {
			return domain.CustomVocabularyDeck{}, ErrEmptyVocabularySelection
		}
		if _, err := tx.Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos)
 SELECT owner_id,$3,language,canonical_lemma,upos FROM vocabulary_browse_selections WHERE owner_id=$1 AND language=$2`, owner, language, deck.ID); err != nil {
			return domain.CustomVocabularyDeck{}, fmt.Errorf("copy Browse selection into Custom deck: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_selections WHERE owner_id=$1 AND language=$2`, owner, language); err != nil {
			return domain.CustomVocabularyDeck{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CustomVocabularyDeck{}, err
	}
	// The transaction is the durable outcome. Do not turn a successful commit
	// into an apparent creation failure by loading the review page afterward;
	// the handler redirects to that page, which can be retried independently.
	return deck, nil
}

func (s *PostgresStore) ListCustomVocabularyDeckIdentities(ctx context.Context, owner, deck string) ([]domain.VocabularyIdentity, error) {
	rows, err := s.pool.Query(ctx, `
WITH target AS (SELECT language FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2), evidence AS (
 SELECT o.book_id,COALESCE(d.canonical_lemma,o.canonical_lemma) AS lemma,o.upos
 FROM target t JOIN concordance_occurrences o ON o.owner_id=$1 AND o.language=t.language
 JOIN current_analysis_identity cai ON cai.owner_id=o.owner_id AND cai.book_id::text=o.book_id
  AND cai.analysis_run_id::text=o.analysis_run_id AND cai.corpus_id::text=o.corpus_id
 JOIN books b ON b.owner_id=o.owner_id AND b.id::text=o.book_id AND b.language_state='chosen' AND b.language_tag=o.language
 LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=o.owner_id AND d.book_id::text=o.book_id
  AND d.corpus_id::text=o.corpus_id AND d.analysis_run_id::text=o.analysis_run_id
  AND d.source_document_id=o.unit_id AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
  WHERE o.upos IN ('NOUN','VERB','ADJ','ADV') AND o.dependency <> 'compound:prt' AND COALESCE(d.excluded,false)=false
   AND COALESCE(d.canonical_lemma,o.canonical_lemma) ~ '[[:alpha:]]'
), counts AS (SELECT lemma,upos,count(*)::bigint occurrences,count(DISTINCT book_id)::bigint books FROM evidence GROUP BY lemma,upos)
SELECT i.canonical_lemma,i.upos,COALESCE(c.occurrences,0),COALESCE(c.books,0),c.lemma IS NULL
FROM custom_vocabulary_deck_identities i JOIN custom_vocabulary_decks d ON d.owner_id=i.owner_id AND d.id=i.deck_id
LEFT JOIN counts c ON c.lemma=i.canonical_lemma AND c.upos=i.upos
WHERE i.owner_id=$1 AND i.deck_id=$2 ORDER BY i.canonical_lemma,i.upos`, owner, deck)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var identities []domain.VocabularyIdentity
	for rows.Next() {
		var identity domain.VocabularyIdentity
		if err := rows.Scan(&identity.CanonicalLemma, &identity.UPOS, &identity.OccurrenceCount, &identity.BookCount, &identity.MissingEvidence); err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	bookRows, err := s.pool.Query(ctx, `
WITH target AS (SELECT language FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2), evidence AS (
 SELECT COALESCE(c.canonical_lemma,o.canonical_lemma) AS lemma,o.upos,o.book_id::text AS book_id,
  o.analysis_run_id::text AS run_id,count(*)::bigint AS occurrences
 FROM target t JOIN concordance_occurrences o ON o.owner_id=$1 AND o.language=t.language
 JOIN current_analysis_identity cai ON cai.owner_id=o.owner_id AND cai.book_id::text=o.book_id
  AND cai.analysis_run_id::text=o.analysis_run_id AND cai.corpus_id::text=o.corpus_id
 JOIN books b ON b.owner_id=o.owner_id AND b.id::text=o.book_id AND b.language_state='chosen' AND b.language_tag=o.language
 LEFT JOIN occurrence_lemma_corrections c ON c.owner_id=o.owner_id AND c.book_id::text=o.book_id
  AND c.corpus_id::text=o.corpus_id AND c.analysis_run_id::text=o.analysis_run_id
  AND c.source_document_id=o.unit_id AND c.start_offset=o.unit_start_offset AND c.end_offset=o.unit_end_offset
  WHERE o.upos IN ('NOUN','VERB','ADJ','ADV') AND o.dependency <> 'compound:prt'
   AND COALESCE(c.excluded,false)=false AND COALESCE(c.canonical_lemma,o.canonical_lemma) ~ '[[:alpha:]]'
 GROUP BY 1,2,3,4
)
SELECT i.canonical_lemma,i.upos,b.id::text,b.title,e.run_id,e.occurrences
FROM custom_vocabulary_deck_identities i
JOIN evidence e ON e.lemma=i.canonical_lemma AND e.upos=i.upos
JOIN books b ON b.owner_id=i.owner_id AND b.id::text=e.book_id
WHERE i.owner_id=$1 AND i.deck_id=$2 ORDER BY i.canonical_lemma,i.upos,b.title,b.id`, owner, deck)
	if err != nil {
		return nil, err
	}
	defer bookRows.Close()
	byIdentity := make(map[string]*domain.VocabularyIdentity, len(identities))
	for i := range identities {
		byIdentity[identities[i].CanonicalLemma+"\x00"+identities[i].UPOS] = &identities[i]
	}
	for bookRows.Next() {
		var lemma, upos string
		var book domain.VocabularyIdentityBook
		if err := bookRows.Scan(&lemma, &upos, &book.ID, &book.Title, &book.AnalysisRunID, &book.OccurrenceCount); err != nil {
			return nil, err
		}
		if identity := byIdentity[lemma+"\x00"+upos]; identity != nil {
			identity.EvidenceBooks = append(identity.EvidenceBooks, book)
		}
	}
	if err := bookRows.Err(); err != nil {
		return nil, err
	}
	return identities, nil
}

// customDeckReviewEvidenceCTE scopes current evidence to the saved identities.
// Ready per-Book projections are indexed by identity; only Books whose
// projection is unavailable fall back to reading their current occurrences.
const customDeckReviewEvidenceCTE = `
WITH target AS MATERIALIZED (
 SELECT id,language,name FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2
), deck_identities AS MATERIALIZED (
 SELECT i.owner_id,i.deck_id,i.language,i.canonical_lemma,i.upos
 FROM custom_vocabulary_deck_identities i JOIN target t ON t.id=i.deck_id
 WHERE i.owner_id=$1
), current_books AS MATERIALIZED (
 SELECT cai.owner_id,cai.book_id,cai.analysis_run_id,cai.corpus_id,cai.source_material_id,cai.snapshot_id,b.title,t.language,
  r.owner_id IS NOT NULL AS counts_ready
 FROM target t
 JOIN current_analysis_identity cai ON cai.owner_id=$1
 JOIN books b ON b.owner_id=cai.owner_id AND b.id=cai.book_id
  AND b.language_state='chosen' AND b.language_tag=t.language
 LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=cai.owner_id AND r.book_id=cai.book_id
  AND r.analysis_run_id=cai.analysis_run_id AND r.corpus_id=cai.corpus_id
  AND r.language=t.language AND r.builder_version=2
), evidence AS (
 SELECT i.canonical_lemma AS lemma,i.upos,b.book_id::text AS book_id,
  b.analysis_run_id::text AS run_id,b.title,c.occurrence_count AS occurrences
 FROM current_books b
 JOIN vocabulary_browse_counts c ON c.owner_id=b.owner_id AND c.book_id=b.book_id
  AND c.analysis_run_id=b.analysis_run_id AND c.corpus_id=b.corpus_id AND c.language=b.language
 JOIN deck_identities i ON i.language=b.language AND i.canonical_lemma=c.canonical_lemma AND i.upos=c.upos
 WHERE b.counts_ready
 UNION ALL
 SELECT i.canonical_lemma,i.upos,b.book_id::text,b.analysis_run_id::text,b.title,count(*)::bigint
 FROM deck_identities i
 JOIN current_books b ON b.language=i.language AND NOT b.counts_ready
 JOIN LATERAL (
  SELECT o.* FROM concordance_occurrences o
  WHERE o.owner_id=b.owner_id AND o.book_id=b.book_id::text
   AND o.analysis_run_id=b.analysis_run_id::text AND o.corpus_id=b.corpus_id::text
   AND o.language=b.language AND o.canonical_lemma=i.canonical_lemma AND o.upos=i.upos
 ) o ON true
 LEFT JOIN occurrence_lemma_corrections c ON c.owner_id=o.owner_id AND c.book_id::text=o.book_id
  AND c.corpus_id::text=o.corpus_id AND c.analysis_run_id::text=o.analysis_run_id
  AND c.source_document_id=o.unit_id AND c.start_offset=o.unit_start_offset AND c.end_offset=o.unit_end_offset
 WHERE o.upos IN ('NOUN','VERB','ADJ','ADV') AND o.dependency <> 'compound:prt'
  AND COALESCE(c.excluded,false)=false AND COALESCE(c.canonical_lemma,o.canonical_lemma)=i.canonical_lemma
  AND i.canonical_lemma ~ '[[:alpha:]]'
 GROUP BY i.canonical_lemma,i.upos,b.book_id,b.analysis_run_id,b.title
 UNION ALL
 SELECT i.canonical_lemma,i.upos,b.book_id::text,b.analysis_run_id::text,b.title,count(*)::bigint
 FROM current_books b
 JOIN occurrence_lemma_corrections c ON c.owner_id=b.owner_id AND c.book_id=b.book_id
  AND c.corpus_id=b.corpus_id AND c.analysis_run_id=b.analysis_run_id
 JOIN corpus_sentences s ON s.owner_id=c.owner_id AND s.analysis_run_id=c.analysis_run_id
  AND s.corpus_id=c.corpus_id AND s.unit_id=c.source_document_id
 JOIN corpus_tokens t ON t.owner_id=s.owner_id AND t.analysis_run_id=s.analysis_run_id
  AND t.corpus_id=s.corpus_id AND t.sentence_ordinal=s.sentence_ordinal
  AND t.start_offset=c.start_offset AND t.end_offset=c.end_offset AND t.language=b.language
 JOIN source_material_units u ON u.owner_id=b.owner_id AND u.source_material_id=b.source_material_id
  AND u.snapshot_id=b.snapshot_id AND u.unit_id=s.unit_id
 JOIN deck_identities i ON i.language=b.language AND i.canonical_lemma=c.canonical_lemma AND i.upos=t.upos
 WHERE NOT b.counts_ready AND NOT COALESCE(c.excluded,false) AND c.canonical_lemma IS NOT NULL
  AND t.upos IN ('NOUN','VERB','ADJ','ADV') AND t.dependency <> 'compound:prt'
  AND c.canonical_lemma ~ '[[:alpha:]]' AND t.canonical_lemma <> c.canonical_lemma
 GROUP BY i.canonical_lemma,i.upos,b.book_id,b.analysis_run_id,b.title
)
`

// ListCustomVocabularyDeckIdentityPage loads only the identities needed to
// render one review page. Totals are computed over the complete saved set, so
// paging and the missing-evidence filter never hide selected identities.
func (s *PostgresStore) ListCustomVocabularyDeckIdentityPage(ctx context.Context, owner, deckID string, page int, missingOnly bool) (domain.CustomVocabularyDeck, int64, error) {
	if page < 1 {
		page = 1
	}
	var deck domain.CustomVocabularyDeck
	totalsQuery := customDeckReviewEvidenceCTE + `, evidence_identities AS (
 SELECT DISTINCT lemma,upos FROM evidence
)
SELECT t.id::text,t.language,t.name,count(i.canonical_lemma)::bigint,
 count(i.canonical_lemma) FILTER (WHERE e.lemma IS NULL)::bigint
FROM target t
LEFT JOIN deck_identities i ON true
LEFT JOIN evidence_identities e ON e.lemma=i.canonical_lemma AND e.upos=i.upos
GROUP BY t.id,t.language,t.name`
	if err := s.pool.QueryRow(ctx, totalsQuery, owner, deckID).Scan(&deck.ID, &deck.Language, &deck.Name, &deck.IdentityCount, &deck.MissingCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CustomVocabularyDeck{}, 0, ErrCustomVocabularyDeckNotFound
		}
		return domain.CustomVocabularyDeck{}, 0, err
	}
	visibleTotal := deck.IdentityCount
	if missingOnly {
		visibleTotal = deck.MissingCount
	}
	lastPage := max(int64(1), (visibleTotal+24)/25)
	if int64(page) > lastPage {
		page = int(lastPage)
	}
	pageQuery := customDeckReviewEvidenceCTE + `, evidence_identities AS (
 SELECT lemma,upos,sum(occurrences)::bigint AS occurrences,count(*)::bigint AS books
 FROM evidence GROUP BY lemma,upos
)
SELECT i.canonical_lemma,i.upos,COALESCE(e.occurrences,0),COALESCE(e.books,0),e.lemma IS NULL
FROM custom_vocabulary_deck_identities i
LEFT JOIN evidence_identities e ON e.lemma=i.canonical_lemma AND e.upos=i.upos
WHERE i.owner_id=$1 AND i.deck_id=$2 AND (NOT $4 OR e.lemma IS NULL)

ORDER BY i.canonical_lemma,i.upos LIMIT 25 OFFSET (($3::bigint-1)*25)`
	rows, err := s.pool.Query(ctx, pageQuery, owner, deckID, page, missingOnly)
	if err != nil {
		return domain.CustomVocabularyDeck{}, 0, err
	}
	for rows.Next() {
		var identity domain.VocabularyIdentity
		if err := rows.Scan(&identity.CanonicalLemma, &identity.UPOS, &identity.OccurrenceCount, &identity.BookCount, &identity.MissingEvidence); err != nil {
			rows.Close()
			return domain.CustomVocabularyDeck{}, 0, err
		}
		deck.Identities = append(deck.Identities, identity)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.CustomVocabularyDeck{}, 0, err
	}
	rows.Close()
	if len(deck.Identities) == 0 {
		return deck, visibleTotal, nil
	}
	bookQuery := customDeckReviewEvidenceCTE + `, selected_page AS (
 SELECT i.canonical_lemma,i.upos FROM custom_vocabulary_deck_identities i
 LEFT JOIN (SELECT DISTINCT lemma,upos FROM evidence) e ON e.lemma=i.canonical_lemma AND e.upos=i.upos
 WHERE i.owner_id=$1 AND i.deck_id=$2 AND (NOT $4 OR e.lemma IS NULL)
 ORDER BY i.canonical_lemma,i.upos LIMIT 25 OFFSET (($3::bigint-1)*25)
)
SELECT p.canonical_lemma,p.upos,e.book_id,e.title,e.run_id,e.occurrences
FROM selected_page p JOIN evidence e ON e.lemma=p.canonical_lemma AND e.upos=p.upos
ORDER BY p.canonical_lemma,p.upos,e.title,e.book_id`
	bookRows, err := s.pool.Query(ctx, bookQuery, owner, deckID, page, missingOnly)
	if err != nil {
		return domain.CustomVocabularyDeck{}, 0, err
	}
	byIdentity := make(map[string]*domain.VocabularyIdentity, len(deck.Identities))
	for i := range deck.Identities {
		byIdentity[deck.Identities[i].CanonicalLemma+"\x00"+deck.Identities[i].UPOS] = &deck.Identities[i]
	}
	for bookRows.Next() {
		var lemma, upos string
		var book domain.VocabularyIdentityBook
		if err := bookRows.Scan(&lemma, &upos, &book.ID, &book.Title, &book.AnalysisRunID, &book.OccurrenceCount); err != nil {
			bookRows.Close()
			return domain.CustomVocabularyDeck{}, 0, err
		}
		if identity := byIdentity[lemma+"\x00"+upos]; identity != nil {
			identity.EvidenceBooks = append(identity.EvidenceBooks, book)
		}
	}
	if err := bookRows.Err(); err != nil {
		bookRows.Close()
		return domain.CustomVocabularyDeck{}, 0, err
	}
	bookRows.Close()
	return deck, visibleTotal, nil
}

func (s *PostgresStore) GetCustomVocabularyDeck(ctx context.Context, owner, deckID string) (domain.CustomVocabularyDeck, error) {
	var deck domain.CustomVocabularyDeck
	if err := s.pool.QueryRow(ctx, `SELECT id::text,language,name FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2`, owner, deckID).Scan(&deck.ID, &deck.Language, &deck.Name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CustomVocabularyDeck{}, ErrCustomVocabularyDeckNotFound
		}
		return domain.CustomVocabularyDeck{}, err
	}
	identities, err := s.ListCustomVocabularyDeckIdentities(ctx, owner, deckID)
	if err != nil {
		return domain.CustomVocabularyDeck{}, err
	}
	deck.Identities = identities
	deck.IdentityCount = int64(len(identities))
	for _, identity := range identities {
		if identity.MissingEvidence {
			deck.MissingCount++
		}
	}
	return deck, nil
}

func (s *PostgresStore) ListCustomVocabularyDecks(ctx context.Context, owner string) ([]domain.CustomVocabularyDeck, error) {
	rows, err := s.pool.Query(ctx, `
WITH decks AS MATERIALIZED (
 SELECT id,owner_id,language,name,created_at FROM custom_vocabulary_decks WHERE owner_id=$1
), identities AS MATERIALIZED (
 SELECT i.owner_id,i.deck_id,i.language,i.canonical_lemma,i.upos
 FROM custom_vocabulary_deck_identities i JOIN decks d ON d.id=i.deck_id
), current_books AS MATERIALIZED (
 SELECT cai.owner_id,cai.book_id,cai.analysis_run_id,cai.corpus_id,cai.source_material_id,cai.snapshot_id,
  b.title,b.language_tag AS language,
  r.owner_id IS NOT NULL AS counts_ready
 FROM current_analysis_identity cai JOIN books b ON b.owner_id=cai.owner_id AND b.id=cai.book_id
  AND b.language_state='chosen'
 JOIN (SELECT DISTINCT language FROM decks) d ON d.language=b.language_tag
 LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=cai.owner_id AND r.book_id=cai.book_id
  AND r.analysis_run_id=cai.analysis_run_id AND r.corpus_id=cai.corpus_id
  AND r.language=b.language_tag AND r.builder_version=2
 WHERE cai.owner_id=$1
), evidence AS (
 SELECT i.canonical_lemma AS lemma,i.upos,b.language
 FROM current_books b
 JOIN vocabulary_browse_counts c ON c.owner_id=b.owner_id AND c.book_id=b.book_id
  AND c.analysis_run_id=b.analysis_run_id AND c.corpus_id=b.corpus_id AND c.language=b.language
 JOIN identities i ON i.language=b.language AND i.canonical_lemma=c.canonical_lemma AND i.upos=c.upos
 WHERE b.counts_ready
 UNION
 SELECT i.canonical_lemma,i.upos,b.language
 FROM identities i
 JOIN current_books b ON b.language=i.language AND NOT b.counts_ready
 JOIN LATERAL (
  SELECT o.* FROM concordance_occurrences o
  WHERE o.owner_id=b.owner_id AND o.book_id=b.book_id::text
   AND o.analysis_run_id=b.analysis_run_id::text AND o.corpus_id=b.corpus_id::text
   AND o.language=b.language AND o.canonical_lemma=i.canonical_lemma AND o.upos=i.upos
 ) o ON true
 LEFT JOIN occurrence_lemma_corrections c ON c.owner_id=o.owner_id AND c.book_id::text=o.book_id
  AND c.corpus_id::text=o.corpus_id AND c.analysis_run_id::text=o.analysis_run_id
  AND c.source_document_id=o.unit_id AND c.start_offset=o.unit_start_offset AND c.end_offset=o.unit_end_offset
 WHERE o.upos IN ('NOUN','VERB','ADJ','ADV') AND o.dependency <> 'compound:prt'
  AND COALESCE(c.excluded,false)=false AND COALESCE(c.canonical_lemma,o.canonical_lemma)=i.canonical_lemma
  AND i.canonical_lemma ~ '[[:alpha:]]'
 UNION
 SELECT i.canonical_lemma,i.upos,b.language
 FROM current_books b
 JOIN occurrence_lemma_corrections c ON c.owner_id=b.owner_id AND c.book_id=b.book_id
  AND c.corpus_id=b.corpus_id AND c.analysis_run_id=b.analysis_run_id
 JOIN corpus_sentences s ON s.owner_id=c.owner_id AND s.analysis_run_id=c.analysis_run_id
  AND s.corpus_id=c.corpus_id AND s.unit_id=c.source_document_id
 JOIN corpus_tokens t ON t.owner_id=s.owner_id AND t.analysis_run_id=s.analysis_run_id
  AND t.corpus_id=s.corpus_id AND t.sentence_ordinal=s.sentence_ordinal
  AND t.start_offset=c.start_offset AND t.end_offset=c.end_offset AND t.language=b.language
 JOIN source_material_units u ON u.owner_id=b.owner_id AND u.source_material_id=b.source_material_id
  AND u.snapshot_id=b.snapshot_id AND u.unit_id=s.unit_id
 JOIN identities i ON i.language=b.language AND i.canonical_lemma=c.canonical_lemma AND i.upos=t.upos
 WHERE NOT b.counts_ready AND NOT COALESCE(c.excluded,false) AND c.canonical_lemma IS NOT NULL
  AND t.upos IN ('NOUN','VERB','ADJ','ADV') AND t.dependency <> 'compound:prt'
  AND c.canonical_lemma ~ '[[:alpha:]]' AND t.canonical_lemma <> c.canonical_lemma
)
SELECT d.id::text,d.language,d.name,count(i.canonical_lemma)::bigint,
 count(i.canonical_lemma) FILTER (WHERE e.lemma IS NULL)::bigint
FROM decks d
LEFT JOIN identities i ON i.owner_id=d.owner_id AND i.deck_id=d.id
LEFT JOIN (SELECT DISTINCT language,lemma,upos FROM evidence) e
 ON e.language=i.language AND e.lemma=i.canonical_lemma AND e.upos=i.upos
GROUP BY d.id,d.language,d.name,d.created_at ORDER BY d.created_at,d.id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var decks []domain.CustomVocabularyDeck
	for rows.Next() {
		var deck domain.CustomVocabularyDeck
		if err := rows.Scan(&deck.ID, &deck.Language, &deck.Name, &deck.IdentityCount, &deck.MissingCount); err != nil {
			return nil, err
		}
		decks = append(decks, deck)
	}
	return decks, rows.Err()
}

func (s *PostgresStore) RenameCustomVocabularyDeck(ctx context.Context, owner, deckID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return ErrCustomVocabularyDeckNameInvalid
	}
	tag, err := s.pool.Exec(ctx, `UPDATE custom_vocabulary_decks SET name=$3 WHERE owner_id=$1 AND id=$2`, owner, deckID, name)
	if err != nil {
		return fmt.Errorf("rename Custom deck: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCustomVocabularyDeckNotFound
	}
	return nil
}

func (s *PostgresStore) SetCustomVocabularyDeckIdentity(ctx context.Context, owner, deckID, lemma, upos string, selected bool) (err error) {
	lemma = strings.TrimSpace(lemma)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	var language string
	if err = tx.QueryRow(ctx, `SELECT language FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, deckID).Scan(&language); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCustomVocabularyDeckNotFound
		}
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1||':'||$2,0))`, owner, deckID); err != nil {
		return err
	}
	if selected {
		_, err = tx.Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos)
 VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, owner, deckID, language, lemma, upos)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM custom_vocabulary_deck_identities WHERE owner_id=$1 AND deck_id=$2 AND canonical_lemma=$3 AND upos=$4`, owner, deckID, lemma, upos)
	}
	if err != nil {
		return fmt.Errorf("edit Custom deck identity: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) DeleteCustomVocabularyDeck(ctx context.Context, owner, deckID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2`, owner, deckID)
	if err != nil {
		return fmt.Errorf("delete Custom deck: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCustomVocabularyDeckNotFound
	}
	return nil
}
