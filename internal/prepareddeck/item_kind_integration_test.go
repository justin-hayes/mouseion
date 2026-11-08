//go:build integration

package prepareddeck

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreparedDeckFreezesLemmaItemKind proves preparation writes the lemma
// kind to every frozen item and its contextual cache identity, and that the
// durable projection reads it back (ADR 0114).
func TestPreparedDeckFreezesLemmaItemKind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, _, _ := newStandardIntegrationRun(t, ctx, 1, 1)

	var kind string
	err := run.store.Pool().QueryRow(ctx,
		`SELECT item_kind FROM deck_preparation_manifest_items WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`,
		run.owner, run.prep.ID, run.run.ID,
	).Scan(&kind)
	require.NoError(t, err)
	assert.Equal(t, string(enrichment.ItemKindLemma), kind)

	snapshot, _, err := run.store.LoadPreparedDeckStorageProjection(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, snapshot.Items, 1)
	assert.Equal(t, enrichment.ItemKindLemma, snapshot.Items[0].ItemKind.Normalized())
	require.NotNil(t, snapshot.Items[0].CacheKey)
	assert.Equal(t, enrichment.ItemKindLemma, snapshot.Items[0].CacheKey.ItemKind.Normalized())
}
