//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCatalogCompleteLoopFromBothSources covers the shipped loop across the
// Catalog's two sources: one search returns an OPDS entry and a classical
// entry, Add queues each Book's work, Remove and Add restore the same Book, and
// My Books exposes only the To Read and Read tabs.
func TestCatalogCompleteLoopFromBothSources(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "catalog-loop-learner", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	connection, err := store.CreateOpdsConnection(ctx, learner.ID, domain.OpdsConnection{
		Name: "Loop catalog", URL: "https://loop.example/opds",
	})
	require.NoError(t, err)
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, learner.ID, connection.ID, []domain.CatalogueIndexEntry{{
		ConnectionID: connection.ID, EntryIdentifier: "homer-loop",
		Title: "Homer Loop", Author: "Homer", LanguageTag: "de",
		EPUBHref: "https://loop.example/homer.epub",
	}}, time.Now()))
	h.services.CatalogueSync = noopCatalogueSync{}

	search := perform(t, h, http.MethodGet, "/catalogs?q=Homer", nil, cookies)
	require.Equal(t, http.StatusOK, search.Code, search.Body.String())
	assert.Contains(t, search.Body.String(), "Homer Loop", "one search finds the OPDS entry")
	assert.Contains(t, search.Body.String(), "Iliad", "one search finds the classical entry")

	addedOPDS := perform(t, h, http.MethodPost, "/catalogs/sources/"+connection.ID+"/add", url.Values{
		"csrf_token": {csrf}, "entry_identifier": {"homer-loop"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, addedOPDS.Code, addedOPDS.Body.String())
	opdsBooks, err := store.ListMyBooksBrowse(ctx, learner.ID, "Homer Loop", "de", string(domain.BookDispositionToRead), false, 0, 10)
	require.NoError(t, err)
	require.Len(t, opdsBooks.Items, 1)
	opdsBookID := opdsBooks.Items[0].Book.ID
	assert.Equal(t, domain.BookDispositionToRead, opdsBooks.Items[0].Disposition)

	addedClassical := perform(t, h, http.MethodPost, "/catalogs/classical/units/homer-iliad-1/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, addedClassical.Code, addedClassical.Body.String())
	classicalBook, err := store.GetBookDetail(ctx, learner.ID, booksByUnitKey(t, ctx, store, learner.ID, "homer-iliad-1"))
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, classicalBook.Disposition)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'book_id'=$2`, analysis.ClassicalPreparationQueue, classicalBook.Book.ID), "classical Add queues its preparation")

	// Remove ends membership only; Add restores the same Book ID.
	removed := perform(t, h, http.MethodPost, "/library/books/"+opdsBookID+"/remove", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, removed.Code, removed.Body.String())
	_, err = store.GetBookDetail(ctx, learner.ID, opdsBookID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	readded := perform(t, h, http.MethodPost, "/catalogs/sources/"+connection.ID+"/add", url.Values{
		"csrf_token": {csrf}, "entry_identifier": {"homer-loop"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, readded.Code, readded.Body.String())
	restored, err := store.GetBookDetail(ctx, learner.ID, opdsBookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, restored.Disposition, "Add restores the same Book as To Read")

	// I've read this records Read without acquiring anything.
	previouslyRead := perform(t, h, http.MethodPost, "/catalogs/classical/units/homer-iliad-1/previously-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, previouslyRead.Code, previouslyRead.Body.String())
	readBook, err := store.GetBookDetail(ctx, learner.ID, classicalBook.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.MyBookBucketRead, readBook.WorkflowBucket())

	library := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, library.Body.String())
	body := library.Body.String()
	assert.Contains(t, body, "To Read")
	assert.Contains(t, body, "Read")
	assert.NotContains(t, body, "Inbox")
	assert.NotContains(t, body, "needs-language")
	assert.Contains(t, body, "Homer Loop")
	tabs := perform(t, h, http.MethodGet, "/library", nil, cookies).Body.String()
	assert.Equal(t, 1, strings.Count(tabs, `href="/library?disposition=to_read"`))
	assert.Equal(t, 1, strings.Count(tabs, `href="/library?history=read"`))
	retired := perform(t, h, http.MethodGet, "/library?needs-language", nil, cookies)
	assert.Equal(t, http.StatusNotFound, retired.Code)
}

// noopCatalogueSync answers only the status read the Catalog page needs. The
// complete loop seeds the index directly; connect and sync through the OPDS
// worker are covered by TestCompleteLearnerLoopFromOnboardingToCurrentReadingCompletion.
type noopCatalogueSync struct{}

func (noopCatalogueSync) RegisterConnection(context.Context, string, string) error { return nil }
func (noopCatalogueSync) UnregisterConnection(string, string) error                { return nil }
func (noopCatalogueSync) ListCatalogueSyncStatuses(context.Context, string) ([]domain.CatalogueSyncStatus, error) {
	return nil, nil
}
func (noopCatalogueSync) List(context.Context, string) ([]cataloguesync.Status, error) {
	return nil, nil
}
func (noopCatalogueSync) Get(context.Context, string, int64) (cataloguesync.Status, error) {
	return cataloguesync.Status{}, nil
}
func (noopCatalogueSync) Enqueue(context.Context, string, string) (cataloguesync.Handle, error) {
	return cataloguesync.Handle{}, nil
}
func (noopCatalogueSync) Retry(context.Context, string, int64) (cataloguesync.Handle, error) {
	return cataloguesync.Handle{}, nil
}
func (noopCatalogueSync) Cancel(context.Context, string, int64) (cataloguesync.Status, error) {
	return cataloguesync.Status{}, nil
}
func (noopCatalogueSync) FindAcquisitionTarget(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
	return cataloguesync.AcquisitionTarget{}, nil
}

func booksByUnitKey(t *testing.T, ctx context.Context, store myBooksBrowser, owner, unitKey string) string {
	t.Helper()
	all, err := store.ListMyBooksBrowse(ctx, owner, "", "", "", false, 0, 100)
	require.NoError(t, err)
	for _, book := range all.Items {
		if book.ReadingUnit != nil && book.ReadingUnit.ReadingUnitKey == unitKey {
			return book.Book.ID
		}
	}
	t.Fatalf("no Book for Reading unit %q", unitKey)
	return ""
}

type myBooksBrowser interface {
	ListMyBooksBrowse(context.Context, string, string, string, string, bool, int, int) (domain.MyBooksBrowseResult, error)
}
