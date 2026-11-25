//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// latinServerBehaviour is how the synthetic archive server answers a request.
type latinServerBehaviour func(w http.ResponseWriter, r *http.Request, archive []byte)

// honourRange answers a Range request as Zenodo does.
func honourRange(w http.ResponseWriter, r *http.Request, archive []byte) {
	first, last, _ := strings.Cut(strings.TrimPrefix(r.Header.Get("Range"), "bytes="), "-")
	start, startErr := strconv.Atoi(first)
	end, endErr := strconv.Atoi(last)
	if startErr != nil || endErr != nil || start > end || end >= len(archive) {
		http.Error(w, "range required", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(archive)))
	w.WriteHeader(http.StatusPartialContent)
	if _, err := w.Write(archive[start : end+1]); err != nil {
		return
	}
}

// newLatinFailureHarness provisions one synthetic Latin Work served from a
// synthetic OLA archive by behaviour. tamper edits the pin the worker holds
// (not the stored definition), so a member's recorded CRC-32 or SHA-256 can
// disagree with the served bytes.
func newLatinFailureHarness(t *testing.T, behaviour *atomic.Pointer[latinServerBehaviour], requests *atomic.Int64, tamper func(*classical.SourcePin)) *classicalFailureHarness {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "classical-latin-failure-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	build := buildLatinLoopArchive(t, "ola-failure-layer")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		(*behaviour.Load())(w, r, build.archive)
	}))
	t.Cleanup(server.Close)

	definition := classical.Definition{
		Key: failureUnitKey, WorkKey: failureUnitKey, WorkTitle: "Synthetic Latin Work", WorkAuthor: "Caesar",
		OriginalTitle: "Opus Syntheticum", EditionKey: failureUnitKey + "-edition", EditionLabel: "Synthetic Latin",
		Language: classical.LanguageLatin, ConstituentBooks: 1, CitationStart: "1.pr.1", CitationEnd: "1.1a",
		WordCount: 8, Opening: "Gallia est omnis diuisa.", Source: build.source,
		InterpretationVersion: classical.InterpretationVersionOLA, Revision: 1,
	}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, []classical.Definition{definition}))
	catalog, err := classical.NewCatalog([]classical.Definition{definition})
	require.NoError(t, err)

	held := definition
	held.Source.Members = append([]classical.ArchiveMember(nil), build.source.Members...)
	tamper(&held.Source)
	layers := classical.Layers{Layers: []classical.Layer{{
		Key: "ola-failure-layer", Release: "v0.2.0", FetchKind: classical.FetchKindArchiveRange,
		ArchiveURL: server.URL + latinLoopArchive, ArchiveSize: int64(len(build.archive)), Record: "10.5281/zenodo.15183688",
		LicenceClass: classical.LicenceClassAnalysisResource, Restriction: classical.RestrictionUnclear,
		DeployerRegisterRow: "synthetic OLA layer",
	}}}
	require.NoError(t, layers.Validate())
	config := analysis.ClassicalPreparationConfig{
		Layers: layers, Definitions: []classical.Definition{held}, HTTPClient: server.Client(), TempDir: t.TempDir(),
		IdleTimeout: 5 * time.Second,
	}
	workers := river.NewWorkers()
	require.NoError(t, river.AddWorkerSafely(workers, &analysis.ClassicalPreparationWorker{
		Store: store, Writer: analysis.NewGreekImportWriter(store), Config: config,
	}))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{analysis.ClassicalPreparationQueue: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	service := analysis.NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: service, ClassicalPreparation: service, Classical: catalog,
		CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), SessionLifetime: time.Hour,
	})
	return &classicalFailureHarness{
		t: t, ctx: ctx, store: store, pool: pool, h: h, service: service, client: client,
		definition: definition,
	}
}

