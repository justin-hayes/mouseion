package persistence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const deckPreparationColumns = `id::text,owner_id::text,source_material_id::text,COALESCE(analysis_run_id::text,''),COALESCE(current_run_id::text,''),state,artifact,filename,deck_name,content_hash,total_cards,cards_with_english,cards_with_contextual_sentence_translations,quality_omissions,error,created_at,updated_at,started_at,completed_at,studying_at,reviewed_at,graduated_at,released_at`

type rowScanner interface {
	Scan(...any) error
}

// CompletePreparedDeck atomically stores the immutable artifact and every card
// and generated-vocabulary row. A failure rolls back all assignment state.
func (s *PostgresStore) CompletePreparedDeck(ctx context.Context, owner, id string, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	ready, err := completePreparedDeckTx(ctx, tx, owner, id, artifact)
	if err != nil {
		return ready, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return ready, nil
}

// CompletePreparedDeckRun extends the existing atomic artifact boundary with a
// current-run and finalizer-token fence. A retry after commit is a no-op only
// when both the immutable artifact and completed run match.
func (s *PostgresStore) CompletePreparedDeckRun(ctx context.Context, owner, preparationID, runID, claimToken string, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	var preparationState domain.DeckPreparationState
	var currentRunID string
	if err = tx.QueryRow(ctx, `SELECT state,COALESCE(current_run_id::text,'') FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, preparationID).Scan(&preparationState, &currentRunID); err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	run, err := scanPreparedDeckRun(tx.QueryRow(ctx, `SELECT `+preparedDeckRunColumns+` FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 FOR UPDATE`, owner, preparationID, runID))
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if preparationState == domain.DeckPreparationReady && run.State == domain.PreparedDeckRunCompleted && currentRunID == runID {
		ready, completeErr := completePreparedDeckTx(ctx, tx, owner, preparationID, artifact)
		if completeErr != nil {
			return ready, completeErr
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.DeckPreparation{}, err
		}
		return ready, nil
	}
	var leaseActive bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE(finalization_lease_expires_at > now(),false) FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3`, owner, preparationID, runID).Scan(&leaseActive); err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	if currentRunID != runID || run.State != domain.PreparedDeckRunFinalizing || run.TranslationState != domain.PreparedDeckTranslationCompleted || run.FinalizationClaimToken == "" || run.FinalizationClaimToken != claimToken || !leaseActive {
		return domain.DeckPreparation{}, ErrFenced
	}
	ready, err := completePreparedDeckTx(ctx, tx, owner, preparationID, artifact)
	if err != nil {
		return ready, err
	}
	tag, err := tx.Exec(ctx, `UPDATE deck_preparation_runs SET state='completed',finalization_claim_token=NULL,finalization_claimed_at=NULL,finalization_lease_expires_at=NULL,error_class='',error_code='',completed_at=now(),updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='finalizing' AND finalization_claim_token=$4`, owner, preparationID, runID, claimToken)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if tag.RowsAffected() != 1 {
		return domain.DeckPreparation{}, ErrFenced
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return ready, nil
}

func completePreparedDeckTx(ctx context.Context, tx pgx.Tx, owner, id string, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	var state domain.DeckPreparationState
	err := tx.QueryRow(ctx, `SELECT state FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id).Scan(&state)
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	if state == domain.DeckPreparationReady {
		p, getErr := scanDeckPreparation(tx.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2`, owner, id))
		if getErr != nil {
			return p, getErr
		}
		if !bytes.Equal(p.Artifact, artifact.APKG) || p.Filename != artifact.Filename || p.DeckName != artifact.DeckName || p.TotalCards != artifact.Completeness.TotalCards || p.CardsWithEnglish != artifact.Completeness.CardsWithEnglish || p.CardsWithContextualSentenceTranslations != artifact.Completeness.CardsWithEnglishSentence || p.QualityOmissions != artifact.Completeness.QualityOmitted {
			return p, ErrImmutable
		}
		return p, nil
	}
	if state != domain.DeckPreparationPreparing {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	for _, item := range artifact.Generated {
		entry, note := item.Entry, item.Note
		var vocabularyState string
		err = tx.QueryRow(ctx, `SELECT state FROM vocabulary_states WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4 FOR UPDATE`, owner, entry.Language, entry.CanonicalLemma, entry.UPOS).Scan(&vocabularyState)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DeckPreparation{}, ErrNotFound
		}
		if err != nil {
			return domain.DeckPreparation{}, err
		}
		if vocabularyState != "candidate" && vocabularyState != "accepted" && vocabularyState != "generated" {
			return domain.DeckPreparation{}, fmt.Errorf("cardexport: vocabulary state is %s", vocabularyState)
		}
		var deckID string
		if err = tx.QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,$2,$3) ON CONFLICT(owner_id,language,name) DO UPDATE SET name=excluded.name RETURNING id::text`, owner, entry.Language, note.BookTitle).Scan(&deckID); err != nil {
			return domain.DeckPreparation{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO cards(owner_id,deck_id,dedup_key,canonical_lemma,upos,front,back) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(owner_id,dedup_key) DO UPDATE SET deck_id=excluded.deck_id,front=excluded.front,back=excluded.back`, owner, deckID, note.Key, entry.CanonicalLemma, entry.UPOS, note.Text, note.BackExtra); err != nil {
			return domain.DeckPreparation{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) SELECT $1,$2,$3,$4,$5,source_material_id FROM deck_preparations WHERE owner_id=$1 AND id=$6 ON CONFLICT(owner_id,language,canonical_lemma,upos) DO NOTHING`, owner, entry.Language, entry.CanonicalLemma, entry.UPOS, deckID, id); err != nil {
			return domain.DeckPreparation{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at)
			SELECT gv.owner_id,$5,gv.language,gv.canonical_lemma,gv.upos,gv.first_generated_at
			FROM generated_vocabulary gv
			WHERE gv.owner_id=$1 AND gv.language=$2 AND gv.canonical_lemma=$3 AND gv.upos=$4
			ON CONFLICT DO NOTHING`, owner, entry.Language, entry.CanonicalLemma, entry.UPOS, id); err != nil {
			return domain.DeckPreparation{}, err
		}
		if vocabularyState != "generated" {
			if _, err = tx.Exec(ctx, `UPDATE vocabulary_states SET state='generated',updated_at=now() WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4`, owner, entry.Language, entry.CanonicalLemma, entry.UPOS); err != nil {
				return domain.DeckPreparation{}, err
			}
			details, _ := json.Marshal(map[string]string{"language": entry.Language, "canonical_lemma": entry.CanonicalLemma, "upos": entry.UPOS, "from": vocabularyState, "to": "generated"})
			if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'vocabulary.transition','completed',$2,$3)`, owner, details, time.Now().UTC()); err != nil {
				return domain.DeckPreparation{}, err
			}
		}
	}
	ready, err := scanDeckPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET state='ready',artifact=$3,filename=$4,deck_name=$5,total_cards=$6,cards_with_english=$7,cards_with_contextual_sentence_translations=$8,quality_omissions=$9,error='',completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND state='preparing' RETURNING `+deckPreparationColumns, owner, id, artifact.APKG, artifact.Filename, artifact.DeckName, artifact.Completeness.TotalCards, artifact.Completeness.CardsWithEnglish, artifact.Completeness.CardsWithEnglishSentence, artifact.Completeness.QualityOmitted))
	if err != nil {
		return ready, err
	}
	return ready, nil
}

