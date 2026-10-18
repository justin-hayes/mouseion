package dictionary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ancientGreekLSJFixtureStatements add the Kaikki greek_entries table, the LSJ
// table introduced with index format 4 (and its sub-entry column in format 5),
// and the metadata they require to the base fixture schema.
var ancientGreekLSJFixtureStatements = []string{
	`CREATE TABLE greek_entries (headword TEXT NOT NULL, upos TEXT NOT NULL, etymology_number TEXT NOT NULL, senses_json TEXT NOT NULL, forms_json TEXT NOT NULL)`,
	`CREATE TABLE lsj_entries (entry_id TEXT NOT NULL, headword TEXT NOT NULL, senses_json TEXT NOT NULL, subordinate INTEGER NOT NULL)`,
	`INSERT INTO metadata VALUES ('index_format', '5')`,
	`INSERT INTO metadata VALUES ('lsj_pin', '56061ca127f4a2844980baffc5f2b6d1332897b3')`,
}

// TestAncientGreekLSJUnambiguousMatchReturnsEntry proves a headword with exactly
// one LSJ main entry matches and carries the frozen entry identity, pin and full
// sense hierarchy.
func TestAncientGreekLSJUnambiguousMatchReturnsEntry(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n61607', 'λόγοσ', '[{"id":"n61607.0","n":"A","level":1,"glosses":["word"],"citations":["X. An. 1.1.1"],"senses":[{"id":"n61607.1","n":"2","level":3,"glosses":["speech"]}]}]', 0)`,
	)...)

	entry, found := index.LookupAncientGreekLSJ(t.Context(), "λόγος")

	require.True(t, found)
	assert.Equal(t, "n61607", entry.EntryID)
	assert.Equal(t, "56061ca127f4a2844980baffc5f2b6d1332897b3", entry.Pin)
	require.Len(t, entry.Senses, 1)
	assert.Equal(t, "n61607.0", entry.Senses[0].ID)
	assert.Equal(t, []string{"word"}, entry.Senses[0].Glosses)
	require.Len(t, entry.Senses[0].Senses, 1)
	assert.Equal(t, "n61607.1", entry.Senses[0].Senses[0].ID)
}

// TestAncientGreekLSJHomographsNeverBorrowEachOther proves a headword with more
// than one LSJ main entry is unresolved: neither a numbered identity nor a bare
// one borrows the other's entry, and the OGA morpheus number is never equated
// with the LSJ homograph number.
func TestAncientGreekLSJHomographsNeverBorrowEachOther(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n1', 'δέω', '[{"id":"n1.0","n":"A","level":1,"glosses":["bind"]}]', 0)`,
		`INSERT INTO lsj_entries VALUES ('n2', 'δέω', '[{"id":"n2.0","n":"A","level":1,"glosses":["lack"]}]', 0)`,
		`INSERT INTO lsj_entries VALUES ('n3', 'λύω', '[{"id":"n3.0","n":"A","level":1,"glosses":["loosen"]}]', 0)`,
		`INSERT INTO lsj_entries VALUES ('n4', 'λύω', '[{"id":"n4.0","n":"A","level":1,"glosses":["undo"]}]', 0)`,
	)...)

	for _, test := range []struct {
		name  string
		lemma string
	}{
		{name: "two numbered homographs", lemma: "δέω"},
		{name: "bare lemma with a numbered sibling", lemma: "λύω"},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry, found := index.LookupAncientGreekLSJ(t.Context(), test.lemma)
			require.False(t, found, "an ambiguous headword must yield no entry, got %+v", entry)
			assert.True(t, entry.Empty())
		})
	}
}

// TestAncientGreekLSJSubordinateSubEntriesNeverCompete proves a phrase sub-entry
// and a cross-reference redirect stub filed under the bare word's key do not
// make the bare lemma ambiguous, so its own entry is matched.
func TestAncientGreekLSJSubordinateSubEntriesNeverCompete(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n75730', 'οὐ', '[{"id":"n75730.0","n":"A","level":1,"glosses":["not"]}]', 0)`,
		`INSERT INTO lsj_entries VALUES ('n75743', 'οὐ', '[{"id":"n75743.0","n":"A","level":1,"glosses":["for not"]}]', 1)`,
		`INSERT INTO lsj_entries VALUES ('n75744', 'οὐ', '[{"id":"n75744.0","n":"A","level":1,"glosses":["no indeed"]}]', 1)`,
		`INSERT INTO lsj_entries VALUES ('n46646', 'ἤδη', '[{"id":"n46646.0","n":"1","level":1,"glosses":["already"]}]', 0)`,
		`INSERT INTO lsj_entries VALUES ('n46647', 'ἤδη', '[{"id":"n46647.0","n":"A","level":1,"glosses":[]}]', 1)`,
	)...)

	t.Run("phrase sub-entries", func(t *testing.T) {
		entry, found := index.LookupAncientGreekLSJ(t.Context(), "οὐ")
		require.True(t, found)
		assert.Equal(t, "n75730", entry.EntryID)
	})
	t.Run("redirect stub", func(t *testing.T) {
		entry, found := index.LookupAncientGreekLSJ(t.Context(), "ἤδη")
		require.True(t, found)
		assert.Equal(t, "n46646", entry.EntryID)
	})
}

