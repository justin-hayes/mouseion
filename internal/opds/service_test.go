package opds

import (
	"bytes"
	"context"
	"errors"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type connectionStoreStub struct {
	connection domain.OpdsConnection
	owner, id  string
}

func (s *connectionStoreStub) GetOpdsConnection(_ context.Context, owner, id string) (domain.OpdsConnection, error) {
	s.owner, s.id = owner, id
	return s.connection, nil
}

type importerStub struct {
	owner, language string
	content         []byte
}

func (s *importerStub) Import(_ context.Context, owner, language string, content []byte) (epub.ImportResult, error) {
	s.owner, s.language, s.content = owner, language, content
	return epub.ImportResult{Source: domain.SourceMaterial{OwnerID: owner}}, nil
}

type acquisitionImporterStub struct {
	importerStub
	acquisition epub.AcquisitionContent
	err         error
}

func (s *acquisitionImporterStub) ImportForAcquisition(_ context.Context, owner, language string, acquisition epub.AcquisitionContent) (epub.ImportResult, error) {
	s.owner, s.language, s.acquisition = owner, language, acquisition
	if s.err != nil {
		return epub.ImportResult{}, s.err
	}
	return epub.ImportResult{Source: domain.SourceMaterial{OwnerID: owner}}, nil
}

// catalogEPUB is a clean fixture with nothing to remove, so its Retained EPUB
// is byte-identical to the Catalog file.
func catalogEPUB(t *testing.T) []byte {
	t.Helper()
	return testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "epub2-clean"))
}

func illustratedEPUB(t *testing.T) []byte {
	t.Helper()
	return testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "illustrated"))
}

func TestAcquireUsesOwnerScopedConnectionAndPrivateImport(t *testing.T) {
	catalog := catalogEPUB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "alice" || password != "encrypted-round-trip" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.Bytes(t, w, catalog)
	}))
	defer server.Close()
	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL, Username: "alice", Password: "encrypted-round-trip"}}
	importer := &importerStub{}
	service := NewService(store, importer, server.Client())
	result, err := service.Acquire(context.Background(), "owner-a", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	require.NoError(t, err)
	assert.Equal(t, "owner-a", store.owner)
	assert.Equal(t, "connection", store.id)
	assert.Equal(t, "owner-a", importer.owner)
	assert.Equal(t, "de", importer.language)
	assert.Equal(t, catalog, importer.content, "a clean Catalog file must be retained byte-for-byte")
	assert.Equal(t, "owner-a", result.Source.OwnerID)
	_, err = service.Acquire(context.Background(), "", "connection", "de", Entry{})
	assert.ErrorIs(t, err, ErrUnauthenticated)
}

func TestAcquireResolvesRelativeEPUBLinkAgainstCatalogOrigin(t *testing.T) {
	catalog := catalogEPUB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/book.epub", r.URL.Path)
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.Bytes(t, w, catalog)
	}))
	defer server.Close()
	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL + "/opds"}}
	importer := &importerStub{}
	_, err := NewService(store, importer, server.Client()).Acquire(context.Background(), "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: "/book.epub"}}})
	require.NoError(t, err)
	assert.Equal(t, catalog, importer.content)
}

func TestFetchCoverResolvesRelativeTargetWithOwnerCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/covers/book.jpg", r.URL.Path)
		user, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "reader", user)
		assert.Equal(t, "secret", password)
		w.Header().Set("Content-Type", "image/jpeg")
		testwrite.String(t, w, "cover")
	}))
	defer server.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{
		URL:      server.URL + "/opds",
		Username: "reader",
		Password: "secret",
	}}
	data, contentType, err := NewService(store, &importerStub{}, server.Client()).FetchCover(context.Background(), "owner", "connection", "/covers/book.jpg")
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", contentType)
	assert.Equal(t, "cover", string(data))
	assert.Equal(t, "owner", store.owner)
	assert.Equal(t, "connection", store.id)
}

func TestFetchCoverRejectsTargetOutsideOwnerCatalogOrigin(t *testing.T) {
	externalHit := false
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		externalHit = true
		testwrite.String(t, w, "must not be fetched")
	}))
	defer external.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: "https://catalog.example/opds"}}
	service := NewService(store, &importerStub{}, external.Client())
	_, _, err := service.FetchCover(context.Background(), "owner", "connection", external.URL+"/cover.jpg")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsafeTarget)
	assert.False(t, externalHit)
}

func TestAcquireRejectsEntryWithoutEPUB(t *testing.T) {
	store := &connectionStoreStub{}
	_, err := NewService(store, &importerStub{}, nil).Acquire(context.Background(), "owner", "connection", "de", Entry{})
	assert.ErrorIs(t, err, ErrNoEPUB)
}

