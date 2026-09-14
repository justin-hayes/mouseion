package cardexport

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dictionaryStub struct {
	result dictionary.Result
	found  bool
}

func (d dictionaryStub) Name() string    { return "fixture" }
func (d dictionaryStub) Version() string { return "fixture-v1" }
func (d dictionaryStub) Lookup(context.Context, dictionary.LookupRequest) (dictionary.Result, bool, error) {
	return d.result, d.found, nil
}

func TestDictionaryFieldsAreFrozenBeforeManifestAndRender(t *testing.T) {
	service := NewServiceWithDictionary(nil, dictionaryStub{found: true, result: dictionary.Result{
		Gloss:      "building",
		Morphology: map[string]string{"Gender": "Neut", "Article": "das", "Plural": "Häuser"},
		Plural:     "Häuser",
	}})
	entry := Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute neben dem Bahnhof.", TargetWord: "Haus"}
	require.NoError(t, service.resolveDictionary(context.Background(), &entry))
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

func TestMissingDictionaryEntryKeepsMorphologyFallback(t *testing.T) {
	service := NewServiceWithDictionary(nil, dictionaryStub{})
	entry := Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Morphology: `{"Gender":"Neut"}`}
	require.NoError(t, service.resolveDictionary(context.Background(), &entry))
	note, err := makeNote("owner", Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute neben dem Bahnhof.", TargetWord: "Haus", Morphology: `{"Gender":"Neut"}`})
	require.NoError(t, err)
	assert.Equal(t, "das", note.Article)
	assert.Empty(t, note.Gloss)
}