// TestClassicalLatinPreparationFailureMatrix records, for each way the Latin
// archive path can fail, the stage and stable code the learner's Book shows.
//
//	case                    stage   code
//	Range ignored (200)     source  PIN_MISMATCH
//	mismatched Content-Range source PIN_MISMATCH
//	member CRC-32 mismatch  source  PIN_MISMATCH
//	member SHA-256 mismatch source  PIN_MISMATCH
//	network error           fetch   SOURCE_DOWNLOAD_FAILED (retryable)
func TestClassicalLatinPreparationFailureMatrix(t *testing.T) {
	keep := func(*classical.SourcePin) {}
	cases := []struct {
		name      string
		behaviour latinServerBehaviour
		tamper    func(*classical.SourcePin)
	}{
		{"range ignored", func(w http.ResponseWriter, _ *http.Request, archive []byte) {
			if _, err := w.Write(archive); err != nil {
				return
			}
		}, keep},
		{"mismatched content range", func(w http.ResponseWriter, _ *http.Request, archive []byte) {
			w.Header().Set("Content-Range", "bytes 0-9/"+strconv.Itoa(len(archive)))
			w.WriteHeader(http.StatusPartialContent)
			if _, err := w.Write(archive[:10]); err != nil {
				return
			}
		}, keep},
		{"conllu member CRC-32 mismatch", honourRange, func(pin *classical.SourcePin) { pin.Members[0].CRC32 = "deadbeef" }},
		{"paula member CRC-32 mismatch", honourRange, func(pin *classical.SourcePin) { pin.Members[1].CRC32 = "deadbeef" }},
		{"paula member SHA-256 mismatch", honourRange, func(pin *classical.SourcePin) { pin.Members[1].SHA256 = strings.Repeat("0", 64) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var requests atomic.Int64
			var behaviour atomic.Pointer[latinServerBehaviour]
			served := c.behaviour
			behaviour.Store(&served)
			h := newLatinFailureHarness(t, &behaviour, &requests, c.tamper)
			learner, cookies, csrf := h.learner("latin-failure-" + strings.ReplaceAll(c.name, " ", "-"))
			bookID := h.addBook(learner.ID)
			moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
			require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

			h.startClient()
			record := h.waitForFailure(learner.ID, bookID)
			assert.Equal(t, domain.BookPreparationStageSource, record.Stage)
			assert.Equal(t, ancientgreek.CodePinMismatch, string(record.Code))
			assert.Contains(t, h.jobsPage(cookies).Body.String(), ancientgreek.CodePinMismatch)
			handle, err := h.submit(learner.ID, bookID)
			require.NoError(t, err)
			assert.True(t, handle.Terminal, "a pin failure is not retried")
			assert.False(t, handle.Queued)
		})
	}

	t.Run("network failure is retryable and then succeeds", func(t *testing.T) {
		var requests atomic.Int64
		var behaviour atomic.Pointer[latinServerBehaviour]
		unreachable := latinServerBehaviour(func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				return
			}
			conn, _, err := hijacker.Hijack()
			if err == nil {
				_ = conn.Close() //nolint:errcheck // dropping the connection is the failure under test
			}
		})
		behaviour.Store(&unreachable)
		h := newLatinFailureHarness(t, &behaviour, &requests, func(*classical.SourcePin) {})
		learner, cookies, csrf := h.learner("latin-failure-network")
		bookID := h.addBook(learner.ID)
		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

		h.startClient()
		record := h.waitForFailure(learner.ID, bookID)
		assert.Equal(t, domain.BookPreparationStageFetch, record.Stage)
		assert.Equal(t, ancientgreek.CodeSourceDownloadFailed, string(record.Code))
		assert.Contains(t, h.jobsPage(cookies).Body.String(), ancientgreek.CodeSourceDownloadFailed)

		recovered := latinServerBehaviour(honourRange)
		behaviour.Store(&recovered)
		handle, err := h.submit(learner.ID, bookID)
		require.NoError(t, err)
		assert.True(t, handle.Queued, "a network failure is retryable")
		h.waitForPrepared(learner.ID, bookID)
	})
}
