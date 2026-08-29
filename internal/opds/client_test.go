package opds

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListRootPaginationAndAcquisition(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "reader" || password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		if r.URL.Path == "/page2" {
			_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><entry><id>urn:2</id><title>Second</title><link rel="http://opds-spec.org/acquisition/open-access" type="application/pdf" href="/two.pdf"/></entry></feed>`))
			return
		}
		_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><link rel="next" href="/page2"/><entry><id>urn:1</id><title>First</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip; charset=binary" href="/one.epub"/></entry></feed>`))
	}))
	defer server.Close()

	feed, err := NewClient(server.Client(), Auth{Username: "reader", Password: "secret"}).ListRoot(context.Background(), server.URL+"/catalog")
	if err != nil {
		t.Fatal(err)
	}
	if feed.Title != "Books" || len(feed.Entries) != 2 {
		t.Fatalf("feed=%+v", feed)
	}
	links := FindEPUBs(feed.Entries[0])
	if len(links) != 1 || links[0].Href != server.URL+"/one.epub" {
		t.Fatalf("EPUB links=%+v", links)
	}
	if links := FindEPUBs(feed.Entries[1]); len(links) != 0 {
		t.Fatalf("PDF treated as EPUB: %+v", links)
	}
}

func TestRelativePaginationPreservesCatalogQueryCredentials(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		if r.URL.Query().Get("access_token") != "catalog-secret" {
			t.Fatalf("access token query=%q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		if r.URL.Query().Get("offset") == "1" {
			_, _ = io.WriteString(w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><entry><id>two</id><title>Two</title></entry></feed>`)
			return
		}
		_, _ = io.WriteString(w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><link rel="next" href="?offset=1"/><entry><id>one</id><title>One</title></entry></feed>`)
	}))
	defer server.Close()

	feed, err := NewClient(server.Client(), Auth{}).List(context.Background(), server.URL+"/catalog?access_token=catalog-secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Entries) != 2 {
		t.Fatalf("entries=%+v", feed.Entries)
	}
	if len(requests) != 2 || requests[1] != "/catalog?access_token=catalog-secret&offset=1" {
		t.Fatalf("pagination requests=%v", requests)
	}
}

