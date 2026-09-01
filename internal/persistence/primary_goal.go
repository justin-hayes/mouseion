package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func scanPrimaryGoal(row pgx.Row) (goal domain.PrimaryGoal, err error) {
	err = row.Scan(&goal.OwnerID, &goal.BookID, &goal.CreatedAt, &goal.UpdatedAt)
	return goal, missing(err)
}

// GetPrimaryGoal returns the owner's current Goal. No row is the ordinary
// absent Goal state.
func (s *PostgresStore) GetPrimaryGoal(ctx context.Context, owner string) (domain.PrimaryGoal, error) {
	goal, err := scanPrimaryGoal(s.pool.QueryRow(ctx, `SELECT owner_id::text,book_id::text,created_at,updated_at FROM primary_goals WHERE owner_id=$1`, owner))
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
	goal, err := scanPrimaryGoal(tx.QueryRow(ctx, `INSERT INTO primary_goals(owner_id,book_id) VALUES($1,$2) ON CONFLICT (owner_id) DO NOTHING RETURNING owner_id::text,book_id::text,created_at,updated_at`, owner, bookID))
	if errors.Is(err, ErrNotFound) {
		return domain.PrimaryGoal{}, ErrGoalExists
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
	err = tx.QueryRow(ctx, `SELECT book_id::text FROM primary_goals WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&currentBookID)
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
	goal, err := scanPrimaryGoal(tx.QueryRow(ctx, `UPDATE primary_goals SET book_id=$2,updated_at=now() WHERE owner_id=$1 RETURNING owner_id::text,book_id::text,created_at,updated_at`, owner, bookID))
	if err != nil {
		return domain.PrimaryGoal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrimaryGoal{}, err
	}
	return goal, nil
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
	err = tx.QueryRow(ctx, `SELECT book_id::text FROM primary_goals WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&currentBookID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if currentBookID != expectedBookID {
		return ErrGoalStale
	}
	if _, err = tx.Exec(ctx, `DELETE FROM primary_goals WHERE owner_id=$1`, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
