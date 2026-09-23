package opds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
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

func TestAcquireUsesOwnerScopedConnectionAndPrivateImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "alice" || password != "encrypted-round-trip" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.String(t, w, "epub")
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
	assert.Equal(t, "epub", string(importer.content))
	assert.Equal(t, "owner-a", result.Source.OwnerID)
	_, err = service.Acquire(context.Background(), "", "connection", "de", Entry{})
	assert.ErrorIs(t, err, ErrUnauthenticated)
}

func TestAcquireResolvesRelativeEPUBLinkAgainstCatalogOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/book.epub", r.URL.Path)
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.String(t, w, "epub")
	}))
	defer server.Close()
	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL + "/opds"}}
	importer := &importerStub{}
	_, err := NewService(store, importer, server.Client()).Acquire(context.Background(), "owner", "connection", "de", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: "/book.epub"}}})
	require.NoError(t, err)
	assert.Equal(t, "epub", string(importer.content))
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
	assert.Error(t, err, "external browse target accepted") //nolint:testifylint // Error and request-side-effect checks are independent.
	assert.False(t, requested, "external browse target reached HTTP client")
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
	assert.Error(t, err, "external eager pagination target was followed") //nolint:testifylint // Error and request-side-effect checks are independent.
	assert.False(t, externalHit, "external eager pagination target was reached")
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
