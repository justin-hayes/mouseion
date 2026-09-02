package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/fixtures"
)

func scopeGet(t *testing.T, h http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestReviewEPUBScopeHandlerRendersAllOnChecklist(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	response := scopeGet(t, h, "/books/"+fixtures.BookID+"/scope", cookies)
	if response.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{"Check all", "Uncheck all", "Chapter one", "Chapter two", `value="epub-unit-v1:0:fixture-001" checked`, `value="epub-unit-v1:1:fixture-002" checked`} {
		if !strings.Contains(body, want) {
			t.Errorf("review missing %q: %s", want, body)
		}
	}
}

func TestConfirmEPUBScopeHandlerPersistsClassifierFreeFillerValues(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	_, units, err := store.GetExtractedUnitSnapshot(nil, fixtures.OwnerID, fixtures.BookID)
	if err != nil {
		t.Fatal(err)
	}
	response := goalRequest(t, h, "/books/"+fixtures.BookID+"/scope", url.Values{
		"csrf_token": {csrf}, "snapshot_id": {"fixture-snapshot"}, "unit_id": {units.Units[0].ID, units.Units[1].ID},
	}, cookies)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("confirm status=%d body=%s", response.Code, response.Body.String())
	}
	books, err := store.ListSourceMaterials(nil, fixtures.OwnerID)
	if err != nil || len(books) == 0 || books[0].ReviewedScopeID == "" {
		t.Fatalf("confirmed scope was not linked: books=%+v err=%v", books, err)
	}
	scope, err := store.GetEPUBReviewedScope(nil, fixtures.OwnerID, fixtures.BookID, books[0].ReviewedScopeID)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Classifier.Name != "none" || scope.Classifier.Version != "none" || scope.SelectionMode != "overridden" {
		t.Fatalf("scope classifier contract=%+v mode=%q", scope.Classifier, scope.SelectionMode)
	}
}

func TestConfirmEPUBScopeHandlerRejectsEmptyUnknownAndDuplicateUnits(t *testing.T) {
	for name, submitted := range map[string][]string{
		"empty":     nil,
		"unknown":   {"unknown-unit"},
		"duplicate": {"epub-unit-v1:0:fixture-001", "epub-unit-v1:0:fixture-001"},
	} {
		t.Run(name, func(t *testing.T) {
			h, cookies, csrf, _ := goalFixtureSession(t)
			response := goalRequest(t, h, "/books/"+fixtures.BookID+"/scope", url.Values{
				"csrf_token": {csrf}, "snapshot_id": {"fixture-snapshot"}, "unit_id": submitted,
			}, cookies)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
