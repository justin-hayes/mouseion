//go:build integration

package webapp

import (
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// idiomMaterial is the learner's current Study material of the Idiom "mit dem
// haus", if any.
func (s *studyHarness) idiomMaterial() (domain.StudyMaterial, bool) {
	s.t.Helper()
	material, err := s.store.ListCurrentStudyMaterial(s.ctx, s.owner.ID, "de")
	require.NoError(s.t, err)
	for _, item := range material {
		if item.IsIdiom() && item.CanonicalLemma == "mit dem haus" {
			return item, true
		}
	}
	return domain.StudyMaterial{}, false
}

// idiomSettled reports that the words have settled and the Idiom's material is
// no longer pending (Today's counts are of words only).
func (s *studyHarness) idiomSettled() (bool, string) {
	if ok, why := s.settled(); !ok {
		return false, why
	}
	item, ok := s.idiomMaterial()
	return ok && item.State != domain.StudyMaterialPending, "the Idiom is still preparing"
}

// startStudy opens the page so planning is queued and starts River.
func (s *studyHarness) startStudy() {
	s.t.Helper()
	require.Equal(s.t, http.StatusOK, getAs(s.t, s.h, "/de/study", s.cookies).Code)
	require.NoError(s.t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(s.t, "study River client", s.client.Stop)
}

// idiomStudyHarness seeds a Book where "haus" occurs in three sentences, each
// an occurrence of the Idiom "mit dem haus", and "buch" in two, and counts the
// Idiom candidates as the job does.
func idiomStudyHarness(t *testing.T, on bool) *studyHarness {
	t.Helper()
	lookup := newFakeIdiomLookup(germanIdiom("mit dem haus"))
	s := newStudyHarnessWith(t, studyHarnessOptions{capabilities: readyGerman(), idioms: germanIdiomAvoidance(t, lookup, on)})
	book := s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.buildIdiomCounts(book, lookup)
	return s
}

func TestStudyPreparesIdiomMaterialAlongsideWordsInStreamOrder(t *testing.T) {
	s := idiomStudyHarness(t, true)
	s.startStudy()
	testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) { return s.idiomSettled() })

	assert.Equal(t, []string{"haus", "mit dem haus", "buch"}, s.lemmasByReservation(),
		"an Idiom ranks by its candidate count among the words, a tie going to the word")
	item, ok := s.idiomMaterial()
	require.True(t, ok)
	assert.Equal(t, domain.StudyMaterialReady, item.State)
	assert.Equal(t, "the idiom mit dem haus", item.Gloss)
	assert.Equal(t, insideSentence, item.Sentence)
	assert.Equal(t, "mit dem haus", item.TargetWord)
	assert.Equal(t, domain.StudyCitations{SenseIDs: []string{"test-idiom/mit dem haus/1"}, EntryIDs: []string{"mit dem haus"}, DictionaryPin: "test-dictionary/1"}, item.Cited, "the Gloss cites the dictionary sense it rests on")
	assert.Equal(t, []int{3, 4, 5}, item.MatchedTokenOrdinals)
	assert.Equal(t, int64(3), item.TokenOrdinal)
	assert.NotEmpty(t, item.SentenceTranslation)
	assert.Equal(t, domain.StudyMaterialCounts{Ready: 3}, s.counts(), "Today counts the Idiom as an item")
	require.Len(t, s.provider.idiomRequests(), 1, "one request for the Idiom")

	page := getAs(t, s.h, "/de/study", s.cookies)
	require.Equal(t, http.StatusOK, page.Code, "ready Idiom material does not break Today")
	assert.Contains(t, page.Body.String(), "2 new words and 1 idiom are ready", "the Idiom is offered in batches with the words")
}

func TestStudySkipsAnIdiomWhoseSentenceIsLiteralOrAmbiguous(t *testing.T) {
	for answer, reason := range map[idiomAnswer]string{idiomLiteralUse: domain.StudyOmissionLiteralUse, idiomAmbiguous: domain.StudyOmissionMeaning} {
		t.Run(string(answer), func(t *testing.T) {
			s := idiomStudyHarness(t, true)
			s.provider.answerIdioms("mit dem haus", answer)
			s.startStudy()
			testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) { return s.idiomSettled() })

			item, ok := s.idiomMaterial()
			require.True(t, ok)
			assert.Equal(t, domain.StudyMaterialUnpreparable, item.State)
			assert.Equal(t, reason, item.OmissionReason)
			assert.Equal(t, domain.StudyMaterialCounts{Ready: 2, Unpreparable: 1}, s.counts(), "the words after it are prepared")
		})
	}
}

