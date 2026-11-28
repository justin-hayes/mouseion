package webapp

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
)

// The Book-scoped Lemma review page is gone (ADR 0136 §6): the remediation
// screen is the only route to a correction, so its page and actions are unknown
// routes.
func TestLemmaReviewRoutesAreNotFound(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)
	path := "/reading/books/" + fixtures.BookID
	form := url.Values{"csrf_token": {csrf}, "stage": {"preview"}, "form": {"Weg"}}
	for _, route := range []string{path + "/lemma-review", path + "/lemma-suggestion"} {
		t.Run("POST "+route, func(t *testing.T) {
			assert.Equal(t, http.StatusNotFound, readingTestRequest(t, h, route, form, cookies).Code)
		})
	}
	t.Run("GET "+path+"/lemma-review", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, catalogPage(t, h, cookies, path+"/lemma-review").Code)
	})
}
