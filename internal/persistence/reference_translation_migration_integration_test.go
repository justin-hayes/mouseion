//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferenceTranslationMigrationCopiesAndDeduplicates replays migration two
// against owner-scoped rows. Identical pin-and-citation rows collapse to the
// newest copy, different pins coexist, and the frozen deck columns are not
// touched.
func TestReferenceTranslationMigrationCopiesAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	// Version 49 is the last schema before the installation-wide re-key.
	moveApplicationMigrationsTo(t, databaseURL, 49)
	owner, err := store.CreateUser(ctx, "translation-migration-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "translation-migration-other", false)
	require.NoError(t, err)
	third, err := store.CreateUser(ctx, "translation-migration-third", false)
	require.NoError(t, err)

	pinA := strings.Repeat("a", 64)
	pinB := strings.Repeat("b", 64)
	rows := []struct {
		owner       string
		citation    string
		translation string
		sourcePin   string
		age         string
	}{
		{owner.ID, "1.2.17", "older copy", "sha256:" + pinA, "2 hours"},
		{other.ID, "1.2.17", "newest copy", "sha256:" + pinA, "1 hour"},
		{owner.ID, "1.2.18", "shared section", "sha256:" + pinA, "1 hour"},
		{third.ID, "1.2.17", "other pin", "sha256:" + pinB, "1 hour"},
	}
	for _, row := range rows {
		_, err = pool.Exec(ctx, `
			INSERT INTO reference_translations(owner_id, language, citation, translation, source_pin, updated_at)
			VALUES ($1, 'grc', $2, $3, $4, now() - $5::interval)`,
			row.owner, row.citation, row.translation, row.sourcePin, row.age)
		require.NoError(t, err)
	}

	// A deck prepared before the migration froze its passage and pin. The
	// migration must not touch those columns.
	frozenPin := "sha256:" + pinA
	var sourceID, preparationID, runID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO source_materials(owner_id, language, source_identifier, title, media_type, content_hash, content, full_text)
		VALUES ($1, 'grc', 'frozen-unit', 'Frozen', 'text/plain', 'frozen-hash', ''::bytea, 'λόγον ὁρᾷ')
		RETURNING id::text`, owner.ID).Scan(&sourceID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO deck_preparations(owner_id, source_material_id, state, filename, deck_name, content_hash)
		VALUES ($1, $2, 'queued', 'frozen.apkg', 'Frozen', 'frozen-hash')
		RETURNING id::text`, owner.ID, sourceID).Scan(&preparationID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO deck_preparation_runs(
			owner_id, preparation_id, run_number, state, translation_state,
			external_translation_consent, external_translation_configured,
			manifest_schema_version, retry_policy_version, max_provider_attempts,
			max_batch_generations, batch_max_requests, batch_max_bytes, candidate_count,
			execution_mode, target_language)
		VALUES ($1, $2, 1, 'translating', 'pending', false, false,
			1, 1, 1, 1, 1, 1, 0, 'standard', 'en')
		RETURNING id::text`, owner.ID, preparationID).Scan(&runID))
	_, err = pool.Exec(ctx, `
		INSERT INTO deck_preparation_manifests(
			owner_id, preparation_id, run_id, schema_version, manifest_digest,
			deck_name, filename, selected_count, accepted_count, omitted_count)
		VALUES ($1, $2, $3, 1, repeat('d', 64), 'Frozen', 'frozen.apkg', 1, 1, 0)`,
		owner.ID, preparationID, runID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO deck_preparation_manifest_items(
			owner_id, preparation_id, run_id, ordinal, disposition, language,
			canonical_lemma, upos, source_sentence, tested_target, first_encounter,
			quality_score, render_payload, candidate_digest,
			passage_context, passage_citation, brownson_pin)
		VALUES ($1, $2, $3, 0, 'accepted', 'grc',
			'λόγος', 'NOUN', 'λόγον ὁρᾷ', 'λόγος', 0,
			50, '{}'::jsonb, repeat('c', 64),
			'frozen passage text', '1.2.17', $4)`, owner.ID, preparationID, runID, frozenPin)
	require.NoError(t, err)

	migrateApplicationMigrationsToLatest(t, databaseURL)

	type translatedRow struct {
		pin, citation, translation string
	}
	result := map[string]translatedRow{}
	readRows, err := pool.Query(ctx, `SELECT translation_pin_sha256, citation, translation FROM reference_translations`)
	require.NoError(t, err)
	defer readRows.Close()
	for readRows.Next() {
		var row translatedRow
		require.NoError(t, readRows.Scan(&row.pin, &row.citation, &row.translation))
		result[row.pin+"\x00"+row.citation] = row
	}
	require.NoError(t, readRows.Err())

	require.Len(t, result, 3, "identical pin-and-citation rows are deduplicated")
	assert.Equal(t, "newest copy", result[pinA+"\x001.2.17"].translation)
	assert.Equal(t, "shared section", result[pinA+"\x001.2.18"].translation)
	assert.Equal(t, "other pin", result[pinB+"\x001.2.17"].translation, "a different pin for the same citation coexists")

	var ownerColumn bool
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name='reference_translations' AND column_name='owner_id'
		)`).Scan(&ownerColumn))
	assert.False(t, ownerColumn, "the installation-wide table has no owner")

	var frozenColumns int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='deck_preparation_manifest_items'
		  AND column_name IN ('passage_context', 'passage_citation', 'brownson_pin')`).Scan(&frozenColumns))
	assert.Equal(t, 3, frozenColumns, "deck-frozen passage fields are untouched")

	var passage, citation, frozenDeckPin string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT passage_context, passage_citation, brownson_pin
		FROM deck_preparation_manifest_items
		WHERE owner_id=$1 AND ordinal=0`, owner.ID).Scan(&passage, &citation, &frozenDeckPin))
	assert.Equal(t, "frozen passage text", passage)
	assert.Equal(t, "1.2.17", citation)
	assert.Equal(t, frozenPin, frozenDeckPin)
}
