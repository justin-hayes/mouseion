package dictionary

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// latinLSFixtureStatements add the Lewis & Short table and its pin to the base
// fixture schema.
var latinLSFixtureStatements = []string{
	`CREATE TABLE greek_entries (headword TEXT NOT NULL, upos TEXT NOT NULL, etymology_number TEXT NOT NULL, senses_json TEXT NOT NULL, forms_json TEXT NOT NULL)`,
	`CREATE TABLE ls_entries (entry_id TEXT NOT NULL, headword TEXT NOT NULL, orth TEXT NOT NULL, senses_json TEXT NOT NULL)`,
	`INSERT INTO metadata VALUES ('index_format', '7')`,
	`INSERT INTO metadata VALUES ('ls_pin', 'lspin')`,
}

func TestLatinLSLookupReturnsEveryHomographInIDOrder(t *testing.T) {
	index := newTestIndex(t, append(latinLSFixtureStatements,
		// Inserted out of order; n10 must follow n9 although "n10" < "n9" as text.
		`INSERT INTO ls_entries VALUES ('n10', 'dico', 'dīco', '[{"id":"n10.0","n":"I","level":1,"definitions":["To dedicate"],"glosses":["To dedicate"],"citations":[{"author":"Cic.","text":"Cic. Att. 1, 2"},{"text":"4, 5"}]}]')`,
		`INSERT INTO ls_entries VALUES ('n9', 'dico', 'dīco', '[{"id":"n9.0","n":"I","level":1,"definitions":["To say"],"glosses":["To say"],"crossrefs":[{"text":"dicto","entry_id":"n11","headword":"dicto"},{"text":"infra"}]}]')`,
		`INSERT INTO ls_entries VALUES ('n11', 'dicto', 'dictō', '[]')`,
	)...)

	entries, found := index.LookupLatinLS(t.Context(), "dīcō")

	require.True(t, found)
	require.Len(t, entries, 2)
	assert.Equal(t, "n9", entries[0].EntryID)
	assert.Equal(t, "n10", entries[1].EntryID)
	assert.Equal(t, "lspin", entries[0].Pin)
	assert.Equal(t, "dīco", entries[0].Orth)
	assert.Equal(t, []string{"To say"}, entries[0].Senses[0].Definitions)
	assert.Equal(t, "n11", entries[0].Senses[0].CrossRefs[0].EntryID)
	assert.Empty(t, entries[0].Senses[0].CrossRefs[1].EntryID)
	assert.Equal(t, "Cic.", entries[1].Senses[0].Citations[0].Author)
	assert.Empty(t, entries[1].Senses[0].Citations[1].Author)
}

func TestLatinLSLookupKeepsAnEntryWithNoSenses(t *testing.T) {
	index := newTestIndex(t, append(latinLSFixtureStatements,
		`INSERT INTO ls_entries VALUES ('n11', 'dicto', 'dictō', '[]')`,
	)...)

	entries, found := index.LookupLatinLS(t.Context(), "dicto")

	require.True(t, found)
	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].Senses)
	assert.False(t, entries[0].Empty())
}

func TestLatinLSLookupUsesTheLatinKey(t *testing.T) {
	index := newTestIndex(t, append(latinLSFixtureStatements,
		`INSERT INTO ls_entries VALUES ('n1', 'Iuppiter', 'Iuppĭter', '[]')`,
		`INSERT INTO ls_entries VALUES ('n2', 'uenio', 'uĕnio', '[]')`,
	)...)

	for _, lemma := range []string{"Juppiter", "iuppiter"} {
		_, found := index.LookupLatinLS(t.Context(), lemma)
		assert.True(t, found, lemma)
	}
	_, found := index.LookupLatinLS(t.Context(), "venio")
	assert.True(t, found, "u/v spellings share a key")
	_, found = index.LookupLatinLS(t.Context(), "absent")
	assert.False(t, found)
	_, found = index.LookupLatinLS(t.Context(), "")
	assert.False(t, found)
}

func TestLatinLSUnavailableWithoutThePin(t *testing.T) {
	index := newTestIndex(t)

	assert.False(t, index.LatinLSAvailable())
	_, found := index.LookupLatinLS(t.Context(), "dico")
	assert.False(t, found)
	assert.False(t, ((*Index)(nil)).LatinLSAvailable())
}

