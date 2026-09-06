package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

func (s *PostgresStore) ListSelectionCandidatesForBook(ctx context.Context, owner, bookID string) ([]domain.SelectionCandidate, error) {
	rows, err := s.pool.Query(ctx, `SELECT sc.owner_id::text,sc.corpus_id,sc.language,sc.canonical_lemma,sc.upos,sc.occurrence_count,sc.observed_forms,sc.eligible_sentence_refs,sc.provenance,sc.selected_at,COALESCE(first_seen.start_offset,9223372036854775807) FROM selection_candidates sc JOIN corpora co ON co.owner_id=sc.owner_id AND co.id::text=sc.corpus_id LEFT JOIN LATERAL (SELECT MIN(COALESCE(ref->'location'->>'start_offset',ref->'location'->>'StartOffset',ref->'Location'->>'StartOffset')::bigint) AS start_offset FROM jsonb_array_elements(sc.eligible_sentence_refs) ref) first_seen ON true WHERE sc.owner_id=$1 AND co.source_material_id=$2 ORDER BY sc.language,sc.canonical_lemma,sc.upos`, owner, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []domain.SelectionCandidate
	for rows.Next() {
		var candidate domain.SelectionCandidate
		if err := rows.Scan(&candidate.OwnerID, &candidate.CorpusID, &candidate.Language, &candidate.CanonicalLemma, &candidate.UPOS, &candidate.OccurrenceCount, &candidate.ObservedForms, &candidate.SentenceReferences, &candidate.Provenance, &candidate.SelectedAt, &candidate.FirstEncounter); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (s *PostgresStore) ListSelectionCandidatesForCorpus(ctx context.Context, owner, corpusID string) ([]domain.SelectionCandidate, error) {
	rows, err := s.pool.Query(ctx, `SELECT sc.owner_id::text,sc.corpus_id,sc.language,sc.canonical_lemma,sc.upos,sc.occurrence_count,sc.observed_forms,sc.eligible_sentence_refs,sc.provenance,sc.selected_at,COALESCE(first_seen.start_offset,9223372036854775807)
		FROM selection_candidates sc
		JOIN corpora co ON co.owner_id=sc.owner_id AND co.id::text=sc.corpus_id
		LEFT JOIN LATERAL (SELECT MIN(COALESCE(ref->'location'->>'start_offset',ref->'location'->>'StartOffset',ref->'Location'->>'StartOffset')::bigint) AS start_offset FROM jsonb_array_elements(sc.eligible_sentence_refs) ref) first_seen ON true
		WHERE sc.owner_id=$1 AND sc.corpus_id=$2 ORDER BY sc.language,sc.canonical_lemma,sc.upos`, owner, corpusID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []domain.SelectionCandidate
	for rows.Next() {
		var candidate domain.SelectionCandidate
		if err := rows.Scan(&candidate.OwnerID, &candidate.CorpusID, &candidate.Language, &candidate.CanonicalLemma, &candidate.UPOS, &candidate.OccurrenceCount, &candidate.ObservedForms, &candidate.SentenceReferences, &candidate.Provenance, &candidate.SelectedAt, &candidate.FirstEncounter); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (s *PostgresStore) GetCoverageEntryForBook(ctx context.Context, owner, bookID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	return s.getCoverageEntryForBook(ctx, owner, bookID, candidate, true)
}

// GetPreparedCoverageEntryForBook loads only immutable render inputs. Exact
// enrichment is applied later from the preparation manifest.
func (s *PostgresStore) GetPreparedCoverageEntryForBook(ctx context.Context, owner, bookID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	return s.getCoverageEntryForBook(ctx, owner, bookID, candidate, false)
}

func (s *PostgresStore) getCoverageEntryForBook(ctx context.Context, owner, bookID string, candidate domain.SelectionCandidate, includeLegacyEnrichment bool) (cardexport.Entry, error) {
	var entry cardexport.Entry
	err := s.pool.QueryRow(ctx, `SELECT sc.owner_id::text,sc.language,sc.canonical_lemma,sc.upos,COALESCE(e.sentence_text,''),'','',COALESCE(sl.morphologies::text,'[]'),sm.title,'',$7::bigint FROM selection_candidates sc JOIN corpora co ON co.owner_id=sc.owner_id AND co.id::text=sc.corpus_id JOIN source_materials sm ON sm.owner_id=co.owner_id AND sm.id=co.source_material_id LEFT JOIN LATERAL (SELECT ex.* FROM example_sentences ex WHERE ex.owner_id=sc.owner_id AND ex.corpus_id=co.id AND ex.language=sc.language AND ex.canonical_lemma=sc.canonical_lemma AND ex.upos=sc.upos ORDER BY ex.is_chosen DESC,ex.selection_rank,ex.id LIMIT 1) e ON true LEFT JOIN LATERAL (SELECT jsonb_agg(morphology ORDER BY frequency DESC,morphology::text) AS morphologies FROM shared_lemmas WHERE content_hash=co.artifact_hash AND language=sc.language AND canonical_lemma=sc.canonical_lemma AND upos=sc.upos) sl ON true WHERE sc.owner_id=$1 AND sm.id=$2 AND sc.corpus_id=$3 AND sc.language=$4 AND sc.canonical_lemma=$5 AND sc.upos=$6`, owner, bookID, candidate.CorpusID, candidate.Language, candidate.CanonicalLemma, candidate.UPOS, candidate.FirstEncounter).Scan(&entry.OwnerID, &entry.Language, &entry.CanonicalLemma, &entry.UPOS, &entry.Sentence, &entry.Translation, &entry.TargetWord, &entry.Morphology, &entry.SourceDocument, &entry.Notes, &entry.FirstEncounter)
	if err = missing(err); err != nil {
		return entry, err
	}
	if evidence, ok := cardexport.BestSentenceEvidence(candidate); ok {
		entry.Sentence = evidence.Sentence
		entry.TargetWord = evidence.Target
		entry.FirstEncounter = evidence.FirstEncounter
	}
	if !includeLegacyEnrichment {
		return entry, nil
	}
	err = s.pool.QueryRow(ctx, `SELECT translation,sentence_translation,sentence_translation_target FROM enrichment_cache WHERE language=$1 AND target_language='en' AND canonical_lemma=$2 AND upos=upper($3) AND sentence_hash=$4 ORDER BY cached_at DESC LIMIT 1`, entry.Language, entry.CanonicalLemma, entry.UPOS, enrichment.SentenceHash(entry.Sentence)).Scan(&entry.Translation, &entry.SentenceTranslation, &entry.SentenceTranslationTarget)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return entry, err
}

func (s *PostgresStore) GetCoverageEntryForCorpus(ctx context.Context, owner, corpusID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	return s.getCoverageEntryForCorpus(ctx, owner, corpusID, candidate, true)
}

// GetPreparedCoverageEntryForCorpus loads only immutable render inputs. It
// deliberately performs no broad provider/version cache lookup.
func (s *PostgresStore) GetPreparedCoverageEntryForCorpus(ctx context.Context, owner, corpusID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	return s.getCoverageEntryForCorpus(ctx, owner, corpusID, candidate, false)
}

func (s *PostgresStore) getCoverageEntryForCorpus(ctx context.Context, owner, corpusID string, candidate domain.SelectionCandidate, includeLegacyEnrichment bool) (cardexport.Entry, error) {
	var entry cardexport.Entry
	err := s.pool.QueryRow(ctx, `SELECT sc.owner_id::text,sc.language,sc.canonical_lemma,sc.upos,COALESCE(e.sentence_text,''),'','',COALESCE(sl.morphologies::text,'[]'),sm.title,'',$6::bigint
		FROM selection_candidates sc
		JOIN corpora co ON co.owner_id=sc.owner_id AND co.id::text=sc.corpus_id
		JOIN source_materials sm ON sm.owner_id=co.owner_id AND sm.id=co.source_material_id
		LEFT JOIN LATERAL (SELECT ex.* FROM example_sentences ex WHERE ex.owner_id=sc.owner_id AND ex.corpus_id=co.id AND ex.language=sc.language AND ex.canonical_lemma=sc.canonical_lemma AND ex.upos=sc.upos ORDER BY ex.is_chosen DESC,ex.selection_rank,ex.id LIMIT 1) e ON true
		LEFT JOIN LATERAL (SELECT jsonb_agg(morphology ORDER BY frequency DESC,morphology::text) AS morphologies FROM shared_lemmas WHERE content_hash=co.artifact_hash AND language=sc.language AND canonical_lemma=sc.canonical_lemma AND upos=sc.upos) sl ON true
		WHERE sc.owner_id=$1 AND sc.corpus_id=$2 AND sc.language=$3 AND sc.canonical_lemma=$4 AND sc.upos=$5`, owner, corpusID, candidate.Language, candidate.CanonicalLemma, candidate.UPOS, candidate.FirstEncounter).Scan(&entry.OwnerID, &entry.Language, &entry.CanonicalLemma, &entry.UPOS, &entry.Sentence, &entry.Translation, &entry.TargetWord, &entry.Morphology, &entry.SourceDocument, &entry.Notes, &entry.FirstEncounter)
	if err = missing(err); err != nil {
		return entry, err
	}
	if evidence, ok := cardexport.BestSentenceEvidence(candidate); ok {
		entry.Sentence = evidence.Sentence
		entry.TargetWord = evidence.Target
		entry.FirstEncounter = evidence.FirstEncounter
	}
	if !includeLegacyEnrichment {
		return entry, nil
	}
	err = s.pool.QueryRow(ctx, `SELECT translation,sentence_translation,sentence_translation_target FROM enrichment_cache WHERE language=$1 AND target_language='en' AND canonical_lemma=$2 AND upos=upper($3) AND sentence_hash=$4 ORDER BY cached_at DESC LIMIT 1`, entry.Language, entry.CanonicalLemma, entry.UPOS, enrichment.SentenceHash(entry.Sentence)).Scan(&entry.Translation, &entry.SentenceTranslation, &entry.SentenceTranslationTarget)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return entry, err
}

func (s *PostgresStore) GetCorpusForAnalysis(ctx context.Context, owner, analysisRunID string) (domain.Corpus, error) {
	var corpus domain.Corpus
	err := s.pool.QueryRow(ctx, `SELECT c.id::text,c.owner_id::text,c.source_material_id::text,c.artifact_hash,COALESCE(c.reviewed_scope_id::text,''),COALESCE(c.analysis_run_id::text,''),c.status,c.created_at
		FROM corpora c JOIN analysis_runs r ON r.owner_id=c.owner_id AND r.id=c.analysis_run_id AND r.source_material_id=c.source_material_id
		WHERE c.owner_id=$1 AND r.id=$2 AND r.state='completed' AND c.status='complete' AND ((r.scope_id IS NULL AND c.reviewed_scope_id IS NULL) OR c.reviewed_scope_id=r.scope_id)`, owner, analysisRunID).Scan(&corpus.ID, &corpus.OwnerID, &corpus.SourceMaterialID, &corpus.ArtifactHash, &corpus.ReviewedScopeID, &corpus.AnalysisRunID, &corpus.Status, &corpus.CreatedAt)
	return corpus, missing(err)
}

func (s *PostgresStore) RecordGenerated(ctx context.Context, owner, deckName string, entry cardexport.Entry, note cardexport.Note) error {
	return s.recordGenerated(ctx, owner, "", deckName, entry, note)
}

// RecordGeneratedForBook atomically persists the exported card and its first
// generated-vocabulary provenance for the source material that produced it.
func (s *PostgresStore) RecordGeneratedForBook(ctx context.Context, owner, bookID, deckName string, entry cardexport.Entry, note cardexport.Note) error {
	return s.recordGenerated(ctx, owner, bookID, deckName, entry, note)
}

func (s *PostgresStore) recordGenerated(ctx context.Context, owner, bookID, deckName string, entry cardexport.Entry, note cardexport.Note) error {
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
	if state != "candidate" && state != "accepted" && state != "generated" {
		return fmt.Errorf("cardexport: vocabulary state is %s", state)
	}
	var deckID string
	if err = tx.QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,$2,$3) ON CONFLICT(owner_id,language,name) DO UPDATE SET name=excluded.name RETURNING id::text`, owner, entry.Language, deckName).Scan(&deckID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cards(owner_id,deck_id,dedup_key,canonical_lemma,upos,front,back) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(owner_id,dedup_key) DO UPDATE SET deck_id=excluded.deck_id,front=excluded.front,back=excluded.back`, owner, deckID, note.Key, entry.CanonicalLemma, entry.UPOS, note.Text, note.BackExtra); err != nil {
		return err
	}
	var sourceMaterialID *string
	if bookID != "" {
		sourceMaterialID = &bookID
	}
	if _, err = tx.Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO NOTHING`, owner, entry.Language, entry.CanonicalLemma, entry.UPOS, deckID, sourceMaterialID); err != nil {
		return err
	}
	// Keep the generated lifecycle state as legacy bookkeeping for compatibility.
	// Cross-book exclusion is driven exclusively by generated_vocabulary above.
	if state != "generated" {
		if _, err = tx.Exec(ctx, `UPDATE vocabulary_states SET state='generated',updated_at=now() WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4`, owner, entry.Language, entry.CanonicalLemma, entry.UPOS); err != nil {
			return err
		}
		details, _ := json.Marshal(map[string]string{"language": entry.Language, "canonical_lemma": entry.CanonicalLemma, "upos": entry.UPOS, "from": state, "to": "generated"})
		completed := time.Now().UTC()
		if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'vocabulary.transition','completed',$2,$3)`, owner, details, completed); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// RecordGeneratedVocabulary records first provenance for an owner-scoped
// vocabulary identity. Repeated records intentionally preserve the first row.
func (s *PostgresStore) RecordGeneratedVocabulary(ctx context.Context, value domain.GeneratedVocabulary) (domain.GeneratedVocabulary, error) {
	_, err := s.pool.Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO NOTHING`, value.OwnerID, value.Language, value.CanonicalLemma, value.UPOS, value.FirstDeckID, value.FirstSourceMaterialID)
	if err != nil {
		return domain.GeneratedVocabulary{}, err
	}
	return s.getGeneratedVocabulary(ctx, value.OwnerID, value.Language, value.CanonicalLemma, value.UPOS)
}

func (s *PostgresStore) getGeneratedVocabulary(ctx context.Context, owner, language, lemma, upos string) (value domain.GeneratedVocabulary, err error) {
	err = s.pool.QueryRow(ctx, `SELECT owner_id::text,language,canonical_lemma,upos,first_deck_id::text,first_source_material_id::text,first_generated_at FROM generated_vocabulary WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4`, owner, language, lemma, upos).Scan(&value.OwnerID, &value.Language, &value.CanonicalLemma, &value.UPOS, &value.FirstDeckID, &value.FirstSourceMaterialID, &value.FirstGeneratedAt)
	err = missing(err)
	return
}

func (s *PostgresStore) ListGeneratedVocabulary(ctx context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	rows, err := s.pool.Query(ctx, `SELECT owner_id::text,language,canonical_lemma,upos,first_deck_id::text,first_source_material_id::text,first_generated_at FROM generated_vocabulary WHERE owner_id=$1 AND language=$2 ORDER BY canonical_lemma,upos`, owner, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.GeneratedVocabulary
	for rows.Next() {
		var value domain.GeneratedVocabulary
		if err := rows.Scan(&value.OwnerID, &value.Language, &value.CanonicalLemma, &value.UPOS, &value.FirstDeckID, &value.FirstSourceMaterialID, &value.FirstGeneratedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
