package main

import (
	"bytes"
	"testing"

	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportBackfillFailsWhileConflictsRemain(t *testing.T) {
	var out bytes.Buffer
	report := persistence.GermanVocabularyBackfillReport{
		Owners: 2, Examined: 5, Updated: 3, Merged: 1,
		Conflicts: []persistence.GermanVocabularyBackfillConflict{{OwnerID: "owner-1", Table: "vocabulary", CanonicalLemma: "Weg", UPOS: "NOUN", Detail: "two identities"}},
	}
	err := reportBackfill(report, &out)
	require.EqualError(t, err, "vocabularybackfill: German vocabulary backfill found unresolved conflicts")
	assert.Equal(t, "German vocabulary backfill complete: owners=2 examined=5 updated=3 merged=1 conflicts=1\n", out.String())
}

func TestReportBackfillSucceedsWithoutConflicts(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, reportBackfill(persistence.GermanVocabularyBackfillReport{Owners: 1}, &out))
	assert.Contains(t, out.String(), "conflicts=0")
}
