package opds

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRootPaginationAndAcquisition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "reader" || password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		if r.URL.Path == "/page2" {
			testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><entry><id>urn:2</id><title>Second</title><link rel="http://opds-spec.org/acquisition/open-access" type="application/pdf" href="/two.pdf"/></entry></feed>`)
			return
		}
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><link rel="next" href="/page2"/><entry><id>urn:1</id><title>First</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip; charset=binary" href="/one.epub"/></entry></feed>`)
	}))
	defer server.Close()

	feed, err := NewClient(server.Client(), Auth{Username: "reader", Password: "secret"}).ListRoot(context.Background(), server.URL+"/catalog")
	require.NoError(t, err)
	assert.Equal(t, "Books", feed.Title)
	require.Len(t, feed.Entries, 2)
	links := FindEPUBs(feed.Entries[0])
	require.Len(t, links, 1)
	assert.Equal(t, server.URL+"/one.epub", links[0].Href)
	assert.Empty(t, FindEPUBs(feed.Entries[1]), "PDF treated as EPUB")
}

func TestListPageParsesAtomAuthorsAndDCCreator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/"><entry><id>authors</id><title>Authors</title><author><name>First Author</name></author><author><name>Second Author</name></author></entry><entry><id>creator</id><title>Creator</title><dc:creator>DC Creator</dc:creator></entry></feed>`)
	}))
	defer server.Close()

	feed, err := NewClient(server.Client(), Auth{}).ListPage(context.Background(), server.URL)
	require.NoError(t, err)
	require.Len(t, feed.Entries, 2)
	assert.Equal(t, "First Author, Second Author", feed.Entries[0].Author)
	assert.Equal(t, "DC Creator", feed.Entries[1].Author)
}

func TestRelativePaginationPreservesCatalogQueryCredentials(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		assert.Equal(t, "catalog-secret", r.URL.Query().Get("access_token"), "access token query=%q", r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/atom+xml")
		if r.URL.Query().Get("offset") == "1" {
			testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><entry><id>two</id><title>Two</title></entry></feed>`)
			return
		}
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><link rel="next" href="?offset=1"/><entry><id>one</id><title>One</title></entry></feed>`)
	}))
	defer server.Close()

	feed, err := NewClient(server.Client(), Auth{}).List(context.Background(), server.URL+"/catalog?access_token=catalog-secret")
	require.NoError(t, err)
	assert.Len(t, feed.Entries, 2)
	require.Len(t, requests, 2)
	assert.Equal(t, "/catalog?access_token=catalog-secret&offset=1", requests[1])
}

func TestRelativeEPUBAcquisitionPreservesCatalogQueryCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "catalog-secret", r.URL.Query().Get("access_token"), "access token query=%q", r.URL.RawQuery)
		switch r.URL.Path {
		case "/catalog":
			w.Header().Set("Content-Type", "application/atom+xml")
			testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><entry><id>one</id><title>One</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="books/one.epub"/></entry></feed>`)
		case "/books/one.epub":
			w.Header().Set("Content-Type", EPUBMediaType)
			testwrite.String(t, w, "epub bytes")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.Client(), Auth{})
	feed, err := client.ListPage(context.Background(), server.URL+"/catalog?access_token=catalog-secret")
	require.NoError(t, err)
	links := FindEPUBs(feed.Entries[0])
	require.Len(t, links, 1)
	assert.Equal(t, server.URL+"/books/one.epub?access_token=catalog-secret", links[0].Href)
	data, err := downloadedBytes(t, client, links[0].Href, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "epub bytes", string(data))
}

func TestSearchViaOpenSearchDescription(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/root":
			testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><link rel="search" type="application/opensearchdescription+xml" href="/open-search.xml"/></feed>`)
		case "/open-search.xml":
			testwrite.String(t, w, `<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/"><Url type="application/atom+xml" template="`+server.URL+`/search?q={searchTerms}"/></OpenSearchDescription>`)
		case "/search":
			assert.Equal(t, "Moby Dick", r.URL.Query().Get("q"), "query=%q", r.URL.RawQuery)
			testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Results</title><entry><id>moby</id><title>Moby Dick</title></entry></feed>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	feed, err := NewClient(server.Client(), Auth{}).Search(context.Background(), server.URL+"/root", "Moby Dick")
	require.NoError(t, err)
	require.Len(t, feed.Entries, 1)
	assert.Equal(t, "moby", feed.Entries[0].ID)
}

