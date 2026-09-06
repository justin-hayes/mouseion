//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
)

func TestRemoveRetiredAnalysisSchemaIsIdempotent(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	var ownerID, bookID, sourceID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('retired-schema-owner','hash') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,'Manual','manual_entry','chosen','de') RETURNING id`, ownerID).Scan(&bookID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO book_membership(owner_id,book_id,state,activated_at) VALUES($1,$2,'active',now())`, ownerID, bookID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,'strong_bibliographic','test','manual')`, ownerID, bookID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text,book_id) VALUES($1,'de','plain','Plain','text/plain','retired-plain',decode('70','hex'),'plain',$2) RETURNING id`, ownerID, bookID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}

	sql, err := migrations.FS.ReadFile("000051_remove_retired_analysis_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute up migration: %v", err)
	}
	if _, err = pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute up migration twice: %v", err)
	}

	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE id=$1`, bookID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("manual book remains: %d", count)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE book_id=$1`, bookID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("manual membership remains: %d", count)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE book_id=$1`, bookID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("manual alias remains: %d", count)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM source_materials WHERE id=$1`, sourceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("plain-text source remains: %d", count)
	}
	for _, table := range []string{"epub_reviewed_scopes", "epub_reviewed_scope_units", "corpus_selected_units"} {
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("retired table remains: %s", table)
		}
	}
}
