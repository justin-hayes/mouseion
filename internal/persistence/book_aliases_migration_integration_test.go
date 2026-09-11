//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionScopedBookAliasesMigrationUpAndDown(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	var nullable bool
	err := pool.QueryRow(ctx, `
		SELECT is_nullable = 'YES'
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'book_aliases' AND column_name = 'connection_id'
	`).Scan(&nullable)
	require.NoError(t, err)
	assert.True(t, nullable, "connection_id is not nullable")

	var foreignKey bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class table_info ON table_info.oid = c.conrelid
			JOIN pg_class referenced_table ON referenced_table.oid = c.confrelid
			WHERE c.conname = 'book_aliases_connection_fkey'
			  AND table_info.relname = 'book_aliases'
			  AND referenced_table.relname = 'opds_connections'
		)
	`).Scan(&foreignKey)
	require.NoError(t, err)
	assert.True(t, foreignKey, "connection_id foreign key is missing")

	for _, index := range []struct {
		name      string
		columns   string
		predicate string
	}{
		{name: "book_aliases_catalogue_entry_identity", columns: "(owner_id, connection_id, namespace, value)", predicate: "connection_id IS NOT NULL"},
		{name: "book_aliases_strong_bibliographic_identity", columns: "(owner_id, namespace, value)", predicate: "connection_id IS NULL"},
	} {
		var definition string
		require.NoError(t, pool.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1`, index.name).Scan(&definition), "index %q", index.name)
		assert.True(t, strings.Contains(definition, index.columns) && strings.Contains(definition, index.predicate), "index %q definition=%q does not contain columns %q and partial predicate %q", index.name, definition, index.columns, index.predicate)
	}

	var legacyUnique bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class table_info ON table_info.oid = c.conrelid
			WHERE c.conname = 'book_aliases_owner_id_namespace_value_key'
			  AND table_info.relname = 'book_aliases'
		)
	`).Scan(&legacyUnique)
	require.NoError(t, err)
	assert.False(t, legacyUnique, "legacy book alias unique constraint was not removed")
	var connectionContract bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint c
			JOIN pg_class table_info ON table_info.oid = c.conrelid
			WHERE c.conname = 'book_aliases_connection_contract'
			  AND table_info.relname = 'book_aliases'
		)
	`).Scan(&connectionContract)
	require.NoError(t, err)
	assert.True(t, connectionContract, "book alias connection contract is missing")
	var invalidRows int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE (alias_type = 'catalog_entry') <> (connection_id IS NOT NULL)`).Scan(&invalidRows)
	require.NoError(t, err)
	assert.Zero(t, invalidRows, "found %d existing aliases invalid under connection contract", invalidRows)
	_, err = pool.Exec(ctx, `ALTER TABLE book_aliases DROP CONSTRAINT book_aliases_connection_contract`)
	require.NoError(t, err, "remove later contract for 000047 down")

	sql, err := migrations.FS.ReadFile("000047_connection_scoped_book_aliases.down.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err, "execute down migration")

	var downColumnExists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'book_aliases' AND column_name = 'connection_id')`).Scan(&downColumnExists)
	require.NoError(t, err)
	assert.False(t, downColumnExists, "down migration left connection_id")
	var downIndexCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname IN ('book_aliases_catalogue_entry_identity', 'book_aliases_strong_bibliographic_identity')`).Scan(&downIndexCount)
	require.NoError(t, err)
	assert.Zero(t, downIndexCount, "down migration left connection-scoped indexes")
	_, err = pool.Exec(ctx, `ALTER TABLE book_aliases ADD CONSTRAINT book_aliases_owner_id_namespace_value_key UNIQUE(owner_id, namespace, value)`)
	require.NoError(t, err, "restore legacy unique for 000048 up")

	var ownerID, bookID string
	err = pool.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('migration-alias-owner','hash') RETURNING id`).Scan(&ownerID)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state) VALUES($1,'Legacy alias','manual','unknown') RETURNING id`, ownerID).Scan(&bookID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,'strong_bibliographic','isbn','legacy-value')`, ownerID, bookID)
	require.NoError(t, err)

	sql, err = migrations.FS.ReadFile("000047_connection_scoped_book_aliases.up.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err)
	sql, err = migrations.FS.ReadFile("000048_book_alias_connection_contract.up.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'book_aliases' AND column_name = 'connection_id')`).Scan(&nullable)
	require.NoError(t, err)
	assert.True(t, nullable, "connection_id was not restored by up migration")
	var aliasBookID string
	err = pool.QueryRow(ctx, `SELECT book_id FROM book_aliases WHERE owner_id = $1 AND namespace = 'isbn' AND value = 'legacy-value'`, ownerID).Scan(&aliasBookID)
	require.NoError(t, err)
	assert.Equal(t, bookID, aliasBookID)
	_, err = pool.Exec(ctx, `ALTER TABLE book_aliases VALIDATE CONSTRAINT book_aliases_connection_contract`)
	require.NoError(t, err, "validate restored contract")
}
