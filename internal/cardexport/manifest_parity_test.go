package cardexport

import (
	"bytes"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestSnapshotRetainsHistoricalDigestAndRenderParity(t *testing.T) {
	entries := []Entry{
		{OwnerID: "owner-1", Language: "de", CanonicalLemma: "haus", UPOS: "noun", CorpusID: "corpus-1", SentenceOrdinal: 7, Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", Plural: "Häuser", IPA: "/haʊ̯s/", PrincipalParts: "geht · ging · gegangen", Translation: "stale", SentenceTranslation: "stale sentence", SentenceTranslationTarget: "stale target", Morphology: `{"Gender":"Neut"}`, SourceDocument: "Buch", Notes: "note", FirstEncounter: 10},
		{OwnerID: "owner-1", Language: "de", CanonicalLemma: "fragment", UPOS: "noun", Sentence: "Fragment.", TargetWord: "Fragment", SourceDocument: "Buch", FirstEncounter: 20},
	}
	manifest := newManifest("owner-1", "Buch", entries)
	candidate := manifest.enrichmentCandidatesProjection()[0]
	key := enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "prompt-v3", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	bound, err := manifest.bindCacheKeys([]enrichment.CacheKey{key})
	require.NoError(t, err)
	snapshot := bound.Snapshot()
	digest, err := snapshot.Digest()
	require.NoError(t, err)
	assert.Equal(t, "da05953d0f86aeed605e6d86bf7c2a5e9c1992543c8d22f4fe0f5d0393452a9b", digest)

	historicalV10 := snapshot
	historicalV10.SchemaVersion = ManifestSchemaVersionV10
	v10Digest, err := historicalV10.Digest()
	require.NoError(t, err)
	assert.Equal(t, "6740c332e8166497cb2edb3965ad7b6d668d6ad567b1088cd892226133f90a9a", v10Digest, "the v10 digest stays stable")

	historicalV9 := snapshot
	historicalV9.SchemaVersion = ManifestSchemaVersionV9
	v9Digest, err := historicalV9.Digest()
	require.NoError(t, err)
	assert.Equal(t, "44c23bc1f640e6389c0cf995190a1bb46f09b22acc6f6c2a34d3c6a949153d80", v9Digest, "the v9 digest stays stable")
	assert.NotEqual(t, v9Digest, digest, "later schemas add frozen render inputs")

	rebuilt, err := manifestFromSnapshot(snapshot)
	require.NoError(t, err)
	provenance := enrichment.Provenance{Provider: key.Provider, ProviderVersion: key.ProviderVersion}
	exact := []ExactEnrichment{{CacheKey: key, Result: enrichment.Result{Candidate: candidate, Translation: enrichment.Field[string]{Value: "house", Available: true, Provenance: provenance}, SentenceTranslation: enrichment.Field[string]{Value: "The old house is surprisingly large.", Available: true, Provenance: provenance}}}}
	want, _, err := renderManifest(t.Context(), bound, exact)
	require.NoError(t, err)
	got, _, err := renderManifest(t.Context(), rebuilt, exact)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(want.APKG, got.APKG))
	assert.Equal(t, want.TSV, got.TSV)
	assert.Equal(t, want.Completeness, got.Completeness)
}
