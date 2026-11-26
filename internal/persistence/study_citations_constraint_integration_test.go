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
// entry, or with an entry count that differs from the senses, whatever the Go layer does (spec
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
		Cited: domain.StudyCitations{SenseIDs: []string{"n1.1", "n1.2"}, EntryIDs: []string{"n1", "n1"}, DictionaryPin: "lsj-1"},
	}))

	update := func(set string) error {
		_, err := store.Pool().Exec(ctx, `UPDATE study_material SET `+set+` WHERE owner_id=$1 AND id=$2`, owner.ID, id)
		return err
	}
	tests := []struct {
		name  string
		set   string
		valid bool
	}{
		{"a cited sense names its entry", `cited_entry_ids='{}'`, false},
		{"each sense names exactly one entry", `cited_entry_ids='{n1}'`, false},
		{"no more entries than senses", `cited_entry_ids='{n1,n2,n3}'`, false},
		{"a pin needs a cited sense", `cited_sense_ids='{}', cited_entry_ids='{}'`, false},
		{"a cited sense needs a non-blank pin", `dictionary_pin=' '`, false},
		{"a Gloss may cite across entries", `cited_sense_ids='{n1.1,n2.1}', cited_entry_ids='{n1,n2}'`, true},
		{"homographs may share a sense ID", `cited_sense_ids='{s1,s1}', cited_entry_ids='{n1,n2}'`, true},
		{"a ready row with no pin is a modern row", `cited_sense_ids='{}', cited_entry_ids='{}', dictionary_pin=''`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.valid {
				require.NoError(t, update(tc.set))
			} else {
				require.Error(t, update(tc.set))
			}
		})
	}
}
