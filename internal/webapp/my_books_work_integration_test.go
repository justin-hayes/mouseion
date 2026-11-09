//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addClassicalWork adds one whole reviewed Work and returns its Books in book
// order, which is also the order the grouped strip renders.
func addClassicalWork(t *testing.T, h *Handler, cookies []*http.Cookie, csrf, workKey string) {
	t.Helper()
	response := perform(t, h, http.MethodPost, "/catalogs/classical/works/"+workKey+"/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, response.Code, response.Body.String())
}

func classicalBooksByPosition(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner string, bookIDs []string) map[int]domain.MyBook {
	t.Helper()
	books := make(map[int]domain.MyBook, len(bookIDs))
	for _, id := range bookIDs {
		book, err := store.GetBookDetail(ctx, owner, id)
		require.NoError(t, err)
		require.NotNil(t, book.ReadingUnit)
		books[book.ReadingUnit.ConstituentBook] = book
	}
	return books
}

func TestMyBooksGroupsOneWorkAndFiltersCountBooks(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-grouped", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	byPosition := classicalBooksByPosition(t, ctx, store, learner.ID, ids)
	require.Contains(t, byPosition, 1)
	require.Contains(t, byPosition, 2)

	// Book 1 is the current reading and To Read; Book 2 is Read.
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, byPosition[1].Book.ID, domain.BookDispositionToRead))
	_, err := pool.Exec(ctx, `INSERT INTO primary_goals(owner_id, book_id, language) VALUES ($1, $2, $3)`, learner.ID, byPosition[1].Book.ID, classical.Language)
	require.NoError(t, err)
	_, err = store.ImportPreviouslyRead(ctx, learner.ID, byPosition[2].Book.ID)
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))

	page := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()

	t.Run("identity", func(t *testing.T) {
		for _, want := range []string{"Iliad", "By Homer", "Ἰλιάς", "2 of 24 books", "Classical catalog", "2 Books in 1 work"} {
			assert.Contains(t, body, want, "grouped Work page missing %q", want)
		}
	})
	t.Run("leading badge and counts", func(t *testing.T) {
		for _, want := range []string{"Currently reading Book 1", "1 Read"} {
			assert.Contains(t, body, want, "grouped Work margin missing %q", want)
		}
		assert.NotContains(t, body, "Currently reading Book 2")
	})
	t.Run("strip links", func(t *testing.T) {
		for _, want := range []string{
			`aria-label="Iliad, Book 1 of 24, Currently reading"`,
			`aria-label="Iliad, Book 2 of 24, Read"`,
			`href="/reading#journey-book-` + byPosition[1].Book.ID + `"`,
			`href="/reading#journey-book-` + byPosition[2].Book.ID + `"`,
		} {
			assert.Contains(t, body, want, "grouped Work strip missing %q", want)
		}
	})
	t.Run("one legend per page", func(t *testing.T) {
		assert.Equal(t, 1, strings.Count(body, `class="library-work-legend"`), "one legend per page")
	})
	t.Run("filters count Books", func(t *testing.T) {
		for _, want := range []string{"All (2)", "To Read (1)", "Read (1)"} {
			assert.Contains(t, body, want, "filter count missing %q", want)
		}
	})

	t.Run("To Read keeps only matching cells", func(t *testing.T) {
		toRead := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
		require.Equal(t, http.StatusOK, toRead.Code)
		toReadBody := toRead.Body.String()
		assert.Contains(t, toReadBody, `aria-label="Iliad, Book 1 of 24, Currently reading"`)
		assert.NotContains(t, toReadBody, `aria-label="Iliad, Book 2 of 24, Read"`)
		assert.Contains(t, toReadBody, "Currently reading Book 1")
		assert.Contains(t, toReadBody, "1 Book in 1 work")
	})

	t.Run("Read keeps only matching cells", func(t *testing.T) {
		read := perform(t, h, http.MethodGet, "/library?history=read", nil, cookies)
		require.Equal(t, http.StatusOK, read.Code)
		readBody := read.Body.String()
		assert.Contains(t, readBody, `aria-label="Iliad, Book 2 of 24, Read"`)
		assert.NotContains(t, readBody, `aria-label="Iliad, Book 1 of 24, Currently reading"`)
		assert.Contains(t, readBody, ">Read</span>")
		assert.Contains(t, readBody, "1 Book in 1 work")
	})

	t.Run("Inbox has no work row", func(t *testing.T) {
		inbox := perform(t, h, http.MethodGet, "/library?disposition=inbox", nil, cookies)
		require.Equal(t, http.StatusOK, inbox.Code)
		assert.NotContains(t, inbox.Body.String(), `class="library-work"`)
		assert.Contains(t, inbox.Body.String(), "No Inbox books in your local collection are in Ancient Greek.")
	})
}

