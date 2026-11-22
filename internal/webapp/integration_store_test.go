//go:build integration

package webapp

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// openWebappIntegrationStore returns a migrated PostgreSQL store with River's
// tables, shared by integration tests that compose the real handler.
func openWebappIntegrationStore(t *testing.T, ctx context.Context) *persistence.PostgresStore {
	t.Helper()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	return store
}
