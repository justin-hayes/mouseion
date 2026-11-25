//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f catalogueAddFixture) previouslyRead(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return perform(t, f.handler, http.MethodPost, "/catalogs/sources/"+f.connection.ID+"/previously-read", url.Values{
		"csrf_token":       {f.csrf},
		"entry_identifier": {"add-entry"},
	}, f.cookies)
}

func TestCataloguePreviouslyReadCreatesAReadBookWithoutAcquiring(t *testing.T) {
	fixture := newCatalogueAddFixture(t)
	ctx := context.Background()
	_, err := fixture.store.PutKnownVocabulary(ctx, fixture.owner.ID, "de", "Haus", "NOUN")
	require.NoError(t, err)
	knownBefore, err := fixture.store.ListKnownVocabulary(ctx, fixture.owner.ID, "de")
	require.NoError(t, err)

	read := fixture.previouslyRead(t)
	require.Equal(t, http.StatusSeeOther, read.Code, read.Body.String())
	location, err := url.Parse(read.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs", location.Path)
	assert.Equal(t, "Recorded “Add Entry” as previously read.", location.Query().Get("message"))
	assert.Empty(t, fixture.requests.snapshot(), "I've read this requested the EPUB")
	assert.Zero(t, fixture.analysis.calls, "I've read this starts no analysis")
	assert.Zero(t, fixture.acquisition.calls, "I've read this enqueues no EPUB acquisition")

	entry := fixture.ownedBook(t)
	detail, err := fixture.store.GetBookDetail(ctx, fixture.owner.ID, entry.BookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, detail.Disposition, "a Read Book is not To Read")
	assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket())
	assert.Equal(t, 1, detail.CompletionCount)
	assert.Equal(t, domain.ReadingCompletionPreviouslyRead, detail.LatestCompletionSource)
	assert.False(t, detail.EverCurrentReading)
	assert.Nil(t, detail.Acquired, "I've read this downloads nothing")

	knownAfter, err := fixture.store.ListKnownVocabulary(ctx, fixture.owner.ID, "de")
	require.NoError(t, err)
	assert.ElementsMatch(t, knownBefore, knownAfter, "I've read this changes no Known vocabulary")
}

func TestCataloguePreviouslyReadRestoresTheSameBookAsRead(t *testing.T) {
	fixture := newCatalogueAddFixture(t)
	ctx := context.Background()
	added, err := fixture.store.AddCatalogueEntry(ctx, fixture.owner.ID, fixture.connection.ID, "add-entry")
	require.NoError(t, err)
	require.NoError(t, fixture.store.RemoveBookFromMyBooks(ctx, fixture.owner.ID, added.Book.ID))
	_, err = fixture.store.GetBookDetail(ctx, fixture.owner.ID, added.Book.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	read := fixture.previouslyRead(t)
	require.Equal(t, http.StatusSeeOther, read.Code, read.Body.String())
	location, err := url.Parse(read.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, added.Book.ID, location.Query().Get("read_book"), "the assertion restores the same Book")

	entry := fixture.ownedBook(t)
	assert.Equal(t, added.Book.ID, entry.BookID)
	detail, err := fixture.store.GetBookDetail(ctx, fixture.owner.ID, added.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, detail.Disposition)
	assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket())
	assert.Equal(t, 1, detail.CompletionCount)
	assert.Nil(t, detail.Acquired)
	assert.Empty(t, fixture.requests.snapshot(), "restoring a read Book requested the EPUB")
}

func TestClassicalPreviouslyReadRecordsAReadBookWithoutPreparing(t *testing.T) {
	ctx, store, pool, h, layerRequests := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-read-learner", "learner-password")
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	read := perform(t, h, http.MethodPost, "/catalogs/classical/units/homer-iliad/previously-read", url.Values{
		"csrf_token": {csrf},
		"return":     {"index"}, "catalog_language": {"grc"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, read.Code, read.Body.String())
	location, err := url.Parse(read.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs", location.Path)
	assert.Equal(t, "grc", location.Query().Get("language"))
	assert.Equal(t, "Recorded “Iliad” as previously read.", location.Query().Get("message"))
	bookID := location.Query().Get("read_book")
	require.NotEmpty(t, bookID)

	detail, err := store.GetBookDetail(ctx, learner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, detail.Disposition, "a Read Book is not To Read")
	assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket())
	assert.Equal(t, 1, detail.CompletionCount)
	assert.Equal(t, domain.ReadingCompletionPreviouslyRead, detail.LatestCompletionSource)
	require.NotNil(t, detail.ReadingUnit)
	assert.Equal(t, "homer-iliad", detail.ReadingUnit.ReadingUnitKey)
	assert.Nil(t, detail.Acquired, "I've read this downloads nothing")

	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM analysis_jobs WHERE owner_id = $1`, learner.ID), "I've read this uses no analysis job")
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue), "I've read this queues no preparation")
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM source_materials WHERE owner_id = $1`, learner.ID), "I've read this downloads nothing")
	assert.Zero(t, layerRequests.Load(), "I've read this makes no layer request")

	page := perform(t, h, http.MethodGet, read.Header().Get("Location"), nil, cookies)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Recorded “Iliad” as previously read.")
	assert.Contains(t, body, `id="catalog-read-confirmation"`)
	assert.Contains(t, body, `href="/catalogs/books/`+bookID+`/in-my-books">View in My Books</a>`)
	assert.NotContains(t, body, `units/homer-iliad/previously-read`, "an owned unit offers no I've read this")
}
