package dictionary

import (
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestOpenIndexRejectsDirectory(t *testing.T) {
	dir := t.TempDir()

	index, err := OpenIndex(t.Context(), dir)

	require.Error(t, err)
	assert.Nil(t, index)
	require.ErrorIs(t, err, ErrInvalidIndex)
	assert.False(t, errors.Is(err, fs.ErrNotExist), "a directory is a misconfiguration, not an absent index")
	assert.Contains(t, err.Error(), "directory")
}

func TestOpenIndexMissingPathIsNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.sqlite")

	index, err := OpenIndex(t.Context(), path)

	require.Error(t, err)
	assert.Nil(t, index)
	assert.ErrorIs(t, err, ErrInvalidIndex) //nolint:testifylint // Invalid-index and missing-file classifications are independently asserted.
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestIndexLookupReadsVersionAndMorphology(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house","Gender":"Neut","Article":"das","Plural":"Häuser"}]', 'Neut', 'das', 'Häuser', '/haʊ̯s/', ''); INSERT INTO entries VALUES ('de', 'duplicate', 'NOUN', '[{"Gloss":"same"},{"Gloss":"same"}]', '', '', '', '', ''); INSERT INTO entries VALUES ('de', 'aufstehen', 'VERB', '[{"Gloss":"to get up"}]', '', '', '', '', 'steht auf · stand auf · aufgestanden'); INSERT INTO entries VALUES ('de', 'regnen', 'VERB', '[{"Gloss":"to rain"}]', '', '', '', '', ''); INSERT INTO entries VALUES ('it', 'casa', 'NOUN', '[{"Gloss":"house","Gender":"Fem","Article":"la","Plural":"case"}]', 'Fem', 'la', 'case', '', ''); INSERT INTO entries VALUES ('el', 'οδοσ', 'NOUN', '[{"Gloss":"road"}]', '', '', '', '', '')`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO entries VALUES ('el', 'άνθρωποσ', 'NOUN', '[{"Gloss":"person","Gender":"Masc","Article":"ο","Plural":"άνθρωποι"}]', 'Masc', 'ο', 'άνθρωποι', '/ˈanθropos/', ''); INSERT INTO entries VALUES ('el', 'είμαι', 'VERB', '[{"Gloss":"to be"}]', '', '', '', '', ''); INSERT INTO entries VALUES ('el', 'δρόμοσ', 'NOUN', '[{"Gloss":"road","Gender":"Masc","Article":"ο","Plural":"δρόμοι"}]', '', 'η', 'δρόμοι', '', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := index.Close(); err != nil {
			t.Errorf("dictionary index cleanup failed: %v", err)
		}
	})
	assert.Equal(t, "kaikki", index.Name())
	assert.Equal(t, "fixture-v1", index.Version())
	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de-DE", CanonicalLemma: "Haus", UPOS: "NOUN", RepresentativeSentence: "Das Haus ist groß."})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "house", result.Senses[0].Gloss)
	assert.Equal(t, "wiktionary", result.Senses[0].Source)
	assert.Equal(t, "meaning", result.Senses[0].Kind)
	assert.Equal(t, "Kaikki.org Wiktextract enwiktionary", result.Senses[0].Origin)
	assert.Equal(t, "fixture-v1", result.Senses[0].Version)
	assert.Equal(t, "exact_lemma_pos", result.Senses[0].MatchStrength)
	assert.NotEmpty(t, result.Senses[0].EvidenceID)
	firstEvidenceID := result.Senses[0].EvidenceID
	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, firstEvidenceID, result.Senses[0].EvidenceID, "an unchanged evidence record has a stable ID")
	assert.Equal(t, "Neut", result.Gender)
	assert.Equal(t, "das", result.Article)
	assert.Equal(t, "Häuser", result.Plural)
	assert.Equal(t, "/haʊ̯s/", result.IPA)
	assert.Empty(t, result.PrincipalParts)
	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "duplicate", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Len(t, result.CandidateSenses, 1, "duplicate source rows are one distinct evidence candidate")

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Aufstehen", UPOS: "VERB"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "to get up", result.Senses[0].Gloss)
	assert.Empty(t, result.IPA)
	assert.Equal(t, "steht auf · stand auf · aufgestanden", result.PrincipalParts)

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Regnen", UPOS: "VERB"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Empty(t, result.IPA)
	assert.Empty(t, result.PrincipalParts)

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "it-IT", CanonicalLemma: "Casa", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "house", result.Senses[0].Gloss)
	assert.Equal(t, "Fem", result.Gender)
	assert.Equal(t, "la", result.Article)
	assert.Equal(t, "case", result.Plural)

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "el", CanonicalLemma: "ΟΔΟΣ", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "road", result.Senses[0].Gloss)

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "el", CanonicalLemma: "ΆΝΘΡΩΠΟΣ", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "person", result.Senses[0].Gloss)
	assert.Equal(t, "Masc", result.Gender)
	assert.Equal(t, "ο", result.Article)
	assert.Equal(t, "άνθρωποι", result.Plural)
	assert.Equal(t, "/ˈanθropos/", result.IPA)

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "el", CanonicalLemma: "ΕΊΜΑΙ", UPOS: "VERB"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Empty(t, result.PrincipalParts)

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "el", CanonicalLemma: "ΔΡΌΜΟΣ", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Empty(t, result.Gender)
	assert.Equal(t, "η", result.Article)
	assert.Equal(t, "δρόμοι", result.Plural)

	_, err = index.db.ExecContext(t.Context(), `INSERT INTO metadata VALUES ('unexpected', 'write')`)
	assert.Error(t, err) //nolint:testifylint // Read-only write rejection and the following lookup are independent checks.

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "nicht-im-index", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, result.Senses)
}

func TestIndexLookupUsesMorphologyFromRankedSense(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'see', 'NOUN', '[{"Gloss":"lake","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"der","Plural":"Seen","IPA":""},{"Gloss":"sea","Examples":[],"Topics":["tief","salzig"],"Tags":[],"Phrase":"","Gender":"Fem","Article":"die","Plural":"Meere","IPA":""}]', 'Masc', 'der', 'Seen', '', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := index.Close(); err != nil {
			t.Errorf("dictionary index cleanup failed: %v", err)
		}
	})
	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{
		Language: "de", CanonicalLemma: "See", UPOS: "NOUN", TargetWord: "See",
		RepresentativeSentence: "Die See ist tief und salzig.",
	})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, result.Senses, 2)
	assert.Equal(t, "sea", result.Senses[0].Gloss)
	assert.Equal(t, "Fem", result.Gender)
	assert.Equal(t, "die", result.Article)
	assert.Equal(t, "Meere", result.Plural)
}
