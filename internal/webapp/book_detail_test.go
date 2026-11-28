package webapp

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookWordStateNamesTheStoreStudyState(t *testing.T) {
	assert.Equal(t, bookWordMature, bookWordStateOf(domain.VocabularyBrowseRow{StudyState: domain.VocabularyStudyMature}))
	assert.Equal(t, bookWordInStudy, bookWordStateOf(domain.VocabularyBrowseRow{StudyState: domain.VocabularyStudyInStudy}))
	assert.Equal(t, bookWordNotIntroduced, bookWordStateOf(domain.VocabularyBrowseRow{StudyState: domain.VocabularyStudyNotIntroduced}))
	assert.Equal(t, bookWordNotIntroduced, bookWordStateOf(domain.VocabularyBrowseRow{}))
}

func TestBookDetailListsWordsByStudyStateAndOffersNoWordEdits(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	response := performReadingRequest(t, h, http.MethodGet, bookDetailURL(fixtures.BookID), nil, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `id="vocabulary-workflow"`)
	assert.Contains(t, body, `id="book-detail-analysis"`)
	for _, state := range []string{"Mature", "In study", "Not yet introduced"} {
		assert.Contains(t, body, `data-study-state>`+state+`</td>`, state)
	}
	assert.Contains(t, body, `href="/de/corpus/concordance?`, "each word opens its Concordance")
	assert.Contains(t, body, `from_book=`+fixtures.BookID, "Concordance knows the way back to this Book")
	for _, retired := range []string{"Known", "Reserved", "Unknown", "Show already accounted-for words", "Browse selection"} {
		assert.NotContains(t, body, retired)
	}
	_, list, found := strings.Cut(body, `id="vocabulary-workflow"`)
	require.True(t, found)
	assert.Equal(t, 1, strings.Count(list, "<form"), "the word list holds only its GET search form")
	assert.NotContains(t, list, `method="post"`)
}

func TestBookDetailRefusesAStaleRevisionAndAPageOutOfRange(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	stale := performReadingRequest(t, h, http.MethodGet, bookDetailURL(fixtures.BookID)+"?rev=old", nil, cookies)
	assert.Equal(t, http.StatusConflict, stale.Code)
	assert.Contains(t, stale.Body.String(), "Current evidence changed")
	malformed := performReadingRequest(t, h, http.MethodGet, bookDetailURL(fixtures.BookID)+"?page=zero", nil, cookies)
	assert.Equal(t, http.StatusBadRequest, malformed.Code)
	beyond := performReadingRequest(t, h, http.MethodGet, bookDetailURL(fixtures.BookID)+"?page=9", nil, cookies)
	assert.Equal(t, http.StatusNotFound, beyond.Code)
}

func TestRetiredWorkingDeskAddressesMoveToTheBookDetailPageOrServeReading(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	moved := map[string]string{
		"/de/reading?reading=" + fixtures.BookID: "/books/" + fixtures.BookID,
		"/de/reading?reading=" + fixtures.BookID + "&q=ha&page=2&rev=r1&row=NOUN%3Ahaus&all=1&snapshot=stale": "/books/" + fixtures.BookID + "?page=2&q=ha&rev=r1&row=NOUN%3Ahaus",
	}
	for address, want := range moved {
		t.Run(address, func(t *testing.T) {
			response := performReadingRequest(t, h, http.MethodGet, address, nil, cookies)
			require.Equal(t, http.StatusSeeOther, response.Code)
			require.Equal(t, want, response.Header().Get("Location"))
		})
	}
	// Without a named Book the address is the Reading page itself.
	require.Equal(t, http.StatusOK, performReadingRequest(t, h, http.MethodGet, "/de/reading?page=1", nil, cookies).Code)
}

func TestBookDetailOpensForNoBookThatIsNotInMyBooks(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	for _, address := range []string{"/books/" + url.PathEscape("../x"), "/books/missing"} {
		assert.Equal(t, http.StatusNotFound, performReadingRequest(t, h, http.MethodGet, address, nil, cookies).Code, address)
	}
}

func TestBookDetailHostsTheBookRetryAction(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	response := performReadingRequest(t, h, http.MethodGet, bookDetailURL("fixture-failed"), nil, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	_, analysis, found := strings.Cut(response.Body.String(), `id="book-detail-analysis"`)
	require.True(t, found)
	assert.Contains(t, analysis, `action="/reading/books/fixture-failed/reanalyze"`, "the Book's retry lives on its detail page")
	assert.Contains(t, analysis, `name="csrf_token"`)
}
