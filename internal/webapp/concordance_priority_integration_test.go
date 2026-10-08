//go:build integration

package webapp

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	priorityGroupPattern = regexp.MustCompile(`concordance-book-title">([^<]*)</span>\s*<span class="concordance-book-count">([^<]*)</span>`)
	priorityRowPattern   = regexp.MustCompile(`id="occurrence-([0-9a-f-]{36})-`)
	priorityHrefPattern  = regexp.MustCompile(`<a class="button button--outline" href="([^"]+)"[^>]*>(Previous|Next)</a>`)
)

func TestConcordanceCapturesPriorityAndKeepsStableOccurrencePagesOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-priority-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "priority-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "priority-bob", "bob-password", false)
	seed := func(owner, key, title string) domain.Book {
		book, _, _, _ := seedMigrationAnalyzedBook(t, ctx, store, owner, key, title, []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 1}})
		return book
	}
	hits := func(n int) [][]lookupToken {
		sentences := make([][]lookupToken, n)
		for i := range sentences {
			sentences[i] = []lookupToken{{"Wir", "wir", "PRON"}, {"gehen", "gehen", "VERB"}}
		}
		return sentences
	}
	zeta := seed(alice.ID, "priority-zeta", "Zeta")
	alpha := seed(alice.ID, "priority-alpha", "Alpha")
	beta := seed(alice.ID, "priority-beta", "beta")
	gleichOne := seed(alice.ID, "priority-gleich-1", "Gleich")
	gleichTwo := seed(alice.ID, "priority-gleich-2", "Gleich")
	leer := seed(alice.ID, "priority-leer", "Leer")
	pending := seed(alice.ID, "priority-pending", "Pending")
	bobBook := seed(bob.ID, "priority-bob", "Bobs Buch")
	italian, italianSource, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "priority-italian", "Libro", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 1}})
	setLookupBookLanguage(t, ctx, store, italian, italianSource, "it")

	seedLookupSentences(t, ctx, store, zeta, "de", hits(60))
	seedLookupSentences(t, ctx, store, alpha, "de", hits(3))
	seedLookupSentences(t, ctx, store, beta, "de", hits(2))
	seedLookupSentences(t, ctx, store, gleichOne, "de", hits(1))
	seedLookupSentences(t, ctx, store, gleichTwo, "de", hits(1))
	seedLookupSentences(t, ctx, store, leer, "de", [][]lookupToken{{{"Haus", "haus", "NOUN"}}})
	seedLookupSentences(t, ctx, store, pending, "de", hits(4))
	seedLookupSentences(t, ctx, store, bobBook, "de", hits(1))
	_, err = store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, pending.ID)
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "priority-alice", "alice-password")
	get := func(rawQuery string) *httptest.ResponseRecorder {
		return perform(t, h, http.MethodGet, "/vocabulary/concordance?"+rawQuery, nil, cookies)
	}
	fresh := func() *httptest.ResponseRecorder {
		return get(url.Values{"language": {"de"}, "term": {"gehen"}}.Encode())
	}
	// groups returns the page-local source labels in order: "Title|count text".
	groups := func(body string) []string {
		var found []string
		for _, match := range priorityGroupPattern.FindAllStringSubmatch(lookupResults(body), -1) {
			found = append(found, match[1]+"|"+match[2])
		}
		return found
	}
	rowBooks := func(body string) []string {
		var ids []string
		for _, match := range priorityRowPattern.FindAllStringSubmatch(lookupResults(body), -1) {
			ids = append(ids, match[1])
		}
		return ids
	}
	pageLink := func(body, label string) string {
		for _, match := range priorityHrefPattern.FindAllStringSubmatch(body, -1) {
			if match[2] == label {
				return html.UnescapeString(match[1])
			}
		}
		return ""
	}

	for _, book := range []domain.Book{zeta, alpha, leer} {
		require.NoError(t, store.SetBookDisposition(ctx, alice.ID, book.ID, domain.BookDispositionToRead))
	}
	startZeta, err := store.StartCurrentReading(ctx, alice.ID, "de", zeta.ID)
	require.NoError(t, err)

	var firstPage, secondPage, thirdPage string
	t.Run("a fresh lookup captures the current Book and pages 60 priority hits as 25, 25, then 10 before other Books", func(t *testing.T) {
		first := fresh()
		require.Equal(t, http.StatusOK, first.Code)
		firstPage = first.Body.String()
		assert.Equal(t, 25, lookupRowCount(firstPage))
		assert.Equal(t, []string{"Zeta|25 occurrences on this page"}, groups(firstPage))
		assert.Contains(t, firstPage, "Results 1–25 on page 1")
		assert.NotContains(t, firstPage, "from the previous page", "the group began on this page")
		assert.Contains(t, firstPage, "Results begin with Zeta, the Book you were reading when you looked this up.")
		assert.NotContains(t, firstPage, `id="concordance-reading-changed"`)
		next := pageLink(firstPage, "Next")
		require.NotEmpty(t, next)
		assert.Contains(t, next, "priority="+zeta.ID)

		second := get(strings.TrimPrefix(next, "/vocabulary/concordance?"))
		require.Equal(t, http.StatusOK, second.Code)
		secondPage = second.Body.String()
		assert.Equal(t, []string{"Zeta|25 occurrences on this page"}, groups(secondPage))
		assert.Contains(t, secondPage, "Results 26–50 on page 2; continues Zeta from the previous page")

		third := get(strings.TrimPrefix(pageLink(secondPage, "Next"), "/vocabulary/concordance?"))
		require.Equal(t, http.StatusOK, third.Code)
		thirdPage = third.Body.String()
		assert.Equal(t, []string{
			"Zeta|10 occurrences on this page", "Alpha|3 occurrences on this page", "beta|2 occurrences on this page",
			"Gleich|1 occurrence on this page", "Gleich|1 occurrence on this page",
		}, groups(thirdPage))
		assert.Contains(t, thirdPage, "Results 51–67 on page 3; continues Zeta from the previous page")
		assert.Empty(t, pageLink(thirdPage, "Next"), "no fabricated further page")
		assert.NotContains(t, thirdPage, "Leer", "a Book without matches gets no empty group")
		assert.NotContains(t, lookupResults(thirdPage), "Pending")
		assert.NotContains(t, lookupResults(thirdPage), "Bobs Buch")
	})

	t.Run("other Books order by case-insensitive title, exact title, then stable identity", func(t *testing.T) {
		third := rowBooks(thirdPage)
		gleich := []string{gleichOne.ID, gleichTwo.ID}
		slices.Sort(gleich)
		tail := third[len(third)-2:]
		assert.Equal(t, gleich, tail, "equal titles fall back to Book identity")
		assert.Equal(t, alpha.ID, third[10], "Alpha precedes lower-case beta regardless of byte order")
		assert.Equal(t, beta.ID, third[13])
	})

	t.Run("Start, End, and Switch never reorder an existing result set and offer a refresh", func(t *testing.T) {
		switched, err := store.SwitchCurrentReading(ctx, alice.ID, "de", alpha.ID, zeta.ID, startZeta.SnapshotID)
		require.NoError(t, err)
		again := get(strings.TrimPrefix(pageLink(firstPage, "Next"), "/vocabulary/concordance?"))
		require.Equal(t, http.StatusOK, again.Code)
		assert.Equal(t, []string{"Zeta|25 occurrences on this page"}, groups(again.Body.String()), "the captured order survives a Switch")
		assert.Contains(t, again.Body.String(), "Results begin with Zeta, the Book you were reading when you looked this up.")
		assert.Contains(t, again.Body.String(), `id="concordance-reading-changed"`)
		assert.Contains(t, again.Body.String(), "Your reading changed after this lookup")
		refreshMatch := regexp.MustCompile(`id="concordance-reading-changed".*?href="([^"]+)">Refresh results`).FindStringSubmatch(strings.ReplaceAll(again.Body.String(), "\n", " "))
		require.Len(t, refreshMatch, 2)
		refresh := html.UnescapeString(refreshMatch[1])
		assert.NotContains(t, refresh, "priority=")
		assert.NotContains(t, refresh, "rev=")
		assert.Contains(t, refresh, "page=1")
		assert.NotContains(t, refresh, "as=", "an independent lookup recognizes its term again")

		refreshed := get(strings.TrimPrefix(refresh, "/vocabulary/concordance?"))
		require.Equal(t, http.StatusOK, refreshed.Code)
		assert.Equal(t, []string{"Alpha|3 occurrences on this page", "beta|2 occurrences on this page", "Gleich|1 occurrence on this page", "Gleich|1 occurrence on this page", "Zeta|18 occurrences on this page"}, groups(refreshed.Body.String()), "refresh captures the new current Book")
		assert.Contains(t, refreshed.Body.String(), "Results begin with Alpha")
		assert.NotContains(t, refreshed.Body.String(), `id="concordance-reading-changed"`)

		require.NoError(t, store.EndCurrentReading(ctx, alice.ID, "de", alpha.ID, switched.SnapshotID))
		ended := get(strings.TrimPrefix(pageLink(firstPage, "Next"), "/vocabulary/concordance?"))
		assert.Equal(t, []string{"Zeta|25 occurrences on this page"}, groups(ended.Body.String()))
		assert.Contains(t, ended.Body.String(), `id="concordance-reading-changed"`)
	})

	t.Run("absence of current reading is captured explicitly and persists until refresh", func(t *testing.T) {
		none := fresh()
		require.Equal(t, http.StatusOK, none.Code)
		body := none.Body.String()
		assert.Equal(t, []string{"Alpha|3 occurrences on this page", "beta|2 occurrences on this page", "Gleich|1 occurrence on this page", "Gleich|1 occurrence on this page", "Zeta|18 occurrences on this page"}, groups(body))
		assert.Contains(t, body, "No Book was being read when you looked this up")
		next := pageLink(body, "Next")
		assert.Contains(t, next, "priority=none")
		restarted, err := store.StartCurrentReading(ctx, alice.ID, "de", leer.ID)
		require.NoError(t, err)
		second := get(strings.TrimPrefix(next, "/vocabulary/concordance?"))
		require.Equal(t, http.StatusOK, second.Code)
		assert.Equal(t, []string{"Zeta|25 occurrences on this page"}, groups(second.Body.String()), "a later Start does not reorder")
		assert.Contains(t, second.Body.String(), "continues Zeta from the previous page")
		assert.Contains(t, second.Body.String(), `id="concordance-reading-changed"`)

		// A copied supported URL restores the same order for another request.
		copied := get(strings.TrimPrefix(next, "/vocabulary/concordance?"))
		assert.Equal(t, rowBooks(second.Body.String()), rowBooks(copied.Body.String()))

		t.Run("no priority-Book matches is distinct from other Books' results", func(t *testing.T) {
			noMatches := fresh()
			require.Equal(t, http.StatusOK, noMatches.Code)
			text := noMatches.Body.String()
			assert.Contains(t, text, "No matches in Leer, the Book you were reading when you looked this up. Results from your other Books follow.")
			assert.Equal(t, "Alpha|3 occurrences on this page", groups(text)[0], "no empty priority group")
			assert.NotContains(t, strings.Join(groups(text), ","), "Leer")
			assert.NotContains(t, text, `id="concordance-reading-changed"`)
		})

		require.NoError(t, store.EndCurrentReading(ctx, alice.ID, "de", leer.ID, restarted.SnapshotID))
	})

	t.Run("an unavailable priority analysis is not reported as zero matches", func(t *testing.T) {
		response := get(url.Values{"language": {"de"}, "term": {"gehen"}, "as": {"lemma"}, "priority": {pending.ID}}.Encode())
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "Pending, the Book you were reading when you looked this up, has no current analysis to search")
		assert.NotContains(t, body, "No matches in Pending")
		assert.Equal(t, "Alpha|3 occurrences on this page", groups(body)[0])
		assert.NotContains(t, strings.Join(groups(body), ","), "Pending")
	})

	t.Run("malformed or foreign priority state explains a safe restart", func(t *testing.T) {
		for name, values := range map[string]url.Values{
			"not an identity":  {"term": {"gehen"}, "priority": {"zeta"}},
			"foreign owner":    {"term": {"gehen"}, "priority": {bobBook.ID}},
			"other language":   {"term": {"gehen"}, "priority": {italian.ID}},
			"unknown Book":     {"term": {"gehen"}, "priority": {"00000000-0000-4000-8000-000000000000"}},
			"revision without": {"term": {"gehen"}, "as": {"lemma"}, "rev": {"abc"}},
		} {
			values.Set("language", "de")
			response := get(values.Encode())
			assert.Equal(t, http.StatusBadRequest, response.Code, name)
			assert.Contains(t, response.Body.String(), "This Concordance link is not valid", name)
			assert.NotContains(t, response.Body.String(), `id="concordance-results"`, name)
			assert.NotContains(t, response.Body.String(), "Bobs Buch", name)
		}
	})

	t.Run("a valid out-of-range page says so and offers page 1", func(t *testing.T) {
		response := get(url.Values{"language": {"de"}, "term": {"gehen"}, "as": {"lemma"}, "priority": {zeta.ID}, "page": {"9"}}.Encode())
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "No results on this page.")
		assert.Contains(t, response.Body.String(), "Go to page 1")
		assert.NotContains(t, response.Body.String(), "No matches in")
		assert.NotContains(t, response.Body.String(), "No analyzed occurrences of this")
		exact := get(url.Values{"language": {"de"}, "term": {"gehen"}, "as": {"lemma"}, "priority": {zeta.ID}, "page": {"4"}}.Encode())
		assert.Contains(t, exact.Body.String(), "No results on this page.", "the boundary after the last full page is out of range")
	})

	t.Run("evidence changes need an explicit refresh while visibility and disposition do not", func(t *testing.T) {
		revision := regexp.MustCompile(`rev=([0-9a-f]+)`).FindStringSubmatch(html.UnescapeString(pageLink(firstPage, "Next")))
		require.Len(t, revision, 2)
		page := func() *httptest.ResponseRecorder {
			return get(url.Values{"language": {"de"}, "term": {"gehen"}, "as": {"lemma"}, "priority": {zeta.ID}, "page": {"2"}, "rev": {revision[1]}}.Encode())
		}
		require.Equal(t, http.StatusOK, page().Code)

		require.NoError(t, store.SetBookDisposition(ctx, alice.ID, beta.ID, domain.BookDispositionToRead))
		_, err := store.Pool().Exec(ctx, `INSERT INTO book_visibility(owner_id,book_id,hidden) VALUES($1,$2,true)`, alice.ID, beta.ID)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, page().Code, "visibility and disposition are not Concordance evidence")

		_, err = store.Pool().Exec(ctx, `UPDATE books SET title='Zeta Retitled' WHERE owner_id=$1 AND id=$2`, alice.ID, zeta.ID)
		require.NoError(t, err)
		stale := page()
		assert.Equal(t, http.StatusConflict, stale.Code)
		assert.Contains(t, stale.Body.String(), "Current evidence changed")
		assert.Contains(t, stale.Body.String(), "Refresh results")
		assert.NotContains(t, lookupResults(stale.Body.String()), "occurrence-")
	})

	t.Run("Study returns carry the captured priority", func(t *testing.T) {
		response := fresh()
		require.Equal(t, http.StatusOK, response.Code)
		study := regexp.MustCompile(`href="(/vocabulary/concordance/sentence\?[^"]+)"`).FindStringSubmatch(response.Body.String())
		require.Len(t, study, 2)
		parsed, err := url.Parse(html.UnescapeString(study[1]))
		require.NoError(t, err)
		back := parsed.Query().Get("return")
		assert.Contains(t, back, "priority=none")
		page := perform(t, h, http.MethodGet, html.UnescapeString(study[1]), nil, cookies)
		require.Equal(t, http.StatusOK, page.Code)
		assert.Contains(t, html.UnescapeString(page.Body.String()), fmt.Sprintf(`href="%s"`, html.UnescapeString(back)))
	})
}