func TestLatinLSPinWithoutTheTableIsAnInvalidIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	for _, statement := range []string{dictionaryFixtureSchema, `CREATE TABLE greek_entries (headword TEXT NOT NULL, upos TEXT NOT NULL, etymology_number TEXT NOT NULL, senses_json TEXT NOT NULL, forms_json TEXT NOT NULL)`, `INSERT INTO metadata VALUES ('index_format', '7')`, `INSERT INTO metadata VALUES ('ls_pin', 'lspin')`} {
		_, err = db.ExecContext(t.Context(), statement)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())

	_, err = OpenIndex(t.Context(), path)

	require.ErrorIs(t, err, ErrInvalidIndex)
}

func TestLatinLSDoesNotChangeGreekLSJ(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`CREATE TABLE ls_entries (entry_id TEXT NOT NULL, headword TEXT NOT NULL, orth TEXT NOT NULL, senses_json TEXT NOT NULL)`,
		`INSERT INTO metadata VALUES ('ls_pin', 'lspin')`,
		`INSERT INTO lsj_entries VALUES ('n1', 'δέω', '[{"id":"n1.0","glosses":["bind"]}]', 0)`,
		`INSERT INTO ls_entries VALUES ('n9', 'dico', 'dīco', '[]')`,
	)...)

	entry, found := index.LookupAncientGreekLSJ(t.Context(), "δέω")
	assert.True(t, found)
	assert.Equal(t, "n1", entry.EntryID)
	assert.True(t, index.LatinLSAvailable())
}

func TestLatinLSNoticeNamesThePinAndLicence(t *testing.T) {
	assert.Contains(t, LatinLSNotice(" abc "), "PerseusDL/lexica@abc")
	assert.Contains(t, LatinLSNotice(""), "CC BY-SA 4.0")
	assert.NotContains(t, LatinLSNotice(""), "@")
}

func TestLatinLSLookupReadsTheSourceHomographNumberFromFormatEight(t *testing.T) {
	statements := []string{
		`CREATE TABLE greek_entries (headword TEXT NOT NULL, upos TEXT NOT NULL, etymology_number TEXT NOT NULL, senses_json TEXT NOT NULL, forms_json TEXT NOT NULL)`,
		`CREATE TABLE ls_entries (entry_id TEXT NOT NULL, headword TEXT NOT NULL, orth TEXT NOT NULL, senses_json TEXT NOT NULL, homograph INTEGER NOT NULL)`,
		`INSERT INTO metadata VALUES ('index_format', '8')`,
		`INSERT INTO metadata VALUES ('ls_pin', 'lspin')`,
		`INSERT INTO ls_entries VALUES ('n20', 'Pennus', 'Pennus', '[]', 2)`,
	}
	entries, found := newTestIndex(t, statements...).LookupLatinLS(t.Context(), "Pennus")

	require.True(t, found)
	require.Len(t, entries, 1)
	assert.Equal(t, 2, entries[0].Homograph)
}

func TestLatinGenericLookupsNeverEnterTheGreekIndex(t *testing.T) {
	index := newTestIndex(t, append(latinLSFixtureStatements,
		`INSERT INTO greek_entries VALUES ('dico', 'VERB', '', '[{"id":"g","glosses":["greek gloss"]}]', '[]')`,
		`INSERT INTO ls_entries VALUES ('n9', 'dico', 'dīco', '[]')`,
		`INSERT INTO ls_entries VALUES ('n10', 'dico', 'dīco', '[]')`,
	)...)

	t.Run("the generic lookup finds no Greek entry for a Latin lemma", func(t *testing.T) {
		_, found, err := index.Lookup(t.Context(), enrichment.LexicalLookupRequest{Language: "la", CanonicalLemma: "dico", UPOS: "VERB"})
		require.NoError(t, err)
		assert.False(t, found)
	})
}

func TestLatinLSPinIsTheIndexPinWithoutALookup(t *testing.T) {
	index := newTestIndex(t, latinLSFixtureStatements...)
	assert.Equal(t, "lspin", index.LatinLSPin())
	assert.Empty(t, index.AncientGreekLSJPin(), "an index with no LSJ table has no LSJ pin")
	var none *Index
	assert.Empty(t, none.LatinLSPin())
}
