package dictionary

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestIndexLookupReadsVersionAndMorphology(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house","Gender":"Neut","Plural":"Häuser"}]', 'Neut', 'das', 'Häuser', ''); INSERT INTO entries VALUES ('de', 'aufstehen', 'VERB', '[{"Gloss":"to get up"}]', '', '', '', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := OpenIndex(path)
	require.NoError(t, err)
	defer index.Close()
	assert.Equal(t, "kaikki", index.Name())
	assert.Equal(t, "fixture-v1", index.Version())
	result, found, err := index.Lookup(context.Background(), enrichment.LexicalLookupRequest{Language: "de-DE", CanonicalLemma: "Haus", UPOS: "NOUN", RepresentativeSentence: "Das Haus ist groß."})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "house", result.Senses[0].Gloss)
	assert.Equal(t, "Neut", result.Gender)
	assert.Equal(t, "das", result.Article)
	assert.Equal(t, "Häuser", result.Plural)

	result, found, err = index.Lookup(context.Background(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Aufstehen", UPOS: "VERB"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "to get up", result.Senses[0].Gloss)

	_, err = index.db.Exec(`INSERT INTO metadata VALUES ('unexpected', 'write')`)
	assert.Error(t, err)

	result, found, err = index.Lookup(context.Background(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "nicht-im-index", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, result.Senses)
}
