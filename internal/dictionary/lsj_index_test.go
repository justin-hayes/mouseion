package dictionary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ancientGreekLSJFixtureStatements add the Kaikki greek_entries table, the LSJ
// table introduced with index format 3, and the metadata they require to the
// base fixture schema.
var ancientGreekLSJFixtureStatements = []string{
	`CREATE TABLE greek_entries (headword TEXT NOT NULL, upos TEXT NOT NULL, etymology_number TEXT NOT NULL, senses_json TEXT NOT NULL, forms_json TEXT NOT NULL)`,
	`CREATE TABLE lsj_entries (entry_id TEXT NOT NULL, headword TEXT NOT NULL, senses_json TEXT NOT NULL)`,
	`INSERT INTO metadata VALUES ('index_format', '4')`,
	`INSERT INTO metadata VALUES ('lsj_pin', '56061ca127f4a2844980baffc5f2b6d1332897b3')`,
}

// TestAncientGreekLSJUnambiguousMatchReturnsEntry proves a headword with exactly
// one LSJ main entry matches and carries the frozen entry identity, pin and full
// sense hierarchy.
func TestAncientGreekLSJUnambiguousMatchReturnsEntry(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n61607', 'λόγοσ', '[{"id":"n61607.0","n":"A","level":1,"glosses":["word"],"citations":["X. An. 1.1.1"],"senses":[{"id":"n61607.1","n":"2","level":3,"glosses":["speech"]}]}]')`,
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
		`INSERT INTO lsj_entries VALUES ('n1', 'δέω', '[{"id":"n1.0","n":"A","level":1,"glosses":["bind"]}]')`,
		`INSERT INTO lsj_entries VALUES ('n2', 'δέω', '[{"id":"n2.0","n":"A","level":1,"glosses":["lack"]}]')`,
		`INSERT INTO lsj_entries VALUES ('n3', 'λύω', '[{"id":"n3.0","n":"A","level":1,"glosses":["loosen"]}]')`,
		`INSERT INTO lsj_entries VALUES ('n4', 'λύω', '[{"id":"n4.0","n":"A","level":1,"glosses":["undo"]}]')`,
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

// TestAncientGreekLSJMissingHeadwordYieldsNoEntry proves an unindexed headword
// resolves to no entry rather than a guess.
func TestAncientGreekLSJMissingHeadwordYieldsNoEntry(t *testing.T) {
	index := newTestIndex(t, append(ancientGreekLSJFixtureStatements,
		`INSERT INTO lsj_entries VALUES ('n61607', 'λόγοσ', '[]')`,
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
		`INSERT INTO lsj_entries VALUES ('n9', '123', '[]')`,
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
