//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

func openIntegrationStore(t *testing.T, ctx context.Context, databaseURL string) *PostgresStore {
	t.Helper()
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "persistence store", store.Close)
	return store
}

func rollbackIntegrationTx(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Errorf("rollback transaction: %v", err)
	}
}
