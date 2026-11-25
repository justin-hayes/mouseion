package webapp

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStudyIsAvailableOnlyOnTheModernLanguageRoute(t *testing.T) {
	assert.True(t, studyAvailable("de"))
	assert.True(t, studyAvailable("it"))
	assert.False(t, studyAvailable("la"))
	assert.False(t, studyAvailable("grc"))
}

func TestStudyPageSaysPlainlyWhenALanguageHasNoStudy(t *testing.T) {
	for _, language := range []string{"la", "grc"} {
		t.Run(language, func(t *testing.T) {
			var body bytes.Buffer
			view := studyPageView{Language: language, LanguageLabel: "Latin", Available: studyAvailable(language)}
			require.NoError(t, StudyPage(domain.User{}, "csrf", view).Render(t.Context(), &body))
			assert.Contains(t, body.String(), "Study isn't available for this language yet")
			assert.NotContains(t, body.String(), "study-ready")
		})
	}
}

func TestStudyPageReportsReadyAndPreparingWords(t *testing.T) {
	var body bytes.Buffer
	view := studyPageView{Language: "de", LanguageLabel: "German", Available: true, Counts: domain.StudyMaterialCounts{Ready: 12, Preparing: 1, Unpreparable: 2}}
	require.NoError(t, StudyPage(domain.User{}, "csrf", view).Render(t.Context(), &body))
	assert.Contains(t, body.String(), "12 new words are ready.")
	assert.Contains(t, body.String(), "1 more word is being prepared.")
	assert.Contains(t, body.String(), "2 words were skipped because they have no usable sentence or meaning.")
}

func TestStudyPageServesAGermanLearnerAndPreparesAhead(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		store, ok := services.Store.Study.(*fixtures.Store)
		require.True(t, ok)
		services.Study = fixtures.Study{Store: store}
	})

	response := getAs(t, h, "/de/study", cookies)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "<h1>Study</h1>")
	assert.Contains(t, response.Body.String(), "new words are ready")
	assert.Contains(t, response.Body.String(), "being prepared", "opening the page reserves material for the next words")

	unprefixed := getAs(t, h, "/study", cookies)
	assert.Equal(t, http.StatusSeeOther, unprefixed.Code, "the unprefixed address redirects to the language's page")
}
