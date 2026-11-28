//go:build integration

package webapp

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *studyHarness) recordPosition(bookID, percent string, extra url.Values) *httptest.ResponseRecorder {
	s.t.Helper()
	form := url.Values{"csrf_token": {s.cookies[0].Value}, "return_to": {bookDetailURL(bookID)}, "percent": {percent}}
	maps.Copy(form, extra)
	return perform(s.t, s.h, http.MethodPost, readingPositionURL(bookID), form, s.cookies)
}

func TestBookDetailWithoutAPositionSaysSo(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})

	page := getAs(t, s.h, bookDetailURL(bookID), s.cookies)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Reading position")
	assert.Contains(t, body, "No position yet")
	assert.Contains(t, body, `<label for="reading-position-percent">Percentage read</label>`)
	assert.Contains(t, body, `inputmode="decimal"`)
}

func TestEnteringAPercentageShowsTheReadingPositionOnBookDetail(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})

	response := s.recordPosition(bookID, "35.7", nil)
	require.Equal(t, http.StatusSeeOther, response.Code, response.Body.String())
	location := response.Header().Get("Location")
	assert.Contains(t, location, bookDetailURL(bookID)+"?message=")

	page := getAs(t, s.h, location, s.cookies)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Reading position recorded.")
	assert.Contains(t, body, "<strong>35%</strong>", "the percentage is whole and rounded down")
	assert.Contains(t, body, `<time datetime="2026-11-25">November 25, 2026</time>`)
	assert.Contains(t, body, "entered by you")
	assert.NotContains(t, body, "No position yet")
}

func TestTheLatestEntryIsTheReadingPositionEvenWhenLower(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})
	require.Equal(t, http.StatusSeeOther, s.recordPosition(bookID, "80", nil).Code)
	require.Equal(t, http.StatusSeeOther, s.recordPosition(bookID, "99.6", nil).Code)
	assert.Contains(t, getAs(t, s.h, bookDetailURL(bookID), s.cookies).Body.String(), "<strong>99%</strong>", "99.6% never shows as 100%")
	require.Equal(t, http.StatusSeeOther, s.recordPosition(bookID, "20", nil).Code)
	assert.Contains(t, getAs(t, s.h, bookDetailURL(bookID), s.cookies).Body.String(), "<strong>20%</strong>")
}

func TestARefusedPositionKeepsTheInput(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})
	for input, want := range map[string]string{
		"":      "Enter a percentage",
		"abc":   "as a number",
		"12.34": "as a number",
		"101":   "must be from 0 to 100",
		"-1":    "must be from 0 to 100",
	} {
		response := s.recordPosition(bookID, input, nil)
		require.Equal(t, http.StatusSeeOther, response.Code, input)
		location := response.Header().Get("Location")
		parsed, err := url.Parse(location)
		require.NoError(t, err)
		assert.Equal(t, bookDetailURL(bookID), parsed.Path)
		assert.Equal(t, input, parsed.Query().Get("percent"), "the entered value rides the redirect")

		page := getAs(t, s.h, location, s.cookies)
		require.Equal(t, http.StatusOK, page.Code)
		assert.Contains(t, page.Body.String(), want)
		assert.Contains(t, page.Body.String(), "No position yet", "a refusal records nothing")
		if input != "" {
			assert.Contains(t, page.Body.String(), `value="`+input+`"`, "the form is refilled")
		}
	}
}

func TestPositionEntryRefusesForeignReturnTargetsAndMissingCSRF(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})

	foreign := s.recordPosition(bookID, "10", url.Values{"return_to": {"https://example.test/"}})
	require.Equal(t, http.StatusSeeOther, foreign.Code)
	assert.NotContains(t, foreign.Header().Get("Location"), "example.test")
	assert.NotContains(t, foreign.Header().Get("Location"), bookDetailURL(bookID))

	noToken := perform(t, s.h, http.MethodPost, readingPositionURL(bookID), url.Values{"percent": {"10"}}, s.cookies)
	assert.Equal(t, http.StatusForbidden, noToken.Code)
}

func TestPositionEntryForABookNotInReadingIsNotFound(t *testing.T) {
	s := newStudyHarness(t)
	bookID := s.preparedBook(testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})
	require.Equal(t, http.StatusNotFound, s.recordPosition("00000000-0000-4000-8000-000000000000", "10", nil).Code)

	removed := perform(t, s.h, http.MethodPost, "/library/books/"+bookID+"/remove", url.Values{"csrf_token": {s.cookies[0].Value}}, s.cookies)
	require.Equal(t, http.StatusSeeOther, removed.Code)
	assert.Equal(t, http.StatusNotFound, s.recordPosition(bookID, "10", nil).Code)
}
