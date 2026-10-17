package dictionary

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/lemmarisk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// dictionaryFixtureSchema is the read-only schema OpenIndex expects.
const dictionaryFixtureSchema = `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1')`

// newTestIndex opens a valid SQLite index seeded with seedStatements; callers
// with no seed get an empty but usable provider handle.
func newTestIndex(t *testing.T, seedStatements ...string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), dictionaryFixtureSchema)
	require.NoError(t, err)
	for _, statement := range seedStatements {
		_, err = db.ExecContext(t.Context(), statement)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	index, err := OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	return index
}

func TestOpenIndexRejectsDirectory(t *testing.T) {
	dir := t.TempDir()

	index, err := OpenIndex(t.Context(), dir)

	require.Error(t, err)
	assert.Nil(t, index)
	require.ErrorIs(t, err, ErrInvalidIndex)
	require.NotErrorIs(t, err, fs.ErrNotExist, "a directory is a misconfiguration, not an absent index")
	assert.Contains(t, err.Error(), "directory")
}

func TestOpenIndexMissingPathIsNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.sqlite")

	index, err := OpenIndex(t.Context(), path)

	require.Error(t, err)
	assert.Nil(t, index)
	t.Run("missing index is an invalid index", func(t *testing.T) {
		require.ErrorIs(t, err, ErrInvalidIndex)
	})
	t.Run("missing file wraps fs.ErrNotExist", func(t *testing.T) {
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestIndexLookupReadsVersionAndMorphology(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house","Gender":"Neut","Article":"das","Plural":"Häuser"}]', 'Neut', 'das', 'Häuser', '/haʊ̯s/', ''); INSERT INTO entries VALUES ('de', 'duplicate', 'NOUN', '[{"Gloss":"same"},{"Gloss":"same"}]', '', '', '', '', ''); INSERT INTO entries VALUES ('de', 'aufstehen', 'VERB', '[{"Gloss":"to get up"}]', '', '', '', '', 'steht auf · stand auf · aufgestanden'); INSERT INTO entries VALUES ('de', 'regnen', 'VERB', '[{"Gloss":"to rain"}]', '', '', '', '', ''); INSERT INTO entries VALUES ('de', 'drache', 'NOUN', '[{"Gloss":"dragon","Examples":["Der Reiter trifft den Drachen mit seinem Speer."]}]', '', '', '', '', ''); INSERT INTO entries VALUES ('it', 'casa', 'NOUN', '[{"Gloss":"house","Gender":"Fem","Article":"la","Plural":"case"}]', 'Fem', 'la', 'case', '', ''); INSERT INTO entries VALUES ('el', 'οδοσ', 'NOUN', '[{"Gloss":"road"}]', '', '', '', '', '')`)
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
	exists, err := index.LemmaExists(t.Context(), "de", "Drach", "NOUN")
	require.NoError(t, err)
	assert.False(t, exists)
	exists, err = index.LemmaExists(t.Context(), "de", "Haus", "NOUN")
	require.NoError(t, err)
	assert.True(t, exists)
	alternatives, err := index.Alternatives(t.Context(), "de", "Drachen", "NOUN", "Ein Reiter trifft den Drachen mit seinem Speer.")
	require.NoError(t, err)
	require.Len(t, alternatives, 1)
	assert.Equal(t, "drache", alternatives[0].Lemma)
	assert.Equal(t, "wiktionary", alternatives[0].Source)
	assert.Equal(t, "fixture-v1", alternatives[0].Version)
	assert.True(t, alternatives[0].ContextRelevant)
	ambiguous, err := index.Alternatives(t.Context(), "de", "Drachen", "NOUN", "Der Drachen steigt bei starkem Wind hoch in den Himmel.")
	require.NoError(t, err)
	assert.Empty(t, ambiguous, "dictionary evidence for the dragon sense must not flag the kite sense")
	compound, err := index.Alternatives(t.Context(), "de", "Basketballspiel", "NOUN", "Das Basketballspiel beginnt.")
	require.NoError(t, err)
	assert.Empty(t, compound, "a valid compound with no competing local entry is not a risk flag")
	flags, assessed, err := lemmarisk.Detect(t.Context(), "de", []lemmarisk.Occurrence{
		{ID: "dragon", Language: "de", Surface: "Drachen", Lemma: "Drach", UPOS: "NOUN", Sentence: "Ein Reiter trifft den Drachen mit seinem Speer."},
		{ID: "existing-1", Language: "de", Surface: "Drache", Lemma: "Drache", UPOS: "NOUN", Sentence: "Der Drache schläft."},
		{ID: "existing-2", Language: "de", Surface: "Drache", Lemma: "Drache", UPOS: "NOUN", Sentence: "Ein Drache wacht."},
	}, index)
	require.NoError(t, err)
	assert.True(t, assessed)
	require.Len(t, flags, 1)
	assert.Equal(t, "dragon", flags[0].OccurrenceID)
	assert.Equal(t, "drache", flags[0].Alternative.Lemma)
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

	t.Run("index rejects writes", func(t *testing.T) {
		_, err := index.db.ExecContext(t.Context(), `INSERT INTO metadata VALUES ('unexpected', 'write')`)
		require.Error(t, err)
	})

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "nicht-im-index", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, result.Senses)
}

func TestIndexLookupRejectsMalformedSenses(t *testing.T) {
	index := newTestIndex(t, `INSERT INTO entries VALUES ('de', 'haus', 'NOUN', 'not-json', '', '', '', '', '')`)

	_, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	require.Error(t, err)
	assert.False(t, found)
	assert.Contains(t, err.Error(), "dictionary senses")
}

func TestIndexLookupWithoutAnIndexOrIdentityIsAbsent(t *testing.T) {
	t.Run("a nil index has no entry", func(t *testing.T) {
		var absent *Index
		result, found, err := absent.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
		require.NoError(t, err)
		assert.False(t, found)
		assert.Empty(t, result.Senses)
	})
	t.Run("an empty identity has no entry", func(t *testing.T) {
		index := newTestIndex(t)
		result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{})
		require.NoError(t, err)
		assert.False(t, found)
		assert.Empty(t, result.Senses)
	})
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

func TestIndexLookupUsesFrenchCanonicalProfile(t *testing.T) {
	index := newTestIndex(t,
		`INSERT INTO entries VALUES ('fr', 'aujourd''hui', 'ADV', '[{"Gloss":"today"}]', '', '', '', '', '')`,
		`INSERT INTO entries VALUES ('fr', 'hache', 'NOUN', '[{"Gloss":"axe","Gender":"Fem","Plural":"haches"}]', 'Fem', '', 'haches', '/aʃ/', '')`,
	)

	// The request carries the typographic apostrophe a French book uses; the
	// stored entry carries the ASCII apostrophe Wiktionary uses. The French
	// profile folds them to one canonical lemma, so both spellings resolve.
	for _, apostrophe := range []struct {
		name    string
		request enrichment.LexicalLookupRequest
	}{
		{"typographic apostrophe", enrichment.LexicalLookupRequest{Language: "fr", CanonicalLemma: "aujourd\u2019hui", UPOS: "ADV"}},
		{"ASCII apostrophe and locale", enrichment.LexicalLookupRequest{Language: "fr-FR", CanonicalLemma: "aujourd'hui", UPOS: "ADV"}},
	} {
		t.Run(apostrophe.name, func(t *testing.T) {
			result, found, err := index.Lookup(t.Context(), apostrophe.request)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, "today", result.Senses[0].Gloss)
		})
	}

	t.Run("accented French morphology still resolves", func(t *testing.T) {
		result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "fr", CanonicalLemma: "Hache", UPOS: "NOUN"})
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, "Fem", result.Gender)
		require.Equal(t, "haches", result.Plural)
	})
}

