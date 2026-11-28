//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addClassicalTestUnits exercises the Add route once per reviewed fixture unit.
func addClassicalTestUnits(t *testing.T, h *Handler, cookies []*http.Cookie, csrf string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		response := perform(t, h, http.MethodPost, classicalUnitAddURL(key), url.Values{"csrf_token": {csrf}}, cookies)
		require.Equal(t, http.StatusSeeOther, response.Code, response.Body.String())
	}
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

func TestMyBooksShowsEveryClassicalBookAsAnOrdinaryRow(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-ordinary", "learner-password")
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")
	addOrRegisterUnits(t, ctx, store, learner.ID, "homer-iliad-1", "homer-iliad-2")

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	byPosition := classicalBooksByPosition(t, ctx, store, learner.ID, ids)
	require.Contains(t, byPosition, 1)
	require.Contains(t, byPosition, 2)

	// Book 1 is the current reading and To Read; Book 2 is Read.
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, byPosition[1].Book.ID, domain.BookDispositionToRead))
	_, err := pool.Exec(ctx, `INSERT INTO primary_goals(owner_id, book_id, language) VALUES ($1, $2, $3)`, learner.ID, byPosition[1].Book.ID, classical.Language)
	require.NoError(t, err)
	require.NoError(t, store.MarkBookPreviouslyRead(ctx, learner.ID, byPosition[2].Book.ID))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))

	toRead := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, toRead.Code, toRead.Body.String())
	toReadBody := toRead.Body.String()
	read := perform(t, h, http.MethodGet, "/library?history=read", nil, cookies)
	require.Equal(t, http.StatusOK, read.Code, read.Body.String())
	readBody := read.Body.String()

	t.Run("each Book is its own row with its own actions", func(t *testing.T) {
		require.Contains(t, toReadBody, `id="book-row-`+byPosition[1].Book.ID+`"`)
		require.Contains(t, toReadBody, "Iliad, Book 1")
		require.Contains(t, toReadBody, "Book 1 of 24")
		require.Contains(t, toReadBody, "By Homer")
		require.Contains(t, toReadBody, "Classical catalog")
		require.NotContains(t, toReadBody, "/library/books/"+byPosition[1].Book.ID+"/remove", "the current Reading's Book cannot be removed")
		require.Contains(t, toReadBody, "Your current reading cannot be removed.")
		require.NotContains(t, toReadBody, "Iliad, Book 2", "the To Read tab keeps only its own Books")
		require.Contains(t, readBody, `id="book-row-`+byPosition[2].Book.ID+`"`)
		require.Contains(t, readBody, "Iliad, Book 2")
		require.Contains(t, readBody, "/library/books/"+byPosition[2].Book.ID+"/remove")
	})
	t.Run("no grouped Work surface remains", func(t *testing.T) {
		for _, body := range []string{toReadBody, readBody} {
			for _, retired := range []string{"library-work", "Books in", "Move the work", "Remove the work", "/library/works/"} {
				require.NotContains(t, body, retired)
			}
		}
	})
	t.Run("tabs count Books", func(t *testing.T) {
		for _, want := range []string{"To Read (1)", "Read (1)"} {
			require.Contains(t, toReadBody, want, "tab count missing %q", want)
		}
		require.NotContains(t, toReadBody, "All (")
	})
	t.Run("the retired Inbox filter is unsupported", func(t *testing.T) {
		inbox := perform(t, h, http.MethodGet, "/library?disposition=inbox", nil, cookies)
		require.Equal(t, http.StatusNotFound, inbox.Code)
	})
}

// addClassicalUnitsByKey adds the named reviewed Reading units and returns each
// Book by its permanent unit key.
func addClassicalUnitsByKey(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner string, units ...string) map[string]domain.Book {
	t.Helper()
	books := make(map[string]domain.Book, len(units))
	for key, bookID := range addOrRegisterUnits(t, ctx, store, owner, units...) {
		detail, err := store.GetBookDetail(ctx, owner, bookID)
		require.NoError(t, err)
		books[key] = detail.Book
	}
	return books
}

