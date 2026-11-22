//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCatalogCompleteLoopFromBothSources covers the shipped loop across the
// Catalog's two sources with no NLP service: the learner connects a catalog
// and syncs it through the real catalogue-sync worker against a local OPDS
// feed, one search finds the synced OPDS entry beside a classical entry, Add
// queues each Book's durable work (an EPUB acquisition and a classical
// preparation), Remove and Add restore the same Book without duplicating the
// live acquisition, "I've read this" moves the classical Book to the Read tab,
// and My Books exposes exactly the To Read and Read tabs.
func TestCatalogCompleteLoopFromBothSources(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "catalog-loop-learner", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	// The OPDS catalog the whole loop runs against.
	catalogue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		switch r.URL.Path {
		case "/opds/language":
			testwrite.String(t, w, `<?xml version="1.0"?><feed><title>Languages</title><entry><title>German</title><link rel="subsection" href="/opds/language/7"/></entry></feed>`)
		case "/opds/language/7":
			testwrite.String(t, w, `<?xml version="1.0"?><feed><title>German books</title><entry><id>homer-loop</id><title>Homer Loop</title><author><name>Homer</name></author><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/homer-loop.epub"/></entry></feed>`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(catalogue.Close)

	// The real catalogue-sync service runs against that feed over a River
	// client that serves only the catalogue_sync queue. The EPUB acquisition
	// worker is registered so Add's insert is accepted, but its queue is not
	// configured, so the acquisition and the classical preparation jobs this
	// loop queues stay available and observable instead of being worked.
	capabilities := readyGerman()
	opdsService := opds.NewService(store, epub.NewService(store), catalogue.Client())
	syncDeps := cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Statuses: store, Pool: pool}
	workers := river.NewWorkers()
	cataloguesync.AddWorker(workers, syncDeps, opdsService, capabilities)
	analysis.AddEPUBAcquisitionWorker(workers, nil, nil, nil)
	syncClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{cataloguesync.Queue: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "catalog loop sync client", syncClient.Stop)
	syncService := cataloguesync.NewService(syncDeps, syncClient, opdsService, capabilities)
	h.services.CatalogueSync = syncService
	h.services.EPUBAcquisition = analysis.NewService(pool, syncClient)
	require.NoError(t, syncClient.Start(ctx))

	// 1. Connect through the Manage sources route, then run a real sync so the
	// Catalog index is populated by the worker, never seeded directly.
	connected := perform(t, h, http.MethodPost, "/catalogs/sources", url.Values{
		"csrf_token": {csrf}, "name": {"Loop catalog"}, "url": {catalogue.URL + "/opds"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, connected.Code, connected.Body.String())
	assert.Contains(t, connected.Header().Get("Location"), url.QueryEscape("Catalog added"))
	connections, err := store.ListOpdsConnections(ctx, learner.ID)
	require.NoError(t, err)
	require.Len(t, connections, 1)
	connection := connections[0]

	synced := perform(t, h, http.MethodPost, "/catalogs/sources/"+connection.ID+"/sync", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, synced.Code, synced.Body.String())
	assert.Contains(t, synced.Header().Get("Location"), url.QueryEscape("Catalog sync submitted."))
	testutil.Eventually(t, 60*time.Second, "catalogue sync", func() (bool, string) {
		status, statusErr := store.GetCatalogueSyncStatus(ctx, learner.ID, connection.ID)
		if statusErr != nil || status.State != domain.CatalogueSyncSynced {
			return false, fmt.Sprintf("sync status=%+v err=%v", status, statusErr)
		}
		if _, entryErr := store.GetCatalogueEntry(ctx, learner.ID, connection.ID, "homer-loop"); entryErr != nil {
			return false, fmt.Sprintf("catalog entry err=%v", entryErr)
		}
		return true, ""
	})

	// 2. One search finds the synced OPDS entry and the classical entry.
	search := perform(t, h, http.MethodGet, "/catalogs?q=Homer", nil, cookies)
	require.Equal(t, http.StatusOK, search.Code, search.Body.String())
	assert.Contains(t, search.Body.String(), "Homer Loop", "one search finds the OPDS entry the sync indexed")
	assert.Contains(t, search.Body.String(), "Iliad", "one search finds the classical entry")

	// 3. Add both entries. The OPDS Book queues one durable EPUB acquisition;
	// the classical Book queues one preparation.
	addedOPDS := perform(t, h, http.MethodPost, "/catalogs/sources/"+connection.ID+"/add", url.Values{
		"csrf_token": {csrf}, "entry_identifier": {"homer-loop"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, addedOPDS.Code, addedOPDS.Body.String())
	assert.Contains(t, addedOPDS.Header().Get("Location"), url.QueryEscape("Downloading and analyzing the text."), "Add confirms the queued EPUB preparation")
	opdsBooks, err := store.ListMyBooksBrowse(ctx, learner.ID, "Homer Loop", "de", string(domain.BookDispositionToRead), false, 0, 10)
	require.NoError(t, err)
	require.Len(t, opdsBooks.Items, 1)
	opdsBookID := opdsBooks.Items[0].Book.ID
	assert.Equal(t, domain.BookDispositionToRead, opdsBooks.Items[0].Disposition)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'book_id'=$3`,
		analysis.EPUBAcquisitionQueue, learner.ID, opdsBookID), "the OPDS Add queues exactly one durable EPUB acquisition")

	addedClassical := perform(t, h, http.MethodPost, "/catalogs/classical/units/homer-iliad-1/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, addedClassical.Code, addedClassical.Body.String())
	classicalBook, err := store.GetBookDetail(ctx, learner.ID, booksByUnitKey(t, ctx, store, learner.ID, "homer-iliad-1"))
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, classicalBook.Disposition)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'book_id'=$2`,
		analysis.ClassicalPreparationQueue, classicalBook.Book.ID), "classical Add queues its preparation")

	// 4. Remove ends membership only; Add restores the same Book ID, and the
	// still-live acquisition is not duplicated.
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
	assert.Contains(t, readded.Header().Get("Location"), url.QueryEscape("Downloading and analyzing the text."),
		"the restored Book reports the same plain preparation confirmation")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'book_id'=$3`,
		analysis.EPUBAcquisitionQueue, learner.ID, opdsBookID), "a second Add while the acquisition is live enqueues nothing (one live job per Book)")

	// 5. "I've read this" records Read without acquiring anything; each Book
	// is then listed on its own language's tab.
	previouslyRead := perform(t, h, http.MethodPost, "/catalogs/classical/units/homer-iliad-1/previously-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, previouslyRead.Code, previouslyRead.Body.String())
	readBook, err := store.GetBookDetail(ctx, learner.ID, classicalBook.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.MyBookBucketRead, readBook.WorkflowBucket())

	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	grcRead := perform(t, h, http.MethodGet, "/library?history=read", nil, cookies)
	require.Equal(t, http.StatusOK, grcRead.Code, grcRead.Body.String())
	assert.Contains(t, grcRead.Body.String(), "Iliad", "the classical Book is listed on the Read tab")
	assert.Contains(t, grcRead.Body.String(), "Read before Mouseion", "the Read tab shows the Previously read completion")
	assert.Contains(t, grcRead.Body.String(), "Read (1)")
	grcToRead := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, grcToRead.Code, grcToRead.Body.String())
	assert.NotContains(t, grcToRead.Body.String(), "Iliad", "the classical Book is absent from the To Read tab")

	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
	deToRead := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, deToRead.Code, deToRead.Body.String())
	assert.Contains(t, deToRead.Body.String(), "Homer Loop", "the OPDS Book is listed on the German To Read tab")

	// 6. My Books exposes exactly the two tabs, and the retired scopes stay
	// unreachable.
	library := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, library.Body.String())
	body := library.Body.String()
	tabs := catalogLoopTabLinks(t, body)
	require.Len(t, tabs, 2, "My Books has exactly the To Read and Read tabs")
	assert.Equal(t, "/library?disposition=to_read", tabs[0][0])
	assert.True(t, strings.HasPrefix(tabs[0][1], "To Read ("), "first tab label is %q", tabs[0][1])
	assert.Equal(t, "/library?history=read", tabs[1][0])
	assert.True(t, strings.HasPrefix(tabs[1][1], "Read ("), "second tab label is %q", tabs[1][1])
	assert.NotContains(t, body, "disposition=inbox", "no Inbox filter survives")
	assert.NotContains(t, body, "needs-language", "no needs-language strip survives")
	assert.Contains(t, body, "Homer Loop", "the German To Read row stays listed")
	retired := perform(t, h, http.MethodGet, "/library?needs-language", nil, cookies)
	assert.Equal(t, http.StatusNotFound, retired.Code)
	inbox := perform(t, h, http.MethodGet, "/library?disposition=inbox", nil, cookies)
	assert.Equal(t, http.StatusNotFound, inbox.Code)
}

// catalogLoopTabNavPattern isolates the My Books tabs nav, whose two <a> links
// are the tab surface the two-tab model must keep exact.
var (
	catalogLoopTabNavPattern  = regexp.MustCompile(`(?s)<nav class="library-filters"[^>]*>(.*?)</nav>`)
	catalogLoopTabLinkPattern = regexp.MustCompile(`<a[^>]*href="([^"]+)"[^>]*>([^<]+)</a>`)
)

// catalogLoopTabLinks returns each tab link's href and label from the page.
func catalogLoopTabLinks(t *testing.T, body string) [][2]string {
	t.Helper()
	nav := catalogLoopTabNavPattern.FindStringSubmatch(body)
	if nav == nil {
		t.Fatalf("no My Books tabs nav in body:\n%s", body)
	}
	matches := catalogLoopTabLinkPattern.FindAllStringSubmatch(nav[1], -1)
	tabs := make([][2]string, 0, len(matches))
	for _, match := range matches {
		tabs = append(tabs, [2]string{match[1], match[2]})
	}
	return tabs
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