func TestLanguageEndpointsFollowPagination(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/atom+xml")
		switch r.URL.Path {
		case "/calibre/opds/language":
			testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Languages</title><entry><id>/opds/language/7</id><title>German</title><link rel="subsection" type="application/atom+xml" href="/calibre/opds/language/7"/></entry></feed>`)
		case "/calibre/opds/language/7":
			if r.URL.Query().Get("offset") == "" {
				testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><link rel="next" href="?offset=1"/><entry><id>epub-1</id><title>EPUB one</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/one.epub"/></entry></feed>`)
			} else {
				testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><entry><id>pdf-1</id><title>PDF only</title><link rel="http://opds-spec.org/acquisition" type="application/pdf" href="/one.pdf"/></entry><entry><id>epub-2</id><title>EPUB two</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/two.epub"/></entry></feed>`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(server.Client(), Auth{})
	languages, err := client.ListLanguages(context.Background(), server.URL+"/calibre/opds?ignored=yes")
	require.NoError(t, err)
	require.Len(t, languages.Entries, 1)
	assert.Equal(t, "German", languages.Entries[0].Title)
	books, err := client.ListLanguage(context.Background(), server.URL+"/calibre/opds", "7")
	require.NoError(t, err)
	require.Len(t, books.Entries, 3)
	filtered := FilterEPUBEntries(books)
	require.Len(t, filtered.Entries, 2)
	assert.Equal(t, "epub-2", filtered.Entries[1].ID)
	assert.Len(t, requests, 3)
	_, err = client.ListLanguage(context.Background(), server.URL+"/calibre/opds", "../authors")
	assert.Error(t, err, "invalid language id accepted")
}

func TestLanguageEndpointPreservesCatalogQueryCredentials(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		assert.Equal(t, "catalog-secret", r.URL.Query().Get("access_token"), "access token query=%q", r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/atom+xml")
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Languages</title></feed>`)
	}))
	defer server.Close()

	_, err := NewClient(server.Client(), Auth{}).ListLanguages(context.Background(), server.URL+"/opds?access_token=catalog-secret&view=books")
	require.NoError(t, err)
	want := "/opds/language?access_token=catalog-secret&view=books"
	require.Len(t, requests, 1)
	assert.Equal(t, want, requests[0])
}

func TestListPageReturnsOnlyRequestedPageAndPaginationLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		if r.URL.Path == "/page-2" {
			testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><link rel="previous" href="/page-1"/><entry><id>two</id><title>Two</title></entry></feed>`)
			return
		}
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><link rel="next" href="/page-2"/><entry><id>one</id><title>One</title></entry></feed>`)
	}))
	defer server.Close()

	feed, err := NewClient(server.Client(), Auth{}).ListPage(context.Background(), server.URL+"/page-1")
	require.NoError(t, err)
	require.Len(t, feed.Entries, 1)
	assert.Equal(t, "one", feed.Entries[0].ID)
	assert.NotEmpty(t, linkByRel(feed.Links, "next"))
}

func TestSearchUnavailableIsTypedAndDiscoverable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>No search</title></feed>`)
	}))
	defer server.Close()
	client := NewClient(server.Client(), Auth{})
	feed, err := client.ListRoot(context.Background(), server.URL)
	require.NoError(t, err)
	assert.False(t, SupportsSearch(feed), "feed without a search link reported search support")
	_, err = client.Search(context.Background(), server.URL, "book")
	assert.ErrorIs(t, err, ErrSearchUnavailable)
}

