//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestListConfirmedIdiomCardsForBookReadsOnlyTheCurrentReadyDeck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	owner, err := store.CreateUser(ctx, "idiom-sample-"+uuid.NewString(), false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner.ID, Title: "Libro", MetadataProvenance: domain.MetadataProvenanceAcquisition,
		LanguageState: domain.LanguageChosen, LanguageTag: "it",
	})
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "it", SourceIdentifier: uuid.NewString(), Title: "Libro",
		MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("testo"), FullText: "testo",
	})
	require.NoError(t, err)
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, BookID: book.ID,
		Filename: cardexport.DownloadFilename("Libro"), DeckName: "Libro", ContentHash: source.ContentHash,
	})
	require.NoError(t, err)
	runID := uuid.NewString()
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO deck_preparation_runs(
			id, owner_id, preparation_id, run_number, state, translation_state,
			external_translation_consent, external_translation_configured,
			manifest_schema_version, retry_policy_version, max_provider_attempts,
			max_batch_generations, batch_max_requests, batch_max_bytes,
			candidate_count, completed_count, failed_count, translation_completed_at, completed_at)
		VALUES($1, $2, $3, 1, 'completed', 'completed', false, false, 15, 1, 1, 1, 1, 1, 1, 1, 0, now(), now())`,
		runID, owner.ID, prep.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET state = 'ready', current_run_id = $3, artifact = $4, completed_at = now(), book_id = $5 WHERE owner_id = $1 AND id = $2`, owner.ID, prep.ID, runID, []byte{1}, book.ID)
	require.NoError(t, err)

	_, err = store.Pool().Exec(ctx, `
		INSERT INTO deck_preparation_manifests(
			owner_id, preparation_id, run_id, schema_version, manifest_digest,
			deck_name, filename, selected_count, accepted_count, omitted_count)
		VALUES($1, $2, $3, 15, $4, 'Libro', 'Libro', 1, 1, 0)`,
		owner.ID, prep.ID, runID, strings.Repeat("b", 64))
	require.NoError(t, err)

	payload := `{
		"sentence_tokens": [
			{"Surface": "È"}, {"Surface": "un"}, {"Surface": "ragazzo"}, {"Surface": "in"}, {"Surface": "gamba."}
		],
		"idiom_matched_token_ordinals": [3, 4],
		"idiom_anchor_canonical_lemma": "gamba",
		"idiom_anchor_upos": "NOUN",
		"idiom_matcher_version": "idiom-matcher/1",
		"idiom_gap_bound": 2,
		"dictionary_provider_version": "fixture-v1"
	}`
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO deck_preparation_manifest_items(
			owner_id, preparation_id, run_id, ordinal, disposition, language, target_language,
			canonical_lemma, upos, item_kind, headword, source_sentence, tested_target,
			first_encounter, quality_score, quality_reasons, render_payload,
			provider, provider_version, sentence_hash, candidate_digest)
		VALUES($1, $2, $3, 0, 'accepted', 'it', 'en', 'in gamba', 'IDIOM', 'idiom', 'in gamba',
			'È un ragazzo in gamba.', 'gamba', 100, 100, '{}', $4::jsonb,
			'openai-compatible', 'gpt-review/idiom-card-v1', '', $5)`,
		owner.ID, prep.ID, runID, payload, strings.Repeat("a", 64))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO enrichment_cache(
			language, canonical_lemma, upos, item_kind, provider, provider_version,
			translation, sentence_hash, sentence_translation, target_language,
			dictionary_provider_version, fallback_gloss, meaning_evidence_hash)
		VALUES('it', 'in gamba', 'IDIOM', 'idiom', 'openai-compatible', 'gpt-review/idiom-card-v1',
			'capable', '', 'He is a capable young man.', 'en', 'fixture-v1', 'capable', '')`)
	require.NoError(t, err)

	cards, err := store.ListConfirmedIdiomCardsForBook(ctx, book.ID, "it")
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.Equal(t, ConfirmedIdiomCard{
		Language: "it", Headword: "in gamba", MatchedWords: []string{"in", "gamba."},
		Sentence: "È un ragazzo in gamba.", Gloss: "capable", Translation: "capable",
		AnchorLemma: "gamba", AnchorUPOS: "NOUN", MatcherVersion: "idiom-matcher/1", GapBound: 2,
	}, cards[0])

	t.Run("another language selects nothing", func(t *testing.T) {
		cards, err := store.ListConfirmedIdiomCardsForBook(ctx, book.ID, "de")
		require.NoError(t, err)
		require.Empty(t, cards)
	})
	t.Run("an unknown book selects nothing", func(t *testing.T) {
		cards, err := store.ListConfirmedIdiomCardsForBook(ctx, uuid.NewString(), "it")
		require.NoError(t, err)
		require.Empty(t, cards)
	})
}
