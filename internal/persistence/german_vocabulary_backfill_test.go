package persistence

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/stretchr/testify/require"
)

func TestMergeSelectionCandidatesRejectsInvalidMinimumOccurrences(t *testing.T) {
	for name, provenance := range map[string]string{
		"negative":   `{"min_occurrences":-1}`,
		"fractional": `{"min_occurrences":1.5}`,
		"overflow":   `{"min_occurrences":9223372036854775808}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := mergeSelectionCandidates([]selectionCandidateRow{{
				provenance: []byte(`{"min_occurrences": 1}`),
			}, {
				provenance: []byte(provenance),
			}})
			require.Error(t, err)
		})
	}
}

func TestCanonicalizeSelectedSentenceGroupsKeepsOnlyAffectedIdentities(t *testing.T) {
	profile := canonicalization.GermanPost1996()
	require.Equal(t, "aufstehen", profile.Canonical("aufstehen"))
	rows := []selectedSentenceRow{
		{corpusID: "second", lemma: "hass", upos: "NOUN"},
		{corpusID: "first", lemma: "haß", upos: "NOUN"},
		{corpusID: "second", lemma: "haß", upos: "NOUN"},
		{corpusID: "unused", lemma: "straße", upos: "NOUN"},
		{corpusID: "first", lemma: "hass", upos: "NOUN"},
	}

	groups := canonicalizeSelectedSentenceGroups(rows, profile)
	require.Len(t, groups, 2)
	require.Equal(t, "first", groups[0].corpusID)
	require.Equal(t, "hass", groups[0].canonicalLemma)
	require.Len(t, groups[0].rows, 2)
	require.Equal(t, "second", groups[1].corpusID)
	require.Len(t, groups[1].rows, 2)
}

func TestReconcileSelectedSentenceGroupPreservesChosenSentence(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	group := selectedSentenceGroup{
		canonicalLemma: "hass",
		upos:           "NOUN",
		rows: []selectedSentenceRow{
			{id: "later", lemma: "hass", selectionRank: 2, isChosen: true, createdAt: created.Add(time.Hour)},
			{id: "earlier", lemma: "haß", selectionRank: 1, createdAt: created},
		},
	}

	reconciled := reconcileSelectedSentenceGroup(group)
	require.Equal(t, "earlier", reconciled.rows[0].id)
	require.Equal(t, 1, reconciled.rows[0].selectionRank)
	require.False(t, reconciled.rows[0].isChosen)
	require.Equal(t, "later", reconciled.rows[1].id)
	require.Equal(t, 2, reconciled.rows[1].selectionRank)
	require.True(t, reconciled.rows[1].isChosen)
}
