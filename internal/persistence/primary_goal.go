package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func scanPrimaryGoal(row pgx.Row) (goal domain.PrimaryGoal, err error) {
	err = row.Scan(&goal.OwnerID, &goal.BookID, &goal.CreatedAt, &goal.UpdatedAt, &goal.ReadingFinishedAt)
	return goal, missing(err)
}

// PrimaryGoalFinishResult is the persisted result of accepting a reading
// finish. Graduated contains only identities that changed known vocabulary in
// this transition; reserved or generated identities are never reported here.
type PrimaryGoalFinishResult struct {
	Goal                    domain.PrimaryGoal
	Campaign                *domain.LearningCampaign
	Graduated               []domain.CampaignVocabulary
	ResidualVocabularyCount int
}

// GetPrimaryGoal returns the owner's current Goal. No row is the ordinary
// absent Goal state.
func (s *PostgresStore) GetPrimaryGoal(ctx context.Context, owner string) (domain.PrimaryGoal, error) {
	goal, err := scanPrimaryGoal(s.pool.QueryRow(ctx, `SELECT owner_id::text,book_id::text,created_at,updated_at,reading_finished_at FROM primary_goals WHERE owner_id=$1`, owner))
	if errors.Is(err, ErrNotFound) {
		return domain.PrimaryGoal{}, nil
	}
	return goal, err
}

