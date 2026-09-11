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

func TestCanonicalizeBookLanguagesMigrationIsIdempotentAndCollisionSafe(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	var ownerID string
	err := pool.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('language-migration-owner','hash') RETURNING id`).Scan(&ownerID)
	require.NoError(t, err)
	for _, tag := range []string{"de-DE", "de"} {
		_, err = pool.Exec(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,$2,'manual_entry','chosen',$3)`, ownerID, "Book "+tag, tag)
		require.NoError(t, err)
	}
	for _, language := range []string{"de-DE", "de"} {
		_, err = pool.Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,$2,'Haus','NOUN')`, ownerID, language)
		require.NoError(t, err)
	}
	for _, language := range []string{"de-DE", "de"} {
		_, err = pool.Exec(ctx, `INSERT INTO supported_languages(language,display_name) VALUES($1,$2)`, language, language)
		require.NoError(t, err)
	}

	sql, err := migrations.FS.ReadFile("000049_canonicalize_book_languages.up.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err, "execute up migration")
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err, "execute up migration twice")

	var bookRegions, knownRows, supportedRows int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE owner_id=$1 AND language_tag <> 'de'`, ownerID).Scan(&bookRegions)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='Haus' AND upos='NOUN'`, ownerID).Scan(&knownRows)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM supported_languages WHERE language='de'`).Scan(&supportedRows)
	require.NoError(t, err)
	assert.Zero(t, bookRegions, "migration convergence")
	assert.Equal(t, 1, knownRows, "migration convergence")
	assert.Equal(t, 1, supportedRows, "migration convergence")

	down, err := migrations.FS.ReadFile("000049_canonicalize_book_languages.down.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(down))
	require.NoError(t, err, "execute down migration")
	var remaining int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE owner_id=$1 AND language_tag='de'`, ownerID).Scan(&remaining)
	require.NoError(t, err)
	assert.Equal(t, 2, remaining, "no-op down changed canonical data")
}
