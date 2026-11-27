package storecontract

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func studyNoun(lemma string) domain.StudyItem {
	return domain.StudyItem{Language: Language, CanonicalLemma: lemma, UPOS: "NOUN"}
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
			assert.Equal(t, studyNoun("haus"), ordered[0].StudyItem, "a Known word is still offered, and current-reading words come first")
			assert.True(t, ordered[0].InCurrentReading)
			assert.EqualValues(t, 3, ordered[0].PersonalOccurrences, "occurrences count across analyzed Books")
			assert.Equal(t, studyNoun("hund"), ordered[1].StudyItem)
			assert.False(t, ordered[1].InCurrentReading)

			none, err := h.Study().ListStudyCandidates(h.Seeds().OtherOwner(), Language)
			require.NoError(t, err)
			assert.Empty(t, none, "another learner's Books are not candidates")
			other, err := h.Study().ListStudyCandidates(owner, "it")
			require.NoError(t, err)
			assert.Empty(t, other, "candidates are scoped to the study language")
		}},
		studyCandidatesAmongScenario(),
		{Name: "study candidates come from the Corpus: Read Books supply them and a removed Book stops, keeping its words' progress", Run: studyCorpusScenario},
		{Name: "study candidates apply confirmed corrections and exclusions and ignore unanalyzed or other-language Books", Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedReviewBook(t, h) // two occurrences of "weg"
			h.Seeds().SeedBook(t, owner, BookSeed{MetadataOnly: true})
			h.Seeds().SeedBook(t, owner, BookSeed{Language: "it", Occurrences: []OccurrenceSeed{{Surface: "Casa", Lemma: "casa", UPOS: "NOUN"}}})

			candidates, err := h.Study().ListStudyCandidates(owner, Language)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			assert.Equal(t, studyNoun("weg"), candidates[0].StudyItem)
			assert.EqualValues(t, 2, candidates[0].PersonalOccurrences)

			corrected := reviewProposal(h, bookID, "correct", "pfad", reviewOccurrence(t, h, bookID, 0))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(corrected, previewProposal(t, h, corrected)))
			candidates, err = h.Study().ListStudyCandidates(owner, Language)
			require.NoError(t, err)
			counts := map[string]int64{}
			for _, candidate := range candidates {
				counts[candidate.CanonicalLemma] = candidate.PersonalOccurrences
			}
			assert.Equal(t, map[string]int64{"weg": 1, "pfad": 1}, counts, "a correction moves one occurrence to the corrected identity")

			excluded := reviewProposal(h, bookID, "exclude", "", reviewOccurrence(t, h, bookID, 1))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(excluded, previewProposal(t, h, excluded)))
			candidates, err = h.Study().ListStudyCandidates(owner, Language)
			require.NoError(t, err)
			require.Len(t, candidates, 1, "an exclusion removes the occurrence, and an identity with none")
			assert.Equal(t, "pfad", candidates[0].CanonicalLemma)
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
			_, _, err = store.ReserveStudyMaterial(owner, domain.StudyItem{Language: Language})
			require.ErrorIs(t, err, domain.ErrInvalidStudyMaterial)

			counts, err := store.CountStudyMaterial(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialCounts{Preparing: 2}, counts)

			content := studyReadyContent()
			requireBlankMaterialRefused(t, store, owner, hausID)
			counts, err = store.CountStudyMaterial(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialCounts{Preparing: 2}, counts, "refused material leaves the word pending")
			require.NoError(t, store.RecordStudyMaterialReady(owner, hausID, content))
			ready, err := store.GetStudyMaterial(owner, hausID)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyMaterialReady, ready.State)
			assert.Equal(t, haus, ready.StudyItem)
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
		{Name: "taking a batch introduces its ready words and declining introduces none", Run: func(t *testing.T, h Harness) {
			owner, store, progressStore := h.Seeds().Owner(), h.Study(), h.StudyProgress()
			haus, hund, tisch := studyNoun("haus"), studyNoun("hund"), studyNoun("tisch")
			hausID, hundID := readyStudyWord(t, store, owner, haus), readyStudyWord(t, store, owner, hund)
			tischID, _, err := store.ReserveStudyMaterial(owner, tisch)
			require.NoError(t, err)
			now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
			batch := []domain.StudyBatchItem{{StudyItem: haus, MaterialID: hausID}, {StudyItem: hund, MaterialID: hundID}, {StudyItem: tisch, MaterialID: tischID}}

			require.NoError(t, progressStore.DeclineStudyBatch(owner, Language, "2026-11-25", 1))
			progress, err := progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			assert.Empty(t, progress, "declining introduces no word")
			ledger, err := progressStore.StudyBatchLedger(owner, Language, "2026-11-25")
			require.NoError(t, err)
			assert.Equal(t, domain.StudyBatchLedger{StudyDay: "2026-11-25", Offered: 1, Declined: 1}, ledger)
			require.ErrorIs(t, progressStore.DeclineStudyBatch(owner, Language, "2026-11-25", 1), domain.ErrStudyBatchLimit, "the day's batch was already answered")

			_, err = progressStore.TakeStudyBatch(owner, Language, "2026-11-26", now, 1, []domain.StudyBatchItem{{StudyItem: tisch, MaterialID: tischID}})
			require.ErrorIs(t, err, domain.ErrNotFound, "a word whose material is not ready is not introduced")
			ledger, err = progressStore.StudyBatchLedger(owner, Language, "2026-11-26")
			require.NoError(t, err)
			assert.Zero(t, ledger.Decided(), "a batch that introduced nothing is not recorded")

			introduced, err := progressStore.TakeStudyBatch(owner, Language, "2026-11-26", now, 1, batch)
			require.NoError(t, err)
			require.Len(t, introduced, 2, "only the words with ready material are introduced")
			for _, word := range introduced {
				assert.Equal(t, "a building", word.LearnedMeaning, "the Gloss shown on introduction is the learned meaning")
				assert.Equal(t, domain.StudyStateNew, word.Memory.State)
				assert.True(t, now.Equal(word.Memory.Due), "an introduced word is due now")
				assert.True(t, now.Equal(word.IntroducedAt))
				assert.Zero(t, word.Memory.Reps)
				assert.False(t, word.Graded())
			}
			progress, err = progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			require.Len(t, progress, 2)
			assert.Equal(t, hausID, progress[0].MaterialID)
			ledger, err = progressStore.StudyBatchLedger(owner, Language, "2026-11-26")
			require.NoError(t, err)
			assert.Equal(t, domain.StudyBatchLedger{StudyDay: "2026-11-26", Offered: 1, Taken: 1}, ledger)
			_, err = progressStore.TakeStudyBatch(owner, Language, "2026-11-26", now, 1, batch)
			require.ErrorIs(t, err, domain.ErrStudyBatchLimit)

			_, err = progressStore.TakeStudyBatch(owner, Language, "2026-11-27", now, 1, []domain.StudyBatchItem{{StudyItem: haus, MaterialID: hausID}})
			require.ErrorIs(t, err, domain.ErrNotFound, "a word already in study is not introduced twice")
			other, err := progressStore.ListStudyProgress(h.Seeds().OtherOwner(), Language)
			require.NoError(t, err)
			assert.Empty(t, other, "progress belongs to its learner")
			italian, err := progressStore.ListStudyProgress(owner, "it")
			require.NoError(t, err)
			assert.Empty(t, italian, "progress belongs to its study language")
		}},
		{Name: "a declined batch is taken in place of itself and the day's limit counts distinct batches offered", Run: declinedBatchTakenOnce},
		{Name: "the ledger counts offers, takes up to the day's limit, and never lowers an offer", Run: func(t *testing.T, h Harness) {
			owner, store, progressStore := h.Seeds().Owner(), h.Study(), h.StudyProgress()
			now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
			words := make([]domain.StudyBatchItem, 0, 4)
			for _, lemma := range []string{"haus", "hund", "tisch", "buch"} {
				identity := studyNoun(lemma)
				words = append(words, domain.StudyBatchItem{StudyItem: identity, MaterialID: readyStudyWord(t, store, owner, identity)})
			}
			const day = "2026-11-25"
			ledger := func() domain.StudyBatchLedger {
				ledger, err := progressStore.StudyBatchLedger(owner, Language, day)
				require.NoError(t, err)
				return ledger
			}
			require.NoError(t, progressStore.RecordStudyBatchOffer(owner, Language, day, 1))
			require.NoError(t, progressStore.RecordStudyBatchOffer(owner, Language, day, 1))
			assert.Equal(t, domain.StudyBatchLedger{StudyDay: day, Offered: 1}, ledger(), "showing an offer again changes nothing")
			for i := range 3 {
				_, err := progressStore.TakeStudyBatch(owner, Language, day, now, 3, words[i:i+1])
				require.NoError(t, err)
				require.NoError(t, progressStore.RecordStudyBatchOffer(owner, Language, day, i+2))
			}
			assert.Equal(t, domain.StudyBatchLedger{StudyDay: day, Offered: 4, Taken: 3}, ledger(), "an offer past the answered batches is kept")
			_, err := progressStore.TakeStudyBatch(owner, Language, day, now, 3, words[3:])
			require.ErrorIs(t, err, domain.ErrStudyBatchLimit, "the limit counts the batches taken")
			other, err := progressStore.StudyBatchLedger(h.Seeds().OtherOwner(), Language, day)
			require.NoError(t, err)
			assert.Zero(t, other.Offered, "the ledger belongs to its learner")
		}},
		{Name: "a word carried over from an earlier day is taken again in the next batch", Run: func(t *testing.T, h Harness) {
			owner, store, progressStore := h.Seeds().Owner(), h.Study(), h.StudyProgress()
			haus, hund := studyNoun("haus"), studyNoun("hund")
			hausID, hundID := readyStudyWord(t, store, owner, haus), readyStudyWord(t, store, owner, hund)
			dayOne := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
			dayTwo := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
			taken, err := progressStore.TakeStudyBatch(owner, Language, "2026-11-25", dayOne, 3, []domain.StudyBatchItem{{StudyItem: haus, MaterialID: hausID}, {StudyItem: hund, MaterialID: hundID}})
			require.NoError(t, err)
			require.Len(t, taken, 2)

			_, err = progressStore.TakeStudyBatch(owner, Language, "2026-11-25", dayOne, 3, []domain.StudyBatchItem{{StudyItem: haus, MaterialID: hausID, ProgressID: taken[0].ID}})
			require.ErrorIs(t, err, domain.ErrNotFound, "a word introduced today is not carried over")

			again, err := progressStore.TakeStudyBatch(owner, Language, "2026-11-26", dayTwo, 3, []domain.StudyBatchItem{{StudyItem: haus, MaterialID: hausID, ProgressID: taken[0].ID}})
			require.NoError(t, err)
			require.Len(t, again, 1)
			assert.Equal(t, taken[0].ID, again[0].ID, "the same progress is taken again, not a second row")
			assert.True(t, dayTwo.Equal(again[0].IntroducedAt))
			assert.True(t, dayTwo.Equal(again[0].Memory.Due))
			progress, err := progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			assert.Len(t, progress, 2)
			ledger, err := progressStore.StudyBatchLedger(owner, Language, "2026-11-26")
			require.NoError(t, err)
			assert.Equal(t, 1, ledger.Taken)
		}},
		{Name: "a graded card is logged with its state before and after and is graded once", Run: func(t *testing.T, h Harness) {
			owner, store, progressStore := h.Seeds().Owner(), h.Study(), h.StudyProgress()
			haus, hund := studyNoun("haus"), studyNoun("hund")
			hausID, hundID := readyStudyWord(t, store, owner, haus), readyStudyWord(t, store, owner, hund)
			now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
			_, err := progressStore.TakeStudyBatch(owner, Language, "2026-11-25", now, 1, []domain.StudyBatchItem{{StudyItem: hund, MaterialID: hundID}, {StudyItem: haus, MaterialID: hausID}})
			require.NoError(t, err)

			_, err = progressStore.NextStudyCard(owner, Language, now.Add(-time.Hour))
			require.ErrorIs(t, err, domain.ErrNotFound, "nothing is due before the words were introduced")
			card, err := progressStore.NextStudyCard(owner, Language, now)
			require.NoError(t, err)
			assert.Equal(t, haus, card.Progress.StudyItem, "ties break on the word, so the order is deterministic")
			assert.Equal(t, hausID, card.Material.ID)
			assert.Equal(t, "Das Haus steht am Ende der Strasse.", card.Material.Sentence)

			next := domain.NextStudyDayStart(now)
			gotIt := domain.StudyMemory{State: domain.StudyStateReview, Due: next, Stability: 2.3, Difficulty: 6.4, ScheduledDays: 1, Reps: 1, LastReview: now}
			record := domain.StudyGradeRecord{ProgressID: card.Progress.ID, ExpectedReps: 0, Grade: domain.StudyGradeGotIt, ReviewedAt: now, StudyDay: "2026-11-25", After: gotIt}
			require.NoError(t, progressStore.RecordStudyGrade(owner, record))
			require.ErrorIs(t, progressStore.RecordStudyGrade(owner, record), domain.ErrStudyStale, "a card is graded once")
			require.ErrorIs(t, progressStore.RecordStudyGrade(h.Seeds().OtherOwner(), record), domain.ErrNotFound, "another learner cannot grade it")

			logs, err := progressStore.ListStudyReviewLog(owner, card.Progress.ID)
			require.NoError(t, err)
			require.Len(t, logs, 1, "every grade writes one review-log row")
			assert.Equal(t, domain.StudyGradeGotIt, logs[0].Grade)
			assert.Equal(t, "2026-11-25", logs[0].StudyDay)
			assert.True(t, now.Equal(logs[0].ReviewedAt))
			assert.Equal(t, hausID, logs[0].MaterialID)
			assert.Equal(t, domain.StudyStateNew, logs[0].Before.State, "the state before the grade")
			assert.True(t, now.Equal(logs[0].Before.Due))
			assert.Equal(t, domain.StudyStateReview, logs[0].After.State, "and after it")
			assert.InDelta(t, 2.3, logs[0].After.Stability, 1e-9)
			assert.InDelta(t, 6.4, logs[0].After.Difficulty, 1e-9)
			assert.True(t, next.Equal(logs[0].After.Due))

			progress, err := progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			var stored domain.StudyProgress
			for _, item := range progress {
				if item.ID == card.Progress.ID {
					stored = item
				}
			}
			assert.Equal(t, domain.StudyStateReview, stored.Memory.State)
			assert.Equal(t, int64(1), stored.Memory.Reps)
			assert.True(t, next.Equal(stored.Memory.Due))
			assert.True(t, now.Equal(stored.Memory.LastReview))
			assert.True(t, stored.Graded())

			card, err = progressStore.NextStudyCard(owner, Language, now)
			require.NoError(t, err)
			assert.Equal(t, hund, card.Progress.StudyItem, "a graded word is no longer due")
			again := domain.StudyMemory{State: domain.StudyStateLearning, Due: now.Add(time.Minute), Stability: 0.2, Difficulty: 6.8, Reps: 1, RemainingSteps: 1, LastReview: now}
			require.NoError(t, progressStore.RecordStudyGrade(owner, domain.StudyGradeRecord{ProgressID: card.Progress.ID, Grade: domain.StudyGradeAgain, ReviewedAt: now, StudyDay: "2026-11-25", After: again}))
			_, err = progressStore.NextStudyCard(owner, Language, now)
			require.ErrorIs(t, err, domain.ErrNotFound, "Again repeats the word later, not now")
			card, err = progressStore.NextStudyCard(owner, Language, now.Add(2*time.Minute))
			require.NoError(t, err)
			assert.Equal(t, hund, card.Progress.StudyItem, "and it is due again the same day")
			assert.Equal(t, int64(1), card.Progress.Memory.Reps)

			logs, err = progressStore.ListStudyReviewLog(owner, card.Progress.ID)
			require.NoError(t, err)
			require.Len(t, logs, 1)
			assert.Equal(t, domain.StudyGradeAgain, logs[0].Grade)
		}},
		{Name: "what is due follows the Study day and a repeat across its boundary is due at it", Run: func(t *testing.T, h Harness) {
			owner, store, progressStore := h.Seeds().Owner(), h.Study(), h.StudyProgress()
			haus, hund := studyNoun("haus"), studyNoun("hund")
			hausID, hundID := readyStudyWord(t, store, owner, haus), readyStudyWord(t, store, owner, hund)
			// 2026-11-25 04:55 Berlin, five minutes before the Study day ends.
			late := time.Date(2026, 11, 25, 3, 55, 0, 0, time.UTC)
			_, err := progressStore.TakeStudyBatch(owner, Language, "2026-11-24", late, 1, []domain.StudyBatchItem{{StudyItem: haus, MaterialID: hausID}, {StudyItem: hund, MaterialID: hundID}})
			require.NoError(t, err)
			progress, err := progressStore.ListStudyProgress(owner, Language)
			require.NoError(t, err)
			byLemma := map[string]domain.StudyProgress{}
			for _, item := range progress {
				byLemma[item.CanonicalLemma] = item
			}
			boundary := domain.NextStudyDayStart(late)
			// Haus was forgotten five minutes before the boundary, so its repeat
			// would fall after it; hund is a review due this afternoon.
			repeat := domain.StudyMemory{State: domain.StudyStateRelearning, Due: late.Add(10 * time.Minute), Stability: 1, Difficulty: 5, Reps: 2, Lapses: 1, LastReview: late}
			require.NoError(t, progressStore.RecordStudyGrade(owner, domain.StudyGradeRecord{ProgressID: byLemma["haus"].ID, Grade: domain.StudyGradeAgain, ReviewedAt: late, StudyDay: "2026-11-24", After: repeat}))
			later := domain.StudyMemory{State: domain.StudyStateReview, Due: boundary.Add(9 * time.Hour), Stability: 3, Difficulty: 5, Reps: 1, LastReview: late.Add(-24 * time.Hour)}
			require.NoError(t, progressStore.RecordStudyGrade(owner, domain.StudyGradeRecord{ProgressID: byLemma["hund"].ID, Grade: domain.StudyGradeGotIt, ReviewedAt: late, StudyDay: "2026-11-24", After: later}))

			_, err = progressStore.NextStudyCard(owner, Language, late.Add(2*time.Minute))
			require.ErrorIs(t, err, domain.ErrNotFound, "the repeat waits a little")
			card, err := progressStore.NextStudyCard(owner, Language, boundary)
			require.NoError(t, err)
			assert.Equal(t, haus, card.Progress.StudyItem, "the repeat is due when the next Study day starts")
			_, err = progressStore.NextStudyCard(owner, Language, boundary.Add(time.Hour))
			require.NoError(t, err)

			// A review due later in a Study day belongs to that day and is due
			// from its start; one due on a later day waits.
			require.NoError(t, progressStore.RecordStudyGrade(owner, domain.StudyGradeRecord{ProgressID: byLemma["haus"].ID, ExpectedReps: 2, Grade: domain.StudyGradeGotIt, ReviewedAt: boundary, StudyDay: "2026-11-25", After: domain.StudyMemory{State: domain.StudyStateReview, Due: boundary.Add(72 * time.Hour), Stability: 3, Difficulty: 5, Reps: 3, Lapses: 1, LastReview: boundary}}))
			card, err = progressStore.NextStudyCard(owner, Language, boundary.Add(time.Minute))
			require.NoError(t, err)
			assert.Equal(t, hund, card.Progress.StudyItem, "a review due at 14:00 is due from the start of its Study day")
			require.NoError(t, progressStore.RecordStudyGrade(owner, domain.StudyGradeRecord{ProgressID: byLemma["hund"].ID, ExpectedReps: 1, Grade: domain.StudyGradeGotIt, ReviewedAt: boundary.Add(time.Minute), StudyDay: "2026-11-25", After: domain.StudyMemory{State: domain.StudyStateReview, Due: boundary.Add(72 * time.Hour), Stability: 3, Difficulty: 5, Reps: 2, LastReview: boundary.Add(time.Minute)}}))
			_, err = progressStore.NextStudyCard(owner, Language, boundary.Add(10*time.Hour))
			require.ErrorIs(t, err, domain.ErrNotFound, "a review due on a later Study day waits for it")
		}},
	}
}

