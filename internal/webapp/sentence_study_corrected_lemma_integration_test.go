//go:build integration

package webapp

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestSentenceStudyShowsACorrectedGreekLemmaAsACitationForm holds that a
// corrected Greek lemma never reaches Sentence study as its folded key. A
// correction stores only the key, so Sentence study shows the headword of any of
// the owner's words that share it, and otherwise the key with its word-final
// sigma unfolded.
func TestSentenceStudyShowsACorrectedGreekLemmaAsACitationForm(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-corrected-study", "learner-password")
	registration, err := registerGreekDefinition(t, ctx, store, learner.ID, greekTestDefinition())
	require.NoError(t, err)
	importGreekFixture(t, ctx, store, learner.ID, registration.BookID, "oga-src-browse.conllu")

	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	chosen := perform(t, h, http.MethodPost, "/active-study-language", url.Values{"csrf_token": {csrf}, "language": {"grc"}}, cookies)
	require.Equal(t, http.StatusSeeOther, chosen.Code, "choose Ancient Greek as the study language: %s", chosen.Body.String())

	occurrences, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, registration.BookID, "δέω")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	reviewPath := "/reading/books/" + registration.BookID + "/lemma-review"
	// λόγος folds to the key λόγοσ, which the Book's λόγος tokens carry with
	// their headword. φίλος folds to φίλοσ, which no word of the owner carries.
	confirmLemmaDecision(t, h, reviewPath, cookies, csrf, "correct", "λόγος", occurrences[0].ID())
	confirmLemmaDecision(t, h, reviewPath, cookies, csrf, "correct", "φίλος", occurrences[1].ID())

	var run, corpus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, learner.ID, registration.BookID).Scan(&run, &corpus))
	unit := domain.ClassicalUnitID(greekTestDefinition().Key)
	studyOf := func(t *testing.T, sentence, target int64) string {
		t.Helper()
		query := url.Values{
			"book": {registration.BookID}, "run": {run}, "corpus": {corpus}, "unit": {unit},
			"sentence": {strconv.FormatInt(sentence, 10)}, "target": {strconv.FormatInt(target, 10)},
		}
		response := perform(t, h, http.MethodGet, "/vocabulary/concordance/sentence?"+query.Encode(), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code, "body=%s", response.Body.String())
		return response.Body.String()
	}

	// Each assertion reads the corrected δέω token's own evidence line, named
	// by its analyzer lemma, so another word's headword cannot satisfy it.
	t.Run("a stored citation form is shown for the corrected key", func(t *testing.T) {
		body := studyOf(t, 0, 1)
		require.Contains(t, body, "analyzer lemma <code>δέω1</code>, citation headword <code>λόγος</code>; morphology")
		require.NotContains(t, body, "λόγοσ")
	})

	t.Run("a key with no stored citation form has its final sigma unfolded", func(t *testing.T) {
		body := studyOf(t, 1, 1)
		require.Contains(t, body, "analyzer lemma <code>δέω2</code>, effective lemma <code>φίλος</code>; morphology")
		require.NotContains(t, body, "φίλοσ")
	})
}
