//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
)

func TestCanonicalizeBookLanguagesMigrationIsIdempotentAndCollisionSafe(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	var ownerID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('language-migration-owner','hash') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"de-DE", "de"} {
		if _, err := pool.Exec(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,$2,'manual_entry','chosen',$3)`, ownerID, "Book "+tag, tag); err != nil {
			t.Fatal(err)
		}
	}
	for _, language := range []string{"de-DE", "de"} {
		if _, err := pool.Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,$2,'Haus','NOUN')`, ownerID, language); err != nil {
			t.Fatal(err)
		}
	}
	for _, language := range []string{"de-DE", "de"} {
		if _, err := pool.Exec(ctx, `INSERT INTO supported_languages(language,display_name) VALUES($1,$2)`, language, language); err != nil {
			t.Fatal(err)
		}
	}

	sql, err := migrations.FS.ReadFile("000049_canonicalize_book_languages.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute up migration: %v", err)
	}
	if _, err = pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute up migration twice: %v", err)
	}

	var bookRegions, knownRows, supportedRows int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE owner_id=$1 AND language_tag <> 'de'`, ownerID).Scan(&bookRegions); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='Haus' AND upos='NOUN'`, ownerID).Scan(&knownRows); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM supported_languages WHERE language='de'`).Scan(&supportedRows); err != nil {
		t.Fatal(err)
	}
	if bookRegions != 0 || knownRows != 1 || supportedRows != 1 {
		t.Fatalf("migration convergence: bookRegions=%d knownRows=%d supportedRows=%d", bookRegions, knownRows, supportedRows)
	}

	down, err := migrations.FS.ReadFile("000049_canonicalize_book_languages.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("execute down migration: %v", err)
	}
	var remaining int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE owner_id=$1 AND language_tag='de'`, ownerID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("no-op down changed canonical data: %d books remain", remaining)
	}
}