func TestBrowseLanguageFiltersNonEPUBFormats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/opds/language/3", r.URL.Path)
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><entry><id>pdf</id><title>PDF</title><link rel="http://opds-spec.org/acquisition" type="application/pdf" href="/book.pdf"/></entry><entry><id>epub</id><title>EPUB</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/book.epub"/></entry></feed>`)
	}))
	defer server.Close()
	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL + "/opds"}}
	feed, err := NewService(store, &importerStub{}, server.Client()).BrowseLanguage(context.Background(), "owner", "connection", "3")
	require.NoError(t, err)
	require.Len(t, feed.Entries, 1)
	assert.Equal(t, "epub", feed.Entries[0].ID)
}

func TestBrowsePageRejectsTargetOutsideOwnerCatalogOrigin(t *testing.T) {
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requested = true
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"/>`)
	}))
	defer server.Close()
	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL + "/opds"}}
	service := NewService(store, &importerStub{}, server.Client())
	_, err := service.BrowsePage(context.Background(), "owner", "connection", "https://evil.example/metadata")
	t.Run("external target is rejected", func(t *testing.T) {
		require.Error(t, err, "external browse target accepted")
	})
	t.Run("HTTP client is not reached", func(t *testing.T) {
		require.False(t, requested, "external browse target reached HTTP client")
	})
}

