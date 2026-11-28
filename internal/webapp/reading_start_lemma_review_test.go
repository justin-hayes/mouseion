package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/justin-hayes/mouseion/internal/lemmarisk"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// threeWegReading gives the route-match Book a third occurrence of the Weg
// form. The local detector flags nothing until one alternative recurs three
// times, and the fixture seeds only two occurrences.
type threeWegReading struct{ *fixtures.Store }

func newThreeWegReading(store *fixtures.Store) threeWegReading {
	store.SeedLemmaReviewOccurrence(domain.LemmaReviewOccurrence{
		OwnerID: fixtures.OwnerID, BookID: "fixture-route-match", CorpusID: "fixture-route-match-corpus", AnalysisRunID: "fixture-route-match-run",
		SourceDocumentID: "fixture-route-match-unit", StartOffset: 40, EndOffset: 43, SentenceOrdinal: 2, TokenOrdinal: 1,
		Surface: "Weg", RawLemma: "Weg", CanonicalLemma: "weg", UPOS: "NOUN", SentenceText: "Der Weg führt zum Haus.",
	})
	return threeWegReading{Store: store}
}

// competingLemmaIndex is a local index that misses every analyzed lemma and
// offers one sentence-supported competing lemma for each occurrence.
type competingLemmaIndex struct{}

func (competingLemmaIndex) LemmaExists(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (competingLemmaIndex) Alternatives(context.Context, string, string, string, string) ([]lemmarisk.Alternative, error) {
	return []lemmarisk.Alternative{{Lemma: "Pfad", UPOS: "NOUN", Source: "fixture-dictionary", Version: "v1", EvidenceID: "pfad-weg", ContextRelevant: true}}, nil
}

// startSessionOver signs the fixture learner in over the given store and index.
func startSessionOver(t *testing.T, reading threeWegReading, index lemmarisk.AlternativeIndex) (http.Handler, []*http.Cookie, string) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "reading-start-lemma-risk-secret-0123456789")
	authService := auth.New(fixtures.NewAuthStore(), time.Hour)
	deps := storeDependencies(reading)
	analysis := fixtures.Analysis{Store: reading.Store}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: deps,
		Analysis: analysis, CatalogueSync: fixtures.NewCatalogueSync(reading.Store),
		SessionLifetime: time.Hour,
		LemmaReview:     lemmareview.New(lemmareview.Config{Store: deps.Reading, RiskIndex: index, BrowseCounts: analysis}),
	})
	loginPage := httptest.NewRecorder()
	h.ServeHTTP(loginPage, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil))
	initialCSRFCookie := findCSRFCookie(t, loginPage.Result().Cookies())
	token := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(loginPage.Body.String())[1]
	loginRequest := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", strings.NewReader(url.Values{"csrf_token": {token}, "username": {fixtures.Username}, "password": {fixtures.Password}}.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRequest.AddCookie(initialCSRFCookie)
	loginResponse := httptest.NewRecorder()
	h.ServeHTTP(loginResponse, loginRequest)
	require.Equal(t, http.StatusSeeOther, loginResponse.Code)
	cookies := loginResponse.Result().Cookies()
	return h, cookies, findCSRFCookie(t, cookies).Value
}

func TestReadNextIsNeitherPausedNorFlaggedByLemmaRiskDetection(t *testing.T) {
	store := fixtures.NewStore()
	reading := newThreeWegReading(store)
	endGermanFixtureReading(t, store)
	h, cookies, csrf := startSessionOver(t, reading, competingLemmaIndex{})

	start := readingTestRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	assert.Contains(t, start.Header().Get("Location"), "is+now+your+current+reading", "lemma review flags never gate the Current reading")
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, "fixture-route-match", current.BookID)
	occurrences, err := reading.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, "fixture-route-match", "Weg")
	require.NoError(t, err)
	require.NotEmpty(t, occurrences)
	assert.Empty(t, occurrences[0].ReviewFlagReason, "Read next runs no lemma risk detection")
}

func TestStartWithoutLemmaRiskIndexStillStartsTheBook(t *testing.T) {
	store := fixtures.NewStore()
	reading := newThreeWegReading(store)
	endGermanFixtureReading(t, store)
	h, cookies, csrf := startSessionOver(t, reading, nil)

	start := readingTestRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	assert.Contains(t, start.Header().Get("Location"), "is+now+your+current+reading", "a missing local index never blocks Start")
}
