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

// The Book detail breakdown reads each word's Study state from Postgres: a
// graded word on a day-scale schedule is Mature, a graded one that is not yet
// is In study.
func TestBookDetailShowsMatureAndInStudyWordsFromStudyProgress(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	s.introduceAllGotIt()
	s.makeMature("haus", 30)

	body := getAs(t, s.h, bookDetailURL(bookID), s.cookies)
	require.Equal(t, http.StatusOK, body.Code)
	assert.Contains(t, body.Body.String(), `data-study-state>Mature</td>`)
	assert.Contains(t, body.Body.String(), `data-study-state>In study</td>`)
}

// Retry posted from the Book detail page returns to it, not to My Books.
func TestBookDetailRetryReturnsToTheBookDetailPage(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})
	s.clock.Advance(time.Minute)

	response := perform(t, s.h, http.MethodPost, readingReanalyzeURL(bookID), url.Values{
		"csrf_token": {s.cookies[0].Value}, "return_to": {bookDetailURL(bookID)},
	}, s.cookies)
	require.Equal(t, http.StatusSeeOther, response.Code, response.Body.String())
	assert.Contains(t, response.Header().Get("Location"), bookDetailURL(bookID)+"?")

	foreign := perform(t, s.h, http.MethodPost, readingReanalyzeURL(bookID), url.Values{
		"csrf_token": {s.cookies[0].Value}, "return_to": {"https://example.test/"},
	}, s.cookies)
	require.Equal(t, http.StatusSeeOther, foreign.Code)
	assert.NotContains(t, foreign.Header().Get("Location"), "example.test")
	assert.NotContains(t, foreign.Header().Get("Location"), bookDetailURL(bookID), "only the Book's own detail page is a return target")
}
