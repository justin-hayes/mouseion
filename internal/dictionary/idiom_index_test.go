package dictionary

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// dictionaryIdiomFixtureSchema is the read-only schema of an index format that
// carries the idiom lookup alongside the tables OpenIndex loads at startup.
const dictionaryIdiomFixtureSchema = `
CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos));
CREATE TABLE greek_entries (headword TEXT NOT NULL, upos TEXT NOT NULL, etymology_number TEXT NOT NULL, senses_json TEXT NOT NULL, forms_json TEXT NOT NULL);
CREATE TABLE idiom_entries (language TEXT NOT NULL, headword TEXT NOT NULL, upos TEXT NOT NULL, words_json TEXT NOT NULL, senses_json TEXT NOT NULL, source TEXT NOT NULL, version TEXT NOT NULL, PRIMARY KEY(language, headword, upos, source));
CREATE TABLE idiom_words (language TEXT NOT NULL, word TEXT NOT NULL, headword TEXT NOT NULL, upos TEXT NOT NULL, source TEXT NOT NULL, PRIMARY KEY(language, word, headword, upos, source));
CREATE INDEX idiom_words_lookup ON idiom_words(language, word);
INSERT INTO metadata VALUES ('provider_version', 'fixture-v1');
INSERT INTO metadata VALUES ('index_format', '5')`

func newIdiomTestIndex(t *testing.T, seedStatements ...string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), dictionaryIdiomFixtureSchema)
	require.NoError(t, err)
	for _, statement := range seedStatements {
		_, err = db.ExecContext(t.Context(), statement)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	index, err := OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := index.Close(); err != nil {
			t.Errorf("idiom index cleanup failed: %v", err)
		}
	})
	return index
}

func TestIndexIdiomEntriesFindsInGambaFromEitherWord(t *testing.T) {
	index := newIdiomTestIndex(t,
		`INSERT INTO idiom_entries VALUES ('it', 'in gamba', 'ADJ', '["in","gamba"]', '[{"Gloss":"capable, on the ball"}]', 'wiktionary', 'fixture-v1')`,
		`INSERT INTO idiom_words VALUES ('it', 'in', 'in gamba', 'ADJ', 'wiktionary')`,
		`INSERT INTO idiom_words VALUES ('it', 'gamba', 'in gamba', 'ADJ', 'wiktionary')`,
	)

	for _, word := range []string{"in", "gamba", "GAMBA"} {
		t.Run(word, func(t *testing.T) {
			entries, err := index.IdiomEntries(t.Context(), "it", word)

			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, "in gamba", entries[0].Headword)
			assert.Equal(t, "ADJ", entries[0].UPOS)
			assert.Equal(t, []string{"in", "gamba"}, entries[0].Words)
			require.Len(t, entries[0].Senses, 1)
			assert.Equal(t, "capable, on the ball", entries[0].Senses[0].Gloss)
			assert.Equal(t, "wiktionary", entries[0].Source)
			assert.Equal(t, "fixture-v1", entries[0].Version)
		})
	}
}

func TestIndexIdiomEntriesKeepsTheReviewedListSource(t *testing.T) {
	index := newIdiomTestIndex(t,
		`INSERT INTO idiom_entries VALUES ('el', 'κάνω τα στραβά μάτια', '', '["κάνω","τα","στραβά","μάτια"]', '[]', 'reviewed-idioms', 'reviewed-idioms/1')`,
		`INSERT INTO idiom_words VALUES ('el', 'κάνω', 'κάνω τα στραβά μάτια', '', 'reviewed-idioms')`,
	)

	entries, err := index.IdiomEntries(t.Context(), "el", "κάνω")

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "κάνω τα στραβά μάτια", entries[0].Headword)
	assert.Empty(t, entries[0].UPOS)
	assert.Empty(t, entries[0].Senses)
	assert.Equal(t, "reviewed-idioms", entries[0].Source)
	assert.Equal(t, "reviewed-idioms/1", entries[0].Version)
}

