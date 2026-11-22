package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubJobsStore is the JobsStore seam for handler tests: one fixed list of
// Book preparation rows.
type stubJobsStore struct {
	rows []domain.BookPreparationJob
	err  error
}

func (s stubJobsStore) ListBookPreparationJobs(context.Context, string) ([]domain.BookPreparationJob, error) {
	return s.rows, s.err
}

// TestJobsHandlerShowsAllThreePreparationKindsAndCatalogSyncOnOnePage drives
// GET /jobs through the real handler with a page holding all three Book
// preparation kinds plus Catalog sync.
func TestJobsHandlerShowsAllThreePreparationKindsAndCatalogSyncOnOnePage(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Store.Jobs = stubJobsStore{rows: bookPreparationFixtureRows()}
	})

	response := jobsPageRequest(t, h, cookies)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	body := response.Body.String()

	for _, want := range []string{
		"<h2>Book preparations</h2>",
		"Classical preparation",
		"EPUB acquisition",
		`href="/jobs/13">Analysis #2</a>`,
		`href="/catalogs/books/book-classical/in-my-books">Anabasis</a>`,
		`href="/catalogs/books/book-epub/in-my-books">Der lange Weg nach Hause</a>`,
		">source<",
		"<code>source_pin_mismatch</code>",
		"<code>dependency_parsing_unavailable</code>",
		`aria-label="Analysis job 2 progress"`,
		"<h2>Catalog sync</h2>",
	} {
		assert.Contains(t, body, want, "GET /jobs missing %q", want)
	}
}

// TestJobsHandlerServesTheFixturePage proves the fixture-backed handler still
// renders the learner's own analysis rows and sync connections end to end.
func TestJobsHandlerServesTheFixturePage(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, nil)

	response := jobsPageRequest(t, h, cookies)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	body := response.Body.String()

	assert.Contains(t, body, "<h2>Book preparations</h2>")
	assert.Contains(t, body, `href="/jobs/43">Analysis #2</a>`)
	assert.Contains(t, body, `href="/catalogs/books/fixture-failed/in-my-books">Fehlgeschlagene Analyse</a>`)
	assert.Contains(t, body, "<h2>Catalog sync</h2>")
	assert.NotContains(t, body, `aria-label="Analysis history"`)
}

// jobsPageRequest runs GET /jobs against the handler with the session's
// cookies.
func jobsPageRequest(t *testing.T, h http.Handler, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/jobs", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}