func TestIndexLookupIncludesMissingPOSOnlyAfterExactIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'bank', 'NOUN', '[{"Gloss":"bench"}]', '', '', '', '', ''); INSERT INTO entries VALUES ('de', 'bank', '', '[{"Gloss":"financial institution"}]', '', '', '', '', ''); INSERT INTO entries VALUES ('de', 'bank', 'VERB', '[{"Gloss":"to bank"}]', '', '', '', '', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })

	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Bank", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, result.CandidateSenses, 2)
	assert.Equal(t, []string{"bench", "financial institution"}, []string{result.CandidateSenses[0].Gloss, result.CandidateSenses[1].Gloss})
	assert.Equal(t, "exact_lemma_pos", result.CandidateSenses[0].MatchStrength)
	assert.Equal(t, "lemma_only_missing_pos", result.CandidateSenses[1].MatchStrength)
	assert.Equal(t, result.CandidateSenses, result.Senses, "both returned views retain evidence metadata in preferred order")

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Bank", UPOS: "ADJ"})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, result.CandidateSenses, 1)
	assert.Equal(t, "financial institution", result.CandidateSenses[0].Gloss)
	assert.Equal(t, "lemma_only_missing_pos", result.CandidateSenses[0].MatchStrength)
	assert.NotContains(t, []string{result.CandidateSenses[0].Gloss}, "to bank", "a known different POS must not leak into the candidate set")
	assert.Equal(t, "lemma_only_missing_pos", result.Senses[0].MatchStrength)
	firstWeakEvidenceID := result.CandidateSenses[0].EvidenceID
	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Bank", UPOS: "VERB"})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, result.CandidateSenses, 2)
	assert.Equal(t, firstWeakEvidenceID, result.CandidateSenses[1].EvidenceID, "the lemma-only source identity is stable across requested POS")
}

