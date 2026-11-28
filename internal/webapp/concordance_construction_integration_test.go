//go:build integration

package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// constructionSource writes n copies of one synthetic sentence, each of which
// holds an indirect statement and a genitive articular infinitive.
func constructionSource(n int) string {
	var out strings.Builder
	for i := range n {
		ref := fmt.Sprintf("Xen. Anab._1.1.%d", i+1)
		base := 5 * i
		fmt.Fprintf(&out, "# source = Synthetic, %s\n# text = ἔφη ὅτι ἔπαυσεν τοῦ λύειν\n# sent_id = %d\n", ref, i+1)
		fmt.Fprintf(&out, "1\tἔφη\tφημί\tv\tv3saia---\t_\t0\tPRED\t_\tt_%d|0|ref=%s\n", base+1, ref)
		fmt.Fprintf(&out, "2\tὅτι\tὅτι\tc\tc--------\t_\t1\tAuxC\t_\tt_%d|t_%d|ref=%s\n", base+2, base+1, ref)
		fmt.Fprintf(&out, "3\tἔπαυσεν\tπαύω\tv\tv3saia---\t_\t2\tOBJ\t_\tt_%d|t_%d|ref=%s\n", base+3, base+2, ref)
		fmt.Fprintf(&out, "4\tτοῦ\tὁ\tl\tl-s---ng-\t_\t5\tATR\t_\tt_%d|t_%d|ref=%s\n", base+4, base+5, ref)
		fmt.Fprintf(&out, "5\tλύειν\tλύω\tv\tv--pna---\t_\t3\tOBJ\t_\tt_%d|t_%d|ref=%s\n\n", base+5, base+3, ref)
	}
	return out.String()
}

// constructionBook is one synthetic Greek Book: its Reading-unit key, Work and
// author, and how many copies of the sentence it holds.
type constructionBook struct {
	key, title, author string
	sentences          int
	// constituent is the Reading unit's constituent Book number, or zero.
	constituent int
	// workOf is the key of the Book whose Work this Book's unit joins.
	workOf string
	id     string
}

