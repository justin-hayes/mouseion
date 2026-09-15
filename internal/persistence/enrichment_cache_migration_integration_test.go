//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrichmentCacheBaselineRequiresTranslation(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	// Legacy gloss-only rows are the state retired by the baseline contract.
	// Set up that state directly instead of replaying a historical migration.
	_, err := pool.Exec(ctx, `
		INSERT INTO enrichment_cache(
			language, canonical_lemma, upos, provider, provider_version,
			sentence_hash, target_language, translation, fallback_gloss,
			sense_selection
		) VALUES ('de', 'legacy-cleanup', 'NOUN', 'migration-test', '1',
			'', 'en', '', 'legacy gloss', '[]')`)
	require.Error(t, err, "baseline rejects gloss-only cache rows")

	_, err = pool.Exec(ctx, `
		INSERT INTO enrichment_cache(
			language, canonical_lemma, upos, provider, provider_version,
			sentence_hash, target_language, translation, fallback_gloss,
			sense_selection
		) VALUES ('de', 'retained-translation', 'NOUN', 'migration-test', '1',
			'', 'en', 'retained', '', '[]')`)
	require.NoError(t, err)

	var retainedCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM enrichment_cache WHERE canonical_lemma='retained-translation'`).Scan(&retainedCount)
	require.NoError(t, err)
	assert.Equal(t, 1, retainedCount)
}
