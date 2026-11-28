//go:build integration

package webapp

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// preparedBook seeds an analyzed Book, opens Study so its words' material is
// enqueued and waits until every word is settled. It returns the Book's ID.
func (s *studyHarness) preparedBook(words ...testutil.CorpusWord) string {
	s.t.Helper()
	book := s.seedBook("reading", words...)
	s.page()
	require.NoError(s.t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(s.t, "study River client", s.client.Stop)
	testutil.Eventually(s.t, 60*time.Second, "material to settle", func() (bool, string) {
		return s.settled()
	})
	return book.ID
}

// word is the learner's progress for one lemma.
func (s *studyHarness) word(lemma string) domain.StudyProgress {
	s.t.Helper()
	for _, item := range s.progress() {
		if item.CanonicalLemma == lemma {
			return item
		}
	}
	s.t.Fatalf("%s is not in study", lemma)
	return domain.StudyProgress{}
}

// material is one Study material record.
func (s *studyHarness) material(id string) domain.StudyMaterial {
	s.t.Helper()
	item, err := s.store.GetStudyMaterial(s.ctx, s.owner.ID, id)
	require.NoError(s.t, err)
	return item
}

// decide drives the shared Lemma decision workflow for the occurrence of
// the word's surface form that is in the given sentence.
func (s *studyHarness) decide(bookID, surface string, sentence int64, decision, lemma string) {
	s.t.Helper()
	occurrences, err := s.store.ListLemmaReviewOccurrences(s.ctx, s.owner.ID, bookID, surface)
	require.NoError(s.t, err)
	for _, occurrence := range occurrences {
		if occurrence.SentenceOrdinal == sentence {
			confirmLemmaDecisionForm(s.t, s.h, s.owner.ID, bookID, surface, decision, lemma, occurrence.ID())
			return
		}
	}
	s.t.Fatalf("no occurrence of %s in sentence %d", surface, sentence)
}

func TestStudyCorrectionReintroducesAWordInNewMaterialAndKeepsItsProgress(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	haus := s.word("haus")
	original := s.material(haus.MaterialID)
	require.EqualValues(t, 1, haus.Memory.Reps)

	// The learner confirms that the sentence Study introduced the word in
	// actually held another word.
	s.decide(bookID, "haus", original.SentenceOrdinal, "correct", "hof")
	s.clock.Advance(24 * time.Hour)
	assert.Equal(t, 2, s.today().Remaining, "both words are due at the start of the next Study day")
	// The other word comes first and its sentence is still valid.
	require.Equal(t, s.word("buch").ID, s.currentCard().ID)
	s.grade(s.word("buch"), "got-it")

	// Opening Study checks the word that is due next: its sentence no longer
	// belongs to it, so the review waits for new material. It does not hold the
	// finish line open while it has nothing to show.
	page := s.page()
	waiting := s.word("haus")
	assert.True(t, waiting.Reintroducing, "its material was replaced")
	assert.Equal(t, haus.MaterialID, waiting.MaterialID)
	assert.Equal(t, haus.Memory, waiting.Memory, "the FSRS state is unchanged")
	assert.Contains(t, page, "waiting for a new sentence after a correction")
	assert.True(t, s.today().Finished, "a review without material does not hold the finish line open")
	assert.NotContains(t, page, original.Sentence, "the corrected sentence is not shown")

	// The study job prepares the replacement; the next visit moves the word
	// onto it.
	testutil.Eventually(t, 60*time.Second, "the word to be re-introduced", func() (bool, string) {
		s.page()
		return s.word("haus").MaterialID != haus.MaterialID, "the word is still waiting for new material"
	})
	reintroduced := s.word("haus")
	replacement := s.material(reintroduced.MaterialID)
	assert.NotEqual(t, original.SentenceOrdinal, replacement.SentenceOrdinal, "the new sentence is another occurrence")
	assert.Equal(t, domain.StudyMaterialReady, replacement.State)
	assert.Equal(t, replacement.Gloss, reintroduced.LearnedMeaning, "the learned meaning is re-anchored to the new Gloss")
	assert.True(t, reintroduced.Corrected(), "the card is marked as corrected")
	assert.False(t, reintroduced.Reintroducing)
	assert.Equal(t, haus.Memory, reintroduced.Memory, "the FSRS state, due time and counts are unchanged")
	assert.Equal(t, haus.IntroducedAt, reintroduced.IntroducedAt)
	assert.Len(t, s.reviewLog(haus.ID), 1, "the review history is kept")

	card := s.currentCard()
	require.Equal(t, haus.ID, card.ID)
	page = getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, `id="study-corrected"`, "the card says it was corrected")
	assert.Contains(t, page, replacement.TargetWord)

	// The word the correction points to is never put into study.
	for _, item := range s.progress() {
		assert.NotEqual(t, "hof", item.CanonicalLemma)
	}
	s.grade(card, "got-it")
	assert.False(t, s.word("haus").Corrected(), "answering the card clears the mark")
	assert.Len(t, s.reviewLog(haus.ID), 2)
}