// dictionaryGreekFixtureSchema is the read-only schema OpenIndex expects from
// an index format that carries Ancient Greek entries.
const dictionaryGreekFixtureSchema = `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); CREATE TABLE greek_entries (headword TEXT NOT NULL, upos TEXT NOT NULL, etymology_number TEXT NOT NULL, senses_json TEXT NOT NULL, forms_json TEXT NOT NULL); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO metadata VALUES ('index_format', '2')`

// newGreekTestIndex opens a Greek-capable index seeded with seedStatements.
func newGreekTestIndex(t *testing.T, seedStatements ...string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), dictionaryGreekFixtureSchema)
	require.NoError(t, err)
	for _, statement := range seedStatements {
		_, err = db.ExecContext(t.Context(), statement)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	index, err := OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	return index
}

// dictionaryFrenchFixtureSchema is the read-only schema of the index format
// that carries both Ancient Greek entries and the French h_status evidence.
const dictionaryFrenchFixtureSchema = `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, h_status TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); CREATE TABLE greek_entries (headword TEXT NOT NULL, upos TEXT NOT NULL, etymology_number TEXT NOT NULL, senses_json TEXT NOT NULL, forms_json TEXT NOT NULL); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO metadata VALUES ('index_format', '3')`

// newFrenchTestIndex opens a format-3 index seeded with seedStatements.
func newFrenchTestIndex(t *testing.T, seedStatements ...string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), dictionaryFrenchFixtureSchema)
	require.NoError(t, err)
	for _, statement := range seedStatements {
		_, err = db.ExecContext(t.Context(), statement)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	index, err := OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	return index
}

// A format-3 index exposes the French aspirated-h evidence column; its absence
// on a row means unknown, never mute.
func TestIndexLookupReadsFrenchHStatus(t *testing.T) {
	index := newFrenchTestIndex(t,
		`INSERT INTO entries VALUES ('fr', 'hache', 'NOUN', '[{"Gloss":"axe","Gender":"Fem"}]', 'Fem', '', 'haches', '/aʃ/', '', 'aspirated')`,
		`INSERT INTO entries VALUES ('fr', 'hôtel', 'NOUN', '[{"Gloss":"hotel","Gender":"Masc"}]', 'Masc', '', 'hôtels', '/otɛl/', '', '')`,
	)

	aspirated, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "fr", CanonicalLemma: "hache", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "aspirated", aspirated.HStatus)
	assert.Equal(t, "Fem", aspirated.Gender)

	unknown, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "fr", CanonicalLemma: "hôtel", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Empty(t, unknown.HStatus, "an unmarked h is unknown, never mute")
}

// An older index format without the h_status column still opens and reports no
// h-status rather than failing.
func TestIndexLookupOmitsHStatusOnOlderIndexFormat(t *testing.T) {
	index := newTestIndex(t, `INSERT INTO entries VALUES ('fr', 'hache', 'NOUN', '[{"Gloss":"axe"}]', '', '', '', '', '')`)

	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "fr", CanonicalLemma: "hache", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Empty(t, result.HStatus)
}

