package storecontract

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fluencyRecord(progressID string, ordinal int64, agreement domain.FluencyAgreement) domain.FluencyContextRecord {
	record := domain.FluencyContextRecord{
		ProgressID: progressID, Agreement: agreement,
		BookID: uuid.NewString(), AnalysisRunID: "00000000-0000-4000-8000-000000000001", CorpusID: uuid.NewString(),
		SentenceOrdinal: ordinal, TokenOrdinal: 1,
		Sentence: "Das Haus ist alt.", TargetWord: "Haus",
		Form: domain.FluencyForm{Surface: "Haus", Features: "Case=Nom|Number=Sing"},
	}
	if agreement == domain.FluencyAgrees {
		record.Gloss, record.SentenceTranslation, record.SentenceTranslationTargets = "a building", "The house is old.", []string{"house"}
	}
	return record
}

// gradedStudyWord introduces a word and returns its progress.
func introducedStudyWord(t *testing.T, h Harness, owner string, identity domain.StudyIdentity, day string) domain.StudyProgress {
	t.Helper()
	materialID := readyStudyWord(t, h.Study(), owner, identity)
	introduced, err := h.StudyProgress().TakeStudyBatch(owner, Language, day, time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC), 3, []domain.StudyBatchWord{{StudyIdentity: identity, MaterialID: materialID}})
	require.NoError(t, err)
	require.Len(t, introduced, 1)
	return introduced[0]
}

func studyFluencyScenarios() []Scenario {
	return []Scenario{
		{Name: "a word keeps at most five agreeing Fluency contexts, one per occurrence", Run: fluencyContextLimitScenario},
		{Name: "a Fluency context of a word the learner does not have is refused", Run: fluencyForeignContextScenario},
		{Name: "a Fluency response is appended and changes no Study progress or review log", Run: fluencyResponseScenario},
	}
}

func fluencyContextLimitScenario(t *testing.T, h Harness) {
	t.Helper()
	owner, store := h.Seeds().Owner(), h.StudyFluency()
	word := introducedStudyWord(t, h, owner, studyNoun("haus"), "2026-11-25")

	for ordinal := int64(1); ordinal <= domain.FluencyContextsPerWord; ordinal++ {
		require.NoError(t, store.RecordStudyFluencyContext(owner, fluencyRecord(word.ID, ordinal, domain.FluencyAgrees)))
	}
	require.ErrorIs(t, store.RecordStudyFluencyContext(owner, fluencyRecord(word.ID, 6, domain.FluencyAgrees)), domain.ErrFluencyContextNotStored, "a sixth agreeing context is refused")
	require.ErrorIs(t, store.RecordStudyFluencyContext(owner, fluencyRecord(word.ID, 1, domain.FluencyDisagrees)), domain.ErrFluencyContextNotStored, "an occurrence is assessed once")
	require.NoError(t, store.RecordStudyFluencyContext(owner, fluencyRecord(word.ID, 7, domain.FluencyDisagrees)), "a disagreeing occurrence is kept and does not count toward the five")

	contexts, err := store.ListStudyFluencyContexts(owner, Language)
	require.NoError(t, err)
	require.Len(t, contexts, 6)
	agreeing := 0
	for _, item := range contexts {
		assert.Equal(t, word.ID, item.ProgressID)
		assert.True(t, item.LastResponseAt.IsZero(), "an unanswered context has no response time")
		assert.Empty(t, item.Responses, "an unanswered context has no answers")
		if item.Agreement == domain.FluencyAgrees {
			agreeing++
			assert.Equal(t, "a building", item.Gloss)
			assert.Equal(t, []string{"house"}, item.SentenceTranslationTargets)
			assert.Equal(t, domain.FluencyForm{Surface: "Haus", Features: "Case=Nom|Number=Sing"}, item.Form)
			assert.Equal(t, "Das Haus ist alt.", item.Sentence)
		}
	}
	assert.Equal(t, domain.FluencyContextsPerWord, agreeing)

	none, err := store.ListStudyFluencyContexts(h.Seeds().OtherOwner(), Language)
	require.NoError(t, err)
	assert.Empty(t, none, "contexts belong to their learner")
	italian, err := store.ListStudyFluencyContexts(owner, "it")
	require.NoError(t, err)
	assert.Empty(t, italian, "contexts are scoped to the study language")
}

func fluencyForeignContextScenario(t *testing.T, h Harness) {
	t.Helper()
	owner, store := h.Seeds().Owner(), h.StudyFluency()
	word := introducedStudyWord(t, h, owner, studyNoun("haus"), "2026-11-25")
	require.ErrorIs(t, store.RecordStudyFluencyContext(h.Seeds().OtherOwner(), fluencyRecord(word.ID, 1, domain.FluencyAgrees)), domain.ErrNotFound)
	require.ErrorIs(t, store.RecordStudyFluencyContext(owner, fluencyRecord(uuid.NewString(), 1, domain.FluencyAgrees)), domain.ErrNotFound)
}