func TestStudyExclusionsWithdrawAWordWithNoValidOccurrenceAndItsProgressIsKept(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "baum", UPOS: "NOUN", Occurrences: 1}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	baum := s.word("baum")
	source := s.material(baum.MaterialID)

	s.decide(bookID, "baum", source.SentenceOrdinal, "exclude", "")
	s.clock.Advance(24 * time.Hour)
	assert.Equal(t, 2, s.today().Remaining)

	page := s.page()
	withdrawn := s.word("baum")
	assert.True(t, withdrawn.Withdrawn, "no valid occurrence remains")
	assert.Equal(t, baum.Memory, withdrawn.Memory, "its progress is kept")
	assert.Equal(t, baum.LearnedMeaning, withdrawn.LearnedMeaning)
	assert.Contains(t, page, "Withdrawn by a correction")
	assert.Contains(t, page, `id="study-withdrawn"`)
	assert.Contains(t, page, "<li>Baum</li>")
	assert.Equal(t, 1, s.today().Remaining, "a withdrawn word is not due and is not counted")
	assert.Contains(t, page, "1 review left today.")
	assert.Equal(t, s.word("buch").ID, s.currentCard().ID, "only the other word is shown")
	assert.Len(t, s.reviewLog(baum.ID), 1)

	// It stays withdrawn on later days.
	s.clock.Advance(24 * time.Hour)
	s.page()
	assert.True(t, s.word("baum").Withdrawn)
	assert.Empty(t, s.progressForUnknownIdentity("baum"), "no second progress row is created for it")
}

// progressForUnknownIdentity lists the learner's progress rows for a lemma.
func (s *studyHarness) progressForUnknownIdentity(lemma string) []domain.StudyProgress {
	var rows []domain.StudyProgress
	for _, item := range s.progress() {
		if item.CanonicalLemma == lemma {
			rows = append(rows, item)
		}
	}
	if len(rows) <= 1 {
		return nil
	}
	return rows
}

// republish simulates an Analysis update that publishes a new run of the Book
// under the same extraction: every sentence is copied with its ordinal shifted,
// so only the token's place in its unit says it is the same occurrence. The
// lemma of the tokens in the given source sentences changes to relemma.
func (s *studyHarness) republish(bookID string, relemma string, sentences ...int64) string {
	s.t.Helper()
	exec := func(query string, args ...any) {
		_, err := s.store.Pool().Exec(s.ctx, query, args...)
		require.NoError(s.t, err)
	}
	var runID, corpusID, sourceID, newRun, newCorpus string
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT analysis_run_id::text, source_material_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, s.owner.ID, bookID).Scan(&runID, &sourceID))
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT corpus_id::text FROM analysis_runs WHERE owner_id=$1 AND id=$2`, s.owner.ID, runID).Scan(&corpusID))
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at)
		SELECT owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity||'-update','completed',now() FROM analysis_runs WHERE owner_id=$1 AND id=$2 RETURNING id::text`, s.owner.ID, runID).Scan(&newRun))
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count)
		SELECT owner_id,source_material_id,artifact_hash,'complete',analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count FROM corpora WHERE owner_id=$1 AND id=$2 RETURNING id::text`, s.owner.ID, corpusID).Scan(&newCorpus))
	exec(`UPDATE corpora SET analysis_run_id=$1 WHERE owner_id=$2 AND id=$3`, newRun, s.owner.ID, newCorpus)
	exec(`UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, newCorpus, s.owner.ID, newRun)
	const shift = 10
	exec(`INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		SELECT owner_id,$3,$4,unit_id,sentence_ordinal+$5,sentence_text,start_offset,end_offset FROM corpus_sentences WHERE owner_id=$1 AND analysis_run_id=$2`, s.owner.ID, runID, newRun, newCorpus, shift)
	exec(`INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset,lexical_discriminator,headword)
		SELECT owner_id,language,$3,$4,sentence_ordinal+$5,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset,lexical_discriminator,headword FROM corpus_tokens WHERE owner_id=$1 AND analysis_run_id=$2`, s.owner.ID, runID, newRun, newCorpus, shift)
	for _, sentence := range sentences {
		exec(`UPDATE corpus_tokens SET canonical_lemma=$4, raw_lemma=$4 WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=$3 AND upos='NOUN'`, s.owner.ID, newRun, sentence+shift, relemma)
	}
	exec(`UPDATE book_current_analyses SET analysis_run_id=$3 WHERE owner_id=$1 AND book_id=$2`, s.owner.ID, bookID, newRun)
	tx, err := s.store.Pool().Begin(s.ctx)
	require.NoError(s.t, err)
	require.NoError(s.t, persistence.BuildVocabularyBrowseCountsTx(s.ctx, tx, s.owner.ID, bookID, sourceID, newRun, newCorpus, "de"))
	require.NoError(s.t, tx.Commit(s.ctx))
	return newRun
}

