package webapp

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookWordStatesSeparateMatureInStudyAndNotIntroduced(t *testing.T) {
	mature := domain.StudyProgress{StudyItem: domain.WordItem("de", "haus", "NOUN", ""), Memory: domain.StudyMemory{State: domain.StudyStateReview, ScheduledDays: domain.FluencyMaturityDays}}
	learning := domain.StudyProgress{StudyItem: domain.WordItem("de", "gehen", "VERB", ""), Memory: domain.StudyMemory{State: domain.StudyStateLearning, ScheduledDays: 1}}
	withdrawn := domain.StudyProgress{StudyItem: domain.WordItem("de", "lesen", "VERB", ""), Withdrawn: true, Memory: mature.Memory}
	states := bookWordStates([]domain.StudyProgress{mature, learning, withdrawn})

	row := func(lemma, upos string) domain.VocabularyBrowseRow {
		return domain.VocabularyBrowseRow{CanonicalLemma: lemma, UPOS: upos}
	}
	assert.Equal(t, bookWordMature, bookWordStateOf(states, "de", row("haus", "NOUN")))
	assert.Equal(t, bookWordInStudy, bookWordStateOf(states, "de", row("gehen", "VERB")))
	assert.Equal(t, bookWordNotIntroduced, bookWordStateOf(states, "de", row("lesen", "VERB")), "a withdrawn word is out of Study")
	assert.Equal(t, bookWordNotIntroduced, bookWordStateOf(states, "de", row("garten", "NOUN")))
	assert.Equal(t, bookWordNotIntroduced, bookWordStateOf(states, "it", row("haus", "NOUN")), "another study language never matches")
}

func TestBookDetailListsWordsByStudyStateAndOffersNoWordEdits(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	store.SeedMatureStudyWord(fixtures.OwnerID, domain.WordItem("de", "haus", "NOUN", ""), domain.StudyMaterialReadyContent{
		BookID: fixtures.BookID, AnalysisRunID: fixtures.ResultRunID, CorpusID: "fixture-corpus", SentenceOrdinal: 1, TokenOrdinal: 1,
		Sentence: "Das Haus ist alt.", TargetWord: "Haus", Gloss: "house", SentenceTranslation: "The house is old.", SentenceTranslationTargets: []string{"house"},
	}, time.Now())

	response := performReadingRequest(t, h, http.MethodGet, bookDetailURL(fixtures.BookID), nil, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `id="vocabulary-workflow"`)
	assert.Contains(t, body, `id="book-detail-analysis"`)
	assert.Contains(t, body, `data-study-state>Mature</td>`, "a Mature word is labelled Mature")
	assert.Contains(t, body, `data-study-state>Not yet introduced</td>`)
	assert.Contains(t, body, `href="/de/vocabulary/concordance?`, "each word opens its Concordance")
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

func TestRetiredWorkingDeskAddressesMoveToTheBookDetailPageOrMyBooks(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	moved := map[string]string{
		"/de/reading?reading=" + fixtures.BookID: "/books/" + fixtures.BookID,
		"/de/reading?reading=" + fixtures.BookID + "&q=ha&page=2&rev=r1&row=NOUN%3Ahaus&all=1&snapshot=stale": "/books/" + fixtures.BookID + "?page=2&q=ha&rev=r1&row=NOUN%3Ahaus",
		"/de/reading?q=ha&all=1&page=2": "/de/library",
		"/de/reading?page=1":            "/de/library",
	}
	for address, want := range moved {
		response := performReadingRequest(t, h, http.MethodGet, address, nil, cookies)
		require.Equal(t, http.StatusSeeOther, response.Code, address)
		assert.Equal(t, want, response.Header().Get("Location"), address)
	}
}

func TestBookDetailOpensForNoBookThatIsNotInMyBooks(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	for _, address := range []string{"/books/" + url.PathEscape("../x"), "/books/missing"} {
		assert.Equal(t, http.StatusNotFound, performReadingRequest(t, h, http.MethodGet, address, nil, cookies).Code, address)
	}
}
