package cardexport

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
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
	if err != nil {
		t.Fatal(err)
	}
	snapshot := bound.Snapshot()
	selected, accepted, omitted := snapshot.Counts()
	if selected != 2 || accepted != 1 || omitted != 1 {
		t.Fatalf("counts=(%d,%d,%d)", selected, accepted, omitted)
	}
	for _, item := range snapshot.Items {
		if item.Entry.OwnerID != "" || item.Entry.Translation != "" || item.Entry.SentenceTranslation != "" || item.Entry.SentenceTranslationTarget != "" || item.Entry.UPOS != "NOUN" {
			t.Fatalf("snapshot retained noncanonical fields: %+v", item.Entry)
		}
	}
	digest, err := snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "1e8f4112b54cb863b2beba9e2a6be9715f3e9e468fecedee2165d8937cb2c50c"
	if digest != wantDigest {
		t.Fatalf("digest=%q want=%q", digest, wantDigest)
	}

	rebuilt, err := ManifestFromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	provenance := enrichment.Provenance{Provider: key.Provider, ProviderVersion: key.ProviderVersion}
	exact := []ExactEnrichment{{CacheKey: key, Result: enrichment.Result{
		Candidate:           candidate,
		Translation:         enrichment.Field[string]{Value: "house", Available: true, Provenance: provenance},
		SentenceTranslation: enrichment.Field[string]{Value: "The old house is surprisingly large.", Available: true, Provenance: provenance},
	}}}
	want, err := (&Service{}).RenderManifest(context.Background(), bound, exact)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&Service{}).RenderManifest(context.Background(), rebuilt, exact)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.APKG, want.APKG) || got.TSV != want.TSV || got.Completeness != want.Completeness || !reflect.DeepEqual(got.Omitted, want.Omitted) {
		t.Fatalf("round trip changed render\ngot=%+v\nwant=%+v", got, want)
	}

	snapshot.Items[0].Entry.Sentence = "mutated"
	snapshot.Items[0].Quality.Reasons[0] = "mutated"
	if cloned := bound.Snapshot(); cloned.Items[0].Entry.Sentence == "mutated" || cloned.Items[0].Quality.Reasons[0] == "mutated" {
		t.Fatal("snapshot mutation changed the manifest")
	}
}

func TestManifestSnapshotRejectsPartialIdentityAndNoncontiguousOrder(t *testing.T) {
	entries := []Entry{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", FirstEncounter: 1},
		{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", FirstEncounter: 2},
	}
	snapshot := NewManifest("owner-1", "Buch", entries).Snapshot()
	snapshot.Items[0].CacheKey = &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(entries[0].Sentence)}
	if _, err := snapshot.Digest(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("partial cache identity error=%v", err)
	}
	snapshot.Items[0].CacheKey = nil
	snapshot.Items[1].Ordinal = 3
	if _, err := ManifestFromSnapshot(snapshot); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("noncontiguous ordinal error=%v", err)
	}
}
