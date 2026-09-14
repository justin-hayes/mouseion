package cardexport

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
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

func TestDictionaryIndexMorphologyRendersOnCard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Neut","Article":"das","Plural":"Häuser","IPA":""}]', 'Neut', 'das', 'Häuser', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := dictionary.OpenIndex(path)
	require.NoError(t, err)
	defer index.Close()
	service := NewServiceWithLexicalProvider(nil, index)
	entry := Entry{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das Haus steht heute neben dem Bahnhof.", TargetWord: "Haus",
		Morphology: `{"Gender":"Masc","Article":"der"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &entry))

	note, err := makeNote("owner", entry)
	require.NoError(t, err)
	assert.Equal(t, "das", note.Article)
	assert.Contains(t, note.Gloss, "house")
	assert.Contains(t, note.Gloss, "(Pl. Häuser)")
	assert.Contains(t, note.BackExtra, "das Haus")
	assert.Contains(t, note.BackExtra, "(Pl. Häuser)")

	unindexed := Entry{
		Language: "de", CanonicalLemma: "baum", UPOS: "NOUN",
		Sentence: "Der Baum steht dort.", TargetWord: "Baum",
		Morphology: `{"Gender":"Masc"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &unindexed))
	assert.Empty(t, unindexed.Gloss)
	note, err = makeNote("owner", unindexed)
	require.NoError(t, err)
	assert.Equal(t, "der", note.Article)
}
