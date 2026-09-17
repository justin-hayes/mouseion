// Package txcleanup contains shared policies for transaction cleanup.
package txcleanup

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Rollback returns consequential rollback failures while treating the
// already-committed transaction result as expected deferred cleanup.
func Rollback(ctx context.Context, tx pgx.Tx) error {
	err := tx.Rollback(ctx)
	if err == nil || errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return fmt.Errorf("rollback transaction: %w", err)
}
