package cardexport

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAttributionIsAV13ManifestDigestInput proves the optional typed
// attribution is frozen by the current codec and ignored by the historical v12
// codec, so old manifests keep their digest and render without it.
func TestAttributionIsAV13ManifestDigestInput(t *testing.T) {
	snapshot := newManifest("owner-1", "Buch", []Entry{{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus",
		Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster"},
	}}).Snapshot()
	without := snapshot
	without.Items = cloneManifestItems(snapshot.Items)
	without.Items[0].Entry.Attribution = domain.SourceAttribution{}

	withoutV12 := without
	withoutV12.SchemaVersion = ManifestSchemaVersionV12
	withV12 := snapshot
	withV12.SchemaVersion = ManifestSchemaVersionV12
	withoutV12Digest, err := withoutV12.Digest()
	require.NoError(t, err)
	withV12Digest, err := withV12.Digest()
	require.NoError(t, err)
	assert.Equal(t, withoutV12Digest, withV12Digest, "v12 must ignore the new attribution field")

	withoutCurrentDigest, err := without.Digest()
	require.NoError(t, err)
	withCurrentDigest, err := snapshot.Digest()
	require.NoError(t, err)
	assert.NotEqual(t, withoutCurrentDigest, withCurrentDigest, "v13 must include the attribution field")
}

func TestItemKindIsAV14ManifestDigestInput(t *testing.T) {
	snapshot := newManifest("owner-1", "Buch", []Entry{{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus",
	}}).Snapshot()
	snapshot.Items[0].Entry.Headword = "haus"
	idiom := snapshot
	idiom.Items = cloneManifestItems(snapshot.Items)
	idiom.Items[0].ItemKind = enrichment.ItemKindIdiom
	fillIdiomEvidence(&idiom.Items[0])

	lemmaV13 := snapshot
	lemmaV13.SchemaVersion = ManifestSchemaVersionV13
	idiomV13 := idiom
	idiomV13.SchemaVersion = ManifestSchemaVersionV13
	lemmaV13Digest, err := lemmaV13.Digest()
	require.NoError(t, err)
	idiomV13Digest, err := idiomV13.Digest()
	require.NoError(t, err)
	assert.Equal(t, lemmaV13Digest, idiomV13Digest, "v13 must ignore the item kind")

	lemmaDigest, err := snapshot.Digest()
	require.NoError(t, err)
	idiomDigest, err := idiom.Digest()
	require.NoError(t, err)
	assert.NotEqual(t, lemmaDigest, idiomDigest, "v14 must include the item kind")
}

func TestIdiomOverflowIsAV16ManifestDigestInput(t *testing.T) {
	snapshot := newManifest("owner-1", "Buch", []Entry{{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus",
	}}).Snapshot()
	withOverflow := snapshot
	withOverflow.IdiomOverflow = 2

	t.Run("v15 ignores the overflow count", func(t *testing.T) {
		withoutV15 := snapshot
		withoutV15.SchemaVersion = ManifestSchemaVersionV15
		withV15 := withOverflow
		withV15.SchemaVersion = ManifestSchemaVersionV15
		withoutDigest, err := withoutV15.Digest()
		require.NoError(t, err)
		withDigest, err := withV15.Digest()
		require.NoError(t, err)
		require.Equal(t, withoutDigest, withDigest)
	})
	t.Run("v16 includes the overflow count", func(t *testing.T) {
		withoutDigest, err := snapshot.Digest()
		require.NoError(t, err)
		withDigest, err := withOverflow.Digest()
		require.NoError(t, err)
		require.NotEqual(t, withoutDigest, withDigest)
	})
}

func TestItemKindIsPartOfTheCacheIdentityDigest(t *testing.T) {
	snapshot := newManifest("owner-1", "Buch", []Entry{{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus",
	}}).Snapshot()
	snapshot.Items[0].CacheKey = &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "prompt-v1", SentenceHash: enrichment.SentenceHash("Das alte Haus ist überraschend groß.")}
	snapshot.Items[0].Entry.Headword = "haus"

	lemmaDigest, err := snapshot.Digest()
	require.NoError(t, err)
	snapshot.Items[0].ItemKind = enrichment.ItemKindIdiom
	fillIdiomEvidence(&snapshot.Items[0])
	snapshot.Items[0].CacheKey.ItemKind = enrichment.ItemKindIdiom
	idiomDigest, err := snapshot.Digest()
	require.NoError(t, err)
	assert.NotEqual(t, lemmaDigest, idiomDigest, "a different cache kind must change v14 identity")
}

func fillIdiomEvidence(item *ManifestItem) {
	item.Entry.MatchedTokenOrdinals = []int{0, 1}
	item.Entry.AnchorLanguage = item.Entry.Language
	item.Entry.AnchorCanonicalLemma = item.Entry.CanonicalLemma
	item.Entry.AnchorUPOS = item.Entry.UPOS
	item.Entry.Source = "wiktionary"
	item.Entry.SourceVersion = "fixture-v1"
	item.Entry.MatcherVersion = "idiom-matcher/1"
	item.Entry.GapBound = 2
}

func TestQualityDiagnosticsAffectTheCurrentManifestDigest(t *testing.T) {
	manifest := newManifest("owner-1", "Buch", []Entry{{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus",
	}})
	snapshot := manifest.Snapshot()
	snapshot.Items[0].Quality.GDEXScore = 0.9

	before, err := snapshot.Digest()
	require.NoError(t, err)
	snapshot.Items[0].Quality.GDEXScore = 0.1
	after, err := snapshot.Digest()
	require.NoError(t, err)
	assert.NotEqual(t, before, after, "the current digest must include the GDEX field")
}
