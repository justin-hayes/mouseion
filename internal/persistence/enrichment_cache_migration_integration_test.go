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

func TestLegacyEnrichmentCacheCleanupIsIdempotent(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	contractDown, err := migrations.FS.ReadFile("000073_llm_fallback_gloss.down.sql")
	require.NoError(t, err)
	// 000073 is immutable shipped history. Rolling it back here models a
	// repaired database whose data still has the legacy gloss-only shape; the
	// normal ordered upgrade has already removed those rows in 000073.
	_, err = pool.Exec(ctx, string(contractDown))
	require.NoError(t, err, "restore the pre-000073 cache shape")
	contractRestored := false
	t.Cleanup(func() {
		if contractRestored {
			return
		}
		contractUp, readErr := migrations.FS.ReadFile("000073_llm_fallback_gloss.up.sql")
		if readErr == nil {
			_, _ = pool.Exec(context.Background(), string(contractUp))
		}
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO enrichment_cache(
			language, canonical_lemma, upos, provider, provider_version,
			sentence_hash, target_language, translation, gloss,
			sentence_translation, sentence_translation_target
		) VALUES ('de', 'legacy-cleanup', 'NOUN', 'migration-test', '1',
			'', 'en', '', 'legacy gloss', '', '')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO enrichment_cache(
			language, canonical_lemma, upos, provider, provider_version,
			sentence_hash, target_language, translation, gloss,
			sentence_translation, sentence_translation_target
		) VALUES ('de', 'retained-translation', 'NOUN', 'migration-test', '1',
			'', 'en', 'retained', '', '', '')`)
	require.NoError(t, err)

	cleanup, err := migrations.FS.ReadFile("000075_remove_legacy_enrichment_cache_rows.up.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(cleanup))
	require.NoError(t, err, "execute legacy cleanup")
	_, err = pool.Exec(ctx, string(cleanup))
	require.NoError(t, err, "retry legacy cleanup")

	var legacyCount, retainedCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM enrichment_cache WHERE canonical_lemma='legacy-cleanup'`).Scan(&legacyCount)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM enrichment_cache WHERE canonical_lemma='retained-translation'`).Scan(&retainedCount)
	require.NoError(t, err)
	assert.Zero(t, legacyCount)
	assert.Equal(t, 1, retainedCount)

	contractUp, err := migrations.FS.ReadFile("000073_llm_fallback_gloss.up.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(contractUp))
	require.NoError(t, err, "restore the final cache contract")
	contractRestored = true
}
