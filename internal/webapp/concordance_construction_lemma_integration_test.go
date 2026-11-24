//go:build integration

package webapp

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var unsearchedLinePattern = regexp.MustCompile(`(?s)<p id="concordance-unsearched" class="concordance-unsearched">(.*?)</p>`)

// TestConcordanceConstructionLemmaFilterAndUnsearchedBooksOverHTTP is the
// primary seam for #2626: "Only with {lemma}" applies the anchor's effective
// lemma (a correction replaces it, an exclusion hides it) and only under the
// filter; the Books a parallels page could not search are named in one line,
// and a rebuilding run stays silent (A15, A16, A17).
func TestConcordanceConstructionLemmaFilterAndUnsearchedBooksOverHTTP(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "construction-lemma", "learner-password")
	books := map[string]*constructionBook{}
	for _, book := range []*constructionBook{
		{key: "xen-anab", title: "Anabasis", author: "Xenophon", sentences: 1},
		{key: "xen-apol", title: "Apology", author: "Xenophon", sentences: 3},
		{key: "plato-crito", title: "Crito", author: "Plato", sentences: 1},
		{key: "plato-meno", title: "Meno", author: "Plato", sentences: 1},
		{key: "plato-phaedo", title: "Phaedo", author: "Plato", sentences: 1},
		{key: "plato-republic", title: "Republic", author: "Plato", sentences: 1},
	} {
		books[book.key] = book
		importConstructionBook(t, ctx, store, pool, learner.ID, book, books)
	}
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))
	h.services.AnalysisUpdates = analysis.NewService(pool, nil)
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	origin, apology := books["xen-anab"], books["xen-apol"]
	runOf := func(book *constructionBook) string {
		var run string
		require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE book_id=$1`, book.id).Scan(&run))
		return run
	}
	// The anchor of the articular infinitive is a content word whose lemma the
	// review accepts; read it from the stored occurrence rather than assume it.
	const term = "articular_infinitive"
	var anchor int
	var anchorSurface, anchorLemma string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT c.anchor_token_ordinal, t.surface, t.canonical_lemma
		  FROM construction_occurrences c
		  JOIN corpus_tokens t ON t.analysis_run_id=c.analysis_run_id AND t.sentence_ordinal=c.sentence_ordinal
		   AND t.token_ordinal=c.anchor_token_ordinal
		 WHERE c.analysis_run_id=$1 AND c.construction_id=$2 AND c.sentence_ordinal=0`, runOf(origin), term).Scan(&anchor, &anchorSurface, &anchorLemma))
	require.NotEmpty(t, anchorLemma)
	const correctedLemma = "λέγω"

	query := func(overrides map[string]string) string {
		values := url.Values{
			"as": {"construction"}, "term": {term}, "priority": {origin.id}, "language": {"grc"},
			"from_run": {runOf(origin)}, "from_sentence": {"0"}, "from_anchor": {strconv.Itoa(anchor)}, "from_lemma": {anchorLemma},
		}
		for key, value := range overrides {
			if value == "" {
				values.Del(key)
			} else {
				values.Set(key, value)
			}
		}
		return "/vocabulary/concordance?" + values.Encode()
	}
	get := func(path string) *testingResponse {
		response := perform(t, h, http.MethodGet, path, nil, cookies)
		return &testingResponse{code: response.Code, body: response.Body.String()}
	}
	keys := map[string]string{}
	for _, book := range books {
		keys[book.id] = book.key
	}
	rows := func(response *testingResponse) []string {
		var out []string
		for _, entry := range constructionPage(t, response.body, keys) {
			if !strings.HasPrefix(entry, "# ") {
				out = append(out, entry)
			}
		}
		return out
	}
	row := func(key string, sentence int) string { return fmt.Sprintf("%s:%d.%d", key, sentence, anchor) }

	// Correct the first Apology occurrence of the anchor and exclude the second.
	reviewPath := "/reading/books/" + apology.id + "/lemma-review"
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, apology.id, anchorSurface)
	require.NoError(t, err)
	require.Len(t, occurrences, 3)
	slices.SortFunc(occurrences, func(a, b domain.LemmaReviewOccurrence) int { return int(a.StartOffset - b.StartOffset) })
	confirmLemmaDecisionForm(t, h, reviewPath, cookies, csrf, anchorSurface, "correct", correctedLemma, occurrences[0].ID())
	confirmLemmaDecisionForm(t, h, reviewPath, cookies, csrf, anchorSurface, "exclude", "", occurrences[1].ID())

	t.Run("without the filter, corrected and excluded anchors still appear", func(t *testing.T) {
		plain := get(query(map[string]string{"from_lemma": ""}))
		require.Equal(t, http.StatusOK, plain.code, plain.body)
		assert.ElementsMatch(t, []string{
			row("xen-apol", 0), row("xen-apol", 1), row("xen-apol", 2),
			row("plato-crito", 0), row("plato-meno", 0), row("plato-phaedo", 0), row("plato-republic", 0),
		}, rows(plain))
		assert.Contains(t, plain.body, "concordance-parallels-only-with", "the offer is read from the origin anchor, not the URL")
		assert.NotContains(t, plain.body, "concordance-parallels-all-lemmas")
	})
	t.Run("the page offers Only with the origin lemma", func(t *testing.T) {
		offered := get(query(nil))
		require.Equal(t, http.StatusOK, offered.code, offered.body)
		link := regexp.MustCompile(`id="concordance-parallels-only-with" href="([^"]+)" aria-label="([^"]+)">Only with <em>([^<]+)</em></a>`).FindStringSubmatch(offered.body)
		require.Len(t, link, 4)
		assert.Equal(t, anchorLemma, html.UnescapeString(link[3]))
		assert.Contains(t, html.UnescapeString(link[2]), "Only with "+anchorLemma)
		target := html.UnescapeString(link[1])
		assert.Contains(t, target, "lemma="+url.QueryEscape(anchorLemma))
		assert.Contains(t, target, "from_lemma="+url.QueryEscape(anchorLemma))
		assert.Equal(t, rows(get(query(map[string]string{"lemma": anchorLemma}))), rows(get(target)))
	})
	t.Run("with the filter, a corrected anchor matches by its corrected lemma and an excluded one is hidden", func(t *testing.T) {
		filtered := get(query(map[string]string{"lemma": anchorLemma}))
		require.Equal(t, http.StatusOK, filtered.code, filtered.body)
		assert.ElementsMatch(t, []string{
			row("xen-apol", 2),
			row("plato-crito", 0), row("plato-meno", 0), row("plato-phaedo", 0), row("plato-republic", 0),
		}, rows(filtered), "the corrected and the excluded Apology anchors leave the original lemma")
		assert.Contains(t, filtered.body, `id="concordance-parallels-lemma"`)
		assert.NotContains(t, filtered.body, "concordance-parallels-only-with")

		corrected := get(query(map[string]string{"lemma": correctedLemma}))
		require.Equal(t, http.StatusOK, corrected.code, corrected.body)
		assert.Equal(t, []string{row("xen-apol", 0)}, rows(corrected), "the corrected anchor matches its corrected lemma only")

		none := get(query(map[string]string{"lemma": "οὐδέν"}))
		require.Equal(t, http.StatusOK, none.code)
		assert.Empty(t, rows(none))
		assert.Contains(t, none.body, "No other analyzed occurrences of")
	})
	t.Run("the lemma is normalised with the language's lemma key", func(t *testing.T) {
		keyed := get(query(map[string]string{"lemma": correctedLemma}))
		decomposed := get(query(map[string]string{"lemma": " λε\u0301γω "}))
		require.Equal(t, http.StatusOK, decomposed.code)
		assert.Equal(t, []string{row("xen-apol", 0)}, rows(decomposed), "a decomposed, padded lemma is the same key")
		assert.Equal(t, rows(keyed), rows(decomposed))
	})
	t.Run("the filter is cleared by All, and paging, restart and Study keep it", func(t *testing.T) {
		filtered := get(query(map[string]string{"lemma": anchorLemma}))
		clearLink := regexp.MustCompile(`id="concordance-parallels-all-lemmas" href="([^"]+)" aria-label="([^"]+)">All ([^<]+)</a>`).FindStringSubmatch(filtered.body)
		require.Len(t, clearLink, 4)
		assert.Equal(t, "Articular infinitive", clearLink[3])
		assert.Contains(t, html.UnescapeString(clearLink[2]), "All Articular infinitive")
		cleared := html.UnescapeString(clearLink[1])
		clearedValues := parseURL(t, cleared)
		assert.Empty(t, clearedValues.Get("lemma"), "the filter parameter is gone")
		assert.Equal(t, anchorLemma, clearedValues.Get("from_lemma"), "the offer to filter again stays")
		assert.Len(t, rows(get(cleared)), 7)
		study := constructionStudyHrefRegexp.FindStringSubmatch(filtered.body)
		require.Len(t, study, 2)
		returnTo := parseURL(t, html.UnescapeString(study[1])).Get("return")
		assert.Equal(t, anchorLemma, parseURL(t, returnTo).Get("lemma"), "the way back from Study keeps the filter")
		assert.Equal(t, anchorLemma, parseURL(t, returnTo).Get("from_lemma"))
	})
	t.Run("a correction made between pages is a change only under the filter", func(t *testing.T) {
		result, err := store.ListVocabularyConcordance(ctx, learner.ID, "grc", domain.ConcordanceLookup{
			Match: domain.ConcordanceMatchConstruction, Term: term, Priority: origin.id, Page: 1, Lemma: anchorLemma,
			Origin: &domain.ConcordanceConstructionOrigin{RunID: runOf(origin), Sentence: 0, Anchor: anchor},
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.Revision)
		plain, err := store.ListVocabularyConcordance(ctx, learner.ID, "grc", domain.ConcordanceLookup{
			Match: domain.ConcordanceMatchConstruction, Term: term, Priority: origin.id, Page: 1,
			Origin: &domain.ConcordanceConstructionOrigin{RunID: runOf(origin), Sentence: 0, Anchor: anchor},
		})
		require.NoError(t, err)
		assert.NotEqual(t, plain.Revision, result.Revision, "the filtered revision covers the decisions")

		confirmLemmaDecisionForm(t, h, reviewPath, cookies, csrf, anchorSurface, "correct", correctedLemma, occurrences[2].ID())
		stale := get(query(map[string]string{"lemma": anchorLemma, "rev": result.Revision}))
		assert.Equal(t, http.StatusConflict, stale.code)
		assert.Contains(t, stale.body, "Current evidence changed")
		unfiltered := get(query(map[string]string{"rev": plain.Revision}))
		assert.Equal(t, http.StatusOK, unfiltered.code, "detection ignores decisions, so an unfiltered page is not stale")
		assert.Equal(t, []string{row("xen-apol", 0), row("xen-apol", 2)}, rows(get(query(map[string]string{"lemma": correctedLemma}))))
	})

	t.Run("the unsearched line names an absent run and a Stanza-path run, and stays silent for a rebuilding one", func(t *testing.T) {
		quiet := get(query(nil))
		assert.NotContains(t, quiet.body, "concordance-unsearched", "every Book is searchable")

		// Crito is an older import: no syntax, so no construction evidence.
		for _, book := range []*constructionBook{books["plato-crito"], books["plato-phaedo"]} {
			_, err := pool.Exec(ctx, `DELETE FROM construction_run_markers WHERE analysis_run_id=$1`, runOf(book))
			require.NoError(t, err)
		}
		_, err := pool.Exec(ctx, `UPDATE analysis_runs SET syntax_state='absent' WHERE id=$1`, runOf(books["plato-crito"]))
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE analysis_runs SET syntax_state='ud' WHERE id=$1`, runOf(books["plato-phaedo"]))
		require.NoError(t, err)
		// Meno's marker is behind: the run is rebuilding, which is silent.
		_, err = pool.Exec(ctx, `UPDATE construction_run_markers SET detector_version=detector_version-1 WHERE analysis_run_id=$1`, runOf(books["plato-meno"]))
		require.NoError(t, err)
		// An update is offered only for a To Read Book, so Phaedo gets none.
		_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='to_read' WHERE book_id=$1`, books["plato-crito"].id)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='inbox' WHERE book_id=$1`, books["plato-phaedo"].id)
		require.NoError(t, err)

		line := get(query(map[string]string{"lemma": anchorLemma}))
		require.Equal(t, http.StatusOK, line.code, line.body)
		match := unsearchedLinePattern.FindStringSubmatch(line.body)
		require.Len(t, match, 2, line.body)
		assert.Equal(t, "Not searched for constructions: Crito — Analysis update available, Phaedo", visibleText(match[1]), "by title, one line, the suffix only with an offer")
		assert.Equal(t, 1, strings.Count(line.body, `id="concordance-unsearched"`))
		assert.Contains(t, line.body, "<em>Crito</em>")
		assert.NotContains(t, match[1], "Meno", "a rebuilding run is silent")
		assert.NotContains(t, match[1], "Republic")
		assert.NotContains(t, rows(line), row("plato-crito", 0), "an absent run contributes no rows")
	})
	t.Run("the line also shows when there are no parallels", func(t *testing.T) {
		empty := get(query(map[string]string{"lemma": "οὐδέν"}))
		require.Equal(t, http.StatusOK, empty.code)
		assert.Contains(t, empty.body, "No other analyzed occurrences of")
		assert.Contains(t, empty.body, "Not searched for constructions: ")
	})
	t.Run("a lemma outside a construction lookup is not accepted", func(t *testing.T) {
		assert.Equal(t, "/vocabulary/concordance", safeConcordanceReturnURL("/vocabulary/concordance?term=haus&as=form&page=1&lemma=haus"))
		response := get("/vocabulary/concordance?as=form&term=" + url.QueryEscape("ὅτι") + "&lemma=x&from_lemma=y")
		assert.Equal(t, http.StatusOK, response.code)
		assert.NotContains(t, response.body, "concordance-parallels")
	})
	t.Run("a folded lemma key is shown as its citation form, and still filters by the key", func(t *testing.T) {
		// The imported anchors carry the folded key and the citation headword.
		_, err := pool.Exec(ctx, `
			UPDATE corpus_tokens SET canonical_lemma='λόγοσ', headword='λόγος'
			 WHERE token_ordinal=$1 AND analysis_run_id IN (SELECT analysis_run_id FROM current_analysis_identity)`, anchor)
		require.NoError(t, err)
		const key = "λόγοσ"
		offered := get(query(map[string]string{"from_lemma": key}))
		require.Equal(t, http.StatusOK, offered.code, offered.body)
		assert.Contains(t, offered.body, `aria-label="Only with λόγος: parallels signalled by this lemma"`)
		assert.Contains(t, offered.body, `>Only with <em>λόγος</em></a>`)
		assert.NotContains(t, offered.body, key, "the folded key is never shown")
		link := regexp.MustCompile(`id="concordance-parallels-only-with" href="([^"]+)"`).FindStringSubmatch(offered.body)
		require.Len(t, link, 2)
		assert.Equal(t, key, parseURL(t, html.UnescapeString(link[1])).Get("lemma"), "the URL keeps the key")

		filtered := get(query(map[string]string{"from_lemma": key, "lemma": key}))
		require.Equal(t, http.StatusOK, filtered.code, filtered.body)
		assert.ElementsMatch(t, []string{row("plato-republic", 0)}, rows(filtered), "Apology is corrected away and Meno is rebuilding")
		assert.Contains(t, filtered.body, `<span id="concordance-parallels-lemma">Only with <em>λόγος</em></span>`)
		assert.NotContains(t, filtered.body, key)
	})
}

