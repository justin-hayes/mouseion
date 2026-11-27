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
	store := openWebappIntegrationStore(t, ctx)
	owner := createAccount(t, ctx, store, "coverage-projection", "learner-password")
	// haus x3, baum x2, tisch x1: six analyzable tokens.
	book, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "coverage-projection", "Coverage projection", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3},
		{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", OccurrenceCount: 2},
		{Language: "de", CanonicalLemma: "tisch", UPOS: "NOUN", OccurrenceCount: 1},
	})
	seedBrowseEvidenceTokens(t, ctx, store, source, corpus, []string{"haus", "haus", "haus", "baum", "baum", "tisch"})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	// haus is Mature and baum is in study but not Mature (ADR 0136 §3).
	seedStudyWord(t, ctx, store, owner.ID, "haus", true)
	seedStudyWord(t, ctx, store, owner.ID, "baum", false)

	// Correct the tisch occurrence into Mature haus and exclude one baum.
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
	chooser := perform(t, h, http.MethodGet, "/de/reading", nil, cookies)
	require.Equal(t, http.StatusOK, chooser.Code)
	assert.Contains(t, chooser.Body.String(), "Updating")
	assert.NotContains(t, chooser.Body.String(), "Mature vocabulary coverage")
	view, err := h.buildReadingChooser(ctx, owner.ID, "de", "Deutsch", "")
	require.NoError(t, err)
	updating := chooserBooksInState(view, readingChooserUpdating)
	require.Len(t, updating, 1)
	assert.Nil(t, updating[0].Coverage)
	unreadyBook := readingBookView{Book: bookSummary(t, ctx, store, owner.ID, book.ID), BookID: book.ID}
	require.NoError(t, h.addReadingEvidence(ctx, owner.ID, &unreadyBook))
	assert.True(t, unreadyBook.CountsUpdating)
	assert.Nil(t, unreadyBook.Coverage)
	assert.Equal(t, readingEvidenceUpdating, readingEvidenceState(unreadyBook))

	buildBrowseProjection(t, ctx, store, book.ID)

	const wantMature, wantInStudy, wantTotal = int64(4), int64(5), int64(6) // 3 haus + corrected tisch; the remaining baum is in study; the excluded baum stays in the total
	myBooks, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, myBooks.Items, 1)
	assert.Equal(t, wantMature, myBooks.Items[0].CoverageMatureTokens)
	assert.Equal(t, wantInStudy, myBooks.Items[0].CoverageInStudyTokens)
	assert.Equal(t, wantTotal, myBooks.Items[0].CoverageTotalTokens)

	view, err = h.buildReadingChooser(ctx, owner.ID, "de", "Deutsch", "")
	require.NoError(t, err)
	below95 := chooserBooksByBand(view, domain.CoverageBandBelow95)
	require.Len(t, below95, 1)
	require.NotNil(t, below95[0].Coverage)
	assert.Equal(t, wantMature, below95[0].Coverage.MatureTokenCount)
	assert.Equal(t, wantInStudy, below95[0].Coverage.InStudyTokenCount)

	// Analysis Insights reads the same projection.
	insights, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	assert.Equal(t, wantMature, insights.MatureTokenCount)
	assert.Equal(t, wantInStudy, insights.InStudyTokenCount)
	assert.Equal(t, wantTotal, insights.AnalyzableTokenCount)

	// My Books shows the same figures on its page.
	myBooksPage := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, myBooksPage.Code)
	assert.Contains(t, myBooksPage.Body.String(), "66.7% of running words Mature.")
	assert.Contains(t, myBooksPage.Body.String(), "83.3% with words in study.")
	assert.Equal(t, wantTotal, below95[0].Coverage.AnalyzableTokenCount)
	assert.Empty(t, chooserBooksInState(view, readingChooserUpdating))
	chooserPage := perform(t, h, http.MethodGet, "/de/reading", nil, cookies)
	require.Equal(t, http.StatusOK, chooserPage.Code)
	assert.Contains(t, chooserPage.Body.String(), "66.7% (4 of 6 word occurrences)")
	assert.Contains(t, chooserPage.Body.String(), "83.3% (5 word occurrences)")

	started := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code)
	page, err := h.buildReadingView(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.NotNil(t, page.CurrentReading)
	require.NotNil(t, page.CurrentReading.Coverage)
	assert.Equal(t, wantMature, page.CurrentReading.Coverage.MatureTokenCount)
	assert.Equal(t, wantInStudy, page.CurrentReading.Coverage.InStudyTokenCount)
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

// seedStudyWord puts a German noun in the learner's German study, Mature (in review
// on a day-scale interval at least FluencyMaturityDays long) or still learning.
func seedStudyWord(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, lemma string, mature bool) {
	t.Helper()
	seedStudyWordIn(t, ctx, store, owner, "de", lemma, "NOUN", mature)
}

// seedStudyWordIn is seedStudyWord for any study language.
func seedStudyWordIn(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, language, lemma, upos string, mature bool) {
	t.Helper()
	state, days := "learning", int64(0)
	if mature {
		state, days = "review", domain.FluencyMaturityDays
	}
	_, err := store.Pool().Exec(ctx, `
WITH m AS (
  INSERT INTO study_material (owner_id, language, canonical_lemma, upos, state, omission_reason)
  VALUES ($1, $6, $2, $3, 'unpreparable', 'sentence-quality') RETURNING id)
INSERT INTO study_progress (owner_id, language, canonical_lemma, upos, material_id, introduced_at, learned_meaning, state, due, stability, difficulty, scheduled_days, reps, last_review)
SELECT $1, $6, $2, $3, id, now(), 'seeded', $4, now(), 5, 5, $5, 1, now() FROM m`, owner, lemma, upos, state, days, language)
	require.NoError(t, err)
}
