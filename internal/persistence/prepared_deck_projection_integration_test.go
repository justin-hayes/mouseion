//go:build integration

package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPreparedDeckStorageProjectionSupportsHistoricalSchemas(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()

	for schema := cardexport.LegacyManifestSchemaVersion; schema <= cardexport.ManifestSchemaVersion; schema++ {
		t.Run(fmt.Sprintf("v%d", schema), func(t *testing.T) {
			owner, preparationID, runID, snapshot := insertProjectionFixture(t, ctx, store, projectionSnapshot(t, "", schema), nil)
			loaded, digest, err := store.LoadPreparedDeckStorageProjection(ctx, owner, preparationID, runID)
			require.NoError(t, err)
			assert.Equal(t, snapshot, loaded)
			wantDigest, err := snapshot.Digest()
			require.NoError(t, err)
			assert.Equal(t, wantDigest, digest)
		})
	}
}

func TestLoadPreparedDeckStorageProjectionRejectsPersistedCorruption(t *testing.T) {
	tests := []struct {
		name      string
		snapshot  func(string) cardexport.ManifestSnapshot
		badSchema bool
		identity  bool
	}{
		{name: "invalid ordinal", snapshot: func(owner string) cardexport.ManifestSnapshot {
			return projectionSnapshotWithItems(owner, cardexport.ManifestSchemaVersion, cardexport.ManifestItem{Ordinal: 1, Disposition: cardexport.ManifestAccepted, Entry: projectionEntry(), Quality: projectionQuality(), CacheKey: projectionCacheKey()})
		}},
		{name: "partial cache identity", snapshot: func(owner string) cardexport.ManifestSnapshot {
			entry := projectionEntry()
			entry.CanonicalLemma, entry.TargetWord, entry.Sentence = "baum", "Baum", "Der alte Baum ist gross."
			return projectionSnapshotWithItems(owner, cardexport.ManifestSchemaVersion,
				cardexport.ManifestItem{Disposition: cardexport.ManifestAccepted, Entry: projectionEntry(), Quality: projectionQuality(), CacheKey: projectionCacheKey()},
				cardexport.ManifestItem{Ordinal: 1, Disposition: cardexport.ManifestAccepted, Entry: entry, Quality: projectionQuality()},
			)
		}},
		{name: "manifest digest mismatch", identity: true, snapshot: func(owner string) cardexport.ManifestSnapshot {
			return projectionSnapshot(t, owner, cardexport.ManifestSchemaVersion)
		}},
		{name: "unsupported schema", badSchema: true, snapshot: func(owner string) cardexport.ManifestSnapshot {
			return projectionSnapshot(t, owner, 99)
		}},
	}

	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := test.snapshot("")
			owner, preparationID, runID, _ := insertProjectionFixture(t, ctx, store, snapshot, func(snapshot cardexport.ManifestSnapshot) (string, []string) {
				if test.badSchema || test.name == "invalid ordinal" {
					return strings.Repeat("a", 64), repeatedDigests(len(snapshot.Items))
				}
				if test.name == "partial cache identity" {
					candidateDigests := make([]string, len(snapshot.Items))
					for i, item := range snapshot.Items {
						candidateDigests[i], err = cardexport.CandidateDigestVersion(item, snapshot.SchemaVersion)
						require.NoError(t, err)
					}
					return strings.Repeat("a", 64), candidateDigests
				}
				_, candidateDigests, err := snapshot.Digests()
				require.NoError(t, err)
				return strings.Repeat("a", 64), candidateDigests
			})
			_, _, err := store.LoadPreparedDeckStorageProjection(ctx, owner, preparationID, runID)
			if test.identity {
				assert.ErrorIs(t, err, ErrPreparedDeckIdentity)
			} else {
				assert.ErrorIs(t, err, cardexport.ErrInvalidInput)
			}
		})
	}
}

