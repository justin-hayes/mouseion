package webapp

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
)

// The Prepared deck is gone (ADR 0136): every deck page and action is an
// unknown route, whether or not the learner is signed in.
func TestDeckRoutesAreNotFound(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)
	form := url.Values{"csrf_token": {csrf}}
	book := fixtures.BookID
	posts := []string{
		"/reading/books/" + book + "/deck/preparations",
		"/reading/books/" + book + "/deck/cancel",
		"/reading/books/" + book + "/deck/retry",
		"/deck-preparations/prep-1/cancel",
		"/deck-preparations/prep-1/reprepare",
		"/deck-preparations/prep-1/rerender",
	}
	for _, path := range posts {
		t.Run("POST "+path, func(t *testing.T) {
			assert.Equal(t, http.StatusNotFound, readingTestRequest(t, h, path, form, cookies).Code)
		})
	}
	gets := []string{
		"/reading/books/" + book + "/deck/preparations/new",
		"/deck-preparations/prep-1/status",
		"/deck-preparations/prep-1/download",
	}
	for _, path := range gets {
		t.Run("GET "+path, func(t *testing.T) {
			assert.Equal(t, http.StatusNotFound, catalogPage(t, h, cookies, path).Code)
		})
	}
}