// TestConcordanceConstructionLemmaParametersAreNormalisedAndTrustedOnlyFromTheServer
// covers the merger's probes: the offered lemma comes from the origin anchor
// read server-side, a corrected anchor shows a citation form, and the lemma
// parameter is a key in every generated URL.
func TestConcordanceConstructionLemmaParametersAreNormalisedAndTrustedOnlyFromTheServer(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "construction-lemma-probes", "learner-password")
	books := map[string]*constructionBook{}
	for _, book := range []*constructionBook{
		{key: "xen-anab", title: "Anabasis", author: "Xenophon", sentences: 1},
		{key: "plato-republic", title: "Republic", author: "Plato", sentences: 1},
	} {
		books[book.key] = book
		importConstructionBook(t, ctx, store, pool, learner.ID, book, books)
	}
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	origin := books["xen-anab"]
	var run string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE book_id=$1`, origin.id).Scan(&run))
	const term = "articular_infinitive"
	var anchor int
	var anchorSurface, anchorLemma string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT c.anchor_token_ordinal, t.surface, t.canonical_lemma
		  FROM construction_occurrences c
		  JOIN corpus_tokens t ON t.analysis_run_id=c.analysis_run_id AND t.sentence_ordinal=c.sentence_ordinal
		   AND t.token_ordinal=c.anchor_token_ordinal
		 WHERE c.analysis_run_id=$1 AND c.construction_id=$2 AND c.sentence_ordinal=0`, run, term).Scan(&anchor, &anchorSurface, &anchorLemma))
	query := func(overrides map[string]string) string {
		values := url.Values{
			"as": {"construction"}, "term": {term}, "priority": {origin.id}, "language": {"grc"},
			"from_run": {run}, "from_sentence": {"0"}, "from_anchor": {strconv.Itoa(anchor)},
		}
		for key, value := range overrides {
			values.Set(key, value)
		}
		return "/vocabulary/concordance?" + values.Encode()
	}
	get := func(path string) *testingResponse {
		response := perform(t, h, http.MethodGet, path, nil, cookies)
		return &testingResponse{code: response.Code, body: response.Body.String()}
	}
	onlyWith := regexp.MustCompile(`id="concordance-parallels-only-with" href="([^"]+)" aria-label="([^"]+)">Only with <em>([^<]+)</em></a>`)

	t.Run("the offered lemma is the origin anchor's, whatever from_lemma says", func(t *testing.T) {
		page := get(query(map[string]string{"from_lemma": "οὐδέν"}))
		require.Equal(t, http.StatusOK, page.code, page.body)
		link := onlyWith.FindStringSubmatch(page.body)
		require.Len(t, link, 4)
		assert.Equal(t, anchorLemma, html.UnescapeString(link[3]))
		target := parseURL(t, html.UnescapeString(link[1]))
		assert.Equal(t, anchorLemma, target.Get("lemma"))
		assert.Equal(t, anchorLemma, target.Get("from_lemma"))
		assert.NotContains(t, page.body, "οὐδέν")
	})
	t.Run("the lemma parameter is a key in every generated URL", func(t *testing.T) {
		upper := strings.ToUpper(anchorLemma)
		require.NotEqual(t, anchorLemma, upper)
		page := get(query(map[string]string{"lemma": upper}))
		require.Equal(t, http.StatusOK, page.code, page.body)
		assert.Contains(t, rowsOf(t, page, books), row2("plato-republic"), "the upper-case lemma filters like its key")
		study := constructionStudyHrefRegexp.FindStringSubmatch(page.body)
		require.Len(t, study, 2)
		returnTo := parseURL(t, parseURL(t, html.UnescapeString(study[1])).Get("return"))
		assert.Equal(t, anchorLemma, returnTo.Get("lemma"))
		assert.NotContains(t, html.UnescapeString(study[1]), url.QueryEscape(upper))
		assert.Contains(t, page.body, `<span id="concordance-parallels-lemma">Only with <em>`+anchorLemma+`</em></span>`)
	})
	t.Run("a corrected anchor shows the citation form of its lemma", func(t *testing.T) {
		// Another occurrence carries the key and its citation headword.
		_, err := pool.Exec(ctx, `UPDATE corpus_tokens SET canonical_lemma='λόγοσ', headword='λόγος' WHERE analysis_run_id=$1 AND token_ordinal=$2`,
			currentRun(t, ctx, pool, books["plato-republic"]), anchor)
		require.NoError(t, err)
		reviewPath := "/reading/books/" + origin.id + "/lemma-review"
		occurrences, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, origin.id, anchorSurface)
		require.NoError(t, err)
		require.Len(t, occurrences, 1)
		confirmLemmaDecisionForm(t, h, reviewPath, cookies, csrf, anchorSurface, "correct", "λόγος", occurrences[0].ID())

		page := get(query(nil))
		require.Equal(t, http.StatusOK, page.code, page.body)
		link := onlyWith.FindStringSubmatch(page.body)
		require.Len(t, link, 4)
		assert.Equal(t, "λόγος", html.UnescapeString(link[3]))
		assert.Contains(t, html.UnescapeString(link[2]), "Only with λόγος")
		assert.Equal(t, "λόγοσ", parseURL(t, html.UnescapeString(link[1])).Get("lemma"), "the URL keeps the key")
		assert.NotContains(t, page.body, "λόγοσ")
		filtered := get(query(map[string]string{"lemma": "λόγοσ"}))
		assert.Contains(t, filtered.body, `<span id="concordance-parallels-lemma">Only with <em>λόγος</em></span>`)
		assert.NotContains(t, filtered.body, "λόγοσ")
	})
}

func currentRun(t *testing.T, ctx context.Context, pool *pgxpool.Pool, book *constructionBook) string {
	t.Helper()
	var run string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE book_id=$1`, book.id).Scan(&run))
	return run
}

func rowsOf(t *testing.T, response *testingResponse, books map[string]*constructionBook) []string {
	t.Helper()
	keys := map[string]string{}
	for _, book := range books {
		keys[book.id] = book.key
	}
	var out []string
	for _, entry := range constructionPage(t, response.body, keys) {
		if !strings.HasPrefix(entry, "# ") {
			out = append(out, entry)
		}
	}
	return out
}

func row2(key string) string { return key + ":0.4" }
