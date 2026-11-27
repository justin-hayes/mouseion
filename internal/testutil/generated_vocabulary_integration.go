//go:build integration

package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// SeedGeneratedVocabulary inserts one legacy Generated vocabulary row, the
// history the retired Prepared deck recorded. Nothing writes it any more, so a
// direct insert is the only way to seed the shape that existing installations
// still hold (ADR 0136). firstSourceMaterialID may be nil.
func SeedGeneratedVocabulary(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID, language, lemma, upos, firstDeckID string, firstSourceMaterialID *string) {
	t.Helper()
	_, err := pool.Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,$2,$3,$4,$5,$6)`,
		ownerID, language, lemma, upos, firstDeckID, firstSourceMaterialID)
	require.NoError(t, err)
}
