package cardexport

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
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
	assert.Equal(t, "building", entry.Gloss)
	assert.Equal(t, "Häuser", entry.Plural)
	assert.Equal(t, "fixture-v1", entry.DictionaryProviderVersion)

	manifest := NewManifest("owner", "Book", []Entry{entry})
	snapshot := manifest.Snapshot()
	assert.Equal(t, entry.Gloss, snapshot.Items[0].Entry.Gloss)
	assert.Equal(t, entry.DictionaryProviderVersion, snapshot.Items[0].Entry.DictionaryProviderVersion)
	note, err := makeNote("owner", snapshot.Items[0].Entry)
	require.NoError(t, err)
	assert.Equal(t, "das", note.Article)
	assert.Equal(t, "Häuser", note.Plural)
	assert.Equal(t, entry.Gloss, note.Gloss)
	assert.Contains(t, note.BackExtra, "das Haus (Pl. Häuser)")
}

func TestIdenticalPluralIsRenderedAndNoPluralSelfSuppresses(t *testing.T) {
	service := NewServiceWithLexicalProvider(nil, lexicalStub{found: true, result: enrichment.LexicalEntry{
		Gender: "Masc", Article: "der", Plural: "Gauner",
		Senses: []enrichment.LexicalSense{{Gloss: "rogue"}},
	}})
	entry := Entry{Language: "de", CanonicalLemma: "gauner", UPOS: "NOUN", Sentence: "Viele Gauner wurden gestern verhaftet.", TargetWord: "Gauner"}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &entry))
	note, err := makeNote("owner", entry)
	require.NoError(t, err)
	assert.Equal(t, "Gauner", note.Plural)
	assert.Contains(t, note.BackExtra, "der Gauner (Pl. Gauner)")

	service = NewServiceWithLexicalProvider(nil, lexicalStub{found: true, result: enrichment.LexicalEntry{
		Gender: "", Article: "", Plural: "",
		Senses: []enrichment.LexicalSense{{Gloss: "parents"}},
	}})
	entry = Entry{Language: "de", CanonicalLemma: "eltern", UPOS: "NOUN", Sentence: "Meine Eltern wohnen seit Jahren am See.", TargetWord: "Eltern"}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &entry))
	note, err = makeNote("owner", entry)
	require.NoError(t, err)
	assert.Empty(t, note.Plural)
	assert.NotContains(t, note.BackExtra, "(Pl.")
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

func TestNonNounPluralIsNotRenderedOnCard(t *testing.T) {
	service := NewServiceWithLexicalProvider(nil, lexicalStub{found: true, result: enrichment.LexicalEntry{
		Plural: "stehen auf",
		Senses: []enrichment.LexicalSense{{Gloss: "to get up"}},
	}})
	for _, test := range []struct {
		name string
		upos string
	}{
		{name: "verb", upos: "VERB"},
		{name: "adjective", upos: "ADJ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := Entry{Language: "de", CanonicalLemma: "aufstehen", UPOS: test.upos, Sentence: "Wir stehen auf.", TargetWord: "stehen auf"}
			require.NoError(t, service.resolveLexicalEntry(context.Background(), &entry))
			assert.Equal(t, "to get up", entry.Gloss)
			assert.Empty(t, entry.Morphology)
		})
	}
}

