package cardexport

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verify that v1 and v2 snapshots still hash to their historical digests while
// the current v3 snapshot has a distinct identity for quality diagnostics.
func TestLegacyAndV2ManifestDigestStability(t *testing.T) {
	entries := []Entry{
		{OwnerID: "owner-1", Language: "de", CanonicalLemma: "haus", UPOS: "noun", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", Translation: "stale", SentenceTranslation: "stale sentence", SentenceTranslationTarget: "stale target", Morphology: `{"Gender":"Neut"}`, SourceDocument: "Buch", Notes: "note", FirstEncounter: 10},
		{OwnerID: "owner-1", Language: "de", CanonicalLemma: "fragment", UPOS: "noun", Sentence: "Fragment.", TargetWord: "Fragment", SourceDocument: "Buch", FirstEncounter: 20},
	}
	manifest := NewManifest("owner-1", "Buch", entries)
	candidate := manifest.EnrichmentCandidates()[0]
	key := enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "prompt-v3", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	bound, err := manifest.BindCacheKeys([]enrichment.CacheKey{key})
	require.NoError(t, err)
	snapshot := bound.Snapshot()

	// Legacy v1 persisted manifests were created with target language empty in
	// the canonical cache key (it was never part of v1 digest inputs). Rebuild
	// with an empty target to verify the historical digest is preserved.
	legacyKey := key
	legacyKey.TargetLanguage = ""
	legacyBound, err := manifest.BindCacheKeys([]enrichment.CacheKey{legacyKey})
	require.NoError(t, err)
	legacy := legacyBound.Snapshot()
	legacy.SchemaVersion = LegacyManifestSchemaVersion
	legacyDigest, err := legacy.Digest()
	require.NoError(t, err)
	const wantLegacyDigest = "1e8f4112b54cb863b2beba9e2a6be9715f3e9e468fecedee2165d8937cb2c50c"
	assert.Equal(t, wantLegacyDigest, legacyDigest, "legacy v1 digest=%q want=%q", legacyDigest, wantLegacyDigest)

	v2 := snapshot
	v2.SchemaVersion = PreviousManifestSchemaVersion
	digest, err := v2.Digest()
	require.NoError(t, err)
	const wantV2Digest = "bcb85683d3ac96c8bc645898d7e98b5bea39cdb058259c8310818bae5930bfa6"
	assert.Equal(t, wantV2Digest, digest, "v2 digest=%q want=%q", digest, wantV2Digest)
	assert.NotEqual(t, digest, legacyDigest, "v1 and v2 digests must differ")
	v3Digest, err := snapshot.Digest()
	require.NoError(t, err)
	assert.NotEqual(t, digest, v3Digest, "v2 and v3 digests must differ")
}

func TestQualityDiagnosticsOnlyAffectTheV3ManifestDigest(t *testing.T) {
	manifest := NewManifest("owner-1", "Buch", []Entry{{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus",
	}})
	snapshot := manifest.Snapshot()
	snapshot.Items[0].Quality.GDEXScore = 0.9

	v2 := snapshot
	v2.Items = cloneManifestItems(snapshot.Items)
	v2.SchemaVersion = PreviousManifestSchemaVersion
	v2Before, err := v2.Digest()
	require.NoError(t, err)
	v2.Items[0].Quality.GDEXScore = 0.1
	v2After, err := v2.Digest()
	require.NoError(t, err)
	assert.Equal(t, v2Before, v2After, "v2 digest must ignore the additive GDEX field")

	v3Before, err := snapshot.Digest()
	require.NoError(t, err)
	snapshot.Items[0].Quality.GDEXScore = 0.1
	v3After, err := snapshot.Digest()
	require.NoError(t, err)
	assert.NotEqual(t, v3Before, v3After, "v3 digest must include the GDEX field")
}
