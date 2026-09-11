//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveRetiredAnalysisSchemaIsIdempotent(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	var ownerID, bookID, sourceID string
	err := pool.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('retired-schema-owner','hash') RETURNING id`).Scan(&ownerID)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,'Manual','manual_entry','chosen','de') RETURNING id`, ownerID).Scan(&bookID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO book_membership(owner_id,book_id,state,activated_at) VALUES($1,$2,'active',now())`, ownerID, bookID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,'strong_bibliographic','test','manual')`, ownerID, bookID)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text,book_id) VALUES($1,'de','plain','Plain','text/plain','retired-plain',decode('70','hex'),'plain',$2) RETURNING id`, ownerID, bookID).Scan(&sourceID)
	require.NoError(t, err)
	// Migration 000051 predates the campaign-table removal in 000061. Recreate
	// only the historical table it cleans so its retry behavior remains covered.
	_, err = pool.Exec(ctx, `CREATE TABLE learning_campaigns (source_material_id uuid NOT NULL)`)
	require.NoError(t, err)
	defer func() { _, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS learning_campaigns`) }()

	sql, err := migrations.FS.ReadFile("000051_remove_retired_analysis_schema.up.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err, "execute up migration")
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err, "execute up migration twice")

	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE id=$1`, bookID).Scan(&count)
	require.NoError(t, err)
	assert.Zero(t, count, "manual book remains")
	err = pool.QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE book_id=$1`, bookID).Scan(&count)
	require.NoError(t, err)
	assert.Zero(t, count, "manual membership remains")
	err = pool.QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE book_id=$1`, bookID).Scan(&count)
	require.NoError(t, err)
	assert.Zero(t, count, "manual alias remains")
	err = pool.QueryRow(ctx, `SELECT count(*) FROM source_materials WHERE id=$1`, sourceID).Scan(&count)
	require.NoError(t, err)
	assert.Zero(t, count, "plain-text source remains")
	for _, table := range []string{"epub_reviewed_scopes", "epub_reviewed_scope_units", "corpus_selected_units"} {
		err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, table).Scan(&count)
		require.NoError(t, err)
		assert.Zero(t, count, "retired table remains: %s", table)
	}
}