func TestMyBooksSingleBookWorkStaysOrdinaryWithPosition(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-single-work", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")
	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	byPosition := classicalBooksByPosition(t, ctx, store, learner.ID, ids)

	// Remove Book 2 from My Books so the Work is owned once.
	_, err := pool.Exec(ctx, `UPDATE book_membership SET state = 'removed', removed_at = now() WHERE owner_id = $1 AND book_id = $2`, learner.ID, byPosition[2].Book.ID)
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))

	page := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, "Iliad, Book 1")
	assert.Contains(t, body, "Book 1 of 24")
	assert.NotContains(t, body, `class="library-work"`, "a single owned Book stays an ordinary row")
	assert.NotContains(t, body, "library-work-legend")
}

func TestMyBooksWholeWorkUnitStaysOrdinaryWithoutPosition(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-whole-work", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "xenophon-hellenica")
	require.Len(t, ownerBooks(t, pool, learner.ID), 1)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))

	page := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, "Hellenica")
	assert.NotContains(t, body, "Book 1 of")
	assert.NotContains(t, body, `class="library-work"`)
}

func TestMyBooksWorkSearchMatchesAuthorEnglishAndGreekTitlesAndNarrowsByBook(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-work-search", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")
	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))

	for _, query := range []string{"homer", "iliad", "Ἰλιάς", "ιλιασ", "ΙΛΙΑΣ"} {
		t.Run(query, func(t *testing.T) {
			page := perform(t, h, http.MethodGet, "/library?q="+url.QueryEscape(query), nil, cookies)
			require.Equal(t, http.StatusOK, page.Code, query)
			body := page.Body.String()
			assert.Contains(t, body, "Iliad", "query %q did not find the Work", query)
			assert.Contains(t, body, `aria-label="Iliad, Book 1 of 24, Inbox"`, "query %q did not keep Book 1", query)
			assert.Contains(t, body, `aria-label="Iliad, Book 2 of 24, Inbox"`, "query %q did not keep Book 2", query)
		})
	}

	t.Run("trailing number narrows to Book 2", func(t *testing.T) {
		narrowed := perform(t, h, http.MethodGet, "/library?q="+url.QueryEscape("iliad 2"), nil, cookies)
		require.Equal(t, http.StatusOK, narrowed.Code)
		narrowedBody := narrowed.Body.String()
		assert.Contains(t, narrowedBody, `aria-label="Iliad, Book 2 of 24, Inbox"`)
		assert.NotContains(t, narrowedBody, `aria-label="Iliad, Book 1 of 24, Inbox"`)
		assert.Contains(t, narrowedBody, "1 Book in 1 work")
	})

	t.Run("missing constituent has no match", func(t *testing.T) {
		missing := perform(t, h, http.MethodGet, "/library?q="+url.QueryEscape("iliad 99"), nil, cookies)
		require.Equal(t, http.StatusOK, missing.Code)
		assert.Contains(t, missing.Body.String(), "No books in your local collection in Ancient Greek match “iliad 99”.")
	})
}

func TestMyBooksNonClassicalRowsKeepTheirSearchFiltersAndPaging(t *testing.T) {
	ctx, store, _, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-ordinary-unchanged", "learner-password")
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")
	for _, title := range []string{"Zweites Buch", "Erstes Buch"} {
		_, err := store.CreateBook(ctx, domain.Book{
			OwnerID: learner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
			LanguageState: domain.LanguageChosen, LanguageTag: "de",
		})
		require.NoError(t, err)
	}
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))

	t.Run("ordinary rows stay ordinary", func(t *testing.T) {
		page := perform(t, h, http.MethodGet, "/library", nil, cookies)
		require.Equal(t, http.StatusOK, page.Code, page.Body.String())
		body := page.Body.String()
		for _, want := range []string{"Zweites Buch", "Erstes Buch"} {
			assert.Contains(t, body, want)
		}
		for _, forbidden := range []string{"library-work-legend", "Books in", "Book 1 of"} {
			assert.NotContains(t, body, forbidden)
		}
	})

	t.Run("search stays a title and author substring", func(t *testing.T) {
		search := perform(t, h, http.MethodGet, "/library?q=erstes", nil, cookies)
		require.Equal(t, http.StatusOK, search.Code)
		assert.Contains(t, search.Body.String(), "Erstes Buch")
		assert.NotContains(t, search.Body.String(), "Zweites Buch")
		assert.Contains(t, search.Body.String(), "1 book found")
	})
}
