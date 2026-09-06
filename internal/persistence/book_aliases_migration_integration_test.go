//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
)

func TestConnectionScopedBookAliasesMigrationUpAndDown(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	var nullable bool
	if err := pool.QueryRow(ctx, `
		SELECT is_nullable = 'YES'
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'book_aliases' AND column_name = 'connection_id'
	`).Scan(&nullable); err != nil {
		t.Fatalf("connection_id column: %v", err)
	}
	if !nullable {
		t.Fatal("connection_id is not nullable")
	}

	var foreignKey bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class table_info ON table_info.oid = c.conrelid
			JOIN pg_class referenced_table ON referenced_table.oid = c.confrelid
			WHERE c.conname = 'book_aliases_connection_fkey'
			  AND table_info.relname = 'book_aliases'
			  AND referenced_table.relname = 'opds_connections'
		)
	`).Scan(&foreignKey); err != nil {
		t.Fatalf("connection_id foreign key: %v", err)
	}
	if !foreignKey {
		t.Fatal("connection_id foreign key is missing")
	}

	for _, index := range []struct {
		name      string
		predicate string
	}{
		{name: "book_aliases_catalogue_entry_identity", predicate: "connection_id IS NOT NULL"},
		{name: "book_aliases_strong_bibliographic_identity", predicate: "connection_id IS NULL"},
	} {
		var definition string
		if err := pool.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1`, index.name).Scan(&definition); err != nil {
			t.Fatalf("index %q: %v", index.name, err)
		}
		if !strings.Contains(definition, index.predicate) {
			t.Fatalf("index %q definition=%q does not contain partial predicate %q", index.name, definition, index.predicate)
		}
	}

	var legacyUnique bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class table_info ON table_info.oid = c.conrelid
			WHERE c.conname = 'book_aliases_owner_id_namespace_value_key'
			  AND table_info.relname = 'book_aliases'
		)
	`).Scan(&legacyUnique); err != nil {
		t.Fatalf("legacy unique constraint: %v", err)
	}
	if !legacyUnique {
		t.Fatal("legacy book alias unique constraint was removed")
	}

	sql, err := migrations.FS.ReadFile("000047_connection_scoped_book_aliases.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute down migration: %v", err)
	}

	var downColumnExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'book_aliases' AND column_name = 'connection_id')`).Scan(&downColumnExists); err != nil {
		t.Fatalf("connection_id after down: %v", err)
	}
	if downColumnExists {
		t.Fatal("down migration left connection_id")
	}
	var downIndexCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname IN ('book_aliases_catalogue_entry_identity', 'book_aliases_strong_bibliographic_identity')`).Scan(&downIndexCount); err != nil {
		t.Fatalf("indexes after down: %v", err)
	}
	if downIndexCount != 0 {
		t.Fatalf("down migration left %d connection-scoped indexes", downIndexCount)
	}

	var ownerID, bookID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('migration-alias-owner','hash') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("create legacy alias owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state) VALUES($1,'Legacy alias','manual','unknown') RETURNING id`, ownerID).Scan(&bookID); err != nil {
		t.Fatalf("create legacy alias book: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,'strong_bibliographic','isbn','legacy-value')`, ownerID, bookID); err != nil {
		t.Fatalf("create legacy alias: %v", err)
	}

	sql, err = migrations.FS.ReadFile("000047_connection_scoped_book_aliases.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute up migration: %v", err)
	}

	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'book_aliases' AND column_name = 'connection_id')`).Scan(&nullable); err != nil {
		t.Fatalf("connection_id after up: %v", err)
	}
	if !nullable {
		t.Fatal("connection_id was not restored by up migration")
	}
	var aliasBookID string
	if err := pool.QueryRow(ctx, `SELECT book_id FROM book_aliases WHERE owner_id = $1 AND namespace = 'isbn' AND value = 'legacy-value'`, ownerID).Scan(&aliasBookID); err != nil {
		t.Fatalf("read preserved legacy alias: %v", err)
	}
	if aliasBookID != bookID {
		t.Fatalf("legacy alias book_id=%q, want %q", aliasBookID, bookID)
	}
}
