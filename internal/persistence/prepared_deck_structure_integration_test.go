//go:build integration

package persistence

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreparedDeckInputFactsLoadExactSourceStructure proves preparation reads
// the exact snapshot's roles and unit text, including for the attribution of a
// sentence that will be frozen later.
func TestPreparedDeckInputFactsLoadExactSourceStructure(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "prepared-deck-structure", false)
	require.NoError(t, err)

	label := speakerTestSentence(t, "Erster.", 0, []speakerTestToken{
		{surface: "Erster", lemma: "Erster", upos: "PROPN", dependency: "root"},
	})
	sentence := speakerTestSentence(t, "Der Dichter spricht heute sehr laut.", label.End+1, []speakerTestToken{
		{surface: "Der", lemma: "der", upos: "DET", dependency: "det"},
		{surface: "Dichter", lemma: "Dichter", upos: "NOUN", dependency: "nsubj"},
		{surface: "spricht", lemma: "sprechen", upos: "VERB", dependency: "root"},
		{surface: "heute", lemma: "heute", upos: "ADV", dependency: "advmod"},
		{surface: "sehr", lemma: "sehr", upos: "ADV", dependency: "advmod"},
		{surface: "laut", lemma: "laut", upos: "ADJ", dependency: "advmod"},
	})
	text := label.Text + "\n" + sentence.Text
	textLength, err := checked.Uint64FromInt(utf8.RuneCountInString(text))
	require.NoError(t, err)
	unit := concordanceUnit(0, "prepared-deck-structure", text, 0, textLength)
	label.UnitID, label.Ordinal = unit.ID, 0
	sentence.UnitID, sentence.Ordinal = unit.ID, 1
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Prepared structure", "prepared-deck-structure", true, []domain.ExtractedUnit{unit})

	snapshotID := insertStructureSnapshot(t, ctx, store.Pool(), owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, []domain.ExtractedUnit{unit})
	_, err = store.Pool().Exec(ctx, `UPDATE source_materials SET current_snapshot_id=$1 WHERE owner_id=$2 AND id=$3`, snapshotID, owner.ID, source.ID)
	require.NoError(t, err)
	require.NoError(t, insertRoleSpan(t, ctx, store.Pool(), owner.ID, source.ID, snapshotID, unit.ID, string(domain.SourceStructureRoleSpeaker), 0, int(label.End)))

	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{label, sentence})
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID))
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID,
		Filename: "prepared-structure.apkg", DeckName: "Prepared structure", ContentHash: source.ContentHash,
	})
	require.NoError(t, err)

	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	facts, err := store.LoadPreparedDeckInputFactsTx(ctx, tx, preparation)
	require.NoError(t, err)
	assert.True(t, facts.Structure.Recorded)
	require.Len(t, facts.Structure.RoleSpans, 1)
	assert.Equal(t, domain.SourceStructureRoleSpeaker, facts.Structure.RoleSpans[0].Role)
	require.Contains(t, facts.SourceUnits, unit.ID)
	assert.Equal(t, text, facts.SourceUnits[unit.ID].Text)
}
