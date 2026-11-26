//go:build integration

package webapp

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// studyScaleDeadline is how long the Study page and its actions may take on a
// library the size of the deployed one (#2707).
const studyScaleDeadline = 300 * time.Millisecond

// studyScaleIdentities is the number of vocabulary identities added to the
// learner's Book: about a deployed library's inventory (a 552,000-token library
// holds well over a hundred thousand identities across its Books).
const studyScaleIdentities = 250000

// inflateLibrary adds identities to the learner's analyzed Book that rank after
// every word the test prepares, then plans against fresh statistics as a
// deployed database has.
func (s *studyHarness) inflateLibrary(n int) {
	s.t.Helper()
	_, err := s.store.Pool().Exec(s.ctx, `
INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count,lexical_discriminator)
SELECT b.owner_id,b.book_id,b.language,b.analysis_run_id,b.corpus_id,'zz'||lpad(g::text,7,'0'),(ARRAY['NOUN','VERB','ADJ','ADV'])[g%4+1],1,''
  FROM generate_series(1,$2::int) g
 CROSS JOIN (SELECT owner_id,book_id,language,analysis_run_id,corpus_id FROM vocabulary_browse_counts WHERE owner_id=$1 LIMIT 1) b`, s.owner.ID, n)
	require.NoError(s.t, err)
	_, err = s.store.Pool().Exec(s.ctx, `ANALYZE vocabulary_browse_counts`)
	require.NoError(s.t, err)
}

// settle opens the page until the preparation it enqueues is done, so a timed
// request does not compete with the jobs of an earlier one.
func (s *studyHarness) settle() {
	s.t.Helper()
	require.Equal(s.t, http.StatusOK, getAs(s.t, s.h, "/de/study", s.cookies).Code)
	testutil.Eventually(s.t, 60*time.Second, "preparation to settle", func() (bool, string) {
		return s.counts().Preparing == 0, "words are still being prepared"
	})
	require.Equal(s.t, http.StatusOK, getAs(s.t, s.h, "/de/study", s.cookies).Code)
	testutil.Eventually(s.t, 60*time.Second, "preparation to settle", func() (bool, string) {
		return s.counts().Preparing == 0, "words are still being prepared"
	})
}

// within fails the test when the request takes longer than the deadline.
func (s *studyHarness) within(what string, request func() int, want int) {
	s.t.Helper()
	started := time.Now()
	code := request()
	elapsed := time.Since(started)
	s.t.Logf("%s took %s", what, elapsed)
	assert.Equal(s.t, want, code, what)
	assert.Less(s.t, elapsed, studyScaleDeadline, "%s took %s on the scale library", what, elapsed)
}

func (s *studyHarness) timedPage(what string) {
	s.t.Helper()
	s.within(what, func() int { return getAs(s.t, s.h, "/de/study", s.cookies).Code }, http.StatusOK)
}

func (s *studyHarness) timedPost(what, path string, form url.Values) {
	s.t.Helper()
	s.within(what, func() int { return s.post(path, form).Code }, http.StatusSeeOther)
}

func TestStudyBatchScreenAndActionsAvoidTheFullCandidateScanOnTheScaleLibrary(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.inflateLibrary(studyScaleIdentities)
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.settle()

	s.timedPage("the new-batch screen")
	assert.Equal(t, 1, s.ledger().Offered)
	s.timedPost("declining the batch", "/study/batches/decline", url.Values{"language": {"de"}, "offer": {"1"}})
	assert.Equal(t, 1, s.ledger().Declined)
	s.timedPage("the Fluency state after declining")
	s.timedPost("taking the declined batch", "/study/batches/take", url.Values{"language": {"de"}})
	assert.Len(t, s.progress(), 10, "the batch is the ten words that are ready, first in stream order")
}

func TestStudyFluencyPageAndResponseAvoidTheFullCandidateScanOnTheScaleLibrary(t *testing.T) {
	s := newStudyHarness(t)
	s.matureHaus()
	s.openFluency()
	s.inflateLibrary(studyScaleIdentities)
	s.settle()

	card := s.shownFluencyContext()
	require.NotEmpty(t, card)
	s.timedPage("the Fluency page")
	s.clock.Advance(time.Minute)
	s.timedPost("answering a Fluency card", "/study/fluency/responses", url.Values{"language": {"de"}, "context": {card}, "response": {"recognised"}})
}

// TestStudyReportReconciliationIsBoundedByOpenReportsOnTheScaleLibrary holds a
// batch of reported words on the scale library and holds that Today's
// reconciliation of them, and the one a Lemma decision triggers, cost a bounded
// number of token lookups per report, never a scan of the candidates.
func TestStudyReportReconciliationIsBoundedByOpenReportsOnTheScaleLibrary(t *testing.T) {
	s := newStudyHarness(t)
	s.preparedTwelve()
	s.inflateLibrary(studyScaleIdentities / 5)
	s.clock.Set(berlinTime(t, 25, 9, 0))
	s.settle()
	s.introduceAllGotIt()
	s.clock.Advance(24 * time.Hour)
	reported := 0
	for _, word := range s.progress() {
		require.Equal(t, http.StatusSeeOther, s.report(word, "wrong_word", "").Code)
		reported++
	}
	require.Len(t, s.openReports(), reported)
	require.Greater(t, reported, 5)

	s.timedPage("Today with every word reported")
	assert.Len(t, s.openReports(), reported, "the occurrences are unchanged, so every report stays open")
	s.timedPage("Today again")
	s.within("the Reported words page", func() int { return getAs(t, s.h, "/de/study/reports", s.cookies).Code }, http.StatusOK)
}
