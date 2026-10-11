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

// ListDeckPreparationsForSourceMaterial reports one ready, snapshotted deck for
// the route-match Book, so a confirmed identity change takes the restart path.
func (s threeWegReading) ListDeckPreparationsForSourceMaterial(ctx context.Context, owner, sourceMaterialID string) ([]domain.DeckPreparation, error) {
	if sourceMaterialID != "fixture-route-match" {
		return s.Store.ListDeckPreparationsForSourceMaterial(ctx, owner, sourceMaterialID)
	}
	return []domain.DeckPreparation{{ID: "fixture-ready-preparation", OwnerID: owner, SourceMaterialID: sourceMaterialID, State: domain.DeckPreparationReady, SnapshotID: "fixture-ready-snapshot"}}, nil
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
	preparedDeck := fixtures.PreparedDeck{Store: reading.Store}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: deps,
		Analysis: analysis, CatalogueSync: fixtures.NewCatalogueSync(reading.Store),
		PreparedDeck: preparedDeck, SessionLifetime: time.Hour,
		LemmaReview: lemmareview.New(lemmareview.Config{Store: deps.Reading, RiskIndex: index, Preparer: preparedDeck, BrowseCounts: analysis}),
	})
	loginPage := httptest.NewRecorder()
	h.ServeHTTP(loginPage, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil))
	initialCSRFCookie := cookieByName(t, loginPage.Result().Cookies(), csrfCookie)
	token := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(loginPage.Body.String())[1]
	loginRequest := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", strings.NewReader(url.Values{"csrf_token": {token}, "username": {fixtures.Username}, "password": {fixtures.Password}}.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRequest.AddCookie(initialCSRFCookie)
	loginResponse := httptest.NewRecorder()
	h.ServeHTTP(loginResponse, loginRequest)
	require.Equal(t, http.StatusSeeOther, loginResponse.Code)
	cookies := loginResponse.Result().Cookies()
	return h, cookies, cookieByName(t, cookies, csrfCookie).Value
}

func TestStartRunsLemmaRiskDetectionBeforeFreezingTheBook(t *testing.T) {
	store := fixtures.NewStore()
	reading := newThreeWegReading(store)
	endGermanFixtureReading(t, store)
	h, cookies, csrf := startSessionOver(t, reading, competingLemmaIndex{})

	start := readingTestRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	assert.Equal(t, "/reading/books/fixture-route-match/lemma-review", start.Header().Get("Location"), "unresolved high-risk flags pause Start reading")
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive(), "a paused Start freezes nothing")
	flagged, err := reading.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, "fixture-route-match", "Weg")
	require.NoError(t, err)
	require.NotEmpty(t, flagged)
	assert.NotEmpty(t, flagged[0].ReviewFlagReason, "detection saved the flag Start consulted")
}

func TestConfirmedIdentityRestartRunsLemmaRiskDetectionBeforeFreezing(t *testing.T) {
	store := fixtures.NewStore()
	reading := newThreeWegReading(store)
	endGermanFixtureReading(t, store)
	lemmaPath := "/reading/books/fixture-route-match/lemma-review"

	// The preview runs without an index, so detection cannot flag anything
	// before the confirmed restart does.
	weg := lemmaOccurrenceIDs(t, reading, "fixture-route-match", "Weg")
	previewHandler, cookies, csrf := startSessionOver(t, reading, nil)
	preview := readingTestRequest(t, previewHandler, lemmaPath, url.Values{"csrf_token": {csrf}, "stage": {"preview"}, "form": {"Weg"}, "target": {weg[0]}, "decision": {"correct"}, "lemma": {"pfad"}}, cookies)
	require.Equal(t, http.StatusOK, preview.Code)
	fingerprint := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(preview.Body.String())
	require.Len(t, fingerprint, 2)

	confirmHandler, cookies, csrf := startSessionOver(t, reading, competingLemmaIndex{})
	confirm := url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {"pfad"}, "fingerprint": {fingerprint[1]}, "selected": {weg[0]}, "reprepare_ready_deck": {"yes"}}
	confirmed := readingTestRequest(t, confirmHandler, lemmaPath, confirm, cookies)
	require.Equal(t, http.StatusSeeOther, confirmed.Code)
	assert.Equal(t, lemmaPath+"?form=Weg", confirmed.Header().Get("Location"), "unresolved flags from the restart's own detection pause the restart")
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive(), "a paused restart freezes no new snapshot")
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