func TestDictionaryIndexMorphologyRendersOnCard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Neut","Article":"das","Plural":"Häuser","IPA":""}]', 'Neut', 'das', 'Häuser', '', ''); INSERT INTO entries VALUES ('de', 'see', 'NOUN', '[{"Gloss":"lake","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"der","Plural":"Seen","IPA":""},{"Gloss":"sea","Examples":[],"Topics":["tief","salzig"],"Tags":[],"Phrase":"","Gender":"Fem","Article":"die","Plural":"Meere","IPA":""}]', 'Masc', 'der', 'Seen', '', '')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO entries VALUES ('de', 'straße', 'NOUN', '[{"Gloss":"street","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Fem","Article":"die","Plural":"Straßen","IPA":""}]', 'Fem', 'die', 'Straßen', '', '')`)
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
	assert.Equal(t, "Häuser", note.Plural)
	assert.NotContains(t, note.Gloss, "(Pl.")
	assert.Contains(t, note.BackExtra, "das Haus")
	assert.Contains(t, note.BackExtra, "(Pl. Häuser)")

	sharpS := Entry{
		Language: "de", CanonicalLemma: "straße", UPOS: "NOUN",
		Sentence: "Die Straße führt zum Bahnhof.", TargetWord: "Straße",
		Morphology: `{"Gender":"Masc"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &sharpS))
	assert.Equal(t, `{"Article":"die","Gender":"Fem","Plural":"Straßen"}`, sharpS.Morphology)
	assert.Equal(t, "street", sharpS.Gloss)
	assert.Equal(t, "Straßen", sharpS.Plural)

	manifest := NewManifest("owner", "Book", []Entry{sharpS})
	snapshot := manifest.Snapshot()
	assert.Equal(t, sharpS.Gloss, snapshot.Items[0].Entry.Gloss)
	note, err = makeNote("owner", snapshot.Items[0].Entry)
	require.NoError(t, err)
	assert.Equal(t, "die", note.Article)
	assert.Equal(t, "street", note.Gloss)
	assert.Equal(t, "Straßen", note.Plural)
	assert.Contains(t, note.BackExtra, "die Straße")

	ranked := Entry{
		Language: "de", CanonicalLemma: "see", UPOS: "NOUN",
		Sentence: "Die See ist tief und salzig.", TargetWord: "See",
		Morphology: `{"Gender":"Masc","Article":"der"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &ranked))
	note, err = makeNote("owner", ranked)
	require.NoError(t, err)
	assert.Equal(t, "die", note.Article)
	assert.True(t, strings.HasPrefix(note.Gloss, "sea · lake"), "ranked gloss = %q", note.Gloss)
	assert.Equal(t, "Meere", note.Plural)

	unindexed := Entry{
		Language: "de", CanonicalLemma: "baum", UPOS: "NOUN",
		Sentence: "Der Baum steht dort.", TargetWord: "Baum",
		Morphology: `{"Gender":"Masc"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(context.Background(), &unindexed))
	assert.Empty(t, unindexed.Gloss)
	assert.Empty(t, unindexed.DictionaryProviderVersion)
	note, err = makeNote("owner", unindexed)
	require.NoError(t, err)
	assert.Equal(t, "der", note.Article)
	assert.NotContains(t, deckDescription([]Entry{unindexed}), dictionary.AttributionNotice)
}

func TestItalianDictionaryIndexMorphologyRendersOnCard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('it', 'casa', 'NOUN', '[{"Gloss":"house","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Fem","Article":"la","Plural":"case","IPA":""},{"Gloss":"home","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Fem","Article":"la","Plural":"case","IPA":""}]', 'Fem', 'la', 'case', '', ''); INSERT INTO entries VALUES ('it', 'libro', 'NOUN', '[{"Gloss":"book","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"il","Plural":"libri","IPA":""}]', 'Masc', 'il', 'libri', '', ''); INSERT INTO entries VALUES ('it', 'zaino', 'NOUN', '[{"Gloss":"backpack","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"lo","Plural":"zaini","IPA":""}]', 'Masc', 'lo', 'zaini', '', ''); INSERT INTO entries VALUES ('it', 'albero', 'NOUN', '[{"Gloss":"tree","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"l''","Plural":"alberi","IPA":""}]', 'Masc', 'l''', 'alberi', '', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := dictionary.OpenIndex(path)
	require.NoError(t, err)
	defer index.Close()
	service := NewServiceWithLexicalProvider(nil, index)
	for _, test := range []struct {
		lemma, target, sentence, article, plural, gloss string
	}{
		{lemma: "casa", target: "casa", sentence: "La casa è grande.", article: "la", plural: "case", gloss: "house · home"},
		{lemma: "libro", target: "libro", sentence: "Il libro è nuovo.", article: "il", plural: "libri", gloss: "book"},
		{lemma: "zaino", target: "zaino", sentence: "Lo zaino è pieno.", article: "lo", plural: "zaini", gloss: "backpack"},
		{lemma: "albero", target: "L'albero", sentence: "L'albero è alto.", article: "l'", plural: "alberi", gloss: "tree"},
	} {
		t.Run(test.lemma, func(t *testing.T) {
			entry := Entry{
				Language: "it", CanonicalLemma: test.lemma, UPOS: "NOUN",
				Sentence: test.sentence, TargetWord: test.target,
				Morphology: `{"Gender":"Masc","Article":"il"}`,
			}
			require.NoError(t, service.resolveLexicalEntry(context.Background(), &entry))

			note, err := makeNote("owner", entry)
			require.NoError(t, err)
			assert.Equal(t, strings.ReplaceAll(test.article, "'", "&#39;"), note.Article)
			assert.Equal(t, test.gloss, note.Gloss)
			assert.Equal(t, test.plural, note.Plural)
			escapedArticle := strings.ReplaceAll(test.article, "'", "&#39;")
			articleLemma := escapedArticle + " " + test.lemma
			if strings.HasSuffix(test.article, "'") {
				articleLemma = escapedArticle + test.lemma
			}
			assert.Contains(t, note.BackExtra, articleLemma)
		})
	}
}
