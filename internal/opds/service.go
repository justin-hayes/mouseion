package opds

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
)

var ErrUnauthenticated = errors.New("opds: authenticated user is required")

type ConnectionStore interface {
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
}
type Importer interface {
	Import(context.Context, string, string, []byte) (epub.ImportResult, error)
}
type Service struct {
	store    ConnectionStore
	importer Importer
	http     *http.Client
}

func NewService(store ConnectionStore, importer Importer, httpClient *http.Client) *Service {
	return &Service{store: store, importer: importer, http: httpClient}
}
func (s *Service) Browse(ctx context.Context, ownerID, connectionID, feedURL string) (Feed, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return Feed{}, err
	}
	if feedURL == "" {
		feedURL = connection.URL
	}
	return client.List(ctx, feedURL)
}
func (s *Service) Languages(ctx context.Context, ownerID, connectionID string) (Feed, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return Feed{}, err
	}
	return client.ListLanguages(ctx, connection.URL)
}
func (s *Service) BrowseLanguage(ctx context.Context, ownerID, connectionID, languageID string) (Feed, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return Feed{}, err
	}
	feed, err := client.ListLanguage(ctx, connection.URL, languageID)
	if err != nil {
		return Feed{}, err
	}
	return FilterEPUBEntries(feed), nil
}
func (s *Service) Search(ctx context.Context, ownerID, connectionID, query string) (Feed, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return Feed{}, err
	}
	return client.Search(ctx, connection.URL, query)
}
func (s *Service) Acquire(ctx context.Context, ownerID, connectionID string, entry Entry) (epub.ImportResult, error) {
	if ownerID == "" {
		return epub.ImportResult{}, ErrUnauthenticated
	}
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return epub.ImportResult{}, err
	}
	links := FindEPUBs(entry)
	if len(links) == 0 {
		return epub.ImportResult{}, ErrNoEPUB
	}
	content, err := client.Download(ctx, links[0].Href)
	if err != nil {
		return epub.ImportResult{}, err
	}
	result, err := s.importer.Import(ctx, ownerID, connection.Language, content)
	if err != nil {
		return epub.ImportResult{}, fmt.Errorf("opds: ingest downloaded EPUB: %w", err)
	}
	return result, nil
}
func (s *Service) client(ctx context.Context, ownerID, connectionID string) (domain.OpdsConnection, *Client, error) {
	if ownerID == "" {
		return domain.OpdsConnection{}, nil, ErrUnauthenticated
	}
	connection, err := s.store.GetOpdsConnection(ctx, ownerID, connectionID)
	if err != nil {
		return domain.OpdsConnection{}, nil, fmt.Errorf("opds: load connection: %w", err)
	}
	client := NewClient(s.http, Auth{Username: connection.Username, Password: connection.Password, Origin: connection.URL})
	return connection, client, nil
}
