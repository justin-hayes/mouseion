package opds

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
)

var (
	ErrUnauthenticated   = errors.New("opds: authenticated user is required")
	ErrUnsafeTarget      = errors.New("opds: catalog target is outside the owner-scoped catalog")
	ErrInvalidCatalogURL = errors.New("opds: catalog URL must use HTTP or HTTPS")
)

type ConnectionStore interface {
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
}
type Importer interface {
	Import(context.Context, string, string, []byte) (epub.ImportResult, error)
}
type AcquisitionImporter interface {
	ImportForAcquisition(context.Context, string, string, []byte) (epub.ImportResult, error)
}
type AcquisitionImporterForBook interface {
	ImportForAcquisitionForBook(context.Context, string, string, string, []byte) (epub.ImportResult, error)
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
	feedURL, err = resolveCatalogTarget(connection.URL, feedURL)
	if err != nil {
		return Feed{}, err
	}
	return client.List(ctx, feedURL)
}

// BrowsePage fetches one page for the web UI. Browse retains eager aggregation
// for existing non-UI callers.
func (s *Service) BrowsePage(ctx context.Context, ownerID, connectionID, feedURL string) (Feed, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return Feed{}, err
	}
	if feedURL == "" {
		feedURL = connection.URL
	}
	feedURL, err = resolveCatalogTarget(connection.URL, feedURL)
	if err != nil {
		return Feed{}, err
	}
	return client.ListPage(ctx, feedURL)
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

func (s *Service) BrowseLanguagePage(ctx context.Context, ownerID, connectionID, languageID, feedURL string) (Feed, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return Feed{}, err
	}
	if feedURL == "" {
		if _, err := strconv.Atoi(languageID); err != nil {
			return Feed{}, err
		}
		feedURL = catalogEndpoint(connection.URL, "language", languageID)
	}
	feedURL, err = resolveCatalogTarget(connection.URL, feedURL)
	if err != nil {
		return Feed{}, err
	}
	feed, err := client.ListPage(ctx, feedURL)
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

func (s *Service) SearchPage(ctx context.Context, ownerID, connectionID, query, feedURL string) (Feed, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return Feed{}, err
	}
	if feedURL != "" {
		feedURL, err = resolveCatalogTarget(connection.URL, feedURL)
		if err != nil {
			return Feed{}, err
		}
		return client.ListPage(ctx, feedURL)
	}
	return client.SearchPage(ctx, connection.URL, query)
}
func (s *Service) Acquire(ctx context.Context, ownerID, connectionID, language string, entry Entry) (epub.ImportResult, error) {
	return s.acquire(ctx, ownerID, connectionID, language, "", entry)
}

// AcquireForBook carries an explicit owner-scoped My Books identity through
// the catalog download so a metadata-only entry is promoted in place.
func (s *Service) AcquireForBook(ctx context.Context, ownerID, connectionID, language, bookID string, entry Entry) (epub.ImportResult, error) {
	return s.acquire(ctx, ownerID, connectionID, language, bookID, entry)
}

func (s *Service) acquire(ctx context.Context, ownerID, connectionID, language, bookID string, entry Entry) (epub.ImportResult, error) {
	if ownerID == "" {
		return epub.ImportResult{}, ErrUnauthenticated
	}
	links := FindEPUBs(entry)
	if len(links) == 0 {
		return epub.ImportResult{}, ErrNoEPUB
	}
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return epub.ImportResult{}, err
	}
	downloadURL, err := resolveCatalogTarget(connection.URL, links[0].Href)
	if err != nil {
		return epub.ImportResult{}, err
	}
	content, err := client.Download(ctx, downloadURL)
	if err != nil {
		return epub.ImportResult{}, err
	}
	var result epub.ImportResult
	if bookID != "" {
		acquisitionImporter, ok := s.importer.(AcquisitionImporterForBook)
		if !ok {
			return epub.ImportResult{}, errors.New("opds: selected acquisition book cannot be promoted")
		}
		result, err = acquisitionImporter.ImportForAcquisitionForBook(ctx, ownerID, language, bookID, content)
	} else {
		importer := s.importer.Import
		if acquisitionImporter, ok := s.importer.(AcquisitionImporter); ok {
			importer = acquisitionImporter.ImportForAcquisition
		}
		result, err = importer(ctx, ownerID, language, content)
	}
	if err != nil {
		return epub.ImportResult{}, fmt.Errorf("opds: ingest downloaded EPUB: %w", err)
	}
	return result, nil
}

func resolveCatalogTarget(catalogURL, target string) (string, error) {
	base, baseErr := url.Parse(catalogURL)
	targetURL, targetErr := url.Parse(target)
	if baseErr != nil || targetErr != nil || base.User != nil || !validCatalogURL(base) {
		return "", ErrUnsafeTarget
	}
	if targetURL.Scheme == "" && targetURL.Host == "" {
		targetURL = base.ResolveReference(targetURL)
	}
	if !validCatalogURL(targetURL) || targetURL.User != nil || !strings.EqualFold(base.Scheme, targetURL.Scheme) || !strings.EqualFold(base.Host, targetURL.Host) {
		return "", ErrUnsafeTarget
	}
	return targetURL.String(), nil
}

func validCatalogURL(target *url.URL) bool {
	return target != nil && (strings.EqualFold(target.Scheme, "http") || strings.EqualFold(target.Scheme, "https")) && target.Host != ""
}

// NormalizeCatalogURL validates a stored or submitted catalog URL and uses a
// canonical lowercase scheme for all subsequent form and service requests.
func NormalizeCatalogURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil || !validCatalogURL(parsed) {
		return "", ErrInvalidCatalogURL
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	return parsed.String(), nil
}

func (s *Service) client(ctx context.Context, ownerID, connectionID string) (domain.OpdsConnection, *Client, error) {
	if ownerID == "" {
		return domain.OpdsConnection{}, nil, ErrUnauthenticated
	}
	connection, err := s.store.GetOpdsConnection(ctx, ownerID, connectionID)
	if err != nil {
		return domain.OpdsConnection{}, nil, fmt.Errorf("opds: load connection: %w", err)
	}
	normalizedURL, err := NormalizeCatalogURL(connection.URL)
	if err != nil {
		return domain.OpdsConnection{}, nil, err
	}
	connection.URL = normalizedURL
	if _, err := resolveCatalogTarget(connection.URL, connection.URL); err != nil {
		return domain.OpdsConnection{}, nil, err
	}
	client := NewClient(s.http, Auth{Username: connection.Username, Password: connection.Password, Origin: connection.URL})
	return connection, client, nil
}
