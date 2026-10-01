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

func (s *PostgresStore) ListVocabularyBrowseSelection(ctx context.Context, owner, language string) ([]domain.VocabularyIdentity, error) {
	const query = `
WITH evidence AS (
 SELECT o.book_id, COALESCE(d.canonical_lemma,o.canonical_lemma) AS lemma,o.upos
 FROM concordance_occurrences o
 JOIN current_analysis_identity cai ON cai.owner_id=o.owner_id AND cai.book_id::text=o.book_id
  AND cai.analysis_run_id::text=o.analysis_run_id AND cai.corpus_id::text=o.corpus_id
 JOIN books b ON b.owner_id=o.owner_id AND b.id::text=o.book_id AND b.language_tag=o.language AND b.language_state='chosen'
 JOIN vocabulary_browse_selections s ON s.owner_id=o.owner_id AND s.language=o.language
 LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=o.owner_id AND d.book_id::text=o.book_id
  AND d.corpus_id::text=o.corpus_id AND d.analysis_run_id::text=o.analysis_run_id
  AND d.source_document_id=o.unit_id AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
 WHERE o.owner_id=$1 AND o.language=$2 AND s.canonical_lemma=COALESCE(d.canonical_lemma,o.canonical_lemma)
  AND s.upos=o.upos AND o.upos IN ('NOUN','VERB','ADJ','ADV') AND o.dependency <> 'compound:prt'
  AND COALESCE(d.excluded,false)=false
), counts AS (
 SELECT lemma,upos,count(*)::bigint AS occurrences,count(DISTINCT book_id)::bigint AS books FROM evidence GROUP BY lemma,upos
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
		if err := tx.QueryRow(ctx, `SELECT id::text,language,name FROM custom_vocabulary_decks WHERE owner_id=$1 AND creation_key=$2`, owner, creationKey).Scan(&deck.ID, &deck.Language, &deck.Name); err != nil {
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
	deck.Identities, err = s.ListCustomVocabularyDeckIdentities(ctx, owner, deck.ID)
	if err != nil {
		return domain.CustomVocabularyDeck{}, err
	}
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
SELECT d.id::text,d.language,d.name,count(i.canonical_lemma)::bigint,
	 count(i.canonical_lemma) FILTER (WHERE NOT EXISTS (
  SELECT 1 FROM concordance_occurrences o
  JOIN current_analysis_identity cai ON cai.owner_id=o.owner_id AND cai.book_id::text=o.book_id
   AND cai.analysis_run_id::text=o.analysis_run_id AND cai.corpus_id::text=o.corpus_id
  JOIN books b ON b.owner_id=o.owner_id AND b.id::text=o.book_id AND b.language_state='chosen' AND b.language_tag=o.language
  LEFT JOIN occurrence_lemma_corrections c ON c.owner_id=o.owner_id AND c.book_id::text=o.book_id
   AND c.corpus_id::text=o.corpus_id AND c.analysis_run_id::text=o.analysis_run_id
   AND c.source_document_id=o.unit_id AND c.start_offset=o.unit_start_offset AND c.end_offset=o.unit_end_offset
  WHERE o.owner_id=i.owner_id AND o.language=i.language AND o.upos=i.upos
   AND COALESCE(c.canonical_lemma,o.canonical_lemma)=i.canonical_lemma
    AND COALESCE(c.excluded,false)=false AND o.upos IN ('NOUN','VERB','ADJ','ADV')
    AND o.dependency <> 'compound:prt'
    AND COALESCE(c.canonical_lemma,o.canonical_lemma) ~ '[[:alpha:]]'
 )) AS missing_count
FROM custom_vocabulary_decks d LEFT JOIN custom_vocabulary_deck_identities i
 ON i.owner_id=d.owner_id AND i.deck_id=d.id
WHERE d.owner_id=$1 GROUP BY d.id,d.language,d.name ORDER BY d.created_at,d.id`, owner)
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