func TestIndexAncientGreekMatchingReturnsEveryHomograph(t *testing.T) {
	index := newGreekTestIndex(t,
		`INSERT INTO greek_entries VALUES ('δέω', 'VERB', '1', '[{"Gloss":"to bind"}]', '[]')`,
		`INSERT INTO greek_entries VALUES ('δέω', 'VERB', '2', '[{"Gloss":"to lack"}]', '[]')`,
		`INSERT INTO greek_entries VALUES ('δέω', '', '3', '[{"Gloss":"δέω without a part of speech"}]', '[]')`,
		`INSERT INTO greek_entries VALUES ('δέω', 'NOUN', '4', '[{"Gloss":"a binding"}]', '[]')`,
		`INSERT INTO greek_entries VALUES ('ἄνθρωπος', 'NOUN', '', '[{"Gloss":"human being"}]', '[]')`,
	)

	require.True(t, index.AncientGreekAvailable())
	assert.Zero(t, index.SkippedGreekHeadwords())

	// Every matching source entry is returned: both δέω homographs for VERB,
	// plus the lemma-only entry as weaker evidence. The discriminator is never
	// used, so both homographs survive one lookup.
	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB"})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, result.CandidateSenses, 3)
	glosses := []string{result.CandidateSenses[0].Gloss, result.CandidateSenses[1].Gloss, result.CandidateSenses[2].Gloss}
	assert.ElementsMatch(t, []string{"to bind", "to lack", "δέω without a part of speech"}, glosses)
	assert.Equal(t, "exact_lemma_pos", result.CandidateSenses[0].MatchStrength)
	assert.Equal(t, "exact_lemma_pos", result.CandidateSenses[1].MatchStrength)
	assert.Equal(t, "lemma_only_missing_pos", result.CandidateSenses[2].MatchStrength, "the lemma-only entry is weaker evidence")

	// A known different POS must not leak, but the lemma-only entry still does.
	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "δέω", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, result.CandidateSenses, 2)
	assert.Equal(t, "a binding", result.CandidateSenses[0].Gloss)
	assert.Equal(t, "exact_lemma_pos", result.CandidateSenses[0].MatchStrength)
	assert.Equal(t, "lemma_only_missing_pos", result.CandidateSenses[1].MatchStrength)

	// With no matching POS at all, only the lemma-only entry remains.
	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "δέω", UPOS: "ADJ"})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, result.CandidateSenses, 1)
	assert.Equal(t, "δέω without a part of speech", result.CandidateSenses[0].Gloss)

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "ἄνθρωπος", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, result.CandidateSenses, 1)
	assert.Equal(t, "human being", result.CandidateSenses[0].Gloss)

	_, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "ἄνθρωπος", UPOS: "VERB"})
	require.NoError(t, err)
	assert.False(t, found, "a non-matching POS with no lemma-only entry has no evidence")

	exists, err := index.LemmaExists(t.Context(), "grc", "δέω", "VERB")
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = index.LemmaExists(t.Context(), "grc", "δέω", "ADJ")
	require.NoError(t, err)
	assert.False(t, exists, "LemmaExists matches the exact part of speech only")
}

func TestIndexAncientGreekKeyNormalization(t *testing.T) {
	decomposed := "δε\u0301ω" // NFD of δέω
	index := newGreekTestIndex(t,
		`INSERT INTO greek_entries VALUES ('`+decomposed+`', 'VERB', '1', '[{"Gloss":"to bind"}]', '[]')`,
		`INSERT INTO greek_entries VALUES ('σοφός', 'ADJ', '', '[{"Gloss":"wise"}]', '[]')`,
	)

	// The stored headword and the request are both normalized to the same key.
	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "to bind", result.CandidateSenses[0].Gloss)

	// Final sigma is normalized to sigma, so the citation form with ς matches a
	// request spelled with σ.
	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "σοφόσ", UPOS: "ADJ"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "wise", result.CandidateSenses[0].Gloss)
}