func TestIndexIdiomEntriesRejectsMalformedJSON(t *testing.T) {
	for name, seed := range map[string][]string{
		"words": {
			`INSERT INTO idiom_entries VALUES ('it', 'in gamba', 'ADJ', 'not-json', '[]', 'wiktionary', 'fixture-v1')`,
			`INSERT INTO idiom_words VALUES ('it', 'gamba', 'in gamba', 'ADJ', 'wiktionary')`,
		},
		"senses": {
			`INSERT INTO idiom_entries VALUES ('it', 'in gamba', 'ADJ', '["in","gamba"]', 'not-json', 'wiktionary', 'fixture-v1')`,
			`INSERT INTO idiom_words VALUES ('it', 'gamba', 'in gamba', 'ADJ', 'wiktionary')`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			index := newIdiomTestIndex(t, seed...)

			entries, err := index.IdiomEntries(t.Context(), "it", "gamba")

			require.Error(t, err)
			assert.Nil(t, entries)
			assert.Contains(t, err.Error(), name)
		})
	}
}

func TestIndexIdiomEntriesRejectsInvalidIdentity(t *testing.T) {
	t.Run("nil index", func(t *testing.T) {
		var nilIndex *Index
		entries, err := nilIndex.IdiomEntries(t.Context(), "it", "gamba")
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	index := newIdiomTestIndex(t,
		`INSERT INTO idiom_entries VALUES ('it', 'in gamba', 'ADJ', '["in","gamba"]', '[]', 'wiktionary', 'fixture-v1')`,
		`INSERT INTO idiom_words VALUES ('it', 'gamba', 'in gamba', 'ADJ', 'wiktionary')`,
	)
	for _, test := range []struct {
		name     string
		language string
		word     string
	}{
		{name: "empty word", language: "it", word: ""},
		{name: "empty language", language: "", word: "gamba"},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries, err := index.IdiomEntries(t.Context(), test.language, test.word)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

// The derived index stores idiom senses without evidence IDs, and the idiom
// prompt can only confirm a sense it can cite (#2385).
func TestIndexIdiomEntriesGiveEverySenseAStableEvidenceID(t *testing.T) {
	index := newIdiomTestIndex(t,
		`INSERT INTO idiom_entries VALUES ('it', 'fare bene', 'VERB', '["fare","bene"]', '[{"Gloss":"to do well"},{"Gloss":"to do the right thing"},{"Gloss":"to do well"},{"evidence_id":"reviewed:fare bene:1","gloss":"to be good for"}]', 'wiktionary', 'fixture-v1')`,
		`INSERT INTO idiom_entries VALUES ('it', 'fare bene', 'NOUN', '["fare","bene"]', '[{"Gloss":"to do well"}]', 'wiktionary', 'fixture-v1')`,
		`INSERT INTO idiom_words VALUES ('it', 'bene', 'fare bene', 'VERB', 'wiktionary')`,
		`INSERT INTO idiom_words VALUES ('it', 'bene', 'fare bene', 'NOUN', 'wiktionary')`,
	)

	first, err := index.IdiomEntries(t.Context(), "it", "bene")
	require.NoError(t, err)
	second, err := index.IdiomEntries(t.Context(), "it", "bene")
	require.NoError(t, err)

	require.Len(t, first, 2)
	noun, verb := first[0], first[1]
	require.Equal(t, "VERB", verb.UPOS)
	require.Len(t, verb.Senses, 3, "a sense that repeats an evidence identity is dropped")
	assert.Equal(t, []string{"to do well", "to do the right thing", "to be good for"}, []string{verb.Senses[0].Gloss, verb.Senses[1].Gloss, verb.Senses[2].Gloss})
	assert.Equal(t, "reviewed:fare bene:1", verb.Senses[2].EvidenceID, "an explicit evidence ID is kept")
	require.Len(t, noun.Senses, 1)

	seen := map[string]struct{}{}
	for _, sense := range append(append([]enrichment.LexicalSense(nil), verb.Senses...), noun.Senses...) {
		assert.NotEmpty(t, sense.EvidenceID, "sense %q", sense.Gloss)
		assert.Equal(t, "wiktionary", sense.Source, "sense %q", sense.Gloss)
		assert.Equal(t, "fixture-v1", sense.Version, "sense %q", sense.Gloss)
		_, duplicate := seen[sense.EvidenceID]
		assert.False(t, duplicate, "evidence ID %q is repeated", sense.EvidenceID)
		seen[sense.EvidenceID] = struct{}{}
	}
	assert.Equal(t, first, second, "the same entry yields the same evidence IDs")
}

func TestIndexIdiomEntriesUnknownWordAndOlderIndexAreEmpty(t *testing.T) {
	t.Run("unknown word", func(t *testing.T) {
		index := newIdiomTestIndex(t,
			`INSERT INTO idiom_entries VALUES ('it', 'in gamba', 'ADJ', '["in","gamba"]', '[]', 'wiktionary', 'fixture-v1')`,
			`INSERT INTO idiom_words VALUES ('it', 'gamba', 'in gamba', 'ADJ', 'wiktionary')`,
		)

		entries, err := index.IdiomEntries(t.Context(), "it", "casa")
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("older index", func(t *testing.T) {
		older := newTestIndex(t)

		entries, err := older.IdiomEntries(t.Context(), "it", "gamba")
		require.NoError(t, err)
		assert.Empty(t, entries, "an index without the idiom lookup reports no candidates")
	})
}

func TestIndexHasIdiomEntriesReportsTheIdiomTables(t *testing.T) {
	t.Run("index with idiom tables", func(t *testing.T) {
		assert.True(t, newIdiomTestIndex(t).HasIdiomEntries())
	})
	t.Run("older index", func(t *testing.T) {
		assert.False(t, newTestIndex(t).HasIdiomEntries())
	})
	t.Run("nil index", func(t *testing.T) {
		var nilIndex *Index
		assert.False(t, nilIndex.HasIdiomEntries())
	})
}

// Wiktionary lists di fronte as both an adjective and an adverb. The lookup
// returns both rows, and the matcher pools them into one candidate, so the
// adverb senses reach the provider (#2378).
func TestIndexIdiomEntriesFeedTheMatcherEveryPartOfSpeech(t *testing.T) {
	index := newIdiomTestIndex(t,
		`INSERT INTO idiom_entries VALUES ('it', 'di fronte', 'ADJ', '["di","fronte"]', '[{"Gloss":"front, fore"}]', 'wiktionary', 'fixture-v1')`,
		`INSERT INTO idiom_entries VALUES ('it', 'di fronte', 'ADV', '["di","fronte"]', '[{"Gloss":"opposite"},{"Gloss":"in front of"}]', 'wiktionary', 'fixture-v1')`,
		`INSERT INTO idiom_words VALUES ('it', 'fronte', 'di fronte', 'ADJ', 'wiktionary')`,
		`INSERT INTO idiom_words VALUES ('it', 'fronte', 'di fronte', 'ADV', 'wiktionary')`,
	)
	entries, err := index.IdiomEntries(t.Context(), "it", "fronte")
	require.NoError(t, err)
	t.Run("lookup returns both parts of speech", func(t *testing.T) {
		require.Len(t, entries, 2)
	})

	tokens := []idiom.Token{
		{Ordinal: 0, Surface: "di", CanonicalLemma: "di"},
		{Ordinal: 1, Surface: "fronte", CanonicalLemma: "fronte"},
	}
	anchors := []idiom.Anchor{{TokenOrdinal: 1, Identity: idiom.AnchorIdentity{Language: "it", CanonicalLemma: "fronte", UPOS: "NOUN"}}}
	candidates := idiom.Match(idiom.DefaultParameters(), tokens, anchors, entries)
	t.Run("matcher pools the senses into one candidate", func(t *testing.T) {
		require.Len(t, candidates, 1)
		glosses := make([]string, 0, len(candidates[0].Entry.Senses))
		for _, sense := range candidates[0].Entry.Senses {
			glosses = append(glosses, sense.Gloss)
		}
		require.Equal(t, []string{"front, fore", "opposite", "in front of"}, glosses)
	})
}
