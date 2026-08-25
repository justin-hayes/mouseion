package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const learningCampaignColumns = `id::text,owner_id::text,source_material_id::text,deck_preparation_id::text,book_status,deck_status,status,created_at,updated_at,activated_at,book_finished_at,deck_reviewed_at,completed_at,abandoned_at,vocabulary_graduated_at`

func scanLearningCampaign(row rowScanner) (domain.LearningCampaign, error) {
	var c domain.LearningCampaign
	err := row.Scan(&c.ID, &c.OwnerID, &c.SourceMaterialID, &c.DeckPreparationID, &c.BookProgress, &c.DeckProgress, &c.Status, &c.CreatedAt, &c.UpdatedAt, &c.ActivatedAt, &c.BookFinishedAt, &c.DeckReviewedAt, &c.CompletedAt, &c.AbandonedAt, &c.VocabularyGraduatedAt)
	return c, missing(err)
}

// CreateLearningCampaign ties a ready prepared deck to its source book and
// snapshots only generated vocabulary with matching, explicit provenance.
func (s *PostgresStore) CreateLearningCampaign(ctx context.Context, owner, sourceMaterialID, deckPreparationID string) (domain.LearningCampaign, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.LearningCampaign{}, err
	}
	defer tx.Rollback(ctx)

	var preparedSourceID string
	var preparedState domain.DeckPreparationState
	if err = tx.QueryRow(ctx, `SELECT source_material_id::text,state FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR SHARE`, owner, deckPreparationID).Scan(&preparedSourceID, &preparedState); err != nil {
		return domain.LearningCampaign{}, missing(err)
	}
	if preparedSourceID != sourceMaterialID || preparedState != domain.DeckPreparationReady {
		return domain.LearningCampaign{}, ErrInvalidTransition
	}
	campaign, err := scanLearningCampaign(tx.QueryRow(ctx, `INSERT INTO learning_campaigns(owner_id,source_material_id,deck_preparation_id) VALUES($1,$2,$3) RETURNING `+learningCampaignColumns, owner, sourceMaterialID, deckPreparationID))
	if err != nil {
		return campaign, campaignConstraintError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO learning_campaign_vocabulary(owner_id,campaign_id,language,canonical_lemma,upos,generated_at)
		SELECT owner_id,$2,language,canonical_lemma,upos,first_generated_at
		FROM generated_vocabulary WHERE owner_id=$1 AND first_source_material_id=$3`, owner, campaign.ID, sourceMaterialID)
	if err != nil {
		return domain.LearningCampaign{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.LearningCampaign{}, campaignConstraintError(err)
	}
	return campaign, nil
}

func (s *PostgresStore) GetLearningCampaign(ctx context.Context, owner, id string) (domain.LearningCampaign, error) {
	return scanLearningCampaign(s.pool.QueryRow(ctx, `SELECT `+learningCampaignColumns+` FROM learning_campaigns WHERE owner_id=$1 AND id=$2`, owner, id))
}

func (s *PostgresStore) ListLearningCampaigns(ctx context.Context, owner string) ([]domain.LearningCampaign, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+learningCampaignColumns+` FROM learning_campaigns WHERE owner_id=$1 ORDER BY created_at,id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var campaigns []domain.LearningCampaign
	for rows.Next() {
		campaign, scanErr := scanLearningCampaign(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		campaigns = append(campaigns, campaign)
	}
	return campaigns, rows.Err()
}

func (s *PostgresStore) GetActiveLearningCampaign(ctx context.Context, owner string) (domain.LearningCampaign, error) {
	return scanLearningCampaign(s.pool.QueryRow(ctx, `SELECT `+learningCampaignColumns+` FROM learning_campaigns WHERE owner_id=$1 AND status='active'`, owner))
}

func (s *PostgresStore) ListLearningCampaignVocabulary(ctx context.Context, owner, campaignID string) ([]domain.CampaignVocabulary, error) {
	rows, err := s.pool.Query(ctx, `SELECT owner_id::text,campaign_id::text,language,canonical_lemma,upos,generated_at,graduated_at FROM learning_campaign_vocabulary WHERE owner_id=$1 AND campaign_id=$2 ORDER BY language,canonical_lemma,upos`, owner, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vocabulary []domain.CampaignVocabulary
	for rows.Next() {
		var item domain.CampaignVocabulary
		if err = rows.Scan(&item.OwnerID, &item.CampaignID, &item.Language, &item.CanonicalLemma, &item.UPOS, &item.GeneratedAt, &item.GraduatedAt); err != nil {
			return nil, err
		}
		vocabulary = append(vocabulary, item)
	}
	return vocabulary, rows.Err()
}

// UpdateLearningCampaignProgress records independent monotonic book/deck
// progress. Status and lifecycle timestamps are derived atomically; completing
// both facts graduates only the campaign's snapshotted vocabulary.
func (s *PostgresStore) UpdateLearningCampaignProgress(ctx context.Context, owner, id string, book domain.BookProgress, deck domain.DeckProgress) (domain.LearningCampaign, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.LearningCampaign{}, err
	}
	defer tx.Rollback(ctx)

	current, err := scanLearningCampaign(tx.QueryRow(ctx, `SELECT `+learningCampaignColumns+` FROM learning_campaigns WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if err != nil {
		return domain.LearningCampaign{}, err
	}
	if current.Status == domain.CampaignAbandoned && (current.BookProgress != book || current.DeckProgress != deck) {
		return domain.LearningCampaign{}, ErrInvalidTransition
	}
	if !current.BookProgress.CanTransitionTo(book) || !current.DeckProgress.CanTransitionTo(deck) {
		return domain.LearningCampaign{}, ErrInvalidTransition
	}
	nextStatus := domain.DeriveCampaignStatus(book, deck)
	updated, err := scanLearningCampaign(tx.QueryRow(ctx, `UPDATE learning_campaigns SET
		book_status=$3,deck_status=$4,updated_at=now(),
		activated_at=CASE WHEN $5 IN ('active','complete') THEN COALESCE(activated_at,now()) ELSE activated_at END,
		book_finished_at=CASE WHEN $3='finished' THEN COALESCE(book_finished_at,now()) ELSE book_finished_at END,
		deck_reviewed_at=CASE WHEN $4='reviewed' THEN COALESCE(deck_reviewed_at,now()) ELSE deck_reviewed_at END,
		completed_at=CASE WHEN $5='complete' THEN COALESCE(completed_at,now()) ELSE completed_at END,
		abandoned_at=CASE WHEN $5='abandoned' THEN COALESCE(abandoned_at,now()) ELSE abandoned_at END
		WHERE owner_id=$1 AND id=$2 RETURNING `+learningCampaignColumns, owner, id, book, deck, nextStatus))
	if err != nil {
		return domain.LearningCampaign{}, campaignConstraintError(err)
	}
	if updated.Status == domain.CampaignComplete && updated.VocabularyGraduatedAt == nil {
		if err = graduateCampaignVocabulary(ctx, tx, owner, id); err != nil {
			return domain.LearningCampaign{}, err
		}
		updated, err = scanLearningCampaign(tx.QueryRow(ctx, `UPDATE learning_campaigns SET vocabulary_graduated_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+learningCampaignColumns, owner, id))
		if err != nil {
			return domain.LearningCampaign{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.LearningCampaign{}, campaignConstraintError(err)
	}
	return updated, nil
}

func graduateCampaignVocabulary(ctx context.Context, tx pgx.Tx, owner, campaignID string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos)
		SELECT owner_id,language,canonical_lemma,upos FROM learning_campaign_vocabulary
		WHERE owner_id=$1 AND campaign_id=$2 ON CONFLICT(owner_id,language,canonical_lemma,upos) DO NOTHING`, owner, campaignID); err != nil {
		return fmt.Errorf("graduate campaign vocabulary: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state)
		SELECT owner_id,language,canonical_lemma,upos,'known' FROM learning_campaign_vocabulary
		WHERE owner_id=$1 AND campaign_id=$2
		ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET state='known',updated_at=now()`, owner, campaignID); err != nil {
		return fmt.Errorf("record graduated vocabulary state: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE learning_campaign_vocabulary SET graduated_at=COALESCE(graduated_at,now()) WHERE owner_id=$1 AND campaign_id=$2`, owner, campaignID); err != nil {
		return fmt.Errorf("timestamp graduated vocabulary: %w", err)
	}
	return nil
}

func campaignConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "learning_campaigns_one_active_per_owner" {
		return ErrActiveCampaign
	}
	return err
}