func TestPostgresRejectsDuplicatePreparedDeckCandidateDigest(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()

	owner, preparationID, runID, snapshot := insertProjectionFixture(t, ctx, store, projectionSnapshot(t, "", cardexport.ManifestSchemaVersion), nil)
	_, candidateDigests, err := snapshot.Digests()
	require.NoError(t, err)
	item := snapshot.Items[0]
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_manifest_items(owner_id, preparation_id, run_id, ordinal, disposition, language, target_language, canonical_lemma, upos, source_sentence, tested_target, first_encounter, quality_score, quality_gdex_score, quality_reasons, render_payload, provider, provider_version, sentence_hash, candidate_digest) VALUES($1,$2,$3,1,$4,$5,'en',$6,$7,$8,$9,$10,$11,$12,$13,'{}',$14,$15,$16,$17)`, owner, preparationID, runID, item.Disposition, item.Entry.Language, item.Entry.CanonicalLemma, item.Entry.UPOS, item.Entry.Sentence, item.Entry.TargetWord, item.Entry.FirstEncounter, item.Quality.Score, item.Quality.GDEXScore, item.Quality.Reasons, item.CacheKey.Provider, item.CacheKey.ProviderVersion, item.CacheKey.SentenceHash, candidateDigests[0])
	assert.Error(t, err)
}

func insertProjectionFixture(t *testing.T, ctx context.Context, store *PostgresStore, snapshot cardexport.ManifestSnapshot, override func(cardexport.ManifestSnapshot) (string, []string)) (string, string, string, cardexport.ManifestSnapshot) {
	t.Helper()
	owner, err := store.CreateUser(ctx, "projection-"+uuid.NewString(), false)
	require.NoError(t, err)
	snapshot.Owner = owner.ID
	if snapshot.DeckName == "" {
		snapshot.DeckName = "German projection"
		snapshot.Filename = cardexport.DownloadFilename(snapshot.DeckName)
	}
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: snapshot.DeckName, MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("Das Haus steht."), FullText: "Das Haus steht."}
	source, err = store.PutSourceMaterial(ctx, source)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: snapshot.Filename, DeckName: snapshot.DeckName, ContentHash: source.ContentHash})
	require.NoError(t, err)
	manifestDigest, candidateDigests, err := snapshot.Digests()
	if override != nil {
		manifestDigest, candidateDigests = override(snapshot)
	}
	if candidateDigests == nil {
		candidateDigests = make([]string, len(snapshot.Items))
		for i := range snapshot.Items {
			candidateDigests[i] = strings.Repeat("a", 64)
		}
	}
	runID := uuid.NewString()
	_, accepted, _ := snapshot.Counts()
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_runs(id, owner_id, preparation_id, run_number, state, translation_state, external_translation_consent, external_translation_configured, manifest_schema_version, retry_policy_version, max_provider_attempts, max_batch_generations, batch_max_requests, batch_max_bytes, candidate_count, translation_completed_at) VALUES($1,$2,$3,1,'finalizing','completed',false,false,$4,1,1,1,1,1,$5,now())`, runID, owner.ID, preparation.ID, snapshot.SchemaVersion, accepted)
	require.NoError(t, err)
	selected, accepted, omitted := snapshot.Counts()
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_manifests(owner_id, preparation_id, run_id, schema_version, manifest_digest, deck_name, filename, selected_count, accepted_count, omitted_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, owner.ID, preparation.ID, runID, snapshot.SchemaVersion, manifestDigest, snapshot.DeckName, snapshot.Filename, selected, accepted, omitted)
	require.NoError(t, err)
	for i, item := range snapshot.Items {
		payload, err := json.Marshal(struct {
			Morphology                string                    `json:"morphology"`
			Gloss                     string                    `json:"gloss"`
			Plural                    string                    `json:"plural"`
			IPA                       string                    `json:"ipa"`
			PrincipalParts            string                    `json:"principal_parts"`
			DictionaryProviderVersion string                    `json:"dictionary_provider_version"`
			CandidateSenses           []enrichment.LexicalSense `json:"candidate_senses,omitempty"`
			SentenceTokens            []analyzer.Token          `json:"sentence_tokens,omitempty"`
			SourceDocument            string                    `json:"source_document"`
			Notes                     string                    `json:"notes"`
		}{item.Entry.Morphology, item.Entry.Gloss, item.Entry.Plural, item.Entry.IPA, item.Entry.PrincipalParts, item.Entry.DictionaryProviderVersion, item.Entry.CandidateSenses, item.Entry.SentenceTokens, item.Entry.SourceDocument, item.Entry.Notes})
		require.NoError(t, err)
		var provider, providerVersion, sentenceHash any
		if item.CacheKey != nil {
			provider, providerVersion, sentenceHash = item.CacheKey.Provider, item.CacheKey.ProviderVersion, item.CacheKey.SentenceHash
		}
		_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_manifest_items(owner_id, preparation_id, run_id, ordinal, disposition, language, target_language, canonical_lemma, upos, source_sentence, tested_target, first_encounter, quality_score, quality_gdex_score, quality_reasons, render_payload, provider, provider_version, sentence_hash, candidate_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, owner.ID, preparation.ID, runID, item.Ordinal, item.Disposition, item.Entry.Language, "en", item.Entry.CanonicalLemma, item.Entry.UPOS, item.Entry.Sentence, item.Entry.TargetWord, item.Entry.FirstEncounter, item.Quality.Score, item.Quality.GDEXScore, item.Quality.Reasons, payload, provider, providerVersion, sentenceHash, candidateDigests[i])
		require.NoError(t, err)
	}
	return owner.ID, preparation.ID, runID, snapshot
}

func projectionSnapshot(t *testing.T, owner string, schema int) cardexport.ManifestSnapshot {
	t.Helper()
	return projectionSnapshotWithItems(owner, schema, cardexport.ManifestItem{Ordinal: 0, Disposition: cardexport.ManifestAccepted, Entry: projectionEntry(), Quality: projectionQuality(), CacheKey: projectionCacheKey()})
}

func repeatedDigests(count int) []string {
	return []string{strings.Repeat("a", 64), strings.Repeat("a", 64)}[:count]
}

func projectionSnapshotWithItems(owner string, schema int, items ...cardexport.ManifestItem) cardexport.ManifestSnapshot {
	for i := range items {
		if items[i].Ordinal == 0 && i > 0 {
			items[i].Ordinal = i
		}
	}
	return cardexport.ManifestSnapshot{SchemaVersion: schema, Owner: owner, DeckName: "German projection", Filename: cardexport.DownloadFilename("German projection"), Items: items}
}

func projectionEntry() cardexport.Entry {
	return cardexport.Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist gross.", TargetWord: "Haus", Gloss: "house", Plural: "Hauser", IPA: "/haus/", PrincipalParts: "", DictionaryProviderVersion: "dict-v1", Morphology: `{"Gender":"Neut"}`, SourceDocument: "German projection"}
}

func projectionQuality() cardexport.SentenceQuality {
	return cardexport.SentenceQuality{Accepted: true, Score: 90, GDEXScore: 0.5, Reasons: []string{"target present"}}
}

func projectionCacheKey() *enrichment.CacheKey {
	return &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "openai", ProviderVersion: "provider-v1", DictionaryProviderVersion: "dict-v1", SentenceHash: enrichment.SentenceHash(projectionEntry().Sentence)}
}