func TestBrowseRejectsExternalEagerPaginationTarget(t *testing.T) {
	var externalHit bool
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		externalHit = true
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"/>`)
	}))
	defer external.Close()

	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><link rel="next" href="`+external.URL+`/next"/></feed>`)
	}))
	defer catalog.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: catalog.URL}}
	service := NewService(store, &importerStub{}, catalog.Client())
	_, err := service.Browse(context.Background(), "owner", "connection", "")
	t.Run("external target is rejected", func(t *testing.T) {
		require.Error(t, err, "external eager pagination target was followed")
	})
	t.Run("external target is not reached", func(t *testing.T) {
		require.False(t, externalHit, "external eager pagination target was reached")
	})
}

func TestAcquireStreamsTheCatalogFileThroughTheConfiguredDirectory(t *testing.T) {
	catalog := catalogEPUB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.Bytes(t, w, catalog)
	}))
	defer server.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL}}
	importer := &acquisitionImporterStub{}
	dir := t.TempDir()
	service := NewService(store, importer, server.Client()).WithAcquisitionTempDir(dir)
	_, err := service.Acquire(t.Context(), "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	require.NoError(t, err)
	t.Run("retains a clean Catalog file byte-for-byte", func(t *testing.T) {
		require.Equal(t, catalog, importer.acquisition.Content)
	})
	t.Run("records the Catalog file digest and size", func(t *testing.T) {
		require.Equal(t, domain.EPUBContentDigest(catalog), importer.acquisition.CatalogDigest)
		require.Equal(t, int64(len(catalog)), importer.acquisition.CatalogSize)
	})
	t.Run("records the slimming version", func(t *testing.T) {
		require.Equal(t, epub.RetainedSlimmingVersion, importer.acquisition.SlimmingVersion)
	})
	t.Run("removes the temporary Catalog file", func(t *testing.T) {
		assertNoTemporaryFiles(t, dir)
	})
}

func TestAcquireRemovesTheTemporaryFileAfterIngestFailure(t *testing.T) {
	catalog := catalogEPUB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.Bytes(t, w, catalog)
	}))
	defer server.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL}}
	importer := &acquisitionImporterStub{err: errors.New("ingest failed")}
	dir := t.TempDir()
	service := NewService(store, importer, server.Client()).WithAcquisitionTempDir(dir)
	_, err := service.Acquire(t.Context(), "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	require.Error(t, err)
	assertNoTemporaryFiles(t, dir)
}

func TestAcquireAcceptsACatalogFileOverTheRetainedLimitWhenSlimmingShrinksIt(t *testing.T) {
	catalog := illustratedEPUB(t)
	retained, err := epub.Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
	require.NoError(t, err)
	require.Less(t, len(retained.Content), len(catalog), "the illustrated fixture must slim down")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.Bytes(t, w, catalog)
	}))
	defer server.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL}}
	importer := &acquisitionImporterStub{}
	dir := t.TempDir()
	service := NewService(store, importer, server.Client()).WithAcquisitionTempDir(dir)
	service.retainedLimit = int64(len(catalog) - 1)
	_, err = service.Acquire(t.Context(), "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	require.NoError(t, err)
	t.Run("imports the Retained EPUB", func(t *testing.T) {
		require.Equal(t, retained.Content, importer.acquisition.Content)
	})
	t.Run("removes the temporary Catalog file", func(t *testing.T) {
		assertNoTemporaryFiles(t, dir)
	})
}

func TestAcquireRejectsARetainedEPUBOverTheLimit(t *testing.T) {
	catalog := catalogEPUB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.Bytes(t, w, catalog)
	}))
	defer server.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL}}
	importer := &acquisitionImporterStub{}
	dir := t.TempDir()
	service := NewService(store, importer, server.Client()).WithAcquisitionTempDir(dir)
	service.retainedLimit = int64(len(catalog) - 1)
	_, err := service.Acquire(t.Context(), "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	t.Run("returns the retained-limit error", func(t *testing.T) {
		require.ErrorIs(t, err, ErrRetainedEPUBTooLarge)
	})
	t.Run("does not import an over-limit EPUB", func(t *testing.T) {
		require.Empty(t, importer.acquisition.Content)
	})
	t.Run("removes the temporary Catalog file", func(t *testing.T) {
		assertNoTemporaryFiles(t, dir)
	})
}

func TestAcquireRejectsAnInvalidEPUB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.String(t, w, "not an EPUB")
	}))
	defer server.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL}}
	importer := &acquisitionImporterStub{}
	dir := t.TempDir()
	service := NewService(store, importer, server.Client()).WithAcquisitionTempDir(dir)
	_, err := service.Acquire(t.Context(), "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	t.Run("returns the invalid-EPUB error", func(t *testing.T) {
		require.ErrorIs(t, err, epub.ErrInvalidEPUB)
	})
	t.Run("does not import anything", func(t *testing.T) {
		require.Empty(t, importer.acquisition.Content)
	})
	t.Run("removes the temporary Catalog file", func(t *testing.T) {
		assertNoTemporaryFiles(t, dir)
	})
}

func TestRetainCatalogFileReportsMissingAndInvalidFiles(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := retainCatalogFile(CatalogDownload{Path: filepath.Join(t.TempDir(), "missing")}, math.MaxInt64)
		require.ErrorContains(t, err, "retain downloaded EPUB")
	})
	t.Run("invalid EPUB", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid.epub")
		require.NoError(t, os.WriteFile(path, []byte("not an EPUB"), 0o600))
		_, err := retainCatalogFile(CatalogDownload{Path: path, Size: int64(len("not an EPUB"))}, math.MaxInt64)
		require.ErrorIs(t, err, epub.ErrInvalidEPUB)
	})
}

func TestAcquireFinishesAfterTheRequestContextIsCancelled(t *testing.T) {
	catalog := catalogEPUB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		time.Sleep(30 * time.Millisecond)
		testwrite.Bytes(t, w, catalog)
	}))
	defer server.Close()

	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL}}
	importer := &acquisitionImporterStub{}
	service := NewService(store, importer, server.Client()).WithAcquisitionTempDir(t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	_, err := service.Acquire(ctx, "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	require.NoError(t, err, "acquisition stopped when the page request ended")
	assert.Equal(t, catalog, importer.acquisition.Content)
}

func TestRemoveDownloadedCatalogFileToleratesMissingAndFailedPaths(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	removeDownloadedCatalogFile("")
	assert.Empty(t, logs.String(), "an empty path is not a cleanup failure")

	removeDownloadedCatalogFile(filepath.Join(t.TempDir(), "already-removed"))
	assert.Empty(t, logs.String(), "a missing temporary file is not a cleanup failure")

	occupied := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(occupied, "file"), []byte("x"), 0o600))
	removeDownloadedCatalogFile(occupied)
	assert.Contains(t, logs.String(), "could not remove downloaded catalog file")
}

func TestAcquireRejectsTargetOutsideOwnerCatalogOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testwrite.String(t, w, "must not be fetched")
	}))
	defer server.Close()
	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: "https://catalog.example/opds"}}
	service := NewService(store, &importerStub{}, server.Client())
	_, err := service.Acquire(context.Background(), "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL + "/book.epub"}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog target")
}

func TestCatalogURLSchemesAreCaseInsensitiveAndNormalized(t *testing.T) {
	for _, raw := range []string{"HTTP://catalog.example/opds", "HTTPS://catalog.example/opds"} {
		got, err := NormalizeCatalogURL(raw)
		require.NoError(t, err)
		assert.False(t, strings.HasPrefix(got, "HTTP:") || strings.HasPrefix(got, "HTTPS:"), "NormalizeCatalogURL(%q) retained uppercase scheme: %q", raw, got)
		_, err = resolveCatalogTarget(raw, got)
		require.NoError(t, err)
	}
}
