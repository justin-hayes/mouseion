//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	customPreparationTable         = "custom_vocabulary_deck_preparations"
	customPreparationIdentityTable = "custom_vocabulary_deck_preparation_identities"
)

// TestCustomDeckPreparationTablesDropReversesCleanly exercises migration 56: it
// drops the retired custom-deck preparation tables, and the development down
// migration recreates their post-000028 shape empty. The expected shape is
// rebuilt by walking the migrations themselves rather than duplicating it here.
func TestCustomDeckPreparationTablesDropReversesCleanly(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)

	t.Run("fresh databases have no preparation tables", func(t *testing.T) {
		assertPreparationTablesAbsent(t, ctx, pool)
	})

	// Rebuild the genuine post-000028 shape: below the outcomes migration, then
	// forward to 55, so no down-migration code recreates the tables.
	moveApplicationMigrationsTo(t, databaseURL, 28)
	moveApplicationMigrationsTo(t, databaseURL, 55)
	original := preparationSchemaShape(t, ctx, pool)

	t.Run("up drops both tables and their schema objects", func(t *testing.T) {
		moveApplicationMigrationsTo(t, databaseURL, 56)
		assertPreparationTablesAbsent(t, ctx, pool)
	})

	t.Run("down recreates the empty 000028 shape", func(t *testing.T) {
		moveApplicationMigrationsTo(t, databaseURL, 55)
		assert.Equal(t, original, preparationSchemaShape(t, ctx, pool), "the down migration recreates the post-000028 schema")
		for _, table := range []string{customPreparationTable, customPreparationIdentityTable} {
			var rows int
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&rows))
			assert.Zero(t, rows, "%s is recreated empty", table)
		}
	})

	t.Run("up after down succeeds again", func(t *testing.T) {
		moveApplicationMigrationsTo(t, databaseURL, 56)
		assertPreparationTablesAbsent(t, ctx, pool)
	})
}

// assertPreparationTablesAbsent checks that both preparation tables, with every
// index and constraint that belonged to them, are gone.
func assertPreparationTablesAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, table := range []string{customPreparationTable, customPreparationIdentityTable} {
		assertRelationAbsent(t, ctx, pool, table)
	}
}

// assertRelationAbsent checks that a table and every index and constraint that
// belonged to it are gone.
func assertRelationAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) {
	t.Helper()
	var present bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&present))
	assert.False(t, present, "%s still exists", table)
	var indexes, constraints int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND tablename = $1`, table).Scan(&indexes))
	assert.Zero(t, indexes, "%s still has indexes", table)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid = to_regclass('public.' || $1)`, table).Scan(&constraints))
	assert.Zero(t, constraints, "%s still has constraints", table)
}

// preparationSchemaShape renders the columns, constraints and indexes of both
// preparation tables as comparable lines.
func preparationSchemaShape(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string][]string {
	t.Helper()
	shape := make(map[string][]string, 2)
	for _, table := range []string{customPreparationTable, customPreparationIdentityTable} {
		rows, err := pool.Query(ctx, `
SELECT 'column:' || column_name || ':' || data_type || ':' || is_nullable || ':' || coalesce(column_default, '')
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1
UNION ALL
SELECT 'constraint:' || conname || ':' || pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conrelid = to_regclass('public.' || $1)
UNION ALL
SELECT 'index:' || indexname || ':' || indexdef
FROM pg_indexes
WHERE schemaname = 'public' AND tablename = $1
ORDER BY 1`, table)
		require.NoError(t, err)
		lines := make([]string, 0, 32)
		for rows.Next() {
			var line string
			require.NoError(t, rows.Scan(&line))
			lines = append(lines, line)
		}
		require.NoError(t, rows.Err())
		shape[table] = lines
	}
	return shape
}
