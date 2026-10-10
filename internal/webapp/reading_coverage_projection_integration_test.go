//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Book shows one coverage figure everywhere: My Books, the Reading chooser,
// and the Reading view all read the Browse count projection, so a confirmed
// Lemma correction moves an occurrence to its effective identity and an
// excluded occurrence stays in the analyzable-token total.
func TestReadingCoverageMatchesMyBooksProjection(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "reading-coverage-projection-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openVisibilityStore(t, ctx)
	owner := createAccount(t, ctx, store, "coverage-projection", "learner-password", false)
	// haus x3, baum x2, tisch x1: six analyzable tokens.
	book, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "coverage-projection", "Coverage projection", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3},
		{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", OccurrenceCount: 2},
		{Language: "de", CanonicalLemma: "tisch", UPOS: "NOUN", OccurrenceCount: 1},
	})
	seedBrowseEvidenceTokens(t, ctx, store, source, corpus, []string{"haus", "haus", "haus", "baum", "baum", "tisch"})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	_, err := store.PutKnownVocabulary(ctx, owner.ID, "de", "haus", "NOUN")
	require.NoError(t, err)

	// Correct the tisch occurrence into Known haus and exclude one baum.
	tisch, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "tisch")
	require.NoError(t, err)
	require.Len(t, tisch, 1)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: tisch[0], CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	baum, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "baum")
	require.NoError(t, err)
	require.Len(t, baum, 2)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: baum[0], CanonicalLemma: "", Excluded: true, NormalizationProfile: "", NormalizationVersion: ""}}))

	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store), PreparedDeck: prepareddeck.NewService(store, nil), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")

	// Without a ready projection the chooser and Reading view say Updating
	// and never a band computed from raw counts.
	chooser := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, chooser.Code)
	assert.Contains(t, chooser.Body.String(), "Updating")
	assert.NotContains(t, chooser.Body.String(), "Known vocabulary coverage")
	view, err := h.buildReadingChooser(ctx, owner.ID, "de", "Deutsch")
	require.NoError(t, err)
	require.Len(t, view.Updating, 1)
	assert.Nil(t, view.Updating[0].Coverage)
	unreadyBook := readingBookView{Book: bookSummary(t, ctx, store, owner.ID, book.ID), BookID: book.ID}
	require.NoError(t, h.addReadingEvidence(ctx, owner.ID, &unreadyBook))
	assert.True(t, unreadyBook.CountsUpdating)
	assert.Nil(t, unreadyBook.Coverage)
	assert.Equal(t, readingEvidenceUpdating, readingEvidenceState(unreadyBook))

	buildBrowseProjection(t, ctx, store, book.ID)

	const wantKnown, wantTotal = int64(4), int64(6) // 3 haus + corrected tisch; excluded baum stays in total
	myBooks, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, myBooks.Items, 1)
	assert.Equal(t, wantKnown, myBooks.Items[0].CoverageKnownTokens)
	assert.Equal(t, wantTotal, myBooks.Items[0].CoverageTotalTokens)

	view, err = h.buildReadingChooser(ctx, owner.ID, "de", "Deutsch")
	require.NoError(t, err)
	require.Len(t, view.Below95, 1)
	require.NotNil(t, view.Below95[0].Coverage)
	assert.Equal(t, wantKnown, view.Below95[0].Coverage.KnownTokenCount)
	assert.Equal(t, wantTotal, view.Below95[0].Coverage.AnalyzableTokenCount)
	assert.Empty(t, view.Updating)

	started := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code)
	page, err := h.buildReadingView(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.NotNil(t, page.CurrentReading)
	require.NotNil(t, page.CurrentReading.Coverage)
	assert.Equal(t, wantKnown, page.CurrentReading.Coverage.KnownTokenCount)
	assert.Equal(t, wantTotal, page.CurrentReading.Coverage.AnalyzableTokenCount)
	assert.False(t, page.CurrentReading.CountsUpdating)
}

func bookSummary(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, bookID string) domain.SourceMaterialSummary {
	t.Helper()
	book, err := store.GetBookDetail(ctx, owner, bookID)
	require.NoError(t, err)
	require.NotNil(t, book.Acquired)
	return *book.Acquired
}
