//go:build integration

package webapp

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fluencyForm is one other occurrence of "haus": a gate-passing sentence whose
// subject is name and whose target token has the surface and morphology given.
type fluencyForm struct {
	name, surface string
	morphology    map[string]string
}

// addFluencySentence adds one more sentence of "haus" to the Book's corpus.
func (s *studyHarness) addFluencySentence(book domain.Book, key string, form fluencyForm) string {
	s.t.Helper()
	sentence := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN"})
	// A name of the same length keeps every offset; the target is the last
	// token, so its surface can change length.
	sentence.Text = strings.Replace(sentence.Text, "Anna", form.name, 1)
	sentence.Tokens[0].Surface, sentence.Tokens[0].RawLemma, sentence.Tokens[0].CanonicalLemma = form.name, form.name, form.name
	target := len(sentence.Tokens) - 1
	start := sentence.Tokens[target].Location.StartOffset
	sentence.Text = sentence.Text[:start] + form.surface + "."
	sentence.Tokens[target].Surface = form.surface
	sentence.Tokens[target].Location = analyzer.SourceLocation{StartOffset: start, EndOffset: start + uint64(len(form.surface))}
	sentence.Tokens[target].Morphology = form.morphology
	var runID, corpusID string
	var ordinal, offset int64
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT analysis_run_id::text, corpus_id::text, max(sentence_ordinal)+1, max(end_offset)+1 FROM corpus_sentences WHERE owner_id=$1 GROUP BY analysis_run_id, corpus_id ORDER BY max(sentence_ordinal) DESC LIMIT 1`, s.owner.ID).Scan(&runID, &corpusID, &ordinal, &offset))
	testutil.SeedCorpusSentence(s.t, s.ctx, s.store.Pool(), s.owner.ID, corpusID, "de", runID, domain.EPUBUnitID(0, key), ordinal, offset, sentence)
	return sentence.Text
}

var fluencyForms = []fluencyForm{
	{"Emma", "haus", map[string]string{"Case": "Nom", "Number": "Sing"}}, // the introduced form
	{"Lena", "haeuser", map[string]string{"Case": "Nom", "Number": "Plur"}},
	{"Mara", "hauses", map[string]string{"Case": "Gen", "Number": "Sing"}},
	{"Nina", "haeusern", map[string]string{"Case": "Dat", "Number": "Plur"}},
	{"Olga", "hause", map[string]string{"Case": "Dat", "Number": "Sing"}},
	{"Rosa", "haeuserns", map[string]string{"Case": "Gen", "Number": "Plur"}},
	// The same features as Lena's: ranked after every different form.
	{"Tina", "haeuschen", map[string]string{"Case": "Nom", "Number": "Plur"}},
}

// matureHaus introduces "haus" from a Book with several other sentences of it,
// answers it Got it, and makes it mature: a 30-day interval, as weeks of
// reviews would leave it.
func (s *studyHarness) matureHaus() domain.StudyProgress {
	s.t.Helper()
	book := s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 1})
	for _, form := range fluencyForms {
		s.addFluencySentence(book, "reading", form)
	}
	require.Equal(s.t, http.StatusOK, getAs(s.t, s.h, "/de/study", s.cookies).Code)
	require.NoError(s.t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(s.t, "study River client", s.client.Stop)
	testutil.Eventually(s.t, 60*time.Second, "material to settle", func() (bool, string) {
		return s.counts().Preparing == 0, "preparing words remain"
	})
	s.clock.Set(berlinTime(s.t, 25, 9, 0))
	require.Equal(s.t, http.StatusSeeOther, s.post("/study/batches/take", url.Values{"language": {"de"}}).Code)
	s.grade(s.currentCard(), "got-it")
	return s.makeMature("haus", 30)
}

// makeMature puts a word on a day-scale schedule of the given interval.
func (s *studyHarness) makeMature(lemma string, days int) domain.StudyProgress {
	s.t.Helper()
	_, err := s.store.Pool().Exec(s.ctx, `UPDATE study_progress SET state='review', scheduled_days=$3, stability=$5, due=$4 WHERE owner_id=$1 AND canonical_lemma=$2`,
		s.owner.ID, lemma, days, s.clock.Now().AddDate(0, 0, days), float64(days))
	require.NoError(s.t, err)
	for _, item := range s.progress() {
		if item.CanonicalLemma == lemma {
			return item
		}
	}
	s.t.Fatalf("no study progress for %s", lemma)
	return domain.StudyProgress{}
}

func (s *studyHarness) fluencyContexts() []domain.FluencyContext {
	s.t.Helper()
	contexts, err := s.store.ListStudyFluencyContexts(s.ctx, s.owner.ID, "de")
	require.NoError(s.t, err)
	return contexts
}

// openFluency opens the Study page, which enqueues the word's Fluency
// preparation, and waits until its job has stored contexts.
func (s *studyHarness) openFluency(agreeing int) {
	s.t.Helper()
	require.Equal(s.t, http.StatusOK, getAs(s.t, s.h, "/de/study", s.cookies).Code)
	testutil.Eventually(s.t, 60*time.Second, "Fluency contexts", func() (bool, string) {
		count := 0
		for _, item := range s.fluencyContexts() {
			if item.Agreement == domain.FluencyAgrees {
				count++
			}
		}
		return count >= agreeing, "contexts are still being prepared"
	})
}

func TestStudyFluencyPreparesUpToFiveAgreeingContextsOfDifferentForms(t *testing.T) {
	s := newStudyHarness(t)
	// One occurrence answers "another sense", so it is discarded.
	s.provider.disagree["Mara ist nicht mit dem hauses."] = true
	word := s.matureHaus()
	require.True(t, domain.IsStudyMature(word))

	// Opening the page enqueues the preparation; the learned meaning goes to
	// the provider from the job alone.
	s.openFluency(domain.FluencyContextsPerWord)
	testutil.Eventually(t, 30*time.Second, "the job to finish asking", func() (bool, string) {
		return len(s.fluencyContexts()) >= domain.FluencyContextsPerWord+1, "still asking"
	})

	contexts := s.fluencyContexts()
	agreeing, disagreeing := 0, 0
	features := map[string]bool{}
	for _, item := range contexts {
		assert.Equal(t, word.ID, item.ProgressID)
		if item.Agreement == domain.FluencyAgrees {
			agreeing++
			features[item.Form.Features] = true
			assert.Equal(t, word.LearnedMeaning, item.Gloss, "a context agrees with the learned meaning")
		} else {
			disagreeing++
			assert.Empty(t, item.Gloss)
		}
	}
	assert.Equal(t, domain.FluencyContextsPerWord, agreeing, "at most five contexts per word")
	assert.Equal(t, 1, disagreeing, "a context that does not agree is kept only as an answer already given")
	assert.Len(t, features, domain.FluencyContextsPerWord, "different forms are preferred: no two contexts share their features")
	for _, request := range s.provider.fluencyRequests() {
		assert.Equal(t, word.LearnedMeaning, request.LearnedMeaning, "the request carries the learned meaning")
		assert.NotContains(t, request.ExampleSentence, "Anna", "the introducing sentence is never asked about")
	}
	assert.Len(t, s.provider.fluencyRequests(), domain.FluencyContextsPerWord+1, "an assessed occurrence is not asked again")
	for _, request := range s.provider.fluencyRequests() {
		assert.NotContains(t, request.ExampleSentence, "Tina", "a sentence of an already chosen form comes after every different form")
	}

	// Opening the page again prepares nothing more.
	getAs(t, s.h, "/de/study", s.cookies)
	time.Sleep(200 * time.Millisecond)
	assert.Len(t, s.provider.fluencyRequests(), domain.FluencyContextsPerWord+1)
}

func TestStudyFluencyCardsAreUngradedAndNeverChangeProgress(t *testing.T) {
	s := newStudyHarness(t)
	word := s.matureHaus()
	s.openFluency(domain.FluencyContextsPerWord)

	page := getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, `id="fluency-card-heading"`)
	assert.Contains(t, page, `<mark class="study-card__target">`)
	assert.Contains(t, page, "Show answer")
	assert.NotContains(t, page, `value="recognised"`, "the answer is hidden until Show answer")
	page = getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String()
	assert.Contains(t, page, `id="fluency-gloss"`)
	assert.Contains(t, page, word.LearnedMeaning)
	assert.Contains(t, page, `id="fluency-translation"`)
	assert.Contains(t, page, `value="recognised"`)
	assert.Contains(t, page, `value="not-recognised"`)
	assert.NotContains(t, page, `value="got-it"`, "Fluency is ungraded")

	progressBefore, logBefore := s.progress(), s.reviewLog(word.ID)
	shown := map[string]bool{}
	for i := 0; i < domain.FluencyContextsPerWord+1; i++ {
		card, ok := domain.NextFluencyCard(s.fluencyContexts(), s.progress())
		require.True(t, ok)
		if i < domain.FluencyContextsPerWord {
			assert.False(t, shown[card.Context.ID], "every context is shown once before one repeats")
		}
		shown[card.Context.ID] = true
		response := "recognised"
		if i%2 == 0 {
			response = "not-recognised"
		}
		s.clock.Advance(time.Minute)
		result := s.post("/study/fluency/responses", url.Values{"language": {"de"}, "context": {card.Context.ID}, "response": {response}})
		require.Equal(t, http.StatusSeeOther, result.Code)
		assert.Equal(t, "/de/study", result.Header().Get("Location"))
	}
	assert.Len(t, shown, domain.FluencyContextsPerWord)

	var responses int
	require.NoError(t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM study_fluency_response WHERE owner_id=$1`, s.owner.ID).Scan(&responses))
	assert.Equal(t, domain.FluencyContextsPerWord+1, responses, "every answer is kept")
	assert.Equal(t, progressBefore, s.progress(), "Fluency never changes Study progress or the schedule")
	assert.Equal(t, logBefore, s.reviewLog(word.ID), "Fluency never writes the review log")

	// A forged, stale or malformed response records nothing.
	assert.Equal(t, http.StatusForbidden, perform(t, s.h, http.MethodPost, "/study/fluency/responses", url.Values{"csrf_token": {"forged"}, "language": {"de"}, "context": {"x"}, "response": {"recognised"}}, s.cookies).Code)
	assert.Equal(t, http.StatusBadRequest, s.post("/study/fluency/responses", url.Values{"language": {"de"}, "context": {"x"}, "response": {"got-it"}}).Code, "a Fluency response is not a grade")
	assert.Equal(t, http.StatusSeeOther, s.post("/study/fluency/responses", url.Values{"language": {"de"}, "context": {"00000000-0000-4000-8000-000000000000"}, "response": {"recognised"}}).Code)
	assert.Equal(t, http.StatusNotFound, s.post("/study/fluency/responses", url.Values{"language": {"la"}, "context": {"x"}, "response": {"recognised"}}).Code)
	require.NoError(t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM study_fluency_response WHERE owner_id=$1`, s.owner.ID).Scan(&responses))
	assert.Equal(t, domain.FluencyContextsPerWord+1, responses)
}

func TestStudyFluencyIsReachableOnlyAfterTheFinishLineAndTheOfferedBatch(t *testing.T) {
	s := newStudyHarness(t)
	s.matureHaus()
	s.openFluency(domain.FluencyContextsPerWord)
	card := func() string { return getAs(t, s.h, "/de/study", s.cookies).Body.String() }

	// Today is finished and the day's batch was taken: Fluency is open, and no
	// word is waiting.
	assert.Contains(t, card(), `id="fluency-card-heading"`)

	// A review is due on the next Study day: Fluency waits for the finish line.
	s.seedBook("library", testutil.CorpusWord{Lemma: "stuhl", UPOS: "NOUN", Occurrences: 2})
	require.Equal(t, http.StatusOK, getAs(t, s.h, "/de/study", s.cookies).Code)
	testutil.Eventually(t, 60*time.Second, "the next word to be prepared", func() (bool, string) {
		counts := s.counts()
		return counts.Preparing == 0 && counts.Ready >= 2, "preparing words remain"
	})
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO study_material(owner_id,language,canonical_lemma,upos,state,book_id,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,sentence,target_word,gloss,sentence_translation)
		SELECT owner_id,language,'buch',upos,'ready',book_id,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,sentence,'buch','the book','The book.' FROM study_material WHERE owner_id=$1 AND canonical_lemma='haus'`, s.owner.ID)
	require.NoError(t, err)
	_, err = s.store.Pool().Exec(s.ctx, `INSERT INTO study_progress(owner_id,language,canonical_lemma,upos,material_id,introduced_at,learned_meaning,state,due,stability,difficulty,scheduled_days,reps,last_review)
		SELECT m.owner_id,m.language,'buch',m.upos,m.id,$2,'the book','review',$3,3,5,3,2,$2 FROM study_material m WHERE m.owner_id=$1 AND m.canonical_lemma='buch'`,
		s.owner.ID, berlinTime(t, 20, 9, 0), berlinTime(t, 26, 9, 0))
	require.NoError(t, err)
	s.clock.Set(berlinTime(t, 26, 9, 0))
	page := card()
	assert.Contains(t, page, "1 review left today.")
	assert.NotContains(t, page, `id="fluency-card-heading"`, "Fluency is reachable only after the finish line")
	card0, ok := domain.NextFluencyCard(s.fluencyContexts(), s.progress())
	require.True(t, ok)
	s.post("/study/fluency/responses", url.Values{"language": {"de"}, "context": {card0.Context.ID}, "response": {"recognised"}})
	var responses int
	require.NoError(t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM study_fluency_response WHERE owner_id=$1`, s.owner.ID).Scan(&responses))
	assert.Zero(t, responses, "a response before the finish line records nothing")

	// The review answered, the day's batch is still on offer and unanswered:
	// Fluency waits for it to be taken or declined.
	s.grade(s.currentCard(), "got-it")
	require.True(t, s.today().Finished)
	page = card()
	assert.Contains(t, page, "Take these words")
	assert.NotContains(t, page, `id="fluency-card-heading"`, "an offered batch must be taken or declined first")

	require.Equal(t, http.StatusSeeOther, s.post("/study/batches/decline", url.Values{"language": {"de"}}).Code)
	assert.Contains(t, card(), `id="fluency-card-heading"`, "declining the batch opens Fluency")
}

func TestStudyFluencyEndsWhenAWordLapsesAndTodayNotesNothingMature(t *testing.T) {
	s := newStudyHarness(t)
	s.matureHaus()
	s.openFluency(domain.FluencyContextsPerWord)
	assert.Contains(t, getAs(t, s.h, "/de/study", s.cookies).Body.String(), `id="fluency-card-heading"`)

	// A word one day short of mature is not eligible.
	s.makeMature("haus", 20)
	page := getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.NotContains(t, page, `id="fluency-card-heading"`)
	assert.Contains(t, page, `id="study-nothing-mature"`, "Today ends by saying nothing is mature yet")
	s.makeMature("haus", 21)
	assert.Contains(t, getAs(t, s.h, "/de/study", s.cookies).Body.String(), `id="fluency-card-heading"`, "an interval of 21 days is mature")

	// A lapse: the word is relearning with a short interval.
	_, err := s.store.Pool().Exec(s.ctx, `UPDATE study_progress SET state='relearning', scheduled_days=0, lapses=1, due=$2 WHERE owner_id=$1`, s.owner.ID, s.clock.Now().Add(10*time.Minute))
	require.NoError(t, err)
	page = getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.NotContains(t, page, `id="fluency-card-heading"`, "a lapsed word leaves Fluency until it is mature again")
}