func fluencyResponseScenario(t *testing.T, h Harness) {
	t.Helper()
	owner, store, progressStore := h.Seeds().Owner(), h.StudyFluency(), h.StudyProgress()
	word := introducedStudyWord(t, h, owner, studyNoun("haus"), "2026-11-25")
	require.NoError(t, progressStore.RecordStudyGrade(owner, domain.StudyGradeRecord{
		ProgressID: word.ID, ExpectedReps: 0, Grade: domain.StudyGradeGotIt, ReviewedAt: time.Date(2026, 11, 25, 8, 5, 0, 0, time.UTC), StudyDay: "2026-11-25",
		After: domain.StudyMemory{State: domain.StudyStateReview, Due: time.Date(2026, 12, 20, 4, 0, 0, 0, time.UTC), Stability: 25, Difficulty: 5, ScheduledDays: 25, Reps: 1, LastReview: time.Date(2026, 11, 25, 8, 5, 0, 0, time.UTC)},
	}))
	require.NoError(t, store.RecordStudyFluencyContext(owner, fluencyRecord(word.ID, 1, domain.FluencyAgrees)))
	require.NoError(t, store.RecordStudyFluencyContext(owner, fluencyRecord(word.ID, 2, domain.FluencyDisagrees)))
	contexts, err := store.ListStudyFluencyContexts(owner, Language)
	require.NoError(t, err)
	var agreeing, disagreeing domain.FluencyContext
	for _, item := range contexts {
		if item.Agreement == domain.FluencyAgrees {
			agreeing = item
		} else {
			disagreeing = item
		}
	}
	progressBefore, err := progressStore.ListStudyProgress(owner, Language)
	require.NoError(t, err)
	logBefore, err := progressStore.ListStudyReviewLog(owner, word.ID)
	require.NoError(t, err)

	first := time.Date(2026, 12, 21, 9, 0, 0, 0, time.UTC)
	require.NoError(t, store.RecordStudyFluencyResponse(owner, domain.FluencyResponseRecord{ContextID: agreeing.ID, Response: domain.FluencyNotRecognised, RespondedAt: first}))
	require.NoError(t, store.RecordStudyFluencyResponse(owner, domain.FluencyResponseRecord{ContextID: agreeing.ID, Response: domain.FluencyRecognised, RespondedAt: first.Add(time.Minute)}))

	count, err := store.CountStudyFluencyResponses(owner, agreeing.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "responses are appended, never replaced")
	contexts, err = store.ListStudyFluencyContexts(owner, Language)
	require.NoError(t, err)
	for _, item := range contexts {
		if item.ID == agreeing.ID {
			assert.True(t, first.Add(time.Minute).Equal(item.LastResponseAt), "the context reports its latest answer")
			require.Len(t, item.Responses, 2, "the context reports every answer")
			assert.Equal(t, domain.FluencyNotRecognised, item.Responses[0].Response)
			assert.True(t, first.Equal(item.Responses[0].At), "answers come oldest first")
			assert.Equal(t, domain.FluencyRecognised, item.Responses[1].Response)
			assert.True(t, first.Add(time.Minute).Equal(item.Responses[1].At))
		}
	}
	progressAfter, err := progressStore.ListStudyProgress(owner, Language)
	require.NoError(t, err)
	assert.Equal(t, progressBefore, progressAfter, "Fluency never changes Study progress")
	logAfter, err := progressStore.ListStudyReviewLog(owner, word.ID)
	require.NoError(t, err)
	assert.Equal(t, logBefore, logAfter, "Fluency never writes the review log")

	require.ErrorIs(t, store.RecordStudyFluencyResponse(owner, domain.FluencyResponseRecord{ContextID: disagreeing.ID, Response: domain.FluencyRecognised, RespondedAt: first}), domain.ErrNotFound, "a context that disagrees is never shown, so never answered")
	require.ErrorIs(t, store.RecordStudyFluencyResponse(h.Seeds().OtherOwner(), domain.FluencyResponseRecord{ContextID: agreeing.ID, Response: domain.FluencyRecognised, RespondedAt: first}), domain.ErrNotFound)
	require.ErrorIs(t, store.RecordStudyFluencyResponse(owner, domain.FluencyResponseRecord{ContextID: uuid.NewString(), Response: domain.FluencyRecognised, RespondedAt: first}), domain.ErrNotFound)
	require.Error(t, store.RecordStudyFluencyResponse(owner, domain.FluencyResponseRecord{ContextID: agreeing.ID, Response: "again", RespondedAt: first}))
}