// addOrRegisterUnits gives the learner a Book for each named Reading unit. A
// current unit is added from the Catalog; a withdrawn per-book unit can only
// be a Book the learner registered before ADR 0128.
func addOrRegisterUnits(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner string, units ...string) map[string]string {
	t.Helper()
	known := append(catalogFixtureDefinitions(), workDefinitions()...)
	byKey := map[string]string{}
	for _, key := range units {
		var definition classical.Definition
		for _, candidate := range known {
			if candidate.Key == key {
				definition = candidate
			}
		}
		require.Equal(t, key, definition.Key, "fixture defines unit %s", key)
		if !definition.Withdrawn {
			report, err := store.AddClassicalReadingUnits(ctx, owner, []string{key})
			require.NoError(t, err)
			byKey[key] = report.Books[0].BookID
			continue
		}
		title := definition.WorkTitle
		registration, err := store.RegisterReadingUnitBook(ctx, owner, domain.ReadingUnitDefinition{
			Key: definition.Key, Language: definition.Language, WorkTitle: title, WorkAuthor: definition.WorkAuthor,
			EditionLabel: definition.EditionLabel, CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
			SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size,
			SourceFileSHA256: definition.Source.SHA256, InterpretationVersion: definition.InterpretationVersion,
		})
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO book_membership(owner_id, book_id, state, activated_at) VALUES ($1, $2, 'active', now()) ON CONFLICT DO NOTHING`, owner, registration.BookID)
		require.NoError(t, err)
		require.NoError(t, store.SetBookDisposition(ctx, owner, registration.BookID, domain.BookDispositionToRead))
		byKey[key] = registration.BookID
	}
	return byKey
}

func TestChooserRendersTheSelectedToReadBookInline(t *testing.T) {
	ctx, store, h, learner := readingChooserWorkHarness(t)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	books := addClassicalUnitsByKey(t, ctx, store, learner.ID, "homer-iliad-1", "homer-iliad-2")
	seedClassicalReadingEvidence(t, ctx, store, learner.ID, books["homer-iliad-1"], "homer-iliad-1", 99)
	seedClassicalReadingEvidence(t, ctx, store, learner.ID, books["homer-iliad-2"], "homer-iliad-2", 92)
	for _, book := range books {
		require.NoError(t, store.SetBookDisposition(ctx, learner.ID, book.ID, domain.BookDispositionToRead))
	}
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")

	chooser := perform(t, h, http.MethodGet, "/grc/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, chooser.Code, chooser.Body.String())
	body := chooser.Body.String()
	entry := books["homer-iliad-1"]
	require.Contains(t, body, `id="book-row-`+entry.ID+`"`)
	require.Contains(t, body, `action="/reading/books/`+entry.ID+`/start"`)
}

func TestMyBooksSingleBookWorkStaysOrdinaryWithPosition(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-single-work", "learner-password")
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")
	addOrRegisterUnits(t, ctx, store, learner.ID, "homer-iliad-1", "homer-iliad-2")
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
	assert.NotContains(t, body, "library-work", "a single owned Book stays an ordinary row")
}

func TestMyBooksWholeWorkUnitStaysOrdinaryWithoutPosition(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-whole-work", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalTestUnits(t, h, cookies, csrf, "xenophon-hellenica")
	require.Len(t, ownerBooks(t, pool, learner.ID), 1)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))

	page := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, "Hellenica")
	assert.NotContains(t, body, "Book 1 of")
	assert.NotContains(t, body, "library-work")
}

func TestMyBooksSearchMatchesAuthorEnglishAndOriginalTitlesButNotATrailingBookNumber(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-work-search", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalTestUnits(t, h, cookies, csrf, "homer-iliad", "xenophon-hellenica")
	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))

	for _, query := range []string{"homer", "iliad", "Ἰλιάς", "ιλιασ", "ΙΛΙΑΣ"} {
		t.Run(query, func(t *testing.T) {
			page := perform(t, h, http.MethodGet, "/library?q="+url.QueryEscape(query), nil, cookies)
			require.Equal(t, http.StatusOK, page.Code, query)
			body := page.Body.String()
			require.Contains(t, body, "Iliad", "query %q did not keep the Iliad", query)
			require.NotContains(t, body, "Hellenica", "query %q kept an unrelated Book", query)
		})
	}

	t.Run("a trailing number is not a Book number", func(t *testing.T) {
		missing := perform(t, h, http.MethodGet, "/library?q="+url.QueryEscape("iliad 2"), nil, cookies)
		require.Equal(t, http.StatusOK, missing.Code)
		assert.Contains(t, missing.Body.String(), "No To Read books in Ancient Greek match “iliad 2”.")
	})
}

func TestMyBooksNonClassicalRowsKeepTheirSearchFiltersAndPaging(t *testing.T) {
	ctx, store, _, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "my-books-ordinary-unchanged", "learner-password")
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")
	for _, title := range []string{"Zweites Buch", "Erstes Buch"} {
		book, err := store.CreateBook(ctx, domain.Book{
			OwnerID: learner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
			LanguageState: domain.LanguageChosen, LanguageTag: "de",
		})
		require.NoError(t, err)
		require.NoError(t, store.SetBookDisposition(ctx, learner.ID, book.ID, domain.BookDispositionToRead))
	}
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))

	t.Run("ordinary rows stay ordinary", func(t *testing.T) {
		page := perform(t, h, http.MethodGet, "/library", nil, cookies)
		require.Equal(t, http.StatusOK, page.Code, page.Body.String())
		body := page.Body.String()
		for _, want := range []string{"Zweites Buch", "Erstes Buch"} {
			assert.Contains(t, body, want)
		}
		for _, forbidden := range []string{"library-work", "Books in", "Book 1 of"} {
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
