package cardexport

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// Verify that a v1 (legacy) snapshot still hashes to its historical digest and
// that the v2 snapshot (with explicit target language in the digest) hashes to
// a distinct value, so existing durable English manifests keep their identity.
func TestLegacyAndV2ManifestDigestStability(t *testing.T) {
	entries := []Entry{
		{OwnerID: "owner-1", Language: "de", CanonicalLemma: "haus", UPOS: "noun", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", Translation: "stale", SentenceTranslation: "stale sentence", SentenceTranslationTarget: "stale target", Morphology: `{"Gender":"Neut"}`, SourceDocument: "Buch", Notes: "note", FirstEncounter: 10},
		{OwnerID: "owner-1", Language: "de", CanonicalLemma: "fragment", UPOS: "noun", Sentence: "Fragment.", TargetWord: "Fragment", SourceDocument: "Buch", FirstEncounter: 20},
	}
	manifest := NewManifest("owner-1", "Buch", entries)
	candidate := manifest.EnrichmentCandidates()[0]
	key := enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "prompt-v3", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	bound, err := manifest.BindCacheKeys([]enrichment.CacheKey{key})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := bound.Snapshot()

	// Legacy v1 persisted manifests were created with target language empty in
	// the canonical cache key (it was never part of v1 digest inputs). Rebuild
	// with an empty target to verify the historical digest is preserved.
	legacyKey := key
	legacyKey.TargetLanguage = ""
	legacyBound, err := manifest.BindCacheKeys([]enrichment.CacheKey{legacyKey})
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacyBound.Snapshot()
	legacy.SchemaVersion = LegacyManifestSchemaVersion
	legacyDigest, err := legacy.Digest()
	if err != nil {
		t.Fatal(err)
	}
	const wantLegacyDigest = "1e8f4112b54cb863b2beba9e2a6be9715f3e9e468fecedee2165d8937cb2c50c"
	if legacyDigest != wantLegacyDigest {
		t.Fatalf("legacy v1 digest=%q want=%q", legacyDigest, wantLegacyDigest)
	}

	digest, err := snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	const wantV2Digest = "bcb85683d3ac96c8bc645898d7e98b5bea39cdb058259c8310818bae5930bfa6"
	if digest != wantV2Digest {
		t.Fatalf("v2 digest=%q want=%q", digest, wantV2Digest)
	}
	if legacyDigest == digest {
		t.Fatal("v1 and v2 digests must differ")
	}
}
