package webapp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/study"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStudyPageSaysPlainlyWhenALanguageHasNoStudy(t *testing.T) {
	cases := []struct {
		name, language string
	}{
		{"Latin", "la"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			view := studyPageView{Language: tc.language, LanguageLabel: tc.name, Available: study.Available(tc.language)}
			require.NoError(t, StudyPage(domain.User{}, "csrf", view).Render(t.Context(), &body))
			require.Contains(t, body.String(), "Study isn't available for this language yet")
			require.NotContains(t, body.String(), "study-ready")
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
	assert.Contains(t, response.Body.String(), "1 new word is ready.", "opening the page prepares material for the next words")
	assert.Contains(t, response.Body.String(), "Take these words")

	unprefixed := getAs(t, h, "/study", cookies)
	assert.Equal(t, http.StatusSeeOther, unprefixed.Code, "the unprefixed address redirects to the language's page")
}

// studyFixtureSession is a signed-in learner over the fixtures store, whose
// clock the test controls.
func studyFixtureSession(t *testing.T) (http.Handler, []*http.Cookie, string, *fixtures.Store, *testutil.ManualClock) {
	t.Helper()
	clock := testutil.NewManualClock(time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC))
	var fixtureStore *fixtures.Store
	h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
		store, ok := services.Store.Study.(*fixtures.Store)
		require.True(t, ok)
		fixtureStore = store
		services.Study = fixtures.Study{Store: store}
		services.StudyScheduler = study.NewScheduler()
		services.Clock = clock.Func()
	})
	return h, cookies, csrf, fixtureStore, clock
}

func TestStudyDecliningABatchOverTheFixtureStoreIntroducesNothing(t *testing.T) {
	h, cookies, csrf, store, _ := studyFixtureSession(t)
	form := url.Values{"csrf_token": {csrf}, "language": {"de"}, "offer": {"1"}}

	// Opening Study prepares the material the batch is made of.
	assert.Contains(t, getAs(t, h, "/de/study", cookies).Body.String(), "Take these words")
	require.Equal(t, http.StatusSeeOther, readingTestRequest(t, h, "/study/batches/decline", form, cookies).Code)
	page := getAs(t, h, "/de/study", cookies).Body.String()
	assert.Contains(t, page, "Nothing is mature yet", "declining opens Fluency")
	assert.NotContains(t, page, "Not today", "Today offers no further batch once the learner declined")
	progress, err := store.ListStudyProgress(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Empty(t, progress)
}

func TestStudyAfterDecliningABatchTheLearnerCanStillTakeItFromFluency(t *testing.T) {
	h, cookies, csrf, store, _ := studyFixtureSession(t)
	form := func(extra url.Values) url.Values {
		extra.Set("csrf_token", csrf)
		extra.Set("language", "de")
		return extra
	}

	assert.Contains(t, getAs(t, h, "/de/study", cookies).Body.String(), "Take these words")
	require.Equal(t, http.StatusSeeOther, readingTestRequest(t, h, "/study/batches/decline", form(url.Values{"offer": {"1"}}), cookies).Code)
	page := getAs(t, h, "/de/study", cookies).Body.String()
	require.Contains(t, page, "Nothing is mature yet", "Fluency is open")
	require.Contains(t, page, "Take these words", "the learner can change their mind")

	require.Equal(t, http.StatusSeeOther, readingTestRequest(t, h, "/study/batches/take", form(url.Values{}), cookies).Code)
	progress, err := store.ListStudyProgress(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	require.Len(t, progress, 1, "the batch was taken from inside Fluency")
	assert.Contains(t, getAs(t, h, "/de/study", cookies).Body.String(), "Show answer", "its word is the next card")
}

func TestStudyTakingABatchOverTheFixtureStoreShowsACardAndGradesIt(t *testing.T) {
	h, cookies, csrf, store, clock := studyFixtureSession(t)
	form := func(extra url.Values) url.Values {
		extra.Set("csrf_token", csrf)
		extra.Set("language", "de")
		return extra
	}

	assert.Contains(t, getAs(t, h, "/de/study", cookies).Body.String(), "Take these words")
	require.Equal(t, http.StatusSeeOther, readingTestRequest(t, h, "/study/batches/take", form(url.Values{}), cookies).Code)
	progress, err := store.ListStudyProgress(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	require.Len(t, progress, 1, "taking the batch introduces its word")
	assert.NotEmpty(t, progress[0].LearnedMeaning, "the learned meaning is recorded on introduction")

	page := getAs(t, h, "/de/study", cookies).Body.String()
	assert.Contains(t, page, "Show answer")
	assert.Contains(t, page, `<mark class="study-card__target">`)
	assert.NotContains(t, page, progress[0].LearnedMeaning, "the answer is hidden until asked for")
	page = getAs(t, h, "/de/study?answer=1", cookies).Body.String()
	assert.Contains(t, page, progress[0].LearnedMeaning)
	assert.Contains(t, page, `value="again"`)
	assert.Contains(t, page, `value="got-it"`)

	grade := func(name string) *httptest.ResponseRecorder {
		return readingTestRequest(t, h, "/study/grades", form(url.Values{"progress": {progress[0].ID}, "reps": {"0"}, "grade": {name}}), cookies)
	}
	require.Equal(t, http.StatusSeeOther, grade("got-it").Code)
	graded, err := store.ListStudyProgress(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.StudyStateReview, graded[0].Memory.State)
	assert.True(t, domain.NextStudyDayStart(clock.Now()).Equal(graded[0].Memory.Due), "Got it on first sight is due the next Study day")
	require.Equal(t, http.StatusSeeOther, grade("again").Code, "a repeated tap of the same card is ignored")
	logs, err := store.ListStudyReviewLog(t.Context(), fixtures.OwnerID, progress[0].ID)
	require.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, http.StatusBadRequest, grade("hard").Code, "Study has two grades")
	assert.Contains(t, getAs(t, h, "/de/study", cookies).Body.String(), "Nothing to study right now.")
}
