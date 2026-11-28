//go:build integration

package webapp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStudyTabLinksToTheActiveLanguageAndIsCurrentOnStudyPages(t *testing.T) {
	h, cookies, _, _, _, _ := languageContextHandler(t)

	books := getAs(t, h, "/de/reading", cookies)
	require.Equal(t, http.StatusOK, books.Code)
	assert.Contains(t, books.Body.String(), `href="/de/study"`, "the Study tab follows the active study language")
	assert.NotContains(t, books.Body.String(), `href="/de/study" aria-current="page"`)

	for _, path := range []string{"/de/study", "/de/study/reports"} {
		t.Run(path, func(t *testing.T) {
			response := getAs(t, h, path, cookies)
			require.Equal(t, http.StatusOK, response.Code)
			body := response.Body.String()
			assert.Contains(t, body, `data-navigation-context="study"`)
			assert.Contains(t, body, `href="/de/study" aria-current="page"`)
			assert.NotContains(t, body, `href="/de/reading" aria-current="page"`, "Reading is not current on Study pages")
		})
	}

	picker := getAs(t, h, "/study-language?to=study&lang=it", cookies)
	require.Equal(t, http.StatusSeeOther, picker.Code)
	assert.Equal(t, "/it/study", picker.Header().Get("Location"))
}

func TestStudyTabIsCurrentOnTheRemediationPage(t *testing.T) {
	s := newStudyHarness(t)
	s.reportedHaus(3)
	id := s.openReports()[0].ID

	response := getAs(t, s.h, "/study/reports/"+id+"/remediation", s.cookies)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `data-navigation-context="study"`)
	assert.Contains(t, body, `href="/de/study" aria-current="page"`)
}
