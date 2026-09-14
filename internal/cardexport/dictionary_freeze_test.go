package cardexport

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lexicalStub struct {
	result enrichment.LexicalEntry
	found  bool
}

func (d lexicalStub) Name() string    { return "fixture" }
func (d lexicalStub) Version() string { return "fixture-v1" }
func (d lexicalStub) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return d.result, d.found, nil
}

func TestLexicalFieldsAreFrozenBeforeManifestAndRender(t *testing.T) {
	service := NewServiceWithLexicalProvider(nil, lexicalStub{found: true, result: enrichment.LexicalEntry{
		Gender:  "Neut",
		Article: "das",
		Plural:  "Häuser",
		Senses:  []enrichment.LexicalSense{{Gloss: "building"}},
	}})
	entry := Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute neben dem Bahnhof.", TargetWord: "Haus"}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &entry))
	assert.Equal(t, "building (Pl. Häuser)", entry.Gloss)
	assert.Equal(t, "fixture-v1", entry.DictionaryProviderVersion)

	manifest := NewManifest("owner", "Book", []Entry{entry})
	snapshot := manifest.Snapshot()
	assert.Equal(t, entry.Gloss, snapshot.Items[0].Entry.Gloss)
	assert.Equal(t, entry.DictionaryProviderVersion, snapshot.Items[0].Entry.DictionaryProviderVersion)
	note, err := makeNote("owner", snapshot.Items[0].Entry)
	require.NoError(t, err)
	assert.Equal(t, "das", note.Article)
	assert.Equal(t, entry.Gloss, note.Gloss)
	assert.Contains(t, note.BackExtra, "building (Pl. Häuser)")
}

func TestMissingLexicalEntryKeepsMorphologyFallback(t *testing.T) {
	service := NewServiceWithLexicalProvider(nil, lexicalStub{})
	entry := Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Morphology: `{"Gender":"Neut"}`}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &entry))
	note, err := makeNote("owner", Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute neben dem Bahnhof.", TargetWord: "Haus", Morphology: `{"Gender":"Neut"}`})
	require.NoError(t, err)
	assert.Equal(t, "das", note.Article)
	assert.Empty(t, note.Gloss)
}