func TestDownloadErrorsAndMediaType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/book":
			w.Header().Set("Content-Type", EPUBMediaType)
			testwrite.String(t, w, "epub bytes")
		case "/pdf":
			w.Header().Set("Content-Type", "application/pdf")
		case "/missing":
			http.NotFound(w, r)
		case "/too-large":
			w.Header().Set("Content-Type", EPUBMediaType)
			_, err := io.CopyN(w, zeroReader{}, 9)
			assert.NoError(t, err)
		}
	}))
	defer server.Close()
	client := NewClient(server.Client(), Auth{})
	client.downloadLimit = 8
	_, err := downloadedBytes(t, client, server.URL+"/too-large", t.TempDir())
	require.ErrorIs(t, err, ErrDownloadTooLarge)
	client.downloadLimit = defaultDownloadLimit
	data, err := downloadedBytes(t, client, server.URL+"/book", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "epub bytes", string(data))
	_, err = downloadedBytes(t, client, server.URL+"/pdf", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incompatible")
	_, err = downloadedBytes(t, client, server.URL+"/missing", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	_, err = NewClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}, Auth{}).List(context.Background(), "http://catalog.invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "offline")
}

func TestUnavailableFormat(t *testing.T) {
	assert.Empty(t, FindEPUBs(Entry{Links: []Link{{Rel: AcquisitionRel, Type: "application/pdf"}}}))
}

func TestFindCoverImagePrefersFullImageOverThumbnail(t *testing.T) {
	entry := Entry{Links: []Link{
		{Rel: CoverThumbnailRel, Href: "/cover-small.jpg"},
		{Rel: CoverImageRel, Href: "/cover.jpg"},
	}}

	cover := FindCoverImage(entry)
	require.NotNil(t, cover)
	assert.Equal(t, "/cover.jpg", cover.Href)
	cover = FindCoverImage(Entry{Links: []Link{{Rel: CoverThumbnailRel, Href: "/cover-small.jpg"}}})
	require.NotNil(t, cover)
	assert.Equal(t, "/cover-small.jpg", cover.Href)
}

func TestDownloadImageIsUnrestrictedButBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/image":
			w.Header().Set("Content-Type", "image/jpeg; charset=binary")
			testwrite.String(t, w, "image bytes")
		case "/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			testwrite.String(t, w, "not an EPUB")
		case "/too-large":
			_, err := w.Write(make([]byte, maxImageResponse+1))
			assert.NoError(t, err)
		}
	}))
	defer server.Close()

	client := NewClient(server.Client(), Auth{})
	data, contentType, err := client.DownloadImage(context.Background(), server.URL+"/image")
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg; charset=binary", contentType)
	assert.Equal(t, "image bytes", string(data))
	data, contentType, err = client.DownloadImage(context.Background(), server.URL+"/pdf")
	require.NoError(t, err)
	assert.Equal(t, "application/pdf", contentType)
	assert.Equal(t, "not an EPUB", string(data))
	_, _, err = client.DownloadImage(context.Background(), server.URL+"/too-large")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "8 MiB")
}

func TestLanguageIDPrefersExactLanguageOverBaseLanguage(t *testing.T) {
	feed := Feed{Entries: []Entry{
		{Title: "German", Links: []Link{{Rel: "subsection", Href: "/language/7"}}},
		{Title: "de-DE", Links: []Link{{Rel: "subsection", Href: "/language/8"}}},
	}}

	assert.Equal(t, "8", LanguageID(feed, "de-DE", "German (Germany)"))
}

// TestLanguageIDMatchesISO639PeriodQualifiedGreekNames pins the ISO 639
// reference names Calibre-Web uses to title its Greek language entries, which
// qualify the language by period instead of naming it plainly.
func TestLanguageIDMatchesISO639PeriodQualifiedGreekNames(t *testing.T) {
	for _, titles := range [][2]string{
		{"Greek, Modern (1453-)", "Greek, Ancient (to 1453)"},
		{"Greek; Modern (1453-)", "Greek; Ancient (to 1453)"},
	} {
		feed := Feed{Entries: []Entry{
			{Title: titles[1], Links: []Link{{Rel: "subsection", Href: "/language/3"}}},
			{Title: titles[0], Links: []Link{{Rel: "subsection", Href: "/language/4"}}},
		}}

		assert.Equal(t, "4", LanguageID(feed, "el", "Greek"), titles[0])
		assert.Equal(t, "3", LanguageID(feed, "grc", "Ancient_Greek"), titles[1])
	}
}