// readyStudyWord reserves a word's Study material and marks it ready.
func readyStudyWord(t *testing.T, store StudyStore, owner string, identity domain.StudyItem) string {
	t.Helper()
	id, created, err := store.ReserveStudyMaterial(owner, identity)
	require.NoError(t, err)
	require.True(t, created)
	content := studyReadyContent()
	content.TargetWord = identity.CanonicalLemma
	require.NoError(t, store.RecordStudyMaterialReady(owner, id, content))
	return id
}

// requireBlankMaterialRefused checks that material missing its sentence, target
// word, Gloss or translation is not recorded as ready.
func requireBlankMaterialRefused(t *testing.T, store StudyStore, owner, id string) {
	t.Helper()
	blank := []struct {
		name     string
		blankOut func(*domain.StudyMaterialReadyContent)
	}{
		{"sentence", func(c *domain.StudyMaterialReadyContent) { c.Sentence = " " }},
		{"target word", func(c *domain.StudyMaterialReadyContent) { c.TargetWord = "" }},
		{"gloss", func(c *domain.StudyMaterialReadyContent) { c.Gloss = "\t" }},
		{"translation", func(c *domain.StudyMaterialReadyContent) { c.SentenceTranslation = "" }},
	}
	for _, tc := range blank {
		content := studyReadyContent()
		tc.blankOut(&content)
		require.ErrorIs(t, store.RecordStudyMaterialReady(owner, id, content), domain.ErrInvalidStudyMaterial, "a blank %s is not ready material", tc.name)
	}
}

