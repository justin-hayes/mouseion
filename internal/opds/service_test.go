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
	owner      string
}

func (s *connectionStoreStub) GetOpdsConnection(_ context.Context, owner, _ string) (domain.OpdsConnection, error) {
	s.owner = owner
	if owner != s.connection.OwnerID {
		return domain.OpdsConnection{}, errors.New("not found")
	}
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

func TestAcquireUsesOwnerConnectionAndPrivateImport(t *testing.T) {
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
	store := &connectionStoreStub{connection: domain.OpdsConnection{OwnerID: "owner-a", URL: server.URL, Username: "alice", Password: "encrypted-round-trip", Language: "de"}}
	importer := &importerStub{}
	service := NewService(store, importer, server.Client())
	result, err := service.Acquire(context.Background(), "owner-a", "connection", Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: server.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	if store.owner != "owner-a" || importer.owner != "owner-a" || importer.language != "de" || string(importer.content) != "epub" || result.Source.OwnerID != "owner-a" {
		t.Fatalf("store owner=%q import=%+v result=%+v", store.owner, importer, result)
	}
	if _, err = service.Acquire(context.Background(), "owner-b", "connection", Entry{}); err == nil {
		t.Fatal("other owner acquired through Alice's connection")
	}
}

func TestAcquireRejectsEntryWithoutEPUB(t *testing.T) {
	store := &connectionStoreStub{connection: domain.OpdsConnection{OwnerID: "owner", Language: "de"}}
	_, err := NewService(store, &importerStub{}, nil).Acquire(context.Background(), "owner", "connection", Entry{})
	if !errors.Is(err, ErrNoEPUB) {
		t.Fatalf("error=%v", err)
	}
}