func TestStudyRetriesAnIdiomAfterAProviderFailure(t *testing.T) {
	s := idiomStudyHarness(t, true)
	s.provider.answerIdioms("mit dem haus", idiomFailure)
	s.startStudy()
	testutil.Eventually(t, 60*time.Second, "the provider to fail once", func() (bool, string) {
		return len(s.provider.idiomRequests()) > 0, "no idiom request yet"
	})
	item, ok := s.idiomMaterial()
	require.True(t, ok)
	assert.Equal(t, domain.StudyMaterialPending, item.State, "a provider failure is not recorded as an omission")

	s.provider.answerIdioms("mit dem haus", idiomConfirmed)
	testutil.Eventually(t, 90*time.Second, "the retry to succeed", func() (bool, string) {
		item, _ := s.idiomMaterial()
		return item.State == domain.StudyMaterialReady, "the Idiom is not ready yet"
	})
}

func TestStudyPreparesNoIdiomWhileTheSwitchIsOff(t *testing.T) {
	s := idiomStudyHarness(t, false)
	s.startStudy()
	testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) { return s.settled() })

	assert.Equal(t, []string{"haus", "buch"}, s.lemmasByReservation())
	assert.Empty(t, s.provider.idiomRequests())
}

func TestStudyIdiomOfTheCurrentReadingIsInTheBoostedGroup(t *testing.T) {
	lookup := newFakeIdiomLookup(germanIdiom("mit dem haus"))
	s := newStudyHarnessWith(t, studyHarnessOptions{capabilities: readyGerman(), idioms: germanIdiomAvoidance(t, lookup, true)})
	reading := s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 1})
	library := s.seedBook("library", testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 5}, testutil.CorpusWord{Lemma: "baum", UPOS: "NOUN", Occurrences: 4})
	s.buildIdiomCounts(reading, lookup)
	s.buildIdiomCounts(library, lookup)
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, s.owner.ID, reading.ID)
	require.NoError(t, err)

	s.startStudy()
	testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) { return s.settled() })

	assert.Equal(t, []string{"haus", "mit dem haus", "buch", "baum"}, s.lemmasByReservation(),
		"below Target coverage the Current reading's words and Idioms lead the stream, by their count within it")
}

func TestStudyIdiomMaterialIgnoresExcludedOccurrences(t *testing.T) {
	s := idiomStudyHarness(t, true)
	// The learner excluded "haus" from the first two sentences, so only the
	// third is a candidate of the Idiom.
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,excluded)
SELECT t.owner_id, cai.book_id, t.corpus_id, t.analysis_run_id, s.unit_id, t.start_offset, t.end_offset, NULL, true
  FROM corpus_tokens t
  JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id AND s.corpus_id=t.corpus_id AND s.sentence_ordinal=t.sentence_ordinal
  JOIN current_analysis_identity cai ON cai.owner_id=t.owner_id AND cai.analysis_run_id=t.analysis_run_id
 WHERE t.owner_id=$1 AND t.surface='haus' AND t.sentence_ordinal < 2`, s.owner.ID)
	require.NoError(t, err)
	var book string
	require.NoError(t, s.store.Pool().QueryRow(s.ctx, `SELECT book_id::text FROM current_analysis_identity WHERE owner_id=$1`, s.owner.ID).Scan(&book))
	s.buildIdiomCounts(domain.Book{ID: book}, newFakeIdiomLookup(germanIdiom("mit dem haus")))

	s.startStudy()
	testutil.Eventually(t, 60*time.Second, "material to settle", func() (bool, string) { return s.idiomSettled() })
	item, ok := s.idiomMaterial()
	require.True(t, ok)
	assert.Equal(t, domain.StudyMaterialReady, item.State)
	assert.Equal(t, int64(2), item.SentenceOrdinal, "the excluded occurrences are not candidate sentences")
}
