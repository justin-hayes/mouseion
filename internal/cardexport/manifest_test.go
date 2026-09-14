package cardexport

import (
	"bytes"
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestSnapshotRoundTripAndDigestFixture(t *testing.T) {
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
	selected, accepted, omitted := snapshot.Counts()
	assert.Equal(t, 2, selected)
	assert.Equal(t, 1, accepted)
	assert.Equal(t, 1, omitted)
	for _, item := range snapshot.Items {
		assert.Equal(t, "", item.Entry.OwnerID, "snapshot retained noncanonical fields: %+v", item.Entry)
		assert.Equal(t, "", item.Entry.Translation, "snapshot retained noncanonical fields: %+v", item.Entry)
		assert.Equal(t, "", item.Entry.SentenceTranslation, "snapshot retained noncanonical fields: %+v", item.Entry)
		assert.Equal(t, "", item.Entry.SentenceTranslationTarget, "snapshot retained noncanonical fields: %+v", item.Entry)
		assert.Equal(t, "NOUN", item.Entry.UPOS, "snapshot retained noncanonical fields: %+v", item.Entry)
	}
	digest, err := snapshot.Digest()
	require.NoError(t, err)
	const wantDigest = "97922a2243d53f2f4cade128c8d523913403174533436d5b17f4f204a48e9583"
	assert.Equal(t, wantDigest, digest, "digest=%q want=%q", digest, wantDigest)

	rebuilt, err := ManifestFromSnapshot(snapshot)
	require.NoError(t, err)
	provenance := enrichment.Provenance{Provider: key.Provider, ProviderVersion: key.ProviderVersion}
	exact := []ExactEnrichment{{CacheKey: key, Result: enrichment.Result{
		Candidate:           candidate,
		Translation:         enrichment.Field[string]{Value: "house", Available: true, Provenance: provenance},
		SentenceTranslation: enrichment.Field[string]{Value: "The old house is surprisingly large.", Available: true, Provenance: provenance},
	}}}
	want, err := (&Service{}).RenderManifest(context.Background(), bound, exact)
	require.NoError(t, err)
	got, err := (&Service{}).RenderManifest(context.Background(), rebuilt, exact)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(got.APKG, want.APKG), "round trip changed render")
	assert.Equal(t, want.TSV, got.TSV, "round trip changed render")
	assert.Equal(t, want.Completeness, got.Completeness, "round trip changed render")
	assert.Equal(t, want.Omitted, got.Omitted, "round trip changed render")

	snapshot.Items[0].Entry.Sentence = "mutated"
	snapshot.Items[0].Quality.Reasons[0] = "mutated"
	cloned := bound.Snapshot()
	assert.NotEqual(t, "mutated", cloned.Items[0].Entry.Sentence, "snapshot mutation changed the manifest")
	assert.NotEqual(t, "mutated", cloned.Items[0].Quality.Reasons[0], "snapshot mutation changed the manifest")
}

func TestManifestSnapshotRejectsPartialIdentityAndNoncontiguousOrder(t *testing.T) {
	entries := []Entry{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", FirstEncounter: 1},
		{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", FirstEncounter: 2},
	}
	snapshot := NewManifest("owner-1", "Buch", entries).Snapshot()
	snapshot.Items[0].CacheKey = &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(entries[0].Sentence)}
	_, err := snapshot.Digest()
	assert.ErrorIs(t, err, ErrInvalidInput, "partial cache identity error=%v", err)
	snapshot.Items[0].CacheKey = nil
	snapshot.Items[1].Ordinal = 3
	_, err = ManifestFromSnapshot(snapshot)
	assert.ErrorIs(t, err, ErrInvalidInput, "noncontiguous ordinal error=%v", err)
}

func TestManifestSnapshotPreservesQualityDiagnosticsForAcceptedAndOmittedItems(t *testing.T) {
	snapshot := ManifestSnapshot{
		SchemaVersion: ManifestSchemaVersion,
		Owner:         "owner-1",
		DeckName:      "Buch",
		Filename:      DownloadFilename("Buch"),
		Items: []ManifestItem{
			{
				Ordinal: 0, Disposition: ManifestAccepted,
			Entry:   Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Gloss: "house", DictionaryProviderVersion: "fixture-v1", Sentence: "Das Haus steht dort.", TargetWord: "Haus"},
				Quality: SentenceQuality{Accepted: true, Score: 94, GDEXScore: 0.94, Reasons: []string{"target present", "optimal length"}},
			},
			{
				Ordinal: 1, Disposition: ManifestQualityOmitted,
				Entry:   Entry{Language: "de", CanonicalLemma: "fragment", UPOS: "NOUN", Sentence: "Fragment.", TargetWord: "Fragment"},
				Quality: SentenceQuality{Score: 0, Reasons: []string{"too short or fragmented"}},
			},
		},
	}

	rebuilt, err := ManifestFromSnapshot(snapshot)
	require.NoError(t, err)
	require.Len(t, rebuilt.decisions, 2)
	assert.Equal(t, snapshot.Items[0].Quality, rebuilt.decisions[0].Quality)
	assert.Equal(t, snapshot.Items[0].Entry.Gloss, rebuilt.decisions[0].Entry.Gloss)
	assert.Equal(t, snapshot.Items[0].Entry.DictionaryProviderVersion, rebuilt.decisions[0].Entry.DictionaryProviderVersion)
	assert.Equal(t, snapshot.Items[1].Quality, rebuilt.decisions[1].Quality)
	require.Len(t, rebuilt.omitted, 1)
	assert.Equal(t, snapshot.Items[1].Entry.CanonicalLemma, rebuilt.omitted[0].CanonicalLemma)
	assert.Equal(t, snapshot.Items[1].Quality.Score, rebuilt.omitted[0].Score)
	assert.Equal(t, snapshot.Items[1].Quality.Reasons, rebuilt.omitted[0].Reasons)
}
