package dictionary

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestIndexLookupReadsVersionAndMorphology(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house","Gender":"Neut","Plural":"Häuser"}]', 'Neut', 'das', 'Häuser', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := OpenIndex(path)
	require.NoError(t, err)
	defer index.Close()
	assert.Equal(t, "kaikki", index.Name())
	assert.Equal(t, "fixture-v1", index.Version())
	result, found, err := index.Lookup(context.Background(), LookupRequest{Language: "de-DE", CanonicalLemma: "Haus", UPOS: "NOUN", ExampleSentence: "Das Haus ist groß."})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "house", result.Gloss)
	assert.Equal(t, "Neut", result.Morphology["Gender"])
	assert.Equal(t, "das", result.Morphology["Article"])
	assert.Equal(t, "Häuser", result.Morphology["Plural"])
}
