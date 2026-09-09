package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/fixtures"
)

func TestAuthenticatedShellLazilyDefaultsWithoutWritingStoredLanguage(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	if err := store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, ""); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/journey", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /journey status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `<option value="it" selected`) {
		t.Fatalf("shell did not render the most recently activated language: %s", response.Body.String())
	}
	if stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID); err != nil || stored != "" {
		t.Fatalf("lazy default wrote stored language=%q err=%v", stored, err)
	}
}

func TestActiveStudyLanguageEmptySubmissionIsNoOp(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	response := goalRequest(t, h, "/active-study-language", url.Values{
		"csrf_token": {csrf}, "language": {""}, "return_to": {"/journey"},
	}, cookies)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/journey" {
		t.Fatalf("empty language submission status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID); err != nil || stored != "de" {
		t.Fatalf("empty language submission changed stored language=%q err=%v", stored, err)
	}
}