// declinedBatchTakenOnce checks that taking a batch the learner declined
// consumes no other batch, so the day's limit counts distinct batches offered.
func declinedBatchTakenOnce(t *testing.T, h Harness) {
	t.Helper()
	owner, store, progressStore := h.Seeds().Owner(), h.Study(), h.StudyProgress()
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	words := make([]domain.StudyBatchItem, 0, 4)
	for _, lemma := range []string{"haus", "hund", "tisch", "buch"} {
		identity := studyNoun(lemma)
		words = append(words, domain.StudyBatchItem{StudyItem: identity, MaterialID: readyStudyWord(t, store, owner, identity)})
	}
	const day = "2026-11-25"
	ledger := func() domain.StudyBatchLedger {
		ledger, err := progressStore.StudyBatchLedger(owner, Language, day)
		require.NoError(t, err)
		return ledger
	}
	require.NoError(t, progressStore.RecordStudyBatchOffer(owner, Language, day, 1))
	require.NoError(t, progressStore.DeclineStudyBatch(owner, Language, day, 3))
	assert.Equal(t, domain.StudyBatchLedger{StudyDay: day, Offered: 1, Declined: 1}, ledger())
	for i := range 3 {
		_, err := progressStore.TakeStudyBatch(owner, Language, day, now, 3, words[i:i+1])
		require.NoError(t, err, "take %d: the declined batch consumes no other", i+1)
	}
	assert.Equal(t, domain.StudyBatchLedger{StudyDay: day, Offered: 3, Taken: 3}, ledger(), "three distinct batches were offered")
	_, err := progressStore.TakeStudyBatch(owner, Language, day, now, 3, words[3:])
	require.ErrorIs(t, err, domain.ErrStudyBatchLimit)

	const brakeDay = "2026-11-26"
	require.NoError(t, progressStore.DeclineStudyBatch(owner, Language, brakeDay, 1))
	_, err = progressStore.TakeStudyBatch(owner, Language, brakeDay, now.Add(24*time.Hour), 1, words[3:])
	require.NoError(t, err, "under the backlog brake the one batch, declined, can still be taken")
	_, err = progressStore.TakeStudyBatch(owner, Language, brakeDay, now.Add(24*time.Hour), 1, words[3:])
	require.ErrorIs(t, err, domain.ErrStudyBatchLimit)
}