func scanDeckPreparation(row rowScanner) (domain.DeckPreparation, error) {
	var p domain.DeckPreparation
	err := row.Scan(&p.ID, &p.OwnerID, &p.SourceMaterialID, &p.AnalysisRunID, &p.CurrentRunID, &p.State, &p.Artifact, &p.Filename, &p.DeckName, &p.ContentHash, &p.TotalCards, &p.CardsWithEnglish, &p.CardsWithContextualSentenceTranslations, &p.QualityOmissions, &p.Error, &p.CreatedAt, &p.UpdatedAt, &p.StartedAt, &p.CompletedAt, &p.StudyingAt, &p.ReviewedAt, &p.GraduatedAt, &p.ReleasedAt)
	return p, missing(err)
}

// ListDeckPreparationVocabulary returns the immutable identity snapshot for a
// prepared deck. Only graduation timestamps can change after completion.
func (s *PostgresStore) ListDeckPreparationVocabulary(ctx context.Context, owner, preparationID string) ([]domain.DeckPreparationVocabulary, error) {
	rows, err := s.pool.Query(ctx, `SELECT owner_id::text,deck_preparation_id::text,language,canonical_lemma,upos,generated_at,graduated_at
		FROM deck_preparation_vocabulary WHERE owner_id=$1 AND deck_preparation_id=$2 ORDER BY language,canonical_lemma,upos`, owner, preparationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.DeckPreparationVocabulary
	for rows.Next() {
		var item domain.DeckPreparationVocabulary
		if err = rows.Scan(&item.OwnerID, &item.DeckPreparationID, &item.Language, &item.CanonicalLemma, &item.UPOS, &item.GeneratedAt, &item.GraduatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ListReservedVocabulary returns the vocabulary currently reserved by the
// owner's book-anchored study: the snapshotted vocabulary of every studying
// deck that has not yet graduated, scoped to one language. Reserved vocabulary
// is neither counted as known nor eligible for another deck until the study is
// resolved.
func (s *PostgresStore) ListReservedVocabulary(ctx context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	rows, err := s.pool.Query(ctx, `SELECT dv.owner_id::text,dv.deck_preparation_id::text,dv.language,dv.canonical_lemma,dv.upos,dv.generated_at,dv.graduated_at
		FROM deck_preparation_vocabulary dv
		JOIN deck_preparations p ON p.owner_id=dv.owner_id AND p.id=dv.deck_preparation_id
		WHERE dv.owner_id=$1 AND dv.language=$2 AND p.studying_at IS NOT NULL AND p.graduated_at IS NULL AND dv.graduated_at IS NULL
		ORDER BY dv.canonical_lemma,dv.upos`, owner, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.DeckPreparationVocabulary
	for rows.Next() {
		var item domain.DeckPreparationVocabulary
		if err = rows.Scan(&item.OwnerID, &item.DeckPreparationID, &item.Language, &item.CanonicalLemma, &item.UPOS, &item.GeneratedAt, &item.GraduatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// CountDeckPreparationVocabularyToGraduate supports the consequential review
// confirmation without making the count authoritative for the transaction.
func (s *PostgresStore) CountDeckPreparationVocabularyToGraduate(ctx context.Context, owner, preparationID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM deck_preparation_vocabulary dv
		WHERE dv.owner_id=$1 AND dv.deck_preparation_id=$2 AND dv.graduated_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM known_vocabulary kv
			WHERE kv.owner_id=dv.owner_id AND kv.language=dv.language AND kv.canonical_lemma=dv.canonical_lemma
				AND (kv.upos=dv.upos OR kv.upos=''))`, owner, preparationID).Scan(&count)
	return count, err
}

// StartDeckVocabularyStudy reserves one ready, non-empty deck for its owner.
// The partial unique index enforces the owner-wide lease atomically.
func (s *PostgresStore) StartDeckVocabularyStudy(ctx context.Context, owner, preparationID string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	current, err := scanDeckPreparation(tx.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, preparationID))
	if err != nil {
		return current, err
	}
	if current.State != domain.DeckPreparationReady || current.TotalCards == 0 || current.GraduatedAt != nil || current.ReviewedAt != nil {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	if current.StudyingAt != nil {
		return current, tx.Commit(ctx)
	}
	var snapshotCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM deck_preparation_vocabulary WHERE owner_id=$1 AND deck_preparation_id=$2`, owner, preparationID).Scan(&snapshotCount); err != nil {
		return domain.DeckPreparation{}, err
	}
	if snapshotCount == 0 {
		// Ready rows created before the snapshot write can be repaired from their
		// immutable generated-vocabulary provenance on first study.
		if _, err = tx.Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at)
			SELECT owner_id,$2,language,canonical_lemma,upos,first_generated_at
			FROM generated_vocabulary WHERE owner_id=$1 AND first_source_material_id=$3
			ON CONFLICT DO NOTHING`, owner, preparationID, current.SourceMaterialID); err != nil {
			return domain.DeckPreparation{}, err
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM deck_preparation_vocabulary WHERE owner_id=$1 AND deck_preparation_id=$2`, owner, preparationID).Scan(&snapshotCount); err != nil {
			return domain.DeckPreparation{}, err
		}
	}
	if snapshotCount == 0 {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	updated, err := scanDeckPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET studying_at=now(),released_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2 AND studying_at IS NULL RETURNING `+deckPreparationColumns, owner, preparationID))
	if err != nil {
		if isConstraint(err, "deck_preparations_one_studying_per_owner") {
			return domain.DeckPreparation{}, ErrActiveVocabularyStudy
		}
		return domain.DeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return updated, nil
}

// ConfirmDeckVocabularyReview graduates exactly the snapshot linked to this
// prepared deck and records the review in one transaction.
func (s *PostgresStore) ConfirmDeckVocabularyReview(ctx context.Context, owner, preparationID string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	current, err := scanDeckPreparation(tx.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, preparationID))
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if current.GraduatedAt != nil {
		return current, tx.Commit(ctx)
	}
	if current.StudyingAt == nil {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	if _, err = tx.Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state)
		SELECT dv.owner_id,dv.language,dv.canonical_lemma,dv.upos,'known'
		FROM deck_preparation_vocabulary dv
		WHERE dv.owner_id=$1 AND dv.deck_preparation_id=$2
		AND NOT EXISTS (SELECT 1 FROM known_vocabulary kv WHERE kv.owner_id=dv.owner_id AND kv.language=dv.language AND kv.canonical_lemma=dv.canonical_lemma AND (kv.upos=dv.upos OR kv.upos=''))
		ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET state='known',updated_at=now()`, owner, preparationID); err != nil {
		return domain.DeckPreparation{}, fmt.Errorf("graduate deck vocabulary state: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos)
		SELECT dv.owner_id,dv.language,dv.canonical_lemma,dv.upos
		FROM deck_preparation_vocabulary dv
		WHERE dv.owner_id=$1 AND dv.deck_preparation_id=$2
		AND NOT EXISTS (SELECT 1 FROM known_vocabulary kv WHERE kv.owner_id=dv.owner_id AND kv.language=dv.language AND kv.canonical_lemma=dv.canonical_lemma AND (kv.upos=dv.upos OR kv.upos=''))
		ON CONFLICT(owner_id,language,canonical_lemma,upos) DO NOTHING`, owner, preparationID); err != nil {
		return domain.DeckPreparation{}, fmt.Errorf("record graduated deck vocabulary: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE deck_preparation_vocabulary SET graduated_at=COALESCE(graduated_at,now()) WHERE owner_id=$1 AND deck_preparation_id=$2`, owner, preparationID); err != nil {
		return domain.DeckPreparation{}, err
	}
	updated, err := scanDeckPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET studying_at=NULL,reviewed_at=COALESCE(reviewed_at,now()),graduated_at=COALESCE(graduated_at,now()),released_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+deckPreparationColumns, owner, preparationID))
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return updated, nil
}

// ReleaseDeckVocabularyStudy releases an unfinished reservation without
// deleting its immutable snapshot or generated provenance.
func (s *PostgresStore) ReleaseDeckVocabularyStudy(ctx context.Context, owner, preparationID string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	current, err := scanDeckPreparation(tx.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, preparationID))
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if current.GraduatedAt != nil || current.ReviewedAt != nil {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	if current.StudyingAt == nil {
		return current, tx.Commit(ctx)
	}
	updated, err := scanDeckPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET studying_at=NULL,released_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND studying_at IS NOT NULL RETURNING `+deckPreparationColumns, owner, preparationID))
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return updated, nil
}

func isConstraint(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == name
}

// CreateDeckPreparation creates at most one legacy or analysis-bound identity
// for an owner and source. Repeated submissions return the same row.
func (s *PostgresStore) CreateDeckPreparation(ctx context.Context, p domain.DeckPreparation) (domain.DeckPreparation, error) {
	if p.AnalysisRunID != "" {
		return scanDeckPreparation(s.pool.QueryRow(ctx, `INSERT INTO deck_preparations(owner_id,source_material_id,analysis_run_id,filename,deck_name,content_hash) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (owner_id,source_material_id,analysis_run_id) WHERE analysis_run_id IS NOT NULL DO UPDATE SET owner_id=excluded.owner_id RETURNING `+deckPreparationColumns, p.OwnerID, p.SourceMaterialID, p.AnalysisRunID, p.Filename, p.DeckName, p.ContentHash))
	}
	return scanDeckPreparation(s.pool.QueryRow(ctx, `INSERT INTO deck_preparations(owner_id,source_material_id,filename,deck_name,content_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT (owner_id,source_material_id,content_hash) WHERE analysis_run_id IS NULL DO UPDATE SET owner_id=excluded.owner_id RETURNING `+deckPreparationColumns, p.OwnerID, p.SourceMaterialID, p.Filename, p.DeckName, p.ContentHash))
}

// ClaimDeckPreparation atomically grants one worker the queued preparation.
func (s *PostgresStore) ClaimDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	p, err := scanDeckPreparation(s.pool.QueryRow(ctx, `UPDATE deck_preparations SET state='preparing',started_at=now(),updated_at=now(),error='' WHERE owner_id=$1 AND id=$2 AND state='queued' RETURNING `+deckPreparationColumns, owner, id))
	if !errors.Is(err, ErrNotFound) {
		return p, err
	}
	if _, getErr := s.GetDeckPreparation(ctx, owner, id); getErr != nil {
		return p, getErr
	}
	return p, ErrInvalidTransition
}

// CompleteDeckPreparation stores the final artifact exactly once. Repeating
// the identical completion is idempotent; any different ready value is rejected.
func (s *PostgresStore) CompleteDeckPreparation(ctx context.Context, owner, id string, ready domain.DeckPreparation) (domain.DeckPreparation, error) {
	p, err := scanDeckPreparation(s.pool.QueryRow(ctx, `UPDATE deck_preparations SET state='ready',artifact=$3,filename=$4,deck_name=$5,total_cards=$6,cards_with_english=$7,cards_with_contextual_sentence_translations=$8,quality_omissions=$9,error='',completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND state='preparing' RETURNING `+deckPreparationColumns, owner, id, ready.Artifact, ready.Filename, ready.DeckName, ready.TotalCards, ready.CardsWithEnglish, ready.CardsWithContextualSentenceTranslations, ready.QualityOmissions))
	if !errors.Is(err, ErrNotFound) {
		return p, err
	}
	existing, getErr := s.GetDeckPreparation(ctx, owner, id)
	if getErr != nil {
		return p, getErr
	}
	if existing.State != domain.DeckPreparationReady {
		return p, ErrInvalidTransition
	}
	if bytes.Equal(existing.Artifact, ready.Artifact) && existing.Filename == ready.Filename && existing.DeckName == ready.DeckName && existing.TotalCards == ready.TotalCards && existing.CardsWithEnglish == ready.CardsWithEnglish && existing.CardsWithContextualSentenceTranslations == ready.CardsWithContextualSentenceTranslations && existing.QualityOmissions == ready.QualityOmissions {
		return existing, nil
	}
	return p, ErrImmutable
}

func (s *PostgresStore) FailDeckPreparation(ctx context.Context, owner, id, message string) (domain.DeckPreparation, error) {
	return s.transitionDeckPreparation(ctx, owner, id, domain.DeckPreparationFailed, message, "preparing")
}

// FailPreparedDeckFinalization fences a completeness failure after a run has
// advanced to finalizing. No artifact is written, and the public preparation
// receives only a bounded message.
func (s *PostgresStore) FailPreparedDeckFinalization(ctx context.Context, owner, preparationID, runID, claimToken, errorClass, errorCode string) error {
	if err := validateBoundedError(errorClass, errorCode); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var runState domain.PreparedDeckRunState
	if err = tx.QueryRow(ctx, `UPDATE deck_preparation_runs SET state='failed',translation_state='failed',error_class=$5,error_code=$6,finalization_claim_token=NULL,finalization_claimed_at=NULL,finalization_lease_expires_at=NULL,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='finalizing' AND finalization_claim_token=$4 RETURNING state`, owner, preparationID, runID, claimToken, errorClass, errorCode).Scan(&runState); err != nil {
		return missing(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE deck_preparations SET state='failed',error='prepared-deck translation was incomplete',completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND current_run_id=$3 AND state='preparing'`, owner, preparationID, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) CancelDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.CancelCurrentPreparedDeckRun(ctx, owner, id)
}

func (s *PostgresStore) RetryDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.transitionDeckPreparation(ctx, owner, id, domain.DeckPreparationQueued, "", "failed", "cancelled")
}

func (s *PostgresStore) transitionDeckPreparation(ctx context.Context, owner, id string, next domain.DeckPreparationState, message string, from ...string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanDeckPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET state=$3,error=$4,current_run_id=CASE WHEN $3='queued' THEN NULL ELSE current_run_id END,started_at=CASE WHEN $3='queued' THEN NULL ELSE started_at END,completed_at=CASE WHEN $3 IN ('failed','cancelled') THEN now() ELSE NULL END,updated_at=now() WHERE owner_id=$1 AND id=$2 AND state=ANY($5) RETURNING `+deckPreparationColumns, owner, id, next, message, from))
	if errors.Is(err, ErrNotFound) {
		_ = tx.Rollback(ctx)
		if _, getErr := s.GetDeckPreparation(ctx, owner, id); getErr != nil {
			return p, getErr
		}
		return p, ErrInvalidTransition
	}
	if err != nil {
		return p, err
	}
	details, marshalErr := json.Marshal(map[string]any{"preparation_id": id, "from": from, "message": message})
	if marshalErr != nil {
		return p, marshalErr
	}
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'prepared_deck',$2,$3,now())`, owner, string(next), details); err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return p, nil
}

func (s *PostgresStore) GetDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return scanDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2`, owner, id))
}

// GetDeckPreparationForAnalysis returns the one owner-scoped preparation
// associated with an exact completed analysis. The unique preparation identity
// makes this lookup safe for result pages and prevents a mutable latest-deck
// lookup from selecting the wrong analysis.
func (s *PostgresStore) GetDeckPreparationForAnalysis(ctx context.Context, owner, sourceMaterialID, analysisRunID string) (domain.DeckPreparation, error) {
	return scanDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND source_material_id=$2 AND analysis_run_id=$3::uuid`, owner, sourceMaterialID, analysisRunID))
}

