package opds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
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
		_, _ = w.Write([]byte("epub"))
	}))
	defer server.Close()
	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL, Username: "alice", Password: "encrypted-round-trip", Language: "de"}}
	importer := &importerStub{}
	service := NewService(store, importer, server.Client())
	result, err := service.Acquire(context.Background(), "owner-a", "connection", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	if store.owner != "owner-a" || store.id != "connection" || importer.owner != "owner-a" || importer.language != "de" || string(importer.content) != "epub" || result.Source.OwnerID != "owner-a" {
		t.Fatalf("connection owner=%q id=%q import=%+v result=%+v", store.owner, store.id, importer, result)
	}
	if _, err = service.Acquire(context.Background(), "", "connection", Entry{}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("unauthenticated acquire error=%v", err)
	}
}

func TestAcquireRejectsEntryWithoutEPUB(t *testing.T) {
	store := &connectionStoreStub{connection: domain.OpdsConnection{Language: "de"}}
	_, err := NewService(store, &importerStub{}, nil).Acquire(context.Background(), "owner", "connection", Entry{})
	if !errors.Is(err, ErrNoEPUB) {
		t.Fatalf("error=%v", err)
	}
}

func TestBrowseLanguageFiltersNonEPUBFormats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/opds/language/3" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><entry><id>pdf</id><title>PDF</title><link rel="http://opds-spec.org/acquisition" type="application/pdf" href="/book.pdf"/></entry><entry><id>epub</id><title>EPUB</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/book.epub"/></entry></feed>`))
	}))
	defer server.Close()
	store := &connectionStoreStub{connection: domain.OpdsConnection{URL: server.URL + "/opds"}}
	feed, err := NewService(store, &importerStub{}, server.Client()).BrowseLanguage(context.Background(), "owner", "connection", "3")
	if err != nil || len(feed.Entries) != 1 || feed.Entries[0].ID != "epub" {
		t.Fatalf("feed=%+v err=%v", feed, err)
	}
}
