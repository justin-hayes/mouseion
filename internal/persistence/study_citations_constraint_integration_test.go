//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestStudyMaterialReadyRowCitationsAreEnforcedByTheDatabase holds that a ready
// row carrying a dictionary pin cannot be stored without a cited sense, its
// entry, or with more entries than senses, whatever the Go layer does (spec
// #2759): the invariant lives in the table.
func TestStudyMaterialReadyRowCitationsAreEnforcedByTheDatabase(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "study-citations-constraint", false)
	require.NoError(t, err)
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Citations", "study-citations", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", "Das Haus steht.", 0, 15),
	})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 0, Text: "Das Haus steht.", Start: 0, End: 15, Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "root", Head: 0, Morphology: map[string]string{}, Start: 4, End: 8},
		}},
	})
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner.ID, book.ID)
	id, _, err := store.ReserveStudyMaterial(ctx, owner.ID, domain.StudyIdentity{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"})
	require.NoError(t, err)
	require.NoError(t, store.RecordStudyMaterialReady(ctx, owner.ID, id, domain.StudyMaterialReadyContent{
		BookID: book.ID, AnalysisRunID: run, CorpusID: corpus, Sentence: "Das Haus steht.", TargetWord: "Haus",
		Gloss: "house", SentenceTranslation: "The house stands.",
		Cited: domain.StudyCitations{SenseIDs: []string{"n1.1", "n1.2"}, EntryIDs: []string{"n1"}, DictionaryPin: "lsj-1"},
	}))

	update := func(set string) error {
		_, err := store.Pool().Exec(ctx, `UPDATE study_material SET `+set+` WHERE owner_id=$1 AND id=$2`, owner.ID, id)
		return err
	}
	require.Error(t, update(`cited_entry_ids='{}'`), "a cited sense names its entry")
	require.Error(t, update(`cited_entry_ids='{n1,n2,n3}'`), "no more entries than senses")
	require.Error(t, update(`cited_sense_ids='{}', cited_entry_ids='{}'`), "a pin needs a cited sense")
	require.Error(t, update(`dictionary_pin=' '`), "a cited sense needs a non-blank pin")
	require.NoError(t, update(`cited_sense_ids='{n1.1,n2.1}', cited_entry_ids='{n1,n2}'`), "a Gloss may cite across entries")
	require.NoError(t, update(`cited_sense_ids='{}', cited_entry_ids='{}', dictionary_pin=''`), "a ready row with no pin is a modern row")
}
