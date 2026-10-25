package opds

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
)

const defaultRetainedLimit = int64(100 << 20)

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
	ImportForAcquisition(context.Context, string, string, epub.AcquisitionContent) (epub.ImportResult, error)
}
type AcquisitionImporterForBook interface {
	ImportForAcquisitionForBook(context.Context, string, string, string, epub.AcquisitionContent) (epub.ImportResult, error)
}
type Service struct {
	store    ConnectionStore
	importer Importer
	http     *http.Client
	// acquisitionTempDir holds Catalog files while they are acquired. Empty uses the
	// system temporary directory.
	acquisitionTempDir string
	// retainedLimit bounds the Retained EPUB. NewService fills the default;
	// tests override it to avoid streaming a real 100 MiB body.
	retainedLimit int64
}

func NewService(store ConnectionStore, importer Importer, httpClient *http.Client) *Service {
	return &Service{store: store, importer: importer, http: httpClient, retainedLimit: defaultRetainedLimit}
}

// WithAcquisitionTempDir sets where Catalog files are streamed. It returns the same
// service so startup can chain construction.
func (s *Service) WithAcquisitionTempDir(dir string) *Service {
	s.acquisitionTempDir = dir
	return s
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

// BrowseLanguageUnfiltered returns every entry in a language feed, including
// entries without an acquisition link. This is used when resolving historical
// catalogue identities rather than when selecting downloadable books.
func (s *Service) BrowseLanguageUnfiltered(ctx context.Context, ownerID, connectionID, languageID string) (Feed, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return Feed{}, err
	}
	return client.ListLanguage(ctx, connection.URL, languageID)
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

// FetchCover downloads an owner-scoped cover target from its catalog origin.
func (s *Service) FetchCover(ctx context.Context, ownerID, connectionID, target string) ([]byte, string, error) {
	connection, client, err := s.client(ctx, ownerID, connectionID)
	if err != nil {
		return nil, "", err
	}
	target, err = resolveCatalogTarget(connection.URL, target)
	if err != nil {
		return nil, "", err
	}
	return client.DownloadImage(ctx, target)
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
	// Acquisition is synchronous but outlives the page request: the response
	// deadline must not abandon a download the learner can only observe by
	// reloading My Books.
	acquisitionCtx := context.WithoutCancel(ctx)
	download, err := client.DownloadToFile(acquisitionCtx, downloadURL, s.acquisitionTempDir)
	if err != nil {
		return epub.ImportResult{}, err
	}
	defer removeDownloadedCatalogFile(download.Path)
	if download.Size > s.retainedLimit {
		return epub.ImportResult{}, ErrRetainedEPUBTooLarge
	}
	content, err := os.ReadFile(download.Path)
	if err != nil {
		return epub.ImportResult{}, fmt.Errorf("opds: read downloaded EPUB: %w", err)
	}
	acquisition := epub.AcquisitionContent{Content: content, CatalogDigest: download.Digest}
	var result epub.ImportResult
	if bookID != "" {
		acquisitionImporter, ok := s.importer.(AcquisitionImporterForBook)
		if !ok {
			return epub.ImportResult{}, errors.New("opds: selected acquisition book cannot be promoted")
		}
		result, err = acquisitionImporter.ImportForAcquisitionForBook(acquisitionCtx, ownerID, language, bookID, acquisition)
	} else if acquisitionImporter, ok := s.importer.(AcquisitionImporter); ok {
		result, err = acquisitionImporter.ImportForAcquisition(acquisitionCtx, ownerID, language, acquisition)
	} else {
		result, err = s.importer.Import(acquisitionCtx, ownerID, language, content)
	}
	if err != nil {
		return epub.ImportResult{}, fmt.Errorf("opds: ingest downloaded EPUB: %w", err)
	}
	return result, nil
}

// removeDownloadedCatalogFile keeps temporary disk use bounded. A removal
// failure must not fail an acquisition that already succeeded.
func removeDownloadedCatalogFile(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("mouseion: could not remove downloaded catalog file %s: %v", path, err)
	}
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