// studyCandidatesAmongScenario holds ListStudyCandidatesAmong to the full
// candidate list for the identities it is asked about.
func studyCandidatesAmongScenario() Scenario {
	return Scenario{Name: "study candidates among identities agree with the full candidate list for those identities", Run: func(t *testing.T, h Harness) {
		owner := h.Seeds().Owner()
		current := h.Seeds().SeedBook(t, owner, BookSeed{Occurrences: []OccurrenceSeed{
			{Surface: "Haus", Lemma: "haus", UPOS: "NOUN"}, {Surface: "Haus", Lemma: "haus", UPOS: "NOUN"},
		}})
		h.Seeds().SeedBook(t, owner, BookSeed{Occurrences: []OccurrenceSeed{
			{Surface: "Hund", Lemma: "hund", UPOS: "NOUN"}, {Surface: "Haus", Lemma: "haus", UPOS: "NOUN"},
			{Surface: "Katze", Lemma: "katze", UPOS: "NOUN"},
		}})
		h.Seeds().SeedCurrentReading(t, owner, Language, current)

		all, err := h.Study().ListStudyCandidates(owner, Language)
		require.NoError(t, err)
		want := map[domain.StudyItem]domain.StudyCandidate{}
		for _, candidate := range all {
			want[candidate.StudyItem] = candidate
		}
		among, err := h.Study().ListStudyCandidatesAmong(owner, Language, []domain.StudyItem{studyNoun("haus"), studyNoun("katze"), studyNoun("nope"), {Language: Language, CanonicalLemma: "hund", UPOS: "VERB"}})
		require.NoError(t, err)
		require.ElementsMatch(t, []domain.StudyCandidate{want[studyNoun("haus")], want[studyNoun("katze")]}, among,
			"only the asked identities that are candidates: a word that is not one, or with another part of speech, is absent")
		t.Run("no identities asked for", func(t *testing.T) {
			none, err := h.Study().ListStudyCandidatesAmong(owner, Language, nil)
			require.NoError(t, err)
			require.Empty(t, none)
		})
		t.Run("another learner", func(t *testing.T) {
			other, err := h.Study().ListStudyCandidatesAmong(h.Seeds().OtherOwner(), Language, []domain.StudyItem{studyNoun("haus")})
			require.NoError(t, err)
			require.Empty(t, other, "another learner's Books are not candidates")
		})
	}}
}

