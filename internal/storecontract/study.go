package storecontract

import (
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func studyNoun(lemma string) domain.StudyIdentity {
	return domain.StudyIdentity{Language: Language, CanonicalLemma: lemma, UPOS: "NOUN"}
}

func studyReadyContent() domain.StudyMaterialReadyContent {
	return domain.StudyMaterialReadyContent{
		BookID: uuid.NewString(), AnalysisRunID: uuid.NewString(), CorpusID: uuid.NewString(),
		SentenceOrdinal: 4, TokenOrdinal: 2,
		Sentence: "Das Haus steht am Ende der Strasse.", TargetWord: "Haus",
		Gloss: "a building", SentenceTranslation: "The house is at the end of the street.",
		SentenceTranslationTargets: []string{"house"},
	}
}

func studyScenarios() []Scenario {
	return []Scenario{
		{Name: "study candidates are the effective identities of analyzed Books, current reading first", Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			current := h.Seeds().SeedBook(t, owner, BookSeed{Occurrences: []OccurrenceSeed{
				{Surface: "Haus", Lemma: "haus", UPOS: "NOUN"}, {Surface: "Haus", Lemma: "haus", UPOS: "NOUN"},
			}})
			h.Seeds().SeedBook(t, owner, BookSeed{Occurrences: []OccurrenceSeed{
				{Surface: "Hund", Lemma: "hund", UPOS: "NOUN"}, {Surface: "Hund", Lemma: "hund", UPOS: "NOUN"},
				{Surface: "Hund", Lemma: "hund", UPOS: "NOUN"}, {Surface: "Haus", Lemma: "haus", UPOS: "NOUN"},
			}})
			h.Seeds().SeedCurrentReading(t, owner, Language, current)
			// Known vocabulary is never consulted.
			h.Seeds().SeedKnownVocabulary(t, owner, []domain.SnapshotIdentity{german("haus", "NOUN")})

			candidates, err := h.Study().ListStudyCandidates(owner, Language)
			require.NoError(t, err)
			ordered := domain.OrderStudyStream(candidates)
			require.Len(t, ordered, 2)
			assert.Equal(t, studyNoun("haus"), ordered[0].StudyIdentity, "a Known word is still offered, and current-reading words come first")
			assert.True(t, ordered[0].InCurrentReading)
			assert.EqualValues(t, 3, ordered[0].PersonalOccurrences, "occurrences count across analyzed Books")
			assert.Equal(t, studyNoun("hund"), ordered[1].StudyIdentity)
			assert.False(t, ordered[1].InCurrentReading)

			none, err := h.Study().ListStudyCandidates(h.Seeds().OtherOwner(), Language)
			require.NoError(t, err)
			assert.Empty(t, none, "another learner's Books are not candidates")
			other, err := h.Study().ListStudyCandidates(owner, "it")
			require.NoError(t, err)
			assert.Empty(t, other, "candidates are scoped to the study language")
		}},
		{Name: "study material is prepared one word at a time and settles once", Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			store := h.Study()
			haus, hund := studyNoun("haus"), studyNoun("hund")

			hausID, created, err := store.ReserveStudyMaterial(owner, haus)
			require.NoError(t, err)
			require.True(t, created)
			_, created, err = store.ReserveStudyMaterial(owner, haus)
			require.NoError(t, err)
			assert.False(t, created, "a word has one current record")
			hundID, created, err := store.ReserveStudyMaterial(owner, hund)
			require.NoError(t, err)
			require.True(t, created)
			_, _, err = store.ReserveStudyMaterial(owner, domain.StudyIdentity{Language: Language})
			require.ErrorIs(t, err, domain.ErrInvalidStudyMaterial)

			counts, err := store.CountStudyMaterial(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialCounts{Preparing: 2}, counts)

			content := studyReadyContent()
			require.NoError(t, store.RecordStudyMaterialReady(owner, hausID, content))
			ready, err := store.GetStudyMaterial(owner, hausID)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialReady, ready.State)
			assert.Equal(t, haus, ready.StudyIdentity)
			assert.Equal(t, content.Sentence, ready.Sentence)
			assert.Equal(t, content.Gloss, ready.Gloss)
			assert.Equal(t, content.BookID, ready.BookID)
			assert.Equal(t, content.SentenceTranslationTargets, ready.SentenceTranslationTargets)
			require.ErrorIs(t, store.RecordStudyMaterialReady(owner, hausID, content), domain.ErrNotFound, "settled material is not rewritten")

			require.ErrorIs(t, store.RecordStudyMaterialUnpreparable(owner, hundID, " "), domain.ErrInvalidStudyMaterial)
			require.NoError(t, store.RecordStudyMaterialUnpreparable(owner, hundID, domain.StudyOmissionSentenceQuality))
			skipped, err := store.GetStudyMaterial(owner, hundID)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialUnpreparable, skipped.State)
			assert.Equal(t, domain.StudyOmissionSentenceQuality, skipped.OmissionReason)
			require.ErrorIs(t, store.RecordStudyMaterialUnpreparable(owner, hundID, domain.StudyOmissionMeaning), domain.ErrNotFound)

			counts, err = store.CountStudyMaterial(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialCounts{Ready: 1, Unpreparable: 1}, counts)
			listed, err := store.ListCurrentStudyMaterial(owner, Language)
			require.NoError(t, err)
			assert.Len(t, listed, 2)
			_, created, err = store.ReserveStudyMaterial(owner, hund)
			require.NoError(t, err)
			assert.False(t, created, "an unpreparable word is not reserved again")

			_, err = store.GetStudyMaterial(h.Seeds().OtherOwner(), hausID)
			require.ErrorIs(t, err, domain.ErrNotFound, "another learner cannot read the record")
			require.ErrorIs(t, store.RecordStudyMaterialUnpreparable(h.Seeds().OtherOwner(), hausID, "x"), domain.ErrNotFound)
			_, err = store.GetStudyMaterial(owner, uuid.NewString())
			require.ErrorIs(t, err, domain.ErrNotFound)
			counts, err = store.CountStudyMaterial(h.Seeds().OtherOwner(), Language)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialCounts{}, counts)
		}},
	}
}