func TestIndexAncientGreekUnavailableOnOlderIndex(t *testing.T) {
	index := newTestIndex(t, `INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house"}]', '', '', '', '', '')`)

	assert.False(t, index.AncientGreekAvailable(), "an index without the Ancient Greek format reports grc unavailable")

	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB"})
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, result.Senses)

	exists, err := index.LemmaExists(t.Context(), "grc", "δέω", "VERB")
	require.NoError(t, err)
	assert.False(t, exists, "an unavailable index reports no Ancient Greek lemma")

	// Modern-language lookups are unchanged by the Ancient Greek additions.
	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "house", result.Senses[0].Gloss)
}

func TestIndexAncientGreekLowerOrNonNumericFormatIsUnavailable(t *testing.T) {
	for _, format := range []string{"1", "not-a-number"} {
		path := filepath.Join(t.TempDir(), "dictionary.sqlite")
		db, err := sql.Open("sqlite", path)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO metadata VALUES ('index_format', ?)`, format)
		require.NoError(t, err)
		require.NoError(t, db.Close())

		index, err := OpenIndex(t.Context(), path)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, index.Close()) })
		assert.False(t, index.AncientGreekAvailable(), "index format %q does not carry Ancient Greek entries", format)
	}
}

func TestIndexAncientGreekFormatWithoutTableIsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO metadata VALUES ('index_format', '2')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := OpenIndex(t.Context(), path)

	require.Error(t, err)
	assert.Nil(t, index)
	require.ErrorIs(t, err, ErrInvalidIndex)
}

func TestIndexAncientGreekRejectsMalformedRows(t *testing.T) {
	// A nullable greek_entries table lets a NULL headword reach the row scan,
	// and a non-JSON senses value reaches the senses decode.
	const schema = `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); CREATE TABLE greek_entries (headword TEXT, upos TEXT, senses_json TEXT); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO metadata VALUES ('index_format', '2')`
	for name, row := range map[string]string{
		"null headword": `INSERT INTO greek_entries VALUES (NULL, 'NOUN', '[{"Gloss":"x"}]')`,
		"bad senses":    `INSERT INTO greek_entries VALUES ('δέω', 'VERB', 'not-json')`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "dictionary.sqlite")
			db, err := sql.Open("sqlite", path)
			require.NoError(t, err)
			_, err = db.ExecContext(t.Context(), schema)
			require.NoError(t, err)
			_, err = db.ExecContext(t.Context(), row)
			require.NoError(t, err)
			require.NoError(t, db.Close())

			index, err := OpenIndex(t.Context(), path)

			require.Error(t, err)
			assert.Nil(t, index)
			require.ErrorIs(t, err, ErrInvalidIndex)
		})
	}
}

func TestIndexAncientGreekAccessorsOnNilIndex(t *testing.T) {
	var absent *Index

	assert.False(t, absent.AncientGreekAvailable())
	assert.Zero(t, absent.SkippedGreekHeadwords())
}

func TestIndexModernLookupsUnchangedWithAncientGreekEntries(t *testing.T) {
	index := newGreekTestIndex(t,
		`INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house"}]', 'Neut', 'das', 'Häuser', '', '')`,
		`INSERT INTO greek_entries VALUES ('δέω', 'VERB', '1', '[{"Gloss":"to bind"}]', '[]')`,
	)

	require.True(t, index.AncientGreekAvailable())
	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "house", result.Senses[0].Gloss)

	result, found, err = index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "to bind", result.Senses[0].Gloss)
}

func TestIndexAncientGreekSkipsInadmissibleHeadwords(t *testing.T) {
	index := newGreekTestIndex(t,
		`INSERT INTO greek_entries VALUES ('λόγος', 'NOUN', '', '[{"Gloss":"word"}]', '[]')`,
		`INSERT INTO greek_entries VALUES ('', 'NOUN', '', '[{"Gloss":"empty"}]', '[]')`,
		`INSERT INTO greek_entries VALUES ('   ', 'NOUN', '', '[{"Gloss":"blank"}]', '[]')`,
		`INSERT INTO greek_entries VALUES ('·', 'PUNCT', '', '[{"Gloss":"punctuation"}]', '[]')`,
	)

	assert.True(t, index.AncientGreekAvailable())
	assert.Equal(t, 3, index.SkippedGreekHeadwords(), "empty, blank, and letterless headwords fail admission")

	result, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "word", result.CandidateSenses[0].Gloss)
}
