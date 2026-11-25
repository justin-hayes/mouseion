//go:build integration

package webapp

import (
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/domain"
)

var fluencyContextField = regexp.MustCompile(`name="context" value="([^"]+)"`)

// shownFluencyContext is the context of the Fluency card the Study page shows.
func (s *studyHarness) shownFluencyContext() string {
	s.t.Helper()
	page := getAs(s.t, s.h, "/de/study?answer=1", s.cookies).Body.String()
	match := fluencyContextField.FindStringSubmatch(page)
	if match == nil {
		return ""
	}
	return match[1]
}

func (s *studyHarness) answerFluency(contextID, response string) {
	s.t.Helper()
	s.clock.Advance(time.Minute)
	result := s.post("/study/fluency/responses", url.Values{"language": {"de"}, "context": {contextID}, "response": {response}})
	require.Equal(s.t, http.StatusSeeOther, result.Code)
}

func TestStudyFluencyBringsAContextNotRecognisedBackInTheSameSitting(t *testing.T) {
	s := newStudyHarness(t)
	word := s.matureHaus()
	s.openFluency()
	progressBefore, logBefore := s.progress(), s.reviewLog(word.ID)

	hard := s.shownFluencyContext()
	require.NotEmpty(t, hard)
	s.answerFluency(hard, "not-recognised")

	// The other contexts are shown, and recognised, before it returns, none
	// repeating and none showing the hard one at once.
	seen := map[string]bool{hard: true}
	for range domain.FluencyReturnAfter {
		next := s.shownFluencyContext()
		require.NotEmpty(t, next)
		assert.NotEqual(t, hard, next, "the context does not repeat at once")
		assert.False(t, seen[next], "another context is shown first")
		seen[next] = true
		s.answerFluency(next, "recognised")
	}
	assert.Equal(t, hard, s.shownFluencyContext(), "the context marked Didn't recognise it comes back in the same sitting")

	// Recognised, it stops returning.
	s.answerFluency(hard, "recognised")
	assert.NotEqual(t, hard, s.shownFluencyContext())

	assert.Equal(t, progressBefore, s.progress(), "Fluency never changes Study progress")
	assert.Equal(t, logBefore, s.reviewLog(word.ID), "Fluency never writes the review log")
}

func TestStudyFluencyFavoursAContextNotRecognisedInALaterSitting(t *testing.T) {
	s := newStudyHarness(t)
	s.matureHaus()
	s.openFluency()

	var hard string
	// Four answers: the hard context first, then three recognised. The fifth
	// context stays unanswered and the hard one would return in this sitting.
	for i := range domain.FluencyReturnAfter + 1 {
		shown := s.shownFluencyContext()
		require.NotEmpty(t, shown)
		if i == 0 {
			hard = shown
			s.answerFluency(shown, "not-recognised")
			continue
		}
		s.answerFluency(shown, "recognised")
	}
	// A day later, in a new sitting, the context not recognised is shown before
	// recognised ones and even before the one never answered.
	s.clock.Advance(24 * time.Hour)
	assert.Equal(t, hard, s.shownFluencyContext())
}