func (s *PostgresStore) GetActiveDeckVocabularyStudy(ctx context.Context, owner, sourceMaterialID string) (domain.DeckPreparation, error) {
	return scanDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND source_material_id=$2 AND studying_at IS NOT NULL AND graduated_at IS NULL ORDER BY studying_at DESC LIMIT 1`, owner, sourceMaterialID))
}

// ListDeckPreparationsForSourceMaterial returns the owner's preparation history
// for one Book's acquired source, including released and graduated studies.
func (s *PostgresStore) ListDeckPreparationsForSourceMaterial(ctx context.Context, owner, sourceMaterialID string) ([]domain.DeckPreparation, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND source_material_id=$2 ORDER BY COALESCE(completed_at,created_at) DESC,id`, owner, sourceMaterialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var preparations []domain.DeckPreparation
	for rows.Next() {
		preparation, scanErr := scanDeckPreparation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		preparations = append(preparations, preparation)
	}
	return preparations, rows.Err()
}

// DownloadDeckPreparation returns bytes only for a ready owner-scoped row.
func (s *PostgresStore) DownloadDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	p, err := scanDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 AND state='ready'`, owner, id))
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if checkErr := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deck_preparations WHERE owner_id=$1 AND id=$2)`, owner, id).Scan(&exists); checkErr != nil && !errors.Is(checkErr, pgx.ErrNoRows) {
			return p, checkErr
		}
		if exists {
			return p, ErrInvalidTransition
		}
	}
	return p, err
}