func TestStudyAnalysisUpdateThatLeavesTheLemmaUnchangedKeepsTheMaterial(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	haus := s.word("haus")
	original := s.material(haus.MaterialID)

	newRun := s.republish(bookID, "")
	s.clock.Advance(24 * time.Hour)
	s.grade(s.word("buch"), "got-it")
	calls := s.provider.Calls()
	page := s.page()

	kept := s.word("haus")
	assert.False(t, kept.Reintroducing, "the occurrence still resolves to the word")
	assert.Equal(t, haus.MaterialID, kept.MaterialID, "the material is kept")
	assert.False(t, kept.Corrected())
	assert.Equal(t, haus.LearnedMeaning, kept.LearnedMeaning)
	assert.Equal(t, haus.Memory, kept.Memory)
	moved := s.material(kept.MaterialID)
	assert.Equal(t, newRun, moved.AnalysisRunID, "its source names the occurrence in the published run")
	assert.Equal(t, original.SentenceOrdinal+10, moved.SentenceOrdinal)
	assert.Equal(t, original.Sentence, moved.Sentence)
	assert.Equal(t, original.Gloss, moved.Gloss)
	assert.Equal(t, haus.ID, s.currentCard().ID, "it is shown as the review it was")
	assert.NotContains(t, page, "study-corrected")
	assert.Equal(t, calls, s.provider.Calls(), "no provider is asked")
	assert.Zero(t, s.counts().Preparing)
}

func TestStudyAnalysisUpdateThatChangesTheLemmaReintroducesTheWord(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	haus := s.word("haus")
	original := s.material(haus.MaterialID)

	s.republish(bookID, "hof", original.SentenceOrdinal)
	s.clock.Advance(24 * time.Hour)
	s.grade(s.word("buch"), "got-it")
	s.page()
	assert.True(t, s.word("haus").Reintroducing, "the introducing occurrence now resolves to another word")

	testutil.Eventually(t, 60*time.Second, "the word to be re-introduced", func() (bool, string) {
		s.page()
		return s.word("haus").MaterialID != haus.MaterialID, "the word is still waiting for new material"
	})
	reintroduced := s.word("haus")
	assert.True(t, reintroduced.Corrected())
	assert.Equal(t, haus.Memory, reintroduced.Memory)
	assert.NotEqual(t, original.SentenceOrdinal+10, s.material(reintroduced.MaterialID).SentenceOrdinal)
}

func TestStudyWordWithdrawnForUnpreparableMaterialReturnsOnlyWhenItsEvidenceChanges(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	haus := s.word("haus")
	s.provider.omit("haus", true)
	s.decide(bookID, "haus", s.material(haus.MaterialID).SentenceOrdinal, "correct", "hof")
	s.clock.Advance(24 * time.Hour)
	s.grade(s.word("buch"), "got-it")

	testutil.Eventually(t, 60*time.Second, "the word to be withdrawn", func() (bool, string) {
		s.page()
		return s.word("haus").Withdrawn, "the word is not withdrawn yet"
	})
	assert.Equal(t, haus.Memory, s.word("haus").Memory, "its progress is kept")
	page := s.page()
	assert.Contains(t, page, "Waiting for a usable sentence", "valid occurrences remain, so the word is not withdrawn by a correction")
	assert.NotContains(t, page, "Withdrawn by a correction")
	assert.True(t, s.today().Finished, "the word stays out of Today")

	// The same evidence is not tried again: no provider is asked.
	testutil.Eventually(t, 30*time.Second, "preparation to settle", s.settled)
	calls := s.provider.Calls()
	for range 3 {
		s.page()
	}
	assert.True(t, s.word("haus").Withdrawn)
	assert.Zero(t, s.counts().Preparing, "nothing is enqueued for it")
	assert.Equal(t, calls, s.provider.Calls(), "the same evidence does not loop provider calls")

	// A new Book adds occurrences of the word: the evidence differs, so it is
	// tried again, and now succeeds.
	s.provider.omit("haus", false)
	s.seedBook("library", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 2})
	testutil.Eventually(t, 60*time.Second, "the word to return", func() (bool, string) {
		s.page()
		returned := s.word("haus")
		return !returned.Withdrawn && returned.MaterialID != haus.MaterialID, "the word has not returned"
	})
	assert.True(t, s.word("haus").Corrected())
	assert.Equal(t, haus.Memory, s.word("haus").Memory)
}
