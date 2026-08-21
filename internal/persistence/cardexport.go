package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
)

func (s *PostgresStore) ListAcceptedCurated(ctx context.Context, owner string) ([]cardexport.Entry, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.owner_id::text,c.language,c.canonical_lemma,c.upos,e.sentence_text,COALESCE(en.translation,''),c.canonical_lemma,COALESCE(sl.morphology::text,'{}'),sm.title,c.notes FROM curated_sentences c JOIN vocabulary_states v ON v.owner_id=c.owner_id AND v.language=c.language AND v.canonical_lemma=c.canonical_lemma AND v.upos=c.upos AND v.state='accepted' JOIN example_sentences e ON e.owner_id=c.owner_id AND e.id=c.example_sentence_id JOIN corpora co ON co.owner_id=e.owner_id AND co.id=e.corpus_id JOIN source_materials sm ON sm.owner_id=co.owner_id AND sm.id=co.source_material_id LEFT JOIN LATERAL (SELECT translation FROM enrichment_cache WHERE language=c.language AND canonical_lemma=c.canonical_lemma AND upos=upper(c.upos) ORDER BY cached_at DESC LIMIT 1) en ON true LEFT JOIN LATERAL (SELECT morphology FROM shared_lemmas WHERE content_hash=co.artifact_hash AND language=c.language AND canonical_lemma=c.canonical_lemma AND upos=c.upos ORDER BY id LIMIT 1) sl ON true WHERE c.owner_id=$1 ORDER BY c.language,c.canonical_lemma,c.upos`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []cardexport.Entry
	for rows.Next() {
		var e cardexport.Entry
		if err := rows.Scan(&e.OwnerID, &e.Language, &e.CanonicalLemma, &e.UPOS, &e.Sentence, &e.Translation, &e.TargetWord, &e.Morphology, &e.SourceDocument, &e.Notes); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *PostgresStore) RecordGenerated(ctx context.Context, owner, deckName string, entry cardexport.Entry, note cardexport.Note) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	err = tx.QueryRow(ctx, `SELECT state FROM vocabulary_states WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4 FOR UPDATE`, owner, entry.Language, entry.CanonicalLemma, entry.UPOS).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != "accepted" && state != "generated" {
		return fmt.Errorf("cardexport: vocabulary state is %s", state)
	}
	var deckID string
	if err = tx.QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,$2,$3) ON CONFLICT(owner_id,language,name) DO UPDATE SET name=excluded.name RETURNING id::text`, owner, entry.Language, deckName).Scan(&deckID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cards(owner_id,deck_id,dedup_key,canonical_lemma,upos,front,back) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(owner_id,dedup_key) DO UPDATE SET deck_id=excluded.deck_id,front=excluded.front,back=excluded.back`, owner, deckID, note.Key, entry.CanonicalLemma, entry.UPOS, note.Text, note.BackExtra); err != nil {
		return err
	}
	if state == "accepted" {
		if _, err = tx.Exec(ctx, `UPDATE vocabulary_states SET state='generated',updated_at=now() WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4`, owner, entry.Language, entry.CanonicalLemma, entry.UPOS); err != nil {
			return err
		}
		details, _ := json.Marshal(map[string]string{"language": entry.Language, "canonical_lemma": entry.CanonicalLemma, "upos": entry.UPOS, "from": "accepted", "to": "generated"})
		completed := time.Now().UTC()
		if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'vocabulary.transition','completed',$2,$3)`, owner, details, completed); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
