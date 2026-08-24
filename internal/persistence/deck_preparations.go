package persistence

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const deckPreparationColumns = `id::text,owner_id::text,source_material_id::text,state,artifact,filename,deck_name,content_hash,total_cards,cards_with_english,cards_with_contextual_sentence_translations,quality_omissions,error,created_at,updated_at,started_at,completed_at`

type rowScanner interface {
	Scan(...any) error
}

func scanDeckPreparation(row rowScanner) (domain.DeckPreparation, error) {
	var p domain.DeckPreparation
	err := row.Scan(&p.ID, &p.OwnerID, &p.SourceMaterialID, &p.State, &p.Artifact, &p.Filename, &p.DeckName, &p.ContentHash, &p.TotalCards, &p.CardsWithEnglish, &p.CardsWithContextualSentenceTranslations, &p.QualityOmissions, &p.Error, &p.CreatedAt, &p.UpdatedAt, &p.StartedAt, &p.CompletedAt)
	return p, missing(err)
}

// CreateDeckPreparation creates at most one active identity for an owner,
// source, and source content hash. Repeated submissions return the same row.
func (s *PostgresStore) CreateDeckPreparation(ctx context.Context, p domain.DeckPreparation) (domain.DeckPreparation, error) {
	return scanDeckPreparation(s.pool.QueryRow(ctx, `INSERT INTO deck_preparations(owner_id,source_material_id,filename,deck_name,content_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,source_material_id,content_hash) DO UPDATE SET owner_id=excluded.owner_id RETURNING `+deckPreparationColumns, p.OwnerID, p.SourceMaterialID, p.Filename, p.DeckName, p.ContentHash))
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

func (s *PostgresStore) CancelDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.transitionDeckPreparation(ctx, owner, id, domain.DeckPreparationCancelled, "", "queued", "preparing")
}

func (s *PostgresStore) RetryDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.transitionDeckPreparation(ctx, owner, id, domain.DeckPreparationQueued, "", "failed", "cancelled")
}

func (s *PostgresStore) transitionDeckPreparation(ctx context.Context, owner, id string, next domain.DeckPreparationState, message string, from ...string) (domain.DeckPreparation, error) {
	p, err := scanDeckPreparation(s.pool.QueryRow(ctx, `UPDATE deck_preparations SET state=$3,error=$4,started_at=CASE WHEN $3='queued' THEN NULL ELSE started_at END,completed_at=CASE WHEN $3 IN ('failed','cancelled') THEN now() ELSE NULL END,updated_at=now() WHERE owner_id=$1 AND id=$2 AND state=ANY($5) RETURNING `+deckPreparationColumns, owner, id, next, message, from))
	if !errors.Is(err, ErrNotFound) {
		return p, err
	}
	if _, getErr := s.GetDeckPreparation(ctx, owner, id); getErr != nil {
		return p, getErr
	}
	return p, ErrInvalidTransition
}

func (s *PostgresStore) GetDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return scanDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2`, owner, id))
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
