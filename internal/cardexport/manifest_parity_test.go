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
	assert.Equal(t, "6ef322dea63a652c6e6216e4b03fe7d2ef543092d17c5ccc0a4e9132a7fc83fc", digest)

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