func TestRelativeEPUBAcquisitionPreservesCatalogQueryCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != "catalog-secret" {
			t.Fatalf("access token query=%q", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/catalog":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = io.WriteString(w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><entry><id>one</id><title>One</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="books/one.epub"/></entry></feed>`)
		case "/books/one.epub":
			w.Header().Set("Content-Type", EPUBMediaType)
			_, _ = io.WriteString(w, "epub bytes")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.Client(), Auth{})
	feed, err := client.ListPage(context.Background(), server.URL+"/catalog?access_token=catalog-secret")
	if err != nil {
		t.Fatal(err)
	}
	links := FindEPUBs(feed.Entries[0])
	if len(links) != 1 || links[0].Href != server.URL+"/books/one.epub?access_token=catalog-secret" {
		t.Fatalf("EPUB links=%+v", links)
	}
	data, err := client.Download(context.Background(), links[0].Href)
	if err != nil || string(data) != "epub bytes" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestSearchViaOpenSearchDescription(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/root":
			_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><link rel="search" type="application/opensearchdescription+xml" href="/open-search.xml"/></feed>`))
		case "/open-search.xml":
			_, _ = w.Write([]byte(`<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/"><Url type="application/atom+xml" template="` + server.URL + `/search?q={searchTerms}"/></OpenSearchDescription>`))
		case "/search":
			if r.URL.Query().Get("q") != "Moby Dick" {
				t.Fatalf("query=%q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>Results</title><entry><id>moby</id><title>Moby Dick</title></entry></feed>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	feed, err := NewClient(server.Client(), Auth{}).Search(context.Background(), server.URL+"/root", "Moby Dick")
	if err != nil || len(feed.Entries) != 1 || feed.Entries[0].ID != "moby" {
		t.Fatalf("feed=%+v err=%v", feed, err)
	}
}

func TestLanguageEndpointsFollowPagination(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/atom+xml")
		switch r.URL.Path {
		case "/calibre/opds/language":
			_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>Languages</title><entry><id>/opds/language/7</id><title>German</title><link rel="subsection" type="application/atom+xml" href="/calibre/opds/language/7"/></entry></feed>`))
		case "/calibre/opds/language/7":
			if r.URL.Query().Get("offset") == "" {
				_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><link rel="next" href="?offset=1"/><entry><id>epub-1</id><title>EPUB one</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/one.epub"/></entry></feed>`))
			} else {
				_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><entry><id>pdf-1</id><title>PDF only</title><link rel="http://opds-spec.org/acquisition" type="application/pdf" href="/one.pdf"/></entry><entry><id>epub-2</id><title>EPUB two</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/two.epub"/></entry></feed>`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(server.Client(), Auth{})
	languages, err := client.ListLanguages(context.Background(), server.URL+"/calibre/opds?ignored=yes")
	if err != nil || len(languages.Entries) != 1 || languages.Entries[0].Title != "German" {
		t.Fatalf("languages=%+v err=%v", languages, err)
	}
	books, err := client.ListLanguage(context.Background(), server.URL+"/calibre/opds", "7")
	if err != nil || len(books.Entries) != 3 {
		t.Fatalf("books=%+v err=%v", books, err)
	}
	filtered := FilterEPUBEntries(books)
	if len(filtered.Entries) != 2 || filtered.Entries[1].ID != "epub-2" {
		t.Fatalf("filtered=%+v", filtered)
	}
	if len(requests) != 3 {
		t.Fatalf("requests=%v", requests)
	}
	if _, err = client.ListLanguage(context.Background(), server.URL+"/calibre/opds", "../authors"); err == nil {
		t.Fatal("invalid language id accepted")
	}
}

func TestLanguageEndpointPreservesCatalogQueryCredentials(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		if r.URL.Query().Get("access_token") != "catalog-secret" {
			t.Fatalf("access token query=%q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = io.WriteString(w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Languages</title></feed>`)
	}))
	defer server.Close()

	if _, err := NewClient(server.Client(), Auth{}).ListLanguages(context.Background(), server.URL+"/opds?access_token=catalog-secret&view=books"); err != nil {
		t.Fatal(err)
	}
	want := "/opds/language?access_token=catalog-secret&view=books"
	if len(requests) != 1 || requests[0] != want {
		t.Fatalf("language requests=%v want [%q]", requests, want)
	}
}

func TestListPageReturnsOnlyRequestedPageAndPaginationLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		if r.URL.Path == "/page-2" {
			_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><link rel="previous" href="/page-1"/><entry><id>two</id><title>Two</title></entry></feed>`))
			return
		}
		_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title><link rel="next" href="/page-2"/><entry><id>one</id><title>One</title></entry></feed>`))
	}))
	defer server.Close()

	feed, err := NewClient(server.Client(), Auth{}).ListPage(context.Background(), server.URL+"/page-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Entries) != 1 || feed.Entries[0].ID != "one" || linkByRel(feed.Links, "next") == "" {
		t.Fatalf("page=%+v", feed)
	}
}