// TestLanguageIDsDeriveIdentifiersFromLanguageLinks pins each way a language
// navigation entry can advertise its identifier, through the public language
// seam, so splitting identification into private steps cannot change it.
func TestLanguageIDsDeriveIdentifiersFromLanguageLinks(t *testing.T) {
	acquired := Link{Rel: AcquisitionRel, Type: EPUBMediaType, Href: "/one.epub"}
	tests := []struct {
		name  string
		entry Entry
		want  []string
	}{
		{name: "subsection link with a trailing slash", entry: Entry{Links: []Link{{Rel: "subsection", Href: "/language/7/"}}}, want: []string{"7"}},
		{name: "alternate link whose whole path is numeric", entry: Entry{Links: []Link{{Rel: "alternate", Href: "/8"}}}, want: []string{"8"}},
		{name: "atom link without an EPUB acquisition", entry: Entry{Links: []Link{{Type: "application/atom+xml", Href: "/language/9"}}}, want: []string{"9"}},
		{name: "atom link alongside an EPUB acquisition", entry: Entry{Links: []Link{{Type: "application/atom+xml", Href: "/language/10"}, acquired}}, want: []string{}},
		{name: "unparseable link target", entry: Entry{Links: []Link{{Rel: "subsection", Href: "%"}}}, want: []string{}},
		{name: "non-numeric language path", entry: Entry{Links: []Link{{Rel: "subsection", Href: "/language/german"}}}, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, LanguageIDs(Feed{Entries: []Entry{tt.entry}}))
		})
	}
}

func TestCredentialsAreScopedToCatalogOrigin(t *testing.T) {
	receivedCredentials := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, receivedCredentials = r.BasicAuth()
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.String(t, w, "epub")
	}))
	defer server.Close()
	client := NewClient(server.Client(), Auth{Username: "reader", Password: "secret", Origin: "https://catalog.example/opds"})
	_, err := client.DownloadToFile(context.Background(), server.URL, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the catalog origin")
	assert.False(t, receivedCredentials, "catalog credentials were sent to a different origin")
}

