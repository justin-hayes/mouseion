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

func TestFetchFeedHTMLResponseYieldsActionableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><link rel="stylesheet" href="/style.css"></head><body><p>Please log in</p></body></html>`))
	}))
	defer server.Close()
	c := NewClient(nil, Auth{})
	_, err := c.List(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for HTML response")
	}
	if !strings.Contains(err.Error(), "HTML page, not an OPDS Atom feed") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestFetchFeedCleanAtomParses(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Cat</title><entry><title>Alphabetical Books</title><link href="/opds/books" type="application/atom+xml;profile=opds-catalog"/><id>/opds/books</id><updated>2026-08-21T22:16:14Z</updated><content type="text">Books</content></entry></feed>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(feed))
	}))
	defer server.Close()
	c := NewClient(nil, Auth{})
	f, err := c.List(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Title != "Cat" || len(f.Entries) != 1 || f.Entries[0].Title != "Alphabetical Books" {
		t.Fatalf("bad parse: %+v", f)
	}
}