func TestSearchUnavailableIsTypedAndDiscoverable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>No search</title></feed>`))
	}))
	defer server.Close()
	client := NewClient(server.Client(), Auth{})
	feed, err := client.ListRoot(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if SupportsSearch(feed) {
		t.Fatal("feed without a search link reported search support")
	}
	if _, err = client.Search(context.Background(), server.URL, "book"); !errors.Is(err, ErrSearchUnavailable) {
		t.Fatalf("search error=%v", err)
	}
}

func TestDownloadErrorsAndMediaType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/book":
			w.Header().Set("Content-Type", EPUBMediaType)
			_, _ = w.Write([]byte("epub bytes"))
		case "/pdf":
			w.Header().Set("Content-Type", "application/pdf")
		case "/missing":
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(server.Client(), Auth{})
	data, err := client.Download(context.Background(), server.URL+"/book")
	if err != nil || string(data) != "epub bytes" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if _, err = client.Download(context.Background(), server.URL+"/pdf"); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("PDF error=%v", err)
	}
	if _, err = client.Download(context.Background(), server.URL+"/missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("404 error=%v", err)
	}
	if _, err = NewClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}, Auth{}).List(context.Background(), "http://catalog.invalid"); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("network error=%v", err)
	}
}

func TestUnavailableFormat(t *testing.T) {
	if links := FindEPUBs(Entry{Links: []Link{{Rel: AcquisitionRel, Type: "application/pdf"}}}); len(links) != 0 {
		t.Fatal(links)
	}
}

func TestCredentialsAreScopedToCatalogOrigin(t *testing.T) {
	receivedCredentials := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, receivedCredentials = r.BasicAuth()
		w.Header().Set("Content-Type", EPUBMediaType)
		_, _ = w.Write([]byte("epub"))
	}))
	defer server.Close()
	client := NewClient(server.Client(), Auth{Username: "reader", Password: "secret", Origin: "https://catalog.example/opds"})
	if _, err := client.Download(context.Background(), server.URL); err == nil || !strings.Contains(err.Error(), "outside the catalog origin") {
		t.Fatalf("external download error=%v", err)
	}
	if receivedCredentials {
		t.Fatal("catalog credentials were sent to a different origin")
	}
}

func TestRedirectsStayOnCatalogOriginAndPreserveCustomRedirectPolicy(t *testing.T) {
	var externalHit bool
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalHit = true
		if _, _, ok := r.BasicAuth(); ok {
			t.Fatalf("catalog credentials crossed an unapproved redirect")
		}
		http.Error(w, "must not fetch", http.StatusForbidden)
	}))
	defer external.Close()

	var customRedirectCalled bool
	var catalog *httptest.Server
	catalog = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if r.Header.Get("X-Custom-Redirect") != "preserved" {
				t.Fatalf("custom redirect behavior was not preserved")
			}
			if user, password, ok := r.BasicAuth(); !ok || user != "reader" || password != "secret" {
				t.Fatalf("same-origin redirect lost credentials")
			}
			_, _ = io.WriteString(w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Books</title></feed>`)
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
	if _, err := opdsClient.ListPage(context.Background(), catalog.URL+"/same-redirect"); err != nil {
		t.Fatal(err)
	}
	if !customRedirectCalled {
		t.Fatal("custom CheckRedirect was not called")
	}
	if _, err := opdsClient.ListPage(context.Background(), catalog.URL+"/external-redirect"); err == nil {
		t.Fatal("external redirect was followed")
	}
	if _, err := opdsClient.Download(context.Background(), catalog.URL+"/download-redirect"); err == nil {
		t.Fatal("external download redirect was followed")
	}
	if externalHit {
		t.Fatal("external redirect target was fetched")
	}
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
	if _, err := client.ListPage(context.Background(), "https://catalog.example/redirect"); err == nil {
		t.Fatal("HTTPS downgrade was followed")
	}
	if len(requests) != 1 || requests[0] != "https://catalog.example/redirect" {
		t.Fatalf("requests=%v", requests)
	}
}

func TestDiscoveredCatalogTargetsAreValidatedBeforeFetching(t *testing.T) {
	var externalHit bool
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalHit = true
		http.Error(w, "must not fetch", http.StatusForbidden)
	}))
	defer external.Close()

	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFeed := func(body string) { _, _ = io.WriteString(w, body) }
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
	if _, err := client.List(context.Background(), catalog.URL+"/next-root"); err == nil {
		t.Fatal("external rel-next target was followed")
	}
	if _, err := NewClient(catalog.Client(), Auth{Origin: catalog.URL}).ListPage(context.Background(), external.URL+"/direct-page"); err == nil {
		t.Fatal("external direct pagination target was fetched")
	}
	if _, err := client.Search(context.Background(), catalog.URL+"/search-root", "book"); err == nil {
		t.Fatal("external rel-search target was fetched")
	}
	if _, err := client.Search(context.Background(), catalog.URL+"/description-root", "book"); err == nil {
		t.Fatal("external OpenSearch description was fetched")
	}
	if _, err := client.Search(context.Background(), catalog.URL+"/expanded-root", "book"); err == nil {
		t.Fatal("external expanded search template was fetched")
	}
	if externalHit {
		t.Fatal("external discovered target was reached")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