// TestAncientGreekLSJProperNameDoesNotMakeLowercaseAmbiguous proves a capitalised
// proper-name entry does not make a lowercase common-word lemma ambiguous, while
// a capitalised lemma still resolves its own proper-name entry.
func TestAncientGreekLSJProperNameDoesNotMakeLowercaseAmbiguous(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n24340', 'δῆλοσ', '[{"id":"n24340.0","n":"A","level":1,"glosses":["clear, visible"]}]', 0)`,
		`INSERT INTO lsj_entries VALUES ('n24339', 'Δῆλοσ', '[{"id":"n24339.0","n":"A","level":1,"glosses":["Delos"]}]', 0)`,
	)...)

	t.Run("lowercase lemma keeps the common word", func(t *testing.T) {
		entry, found := index.LookupAncientGreekLSJ(t.Context(), "δῆλος")
		require.True(t, found)
		assert.Equal(t, "n24340", entry.EntryID)
	})
	t.Run("capitalised lemma keeps the proper name", func(t *testing.T) {
		entry, found := index.LookupAncientGreekLSJ(t.Context(), "Δῆλος")
		require.True(t, found)
		assert.Equal(t, "n24339", entry.EntryID)
	})
}

// TestAncientGreekLSJLoneCapitalisedEntryIsStillMatched proves a capitalised
// entry with no lowercase sibling is still the lowercase lemma's evidence, so a
// common adjective written with a capital resolves rather than vanishing.
func TestAncientGreekLSJLoneCapitalisedEntryIsStillMatched(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n33822', 'Ἑλληνικόσ', '[{"id":"n33822.0","n":"A","level":1,"glosses":["of Greece"]}]', 0)`,
	)...)

	entry, found := index.LookupAncientGreekLSJ(t.Context(), "ἑλληνικός")

	require.True(t, found)
	assert.Equal(t, "n33822", entry.EntryID)
}

// TestAncientGreekLSJMissingHeadwordYieldsNoEntry proves an unindexed headword
// resolves to no entry rather than a guess.
func TestAncientGreekLSJMissingHeadwordYieldsNoEntry(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n61607', 'λόγοσ', '[]', 0)`,
	)...)

	entry, found := index.LookupAncientGreekLSJ(t.Context(), "ἄνθρωπος")

	require.False(t, found)
	assert.True(t, entry.Empty())
}

// TestAncientGreekLSJSkippedHeadwordIsCounted proves an LSJ headword whose
// ancient-greek/1 key is not admissible is skipped and counted rather than
// matched.
func TestAncientGreekLSJSkippedHeadwordIsCounted(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n9', '123', '[]', 0)`,
	)...)

	entry, found := index.LookupAncientGreekLSJ(t.Context(), "123")

	require.False(t, found)
	assert.True(t, entry.Empty())
	assert.Equal(t, 1, index.SkippedLSJHeadwords())
}

// TestAncientGreekLSJUnavailableBeforeFormat proves an older index reports the
// LSJ path unavailable and matches nothing.
func TestAncientGreekLSJUnavailableBeforeFormat(t *testing.T) {
	index := newTestIndex(t)

	t.Run("reports unavailable", func(t *testing.T) {
		require.False(t, index.AncientGreekLSJAvailable())
	})
	t.Run("matches nothing", func(t *testing.T) {
		entry, found := index.LookupAncientGreekLSJ(t.Context(), "λόγος")
		require.False(t, found)
		assert.True(t, entry.Empty())
	})
}

// TestAncientGreekLSJNilIndexIsSafe proves the optional provider seam tolerates
// a nil index.
func TestAncientGreekLSJNilIndexIsSafe(t *testing.T) {
	var index *Index

	t.Run("reports unavailable", func(t *testing.T) {
		require.False(t, index.AncientGreekLSJAvailable())
	})
	t.Run("counts no skipped headwords", func(t *testing.T) {
		require.Zero(t, index.SkippedLSJHeadwords())
	})
	t.Run("matches nothing", func(t *testing.T) {
		entry, found := index.LookupAncientGreekLSJ(t.Context(), "λόγος")
		require.False(t, found)
		assert.True(t, entry.Empty())
	})
}