// importConstructionBook registers one Reading unit and imports its sentences as
// a native_agdt run, which writes its Construction occurrences and marker.
func importConstructionBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, pool *pgxpool.Pool, owner string, book *constructionBook, books map[string]*constructionBook) {
	t.Helper()
	data := []byte(constructionSource(book.sentences))
	path := filepath.Join(t.TempDir(), "source.conllu")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	sum := sha256.Sum256(data)
	definition := greekTestDefinition()
	definition.Key, definition.WorkTitle, definition.WorkAuthor = book.key, book.title, book.author
	definition.SourceFileSize, definition.SourceFileSHA256 = int64(len(data)), hex.EncodeToString(sum[:])
	definition.CitationStart, definition.CitationEnd = "1.1.1", fmt.Sprintf("1.1.%d", book.sentences)
	definition.InterpretationVersion = ancientgreek.InterpretationVersionNativeAGDT
	registration, err := registerGreekDefinition(t, ctx, store, owner, definition)
	require.NoError(t, err)
	book.id = registration.BookID
	if book.workOf != "" {
		// Units of one Work share its row; the registration path keys a Work by
		// the unit, so join them here the way the catalog does.
		_, err = pool.Exec(ctx, `
			UPDATE editions SET work_id=(
			  SELECT e.work_id FROM editions e JOIN reading_units ru ON ru.edition_id=e.id WHERE ru.definition_key=$2)
			WHERE definition_key=$1`, book.key, books[book.workOf].key)
		require.NoError(t, err)
	}
	if book.constituent > 0 {
		_, err = pool.Exec(ctx, `UPDATE reading_units SET constituent_book=$2, constituent_books=5 WHERE definition_key=$1`, book.key, book.constituent)
		require.NoError(t, err)
	}
	_, err = importGreekSource(ctx, greekImportRequest{
		OwnerID: owner, BookID: registration.BookID, SourcePath: path, Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)
}

var (
	constructionPageRowPattern  = regexp.MustCompile(`<h3 class="concordance-tier" id="concordance-tier-([a-z_]+)">([^<]*)</h3>|<details class="concordance-row" name="concordance-occurrences" id="occurrence-([0-9a-f-]{36})-(\d+)-(\d+)"`)
	constructionHrefPattern     = regexp.MustCompile(`<a class="button button--outline" href="([^"]+)"[^>]*>(Previous|Next)</a>`)
	constructionStudyHrefRegexp = regexp.MustCompile(`class="concordance-study-link"[^>]*href="([^"]+)"`)
)

// constructionPage is the tier headings and row keys of one parallels page, in
// document order: a heading is "# {label}", a row is "{unit key}:{sentence}".
func constructionPage(t *testing.T, body string, keys map[string]string) []string {
	t.Helper()
	var out []string
	for _, match := range constructionPageRowPattern.FindAllStringSubmatch(body, -1) {
		if match[1] != "" {
			out = append(out, "# "+match[2])
			continue
		}
		key, known := keys[match[3]]
		require.True(t, known, "row for unknown Book %s", match[3])
		out = append(out, key+":"+match[4]+"."+match[5])
	}
	return out
}

func constructionHref(t *testing.T, body, kind string) string {
	t.Helper()
	for _, match := range constructionHrefPattern.FindAllStringSubmatch(body, -1) {
		if match[2] == kind {
			return html.UnescapeString(match[1])
		}
	}
	return ""
}

// TestConcordanceConstructionParallelsOverHTTP is the primary seam for #2625:
// Parallels lists other occurrences of a construction in three tiers, in the
// reviewed order, 25 to a page, without the origin occurrence, and the origin
// rule, the revision guard and the way back all hold over HTTP (A15, A17, A18).
func TestConcordanceConstructionParallelsOverHTTP(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "construction-parallels", "learner-password")
	stranger := createAccount(t, ctx, store, "construction-stranger", "learner-password")

	// Registered out of order on purpose: Cyropaedia before Apology, and the
	// origin Work's units out of key order, so a sort is what orders them.
	books := map[string]*constructionBook{}
	for _, book := range []*constructionBook{
		{key: "xen-anab-b3", title: "Anabasis", author: "Xenophon", sentences: 2, constituent: 3},
		{key: "xen-anab-zz", title: "Anabasis", author: "Xenophon", sentences: 1, constituent: 1, workOf: "xen-anab-b3"},
		{key: "xen-anab-aa", title: "Anabasis", author: "Xenophon", sentences: 1, constituent: 2, workOf: "xen-anab-b3"},
		{key: "xen-cyr", title: "Cyropaedia", author: "Xenophon", sentences: 1},
		{key: "xen-apol", title: "Apology", author: "Xenophon", sentences: 1},
		{key: "plato-crito", title: "Crito", author: "Plato", sentences: 27},
		{key: "aesch-ctes", title: "Against Ctesiphon", author: "Aeschines", sentences: 1},
		{key: "lower-xen", title: "Hellenica", author: "xenophon", sentences: 1},
	} {
		books[book.key] = book
		importConstructionBook(t, ctx, store, pool, learner.ID, book, books)
	}
	origin := books["xen-anab-b3"]
	strangerBook := &constructionBook{key: "stranger-anab", title: "Anabasis", author: "Xenophon", sentences: 1}
	importConstructionBook(t, ctx, store, pool, stranger.ID, strangerBook, books)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")

	keys := map[string]string{}
	for _, book := range books {
		keys[book.id] = book.key
	}
	runOf := func(book *constructionBook) string {
		var run string
		require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE book_id=$1`, book.id).Scan(&run))
		return run
	}
	originRun := runOf(origin)
	query := func(overrides map[string]string) string {
		values := url.Values{
			"as": {"construction"}, "term": {"indirect_statement"}, "priority": {origin.id},
			"from_run": {originRun}, "from_sentence": {"0"}, "from_anchor": {"1"},
		}
		for key, value := range overrides {
			if value == "" {
				values.Del(key)
			} else {
				values.Set(key, value)
			}
		}
		return "/grc/vocabulary/concordance?" + values.Encode()
	}
	get := func(path string) *testingResponse {
		response := perform(t, h, http.MethodGet, path, nil, cookies)
		return &testingResponse{code: response.Code, body: response.Body.String()}
	}
	row := func(key string, sentence int) string { return fmt.Sprintf("%s:%d.1", key, sentence) }

	var expected []string
	expected = append(expected, "# This Book", row("xen-anab-b3", 1))
	expected = append(expected, "# This author", row("xen-anab-zz", 0), row("xen-anab-aa", 0), row("xen-apol", 0), row("xen-cyr", 0))
	expected = append(expected, "# Other authors", row("aesch-ctes", 0))
	for i := range 27 {
		expected = append(expected, row("plato-crito", i))
	}
	expected = append(expected, row("lower-xen", 0))

	first := get(query(nil))
	require.Equal(t, http.StatusOK, first.code, first.body)
	t.Run("page one lists the tiers in the reviewed order without the origin occurrence", func(t *testing.T) {
		page := constructionPage(t, first.body, keys)
		// 25 rows on the page plus the three tier headings.
		assert.Equal(t, expected[:28], page)
		assert.NotContains(t, page, row("xen-anab-b3", 0), "the origin occurrence is left out")
		assert.Contains(t, first.body, `<h2 id="concordance-summary" tabindex="-1">Parallels of Indirect statement</h2>`)
		assert.Contains(t, first.body, `Results 1–25 on page 1; more results are available.`)
	})
	t.Run("the heading links back to the origin sentence", func(t *testing.T) {
		link := regexp.MustCompile(`id="concordance-parallels-origin" href="([^"]+)"`).FindStringSubmatch(first.body)
		require.Len(t, link, 2)
		target := html.UnescapeString(link[1])
		assert.Contains(t, target, "/grc/vocabulary/concordance/sentence?")
		assert.Contains(t, target, "sentence=0")
		assert.Contains(t, target, "target=1")
		assert.Contains(t, target, "construction=indirect_statement")
		assert.True(t, strings.HasSuffix(target, "#construction-indirect_statement-1"), target)
		study := get(strings.SplitN(target, "#", 2)[0])
		assert.Equal(t, http.StatusOK, study.code)
		assert.Contains(t, study.body, "· from Concordance")
	})
	t.Run("page two repeats the tier heading and the same global order", func(t *testing.T) {
		next := constructionHref(t, first.body, "Next")
		require.NotEmpty(t, next)
		assert.Contains(t, next, "as=construction")
		assert.Contains(t, next, "from_run="+originRun)
		second := get(next)
		require.Equal(t, http.StatusOK, second.code, second.body)
		page := constructionPage(t, second.body, keys)
		assert.Equal(t, append([]string{"# Other authors"}, expected[28:]...), page)
		assert.Contains(t, second.body, "Results 26–34 on page 2; continues Crito from the previous page.")
		assert.Empty(t, constructionHref(t, second.body, "Next"))
		assert.NotEmpty(t, constructionHref(t, second.body, "Previous"))
	})

	t.Run("the origin rule sends the safe restart", func(t *testing.T) {
		for name, path := range map[string]string{
			"a missing priority":         query(map[string]string{"priority": ""}),
			"a priority of none":         query(map[string]string{"priority": "none"}),
			"a priority of another Book": query(map[string]string{"priority": books["plato-crito"].id}),
			"a missing from_run":         query(map[string]string{"from_run": ""}),
			"a malformed from_run":       query(map[string]string{"from_run": "not-a-run"}),
			"a malformed sentence":       query(map[string]string{"from_sentence": "-1"}),
			"an unknown sentence":        query(map[string]string{"from_sentence": "99"}),
			"an unknown anchor":          query(map[string]string{"from_anchor": "99"}),
			"a run of another learner":   query(map[string]string{"from_run": runOf(strangerBook), "priority": strangerBook.id}),
			"an unknown construction":    query(map[string]string{"term": "optative"}),
			"an unlabelled subtype":      query(map[string]string{"term": "result", "subtype": "actual"}),
		} {
			t.Run(name, func(t *testing.T) {
				response := get(path)
				assert.Equal(t, http.StatusBadRequest, response.code)
				assert.Contains(t, response.body, concordanceNoticeState)
				assert.NotContains(t, response.body, "concordance-tier-")
			})
		}
	})
	t.Run("a language without a construction scheme sends the safe restart", func(t *testing.T) {
		seedMigrationAnalyzedBook(t, ctx, store, learner.ID, "german-book", "Buch", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 1}})
		require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
		testutil.Cleanup(t, "restore Greek study language", func() error {
			return store.SetActiveStudyLanguage(ctx, learner.ID, "grc")
		})
		response := get(strings.Replace(query(nil), "/grc/", "/de/", 1))
		assert.Equal(t, http.StatusBadRequest, response.code)
		assert.Contains(t, response.body, concordanceNoticeState)
	})
	t.Run("a Book of the origin run's Work with no Reading unit has no origin", func(t *testing.T) {
		_, err := pool.Exec(ctx, `DELETE FROM book_reading_units WHERE book_id=$1`, books["lower-xen"].id)
		require.NoError(t, err)
		testutil.Cleanup(t, "restore Book's Reading unit", func() error {
			_, err := pool.Exec(ctx, `INSERT INTO book_reading_units(owner_id, book_id, reading_unit_id) SELECT $1, $2, id FROM reading_units WHERE definition_key='lower-xen'`, learner.ID, books["lower-xen"].id)
			return err
		})
		response := get(query(map[string]string{"from_run": runOf(books["lower-xen"]), "priority": books["lower-xen"].id}))
		assert.Equal(t, http.StatusBadRequest, response.code)
		assert.Contains(t, response.body, concordanceNoticeState)
	})

	t.Run("a labelled subtype filters, and All widens to the whole construction", func(t *testing.T) {
		filtered := get(query(map[string]string{"term": "articular_infinitive", "subtype": "genitive"}))
		require.Equal(t, http.StatusOK, filtered.code, filtered.body)
		assert.Contains(t, filtered.body, "Parallels of Articular infinitive (genitive)")
		assert.Contains(t, filtered.body, `id="concordance-parallels-widen"`)
		assert.Contains(t, filtered.body, ">All Articular infinitive</a>")
		widen := regexp.MustCompile(`id="concordance-parallels-widen" href="([^"]+)"`).FindStringSubmatch(filtered.body)
		require.Len(t, widen, 2)
		assert.NotContains(t, html.UnescapeString(widen[1]), "subtype=")
		assert.Contains(t, html.UnescapeString(widen[1]), "from_run="+originRun)
		assert.Equal(t, http.StatusOK, get(html.UnescapeString(widen[1])).code)
		// The same query without the subtype has no widen link.
		assert.NotContains(t, get(query(map[string]string{"term": "articular_infinitive"})).body, "concordance-parallels-widen")
	})
	t.Run("a construction no Book holds is a corpus result, not a failure", func(t *testing.T) {
		response := get(query(map[string]string{"term": "result"}))
		require.Equal(t, http.StatusOK, response.code)
		assert.Contains(t, response.body, "No other analyzed occurrences of Result clause in your grc Books. This is a corpus result, not a lookup failure.")
	})

	t.Run("Study opens Sentence study with the construction focused, and the way back keeps the page", func(t *testing.T) {
		link := constructionStudyHrefRegexp.FindStringSubmatch(first.body)
		require.Len(t, link, 2)
		target := html.UnescapeString(link[1])
		assert.Contains(t, target, "construction=indirect_statement")
		assert.True(t, strings.HasSuffix(target, "#construction-indirect_statement-1"), target)
		studyURL, err := url.Parse(target)
		require.NoError(t, err)
		study := get(studyURL.RequestURI())
		require.Equal(t, http.StatusOK, study.code, study.body)
		assert.Contains(t, study.body, `<li id="construction-indirect_statement-1" class="sentence-study-construction sentence-study-construction--focused">`)
		assert.Contains(t, study.body, `<span class="sentence-study-construction-from"> · from Concordance</span>`)
		assert.Equal(t, 1, strings.Count(study.body, "· from Concordance"))
		back := regexp.MustCompile(`<p><a href="([^"]+)">Return to Concordance results</a></p>`).FindStringSubmatch(study.body)
		require.Len(t, back, 2)
		backURL := html.UnescapeString(back[1])
		assert.Contains(t, backURL, "as=construction")
		assert.Contains(t, backURL, "from_run="+originRun)
		assert.Contains(t, backURL, "priority="+origin.id)
		returned := get(strings.SplitN(backURL, "#", 2)[0])
		require.Equal(t, http.StatusOK, returned.code)
		assert.Equal(t, constructionPage(t, first.body, keys), constructionPage(t, returned.body, keys))
	})
	t.Run("Parallels on a Sentence study entry opens this tiered page", func(t *testing.T) {
		studyQuery := url.Values{
			"book": {origin.id}, "run": {originRun}, "unit": {domain.ClassicalUnitID("xen-anab-b3")},
			"sentence": {"0"}, "target": {"1"},
		}
		var corpus string
		require.NoError(t, pool.QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE book_id=$1`, origin.id).Scan(&corpus))
		studyQuery.Set("corpus", corpus)
		study := get("/grc/vocabulary/concordance/sentence?" + studyQuery.Encode())
		require.Equal(t, http.StatusOK, study.code, study.body)
		assert.Contains(t, study.body, `aria-label="Parallels for Indirect statement"`)
		assert.Contains(t, study.body, `aria-label="Parallels for Articular infinitive, genitive"`)
		parallels := regexp.MustCompile(`<a href="([^"]+)" aria-label="Parallels for Indirect statement">`).FindStringSubmatch(study.body)
		require.Len(t, parallels, 2)
		followed := get(html.UnescapeString(parallels[1]))
		require.Equal(t, http.StatusOK, followed.code, followed.body)
		assert.Equal(t, constructionPage(t, first.body, keys), constructionPage(t, followed.body, keys))
	})

	t.Run("a run whose marker is behind contributes nothing, and a rebuild between pages changes the revision", func(t *testing.T) {
		next := constructionHref(t, first.body, "Next")
		require.NotEmpty(t, next)
		_, err := pool.Exec(ctx, `UPDATE construction_run_markers SET detector_version=$2 WHERE analysis_run_id=$1`, runOf(books["plato-crito"]), constructions.AGDTDetectorVersion-1)
		require.NoError(t, err)
		stale := get(next)
		assert.Equal(t, http.StatusConflict, stale.code)
		assert.Contains(t, stale.body, "Current evidence changed")
		refresh := regexp.MustCompile(`<a class="button button--outline" href="([^"]+)">Refresh results</a>`).FindStringSubmatch(stale.body)
		require.Len(t, refresh, 2)
		refreshURL := html.UnescapeString(refresh[1])
		assert.Contains(t, refreshURL, "as=construction")
		assert.Contains(t, refreshURL, "priority="+origin.id)
		assert.NotContains(t, refreshURL, "rev=")
		refreshed := get(refreshURL)
		require.Equal(t, http.StatusOK, refreshed.code)
		page := constructionPage(t, refreshed.body, keys)
		assert.NotContains(t, strings.Join(page, " "), "plato-crito", "the run with an old marker is silent")
		assert.Contains(t, page, row("aesch-ctes", 0))
		_, err = pool.Exec(ctx, `UPDATE construction_run_markers SET detector_version=$2 WHERE analysis_run_id=$1`, runOf(books["plato-crito"]), constructions.AGDTDetectorVersion)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, get(next).code, "the original revision is valid again once the marker is")
	})

	t.Run("rebuilding construction evidence between pages explains how to refresh", func(t *testing.T) {
		run := runOf(books["xen-apol"])
		_, err := pool.Exec(ctx, `UPDATE construction_run_markers SET detector_version=$2 WHERE analysis_run_id=$1`, run, constructions.AGDTDetectorVersion-1)
		require.NoError(t, err)
		worker := &analysis.ConstructionRebuildWorker{Pool: pool}
		job := &river.Job[analysis.ConstructionRebuildArgs]{Args: analysis.ConstructionRebuildArgs{
			OwnerID: learner.ID, RunID: run, DetectorVersion: constructions.AGDTDetectorVersion,
		}}
		testutil.Cleanup(t, "restore current construction evidence", func() error { return worker.Work(ctx, job) })

		before := get(query(nil))
		require.Equal(t, http.StatusOK, before.code, before.body)
		next := constructionHref(t, before.body, "Next")
		require.NotEmpty(t, next)
		require.Equal(t, http.StatusOK, get(next).code)

		require.NoError(t, worker.Work(ctx, job))
		stale := get(next)
		require.Equal(t, http.StatusConflict, stale.code)
		require.Contains(t, stale.body, "This page could not be shown because the evidence changed since the previous page, for example after a construction evidence rebuild. Refresh results to start at page 1 with the same search.")
	})

	t.Run("the origin is validated before the revision", func(t *testing.T) {
		for name, path := range map[string]string{
			"an unowned run":      query(map[string]string{"from_run": runOf(strangerBook), "priority": strangerBook.id, "rev": "stale"}),
			"a mismatched Book":   query(map[string]string{"priority": books["plato-crito"].id, "rev": "stale"}),
			"an unknown sentence": query(map[string]string{"from_sentence": "99", "rev": "stale"}),
		} {
			response := get(path)
			assert.Equal(t, http.StatusBadRequest, response.code, name)
			assert.Contains(t, response.body, concordanceNoticeState, name)
		}
		assert.Equal(t, http.StatusConflict, get(query(map[string]string{"rev": "stale"})).code, "a valid origin with a stale revision is still a change")
	})
	t.Run("an owned historical run still names its Book as the origin", func(t *testing.T) {
		_, err := pool.Exec(ctx, `DELETE FROM book_current_analyses WHERE book_id=$1`, origin.id)
		require.NoError(t, err)
		response := get(query(nil))
		require.Equal(t, http.StatusOK, response.code, response.body)
		page := constructionPage(t, response.body, keys)
		assert.NotContains(t, page, "# This Book", "only current analyses contribute rows")
		assert.Contains(t, page, "# This author")
		assert.Contains(t, page, row("xen-anab-zz", 0))
		// Sentence study reads only current analyses, so a historical origin
		// has no sentence to link back to; a link would be a 404.
		assert.NotContains(t, response.body, "concordance-parallels-origin")
		for _, link := range constructionStudyHrefRegexp.FindAllStringSubmatch(response.body, -1) {
			target, err := url.Parse(html.UnescapeString(link[1]))
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, get(target.RequestURI()).code, "every rendered Study link resolves")
		}
	})
	t.Run("the word lookups still work beside a parallels page", func(t *testing.T) {
		response := get("/grc/vocabulary/concordance?as=form&term=" + url.QueryEscape("ὅτι"))
		assert.Equal(t, http.StatusOK, response.code)
		assert.NotContains(t, response.body, "concordance-tier")
		assert.Contains(t, response.body, "Word form ὅτι")
	})
}

// testingResponse is one HTTP result of the parallels test.
type testingResponse struct {
	code int
	body string
}
