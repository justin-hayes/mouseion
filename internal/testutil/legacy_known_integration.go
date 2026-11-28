//go:build integration

package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// LegacyKnownLemmas lists the canonical lemmas of the owner's retained legacy
// Known vocabulary rows in one language, in lemma order. The store no longer
// reads Known vocabulary (ADR 0136); scenarios that pin "this changes no Known
// row" read the table directly until #2746 drops it.
func LegacyKnownLemmas(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID, language string) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT canonical_lemma FROM known_vocabulary WHERE owner_id = $1 AND language = $2 ORDER BY canonical_lemma, upos, lexical_discriminator, id`, ownerID, language)
	require.NoError(t, err)
	defer rows.Close()
	lemmas := []string{}
	for rows.Next() {
		var lemma string
		require.NoError(t, rows.Scan(&lemma))
		lemmas = append(lemmas, lemma)
	}
	require.NoError(t, rows.Err())
	return lemmas
}