// studyCorpusScenario holds the Corpus (ADR 0136 §2): the analysed Books in My
// Books, Read Books included, supply the Study stream. Removing a Book stops
// its words being offered but costs no Study progress, material or review.
func studyCorpusScenario(t *testing.T, h Harness) {
	t.Helper()
	owner := h.Seeds().Owner()
	read := h.Seeds().SeedBook(t, owner, BookSeed{Occurrences: []OccurrenceSeed{{Surface: "Haus", Lemma: "haus", UPOS: "NOUN"}}})
	toRead := h.Seeds().SeedBook(t, owner, BookSeed{Occurrences: []OccurrenceSeed{{Surface: "Hund", Lemma: "hund", UPOS: "NOUN"}}})
	require.NoError(t, h.BookState().MarkBookPreviouslyRead(owner, read))

	lemmas := func() []string {
		candidates, err := h.Study().ListStudyCandidates(owner, Language)
		require.NoError(t, err)
		var found []string
		for _, candidate := range candidates {
			found = append(found, candidate.CanonicalLemma)
		}
		return found
	}
	require.ElementsMatch(t, []string{"haus", "hund"}, lemmas(), "a Read Book supplies Study candidates")

	word := introducedStudyWord(t, h, owner, studyNoun("haus"))
	require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, read))
	assert.Equal(t, []string{"hund"}, lemmas(), "a removed Book's words are no longer offered")
	among, err := h.Study().ListStudyCandidatesAmong(owner, Language, []domain.StudyItem{studyNoun("haus"), studyNoun("hund")})
	require.NoError(t, err)
	require.Len(t, among, 1)
	assert.Equal(t, "hund", among[0].CanonicalLemma)

	progress, err := h.StudyProgress().ListStudyProgress(owner, Language)
	require.NoError(t, err)
	require.Len(t, progress, 1, "Remove keeps Study progress")
	assert.Equal(t, word.ID, progress[0].ID)
	material, err := h.Study().ListCurrentStudyMaterial(owner, Language)
	require.NoError(t, err)
	require.Len(t, material, 1, "Remove keeps Study material")
	assert.Equal(t, domain.StudyMaterialReady, material[0].State)
	card, err := h.StudyProgress().NextStudyCard(owner, Language, time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC))
	require.NoError(t, err, "reviews continue")
	assert.Equal(t, word.ID, card.Progress.ID)

	require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, toRead))
	assert.Empty(t, lemmas(), "the Corpus is the Books in My Books")
}
