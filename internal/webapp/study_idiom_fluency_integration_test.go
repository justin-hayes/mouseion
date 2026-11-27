//go:build integration

package webapp

import (
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// matureIdiomHarness introduces "mit dem haus" from a Book with several other
// sentences of it (forms of "haus" that differ), answers it Got it, and makes
// it mature: a 30-day interval. The returned switch can be turned off, as the
// operator can.
func matureIdiomHarness(t *testing.T) (*studyHarness, domain.StudyProgress, *atomic.Bool) {
	t.Helper()
	lookup := newFakeIdiomLookup(withSecondSense(germanIdiom("mit dem haus")))
	avoidance := germanIdiomAvoidance(t, lookup, true)
	var on atomic.Bool
	on.Store(true)
	enabled := avoidance.Enabled
	avoidance.Enabled = func(language string) bool { return on.Load() && enabled(language) }
	s := newStudyHarnessWith(t, studyHarnessOptions{capabilities: readyGerman(), idioms: avoidance})
	book := s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	for _, form := range fluencyForms {
		s.addFluencySentence("reading", form)
	}
	s.buildIdiomCounts(book, lookup)
	s.preparedIdiom()
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.take()
	s.gradeAll()
	// The words are reviewed in a few days; only the Idiom is mature.
	_, err := s.store.Pool().Exec(s.ctx, `UPDATE study_progress SET state='review', scheduled_days=3, stability=3, due=$2 WHERE owner_id=$1 AND item_kind='word'`,
		s.owner.ID, s.clock.Now().AddDate(0, 0, 3))
	require.NoError(t, err)
	idiomWord, ok := s.progressOfIdiom()
	require.True(t, ok)
	require.NotEmpty(t, idiomWord.LearnedCitations.SenseIDs, "the Learned meaning cites the sense it was introduced with")
	matured := s.makeMature("mit dem haus", 30)
	return s, matured, &on
}

// idiomContexts are the stored Fluency contexts of the Idiom.
func (s *studyHarness) idiomContexts() []domain.FluencyContext {
	s.t.Helper()
	var contexts []domain.FluencyContext
	for _, item := range s.fluencyContexts() {
		if item.ItemKind == domain.StudyItemIdiom {
			contexts = append(contexts, item)
		}
	}
	return contexts
}

// fluencyJobs counts the Fluency preparation jobs still to run.
func (s *studyHarness) fluencyJobs() int {
	s.t.Helper()
	var n int
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM river_job WHERE kind='study_prepare_fluency' AND state IN ('available','pending','retryable','scheduled','running')`).Scan(&n))
	return n
}

func TestStudyFluencyPreparesUpToFiveContextsForAMatureIdiomAndKeepsDisagreeingOnesAsEvidence(t *testing.T) {
	s, idiomWord, _ := matureIdiomHarness(t)
	// One sentence uses the words literally; another cites the entry's other sense.
	s.provider.answerIdiomSentence("Mara ist nicht mit dem hauses.", idiomLiteralUse)
	s.provider.answerIdiomSentence("Nina ist nicht mit dem haeusern.", idiomOtherSense)
	require.True(t, domain.IsStudyMature(idiomWord))
	before := len(s.provider.idiomRequests())

	s.openFluency()
	testutil.Eventually(t, 30*time.Second, "the job to finish asking", func() (bool, string) {
		return len(s.idiomContexts()) >= domain.FluencyContextsPerWord+2, "still asking"
	})

	features := map[string]bool{}
	agreeing := 0
	for _, item := range s.idiomContexts() {
		assert.Equal(t, idiomWord.ID, item.ProgressID)
		assert.Equal(t, domain.StudyItemIdiom, item.ItemKind)
		assert.NotContains(t, item.Sentence, "Anna", "the introducing sentence is never asked about")
		if item.Agreement == domain.FluencyAgrees {
			agreeing++
			features[item.Form.Features] = true
			assert.Equal(t, idiomWord.LearnedMeaning, item.Gloss)
			assert.Equal(t, idiomWord.LearnedCitations.SenseIDs, item.Cited.SenseIDs, "an agreeing context cites the Learned meaning's sense")
			assert.Regexp(t, `^mit dem \S+$`, item.TargetWord, "the matched words are stored")
			continue
		}
		switch item.Sentence {
		case "Mara ist nicht mit dem hauses.":
			assert.Equal(t, domain.FluencyReasonLiteralUse, item.Reason, "literal use")
		case "Nina ist nicht mit dem haeusern.":
			assert.Equal(t, domain.FluencyReasonDifferentMeaning, item.Reason, "another sense")
			assert.Equal(t, "the idiom mit dem haus", item.Gloss, "a disagreeing context keeps its Gloss as evidence")
			assert.NotEmpty(t, item.SentenceTranslation)
		default:
			assert.Fail(t, "an unexpected context disagrees", item.Sentence)
		}
	}
	assert.Equal(t, domain.FluencyContextsPerWord, agreeing, "at most five contexts per Idiom")
	assert.Len(t, features, 4, "the three agreeing contexts of other forms come first; Tina and Emma, whose forms were already chosen, fill the rest")
	assert.Len(t, s.provider.idiomRequests()[before:], domain.FluencyContextsPerWord+2, "an assessed occurrence is not asked again")

	// A disagreeing context is never shown.
	for _, answer := range []string{"", "?answer=1"} {
		page := getAs(t, s.h, "/de/study"+answer, s.cookies).Body.String()
		assert.NotContains(t, page, "Mara ist nicht mit dem hauses.")
		assert.NotContains(t, page, "Nina ist nicht mit dem haeusern.")
	}
}

func TestStudyFluencyShowsAnIdiomContextInTheIdiomLayoutAndNeverChangesProgress(t *testing.T) {
	s, idiomWord, _ := matureIdiomHarness(t)
	s.openFluency()

	page := getAs(t, s.h, "/de/study", s.cookies).Body.String()
	require.Contains(t, page, `id="fluency-card-heading"`)
	for _, word := range []string{"mit", "dem"} {
		assert.Contains(t, page, `<mark class="study-card__target"><strong>`+word+`</strong></mark>`, "every matched word is marked")
	}
	assert.NotContains(t, page, `id="fluency-kind"`, "the idiom label is on the answer side")
	back := getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String()
	assert.Contains(t, back, `id="fluency-kind"`)
	assert.Contains(t, back, `id="fluency-headword"`)
	assert.Contains(t, back, idiomWord.LearnedMeaning)
	assert.Contains(t, back, "<b>mit</b> <b>dem</b>", "the translation is highlighted")
	assert.Contains(t, back, `value="recognised"`)
	assert.Contains(t, back, `value="not-recognised"`)
	assert.NotContains(t, back, `value="got-it"`, "Fluency is ungraded")

	progressBefore, logBefore := s.progress(), s.reviewLog(idiomWord.ID)
	for i := range domain.FluencyContextsPerWord + 1 {
		card, ok := domain.NextFluencyCard(domain.FluencyRotation{Contexts: s.fluencyContexts(), Progress: s.progress(), Now: s.clock.Now()})
		require.True(t, ok)
		assert.Equal(t, domain.StudyItemIdiom, card.Context.ItemKind)
		response := "recognised"
		if i%2 == 0 {
			response = "not-recognised"
		}
		s.clock.Advance(time.Minute)
		require.Equal(t, 303, s.post("/study/fluency/responses", url.Values{"language": {"de"}, "context": {card.Context.ID}, "response": {response}}).Code)
	}
	var responses int
	require.NoError(t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM study_fluency_response WHERE owner_id=$1`, s.owner.ID).Scan(&responses))
	assert.Equal(t, domain.FluencyContextsPerWord+1, responses, "every answer is kept")
	assert.Equal(t, progressBefore, s.progress(), "Fluency never changes Study progress or the schedule")
	assert.Equal(t, logBefore, s.reviewLog(idiomWord.ID), "Fluency never writes the review log")
}

func TestStudyFluencyPreparesNoIdiomContextsWhileTheSwitchIsOff(t *testing.T) {
	s, _, on := matureIdiomHarness(t)
	on.Store(false)
	before := len(s.provider.idiomRequests())

	require.Equal(t, 200, getAs(t, s.h, "/de/study", s.cookies).Code)
	require.Eventually(t, func() bool { return s.planJobs() == 0 && s.fluencyJobs() == 0 }, 30*time.Second, 100*time.Millisecond, "the preparation job to run")
	assert.Empty(t, s.idiomContexts(), "no Idiom context is prepared")
	assert.Len(t, s.provider.idiomRequests(), before, "nothing is sent")
}