func TestRedirectsStayOnCatalogOriginAndPreserveCustomRedirectPolicy(t *testing.T) {
	var externalHit bool
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalHit = true
		_, _, ok := r.BasicAuth()
		assert.False(t, ok, "catalog credentials crossed an unapproved redirect")
		http.Error(w, "must not fetch", http.StatusForbidden)
	}))
	defer external.Close()

	var customRedirectCalled bool
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/same-redirect" {
			w.Header().Set("Location", "/same-final")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.URL.Path == "/external-redirect" {
			w.Header().Set("Location", external.URL+"/stolen")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.URL.Path == "/download-redirect" {
			w.Header().Set("Location", external.URL+"/book.epub")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.URL.Path == "/same-final" {
			assert.Equal(t, "preserved", r.Header.Get("X-Custom-Redirect"), "custom redirect behavior was not preserved")
			user, password, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "reader", user, "same-origin redirect lost credentials")
			assert.Equal(t, "secret", password, "same-origin redirect lost credentials")
			testwrite.String(t, w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title></feed>`)
			return
		}
		http.NotFound(w, r)
	}))
	defer catalog.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		customRedirectCalled = true
		req.Header.Set("X-Custom-Redirect", "preserved")
		return nil
	}}
	opdsClient := NewClient(client, Auth{Username: "reader", Password: "secret", Origin: catalog.URL})
	_, err := opdsClient.ListPage(context.Background(), catalog.URL+"/same-redirect")
	require.NoError(t, err)
	assert.True(t, customRedirectCalled, "custom CheckRedirect was not called")
	externalRedirects := []struct {
		name  string
		fetch func() error
	}{
		{"external redirect", func() error {
			_, err := opdsClient.ListPage(context.Background(), catalog.URL+"/external-redirect")
			return err
		}},
		{"external download redirect", func() error {
			_, err := opdsClient.DownloadToFile(context.Background(), catalog.URL+"/download-redirect", t.TempDir())
			return err
		}},
		{"external image redirect", func() error {
			_, _, err := opdsClient.DownloadImage(context.Background(), catalog.URL+"/download-redirect")
			return err
		}},
	}
	for _, tc := range externalRedirects {
		t.Run(tc.name+" is rejected", func(t *testing.T) {
			require.Error(t, tc.fetch(), "external redirect was followed")
		})
	}
	t.Run("external target is not fetched", func(t *testing.T) {
		require.False(t, externalHit, "external redirect target was fetched")
	})
}

func TestHTTPSRedirectCannotDowngradeCatalogRequest(t *testing.T) {
	var requests []string
	client := NewClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.String())
		return &http.Response{
			StatusCode: http.StatusFound,
			Status:     "302 Found",
			Header:     http.Header{"Location": []string{"http://catalog.example/final"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})}, Auth{Username: "reader", Password: "secret", Origin: "https://catalog.example"})
	_, err := client.ListPage(context.Background(), "https://catalog.example/redirect")
	t.Run("downgrade is rejected", func(t *testing.T) {
		require.Error(t, err, "HTTPS downgrade was followed")
	})
	t.Run("only the original request is sent", func(t *testing.T) {
		require.Len(t, requests, 1)
		require.Equal(t, "https://catalog.example/redirect", requests[0])
	})
}

func TestDiscoveredCatalogTargetsAreValidatedBeforeFetching(t *testing.T) {
	var externalHit bool
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalHit = true
		http.Error(w, "must not fetch", http.StatusForbidden)
	}))
	defer external.Close()

	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFeed := func(body string) { testwrite.String(t, w, body) }
		switch r.URL.Path {
		case "/next-root":
			writeFeed(`<feed xmlns="http://www.w3.org/2005/Atom"><link rel="next" href="` + external.URL + `/next"/></feed>`)
		case "/search-root":
			writeFeed(`<feed xmlns="http://www.w3.org/2005/Atom"><link rel="search" href="` + external.URL + `/search{?q}"/></feed>`)
		case "/description-root":
			writeFeed(`<feed xmlns="http://www.w3.org/2005/Atom"><link rel="search" type="application/opensearchdescription+xml" href="` + external.URL + `/description"/></feed>`)
		case "/expanded-root":
			writeFeed(`<feed xmlns="http://www.w3.org/2005/Atom"><link rel="search" type="application/opensearchdescription+xml" href="/description"/></feed>`)
		case "/description":
			writeFeed(`<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/"><Url type="application/atom+xml" template="` + external.URL + `/expanded?q={searchTerms}"/></OpenSearchDescription>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer catalog.Close()

	client := NewClient(catalog.Client(), Auth{})
	directClient := NewClient(catalog.Client(), Auth{Origin: catalog.URL})
	discoveredTargets := []struct {
		name  string
		fetch func() error
		// failure is the message shown when the external target is reached.
		failure string
	}{
		{"rel-next target", func() error {
			_, err := client.List(context.Background(), catalog.URL+"/next-root")
			return err
		}, "external rel-next target was followed"},
		{"direct pagination target", func() error {
			_, err := directClient.ListPage(context.Background(), external.URL+"/direct-page")
			return err
		}, "external direct pagination target was fetched"},
		{"rel-search target", func() error {
			_, err := client.Search(context.Background(), catalog.URL+"/search-root", "book")
			return err
		}, "external rel-search target was fetched"},
		{"OpenSearch description", func() error {
			_, err := client.Search(context.Background(), catalog.URL+"/description-root", "book")
			return err
		}, "external OpenSearch description was fetched"},
		{"expanded search template", func() error {
			_, err := client.Search(context.Background(), catalog.URL+"/expanded-root", "book")
			return err
		}, "external expanded search template was fetched"},
	}
	for _, tc := range discoveredTargets {
		t.Run(tc.name+" is rejected", func(t *testing.T) {
			require.Error(t, tc.fetch(), tc.failure)
		})
	}
	t.Run("external target is not reached", func(t *testing.T) {
		require.False(t, externalHit, "external discovered target was reached")
	})
}

// zeroReader streams zero bytes, so a test can send an oversized body without
// allocating it.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