// CreatePrimaryGoal creates the owner's only current Goal for an owner book.
// The book only needs to exist; analysis and deck preparation are optional.
func (s *PostgresStore) CreatePrimaryGoal(ctx context.Context, owner, bookID string) (domain.PrimaryGoal, error) {
	goal := domain.PrimaryGoal{OwnerID: owner, BookID: bookID}
	if err := goal.Validate(); err != nil {
		return domain.PrimaryGoal{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	defer tx.Rollback(ctx)
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	goal, err = insertPrimaryGoal(ctx, tx, owner, bookID)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	return goal, nil
}

func insertPrimaryGoal(ctx context.Context, tx pgx.Tx, owner, bookID string) (domain.PrimaryGoal, error) {
	goal, err := scanPrimaryGoal(tx.QueryRow(ctx, `INSERT INTO primary_goals(owner_id,book_id) VALUES($1,$2) ON CONFLICT (owner_id) DO NOTHING RETURNING owner_id::text,book_id::text,created_at,updated_at,reading_finished_at`, owner, bookID))
	if errors.Is(err, ErrNotFound) {
		goal, err = scanPrimaryGoal(tx.QueryRow(ctx, `UPDATE primary_goals SET book_id=$2,reading_finished_at=NULL,updated_at=now() WHERE owner_id=$1 AND reading_finished_at IS NOT NULL RETURNING owner_id::text,book_id::text,created_at,updated_at,reading_finished_at`, owner, bookID))
		if errors.Is(err, ErrNotFound) {
			return domain.PrimaryGoal{}, ErrGoalExists
		}
	}
	return goal, err
}

// ChangePrimaryGoal changes the owner's Goal only when expectedBookID still
// names the current Goal, protecting callers from overwriting stale state.
func (s *PostgresStore) ChangePrimaryGoal(ctx context.Context, owner, bookID, expectedBookID string) (domain.PrimaryGoal, error) {
	if err := (domain.PrimaryGoal{OwnerID: owner, BookID: bookID}).Validate(); err != nil {
		return domain.PrimaryGoal{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	defer tx.Rollback(ctx)
	var currentBookID string
	var readingFinishedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT book_id::text,reading_finished_at FROM primary_goals WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&currentBookID, &readingFinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PrimaryGoal{}, ErrNotFound
	}
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if currentBookID != expectedBookID {
		return domain.PrimaryGoal{}, ErrGoalStale
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return domain.PrimaryGoal{}, err
	}
	goal, err := scanPrimaryGoal(tx.QueryRow(ctx, `UPDATE primary_goals SET book_id=$2,reading_finished_at=NULL,updated_at=now() WHERE owner_id=$1 RETURNING owner_id::text,book_id::text,created_at,updated_at,reading_finished_at`, owner, bookID))
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	return goal, nil
}

// FinishReadingPrimaryGoal accepts the reading-finished fact for the current
// Goal. The Goal row and any matching active campaign are changed in one
// transaction, guarded by expectedBookID. A repeated request is idempotent.
func (s *PostgresStore) FinishReadingPrimaryGoal(ctx context.Context, owner, expectedBookID string) (PrimaryGoalFinishResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PrimaryGoalFinishResult{}, err
	}
	defer tx.Rollback(ctx)

	goal, err := scanPrimaryGoal(tx.QueryRow(ctx, `SELECT owner_id::text,book_id::text,created_at,updated_at,reading_finished_at FROM primary_goals WHERE owner_id=$1 FOR UPDATE`, owner))
	if err != nil {
		return PrimaryGoalFinishResult{}, err
	}
	if goal.BookID != expectedBookID {
		return PrimaryGoalFinishResult{}, ErrGoalStale
	}
	result := PrimaryGoalFinishResult{Goal: goal}
	if goal.ReadingFinishedAt == nil {
		result.Goal, err = scanPrimaryGoal(tx.QueryRow(ctx, `UPDATE primary_goals SET reading_finished_at=now(),updated_at=now() WHERE owner_id=$1 RETURNING owner_id::text,book_id::text,created_at,updated_at,reading_finished_at`, owner))
		if err != nil {
			return PrimaryGoalFinishResult{}, err
		}

		var campaign domain.LearningCampaign
		campaign, err = scanLearningCampaign(tx.QueryRow(ctx, `SELECT `+learningCampaignColumnsC+` FROM learning_campaigns c JOIN source_materials s ON s.owner_id=c.owner_id AND s.id=c.source_material_id WHERE c.owner_id=$1 AND s.book_id=$2 AND c.status='active' ORDER BY c.created_at DESC,c.id DESC LIMIT 1 FOR UPDATE`, owner, goal.BookID))
		if errors.Is(err, ErrNotFound) {
			err = nil
		} else if err != nil {
			return PrimaryGoalFinishResult{}, err
		} else {
			result.Campaign = &campaign
			if campaign.BookProgress != domain.BookFinished {
				if !campaign.BookProgress.CanTransitionTo(domain.BookFinished) {
					return PrimaryGoalFinishResult{}, ErrInvalidTransition
				}
				nextStatus := domain.DeriveCampaignStatus(domain.BookFinished, campaign.DeckProgress)
				updated, updateErr := scanLearningCampaign(tx.QueryRow(ctx, `UPDATE learning_campaigns SET book_status='finished',updated_at=now(),activated_at=COALESCE(activated_at,now()),book_finished_at=COALESCE(book_finished_at,now()),deck_reviewed_at=CASE WHEN deck_status='reviewed' THEN COALESCE(deck_reviewed_at,now()) ELSE deck_reviewed_at END,completed_at=CASE WHEN $3='complete' THEN COALESCE(completed_at,now()) ELSE completed_at END WHERE owner_id=$1 AND id=$2 RETURNING `+learningCampaignColumns, owner, campaign.ID, nextStatus))
				if updateErr != nil {
					return PrimaryGoalFinishResult{}, campaignConstraintError(updateErr)
				}
				campaign = updated
			}
			if campaign.Status == domain.CampaignComplete && campaign.VocabularyGraduatedAt == nil {
				result.Graduated, err = campaignVocabularyToGraduate(ctx, tx, owner, campaign.ID)
				if err != nil {
					return PrimaryGoalFinishResult{}, err
				}
				if err = graduateCampaignVocabulary(ctx, tx, owner, campaign.ID); err != nil {
					return PrimaryGoalFinishResult{}, err
				}
				campaign, err = scanLearningCampaign(tx.QueryRow(ctx, `UPDATE learning_campaigns SET vocabulary_graduated_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+learningCampaignColumns, owner, campaign.ID))
				if err != nil {
					return PrimaryGoalFinishResult{}, err
				}
			}
			result.Campaign = &campaign
			if campaign.Status == domain.CampaignActive {
				result.ResidualVocabularyCount, err = countCampaignVocabularyToGraduateTx(ctx, tx, owner, campaign.ID)
				if err != nil {
					return PrimaryGoalFinishResult{}, err
				}
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return PrimaryGoalFinishResult{}, err
	}
	return result, nil
}

func campaignVocabularyToGraduate(ctx context.Context, tx pgx.Tx, owner, campaignID string) ([]domain.CampaignVocabulary, error) {
	rows, err := tx.Query(ctx, `SELECT owner_id::text,campaign_id::text,language,canonical_lemma,upos,generated_at,graduated_at FROM learning_campaign_vocabulary cv WHERE cv.owner_id=$1 AND cv.campaign_id=$2 AND cv.graduated_at IS NULL AND NOT EXISTS (SELECT 1 FROM known_vocabulary kv WHERE kv.owner_id=cv.owner_id AND kv.language=cv.language AND kv.canonical_lemma=cv.canonical_lemma AND (kv.upos=cv.upos OR kv.upos='')) ORDER BY cv.canonical_lemma,cv.upos`, owner, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.CampaignVocabulary
	for rows.Next() {
		var item domain.CampaignVocabulary
		if err = rows.Scan(&item.OwnerID, &item.CampaignID, &item.Language, &item.CanonicalLemma, &item.UPOS, &item.GeneratedAt, &item.GraduatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func countCampaignVocabularyToGraduateTx(ctx context.Context, tx pgx.Tx, owner, campaignID string) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM learning_campaign_vocabulary cv WHERE cv.owner_id=$1 AND cv.campaign_id=$2 AND cv.graduated_at IS NULL AND NOT EXISTS (SELECT 1 FROM known_vocabulary kv WHERE kv.owner_id=cv.owner_id AND kv.language=cv.language AND kv.canonical_lemma=cv.canonical_lemma AND (kv.upos=cv.upos OR kv.upos=''))`, owner, campaignID).Scan(&count)
	return count, err
}

// ClearPrimaryGoal removes the owner's Goal only when expectedBookID still
// names the current Goal.
func (s *PostgresStore) ClearPrimaryGoal(ctx context.Context, owner, expectedBookID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var currentBookID string
	var readingFinishedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT book_id::text,reading_finished_at FROM primary_goals WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&currentBookID, &readingFinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if currentBookID != expectedBookID {
		return ErrGoalStale
	}
	if readingFinishedAt != nil {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, `DELETE FROM primary_goals WHERE owner_id=$1`, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
