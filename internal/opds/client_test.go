package opds

import (
	"context"
	"errors"
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
		switch r.URL.RequestURI() {
		case "/calibre/opds/language":
			_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>Languages</title><entry><id>/opds/language/7</id><title>German</title><link rel="subsection" type="application/atom+xml" href="/calibre/opds/language/7"/></entry></feed>`))
		case "/calibre/opds/language/7":
			_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><link rel="next" href="?offset=1"/><entry><id>epub-1</id><title>EPUB one</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/one.epub"/></entry></feed>`))
		case "/calibre/opds/language/7?offset=1":
			_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><entry><id>pdf-1</id><title>PDF only</title><link rel="http://opds-spec.org/acquisition" type="application/pdf" href="/one.pdf"/></entry><entry><id>epub-2</id><title>EPUB two</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/two.epub"/></entry></feed>`))
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
	if _, err := client.Download(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	if receivedCredentials {
		t.Fatal("catalog credentials were sent to a different origin")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
