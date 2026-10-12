// Package cataloguesync implements the owner-scoped metadata-only catalogue
// reconciliation path shared by explicit enqueueing and River periodic jobs.
package cataloguesync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/bookcover"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const (
	Queue                 = "catalogue_sync"
	Kind                  = "catalogue_sync"
	CoverKind             = "book_cover"
	DefaultCadence        = 24 * time.Hour
	defaultMaxAttempts    = 3
	periodicJobIDPrefix   = "catalogue_sync:"
	statusCancelledReason = "Catalog sync cancelled before completion. Retry when ready."
)

var (
	ErrNotFound           = errors.New("catalogue sync job: not found")
	ErrConnectionNotFound = errors.New("catalogue sync: alias connection not found")
)

var liveJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateRetryable,
	rivertype.JobStateScheduled,
}

// SyncArgs deliberately contains only stable owner and connection identities.
// Credentials are loaded and decrypted by the repository during Work.
type SyncArgs struct {
	OwnerID      string `json:"owner_id" river:"unique"`
	ConnectionID string `json:"connection_id" river:"unique"`
}

// CoverArgs deliberately carries no URL, credential, or image data. The
// worker reloads all mutable catalog state before it fetches anything. The
// source identifier is the owner-scoped Catalog entry identity, not a URL.
// AdvertisedAt fences completion against a newer pending source generation.
type CoverArgs struct {
	OwnerID          string    `json:"owner_id" river:"unique"`
	BookID           string    `json:"book_id" river:"unique"`
	ConnectionID     string    `json:"connection_id" river:"unique"`
	SourceIdentifier string    `json:"source_identifier" river:"unique"`
	AdvertisedAt     time.Time `json:"advertised_at" river:"unique"`
}

func (CoverArgs) Kind() string { return CoverKind }

func (SyncArgs) Kind() string { return Kind }

type Handle struct {
	ID, DisplayNumber int64
}

type Status struct {
	ID, Attempt, AttemptCount             int64
	OwnerID, ConnectionID, ConnectionName string
	State, LogicalState, Error            string
	Progress                              int
	CreatedAt, UpdatedAt                  time.Time
	FinalizedAt                           *time.Time
}

// AcquisitionTarget is the owner-scoped catalogue entry used to acquire a
// metadata-only Book. The web layer signs the Href before placing it in a
// form; callers never receive credentials from this lookup.
type AcquisitionTarget struct {
	ConnectionID string
	Language     string
	Entry        opds.Entry
	Href         string
}

// RefreshResult describes a single-book metadata refresh without implying any
// content, analysis, or deck work.
type RefreshResult struct {
	Book         domain.Book
	Updated      bool
	Created      bool
	Missing      bool
	Failed       bool
	CoverPending bool
	// CurrentReadingEnded reports that a language correction ended an
	// incompatible Current reading.
	CurrentReadingEnded bool
}

// AliasBackfillResult reports the deterministic legacy alias migration.
type AliasBackfillResult struct {
	Examined int
	Updated  int
}

// ConnectionStore contains only the owner-scoped connection lookups used by
// catalogue sync scheduling and feed access.
type ConnectionStore interface {
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
	ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error)
	OpdsConnectionExists(context.Context, string, string) (bool, error)
	ListAllOpdsConnectionIDs(context.Context) ([]domain.OpdsConnection, error)
}

// CatalogueStore contains the book and metadata reconciliation operations used
// by catalogue sync. It deliberately excludes connection and status concerns.
type CatalogueStore interface {
	GetBook(context.Context, string, string) (domain.Book, error)
	GetBookCatalogEntryAlias(context.Context, string, string) (domain.BookAlias, error)
	GetBookCatalogEntryAliasForConnection(context.Context, string, string, string) (domain.BookAlias, error)
	SyncSupportedLanguages(context.Context, []domain.SupportedLanguage) error
	ListSupportedLanguages(context.Context) ([]domain.SupportedLanguage, error)
	ReconcileCatalogueEntry(context.Context, string, string, string, string, string, string) (persistence.CatalogueEntryReconcileResult, error)
}

type coverStore interface {
	CatalogueStore
	RecordBookCoverAdvertisement(context.Context, string, string, string, string, bool) (bool, time.Time, error)
	SaveBookCover(context.Context, string, string, string, string, time.Time, string, int, int, string, []byte) error
	MarkBookCoverUnavailable(context.Context, string, string, string, string, time.Time, string) error
	GetBookCoverForRetrieval(context.Context, string, string) (domain.BookCoverRetrieval, error)
}

// CatalogueAliasStore contains the explicit legacy alias backfill operations.
type CatalogueAliasStore interface {
	ListUnscopedCatalogueEntryAliases(context.Context) ([]domain.BookAlias, error)
	SetCatalogueEntryAliasConnection(context.Context, string, string, string) error
}

// CatalogueSyncStatusStore contains the durable sync status operations.
type CatalogueSyncStatusStore interface {
	SetCatalogueSyncStatus(context.Context, domain.CatalogueSyncStatus) error
	GetCatalogueSyncStatus(context.Context, string, string) (domain.CatalogueSyncStatus, error)
	ListCatalogueSyncStatuses(context.Context, string) ([]domain.CatalogueSyncStatus, error)
}

type StoreDependencies struct {
	Connections ConnectionStore
	Catalogue   CatalogueStore
	Aliases     CatalogueAliasStore
	Statuses    CatalogueSyncStatusStore
	Pool        *pgxpool.Pool
}

type catalogueReader interface {
	Languages(context.Context, string, string) (opds.Feed, error)
	BrowseLanguage(context.Context, string, string, string) (opds.Feed, error)
	BrowseLanguageUnfiltered(context.Context, string, string, string) (opds.Feed, error)
}

type coverReader interface {
	catalogueReader
	FetchCover(context.Context, string, string, string) ([]byte, string, error)
}

type Service struct {
	connections  ConnectionStore
	catalogue    CatalogueStore
	aliases      CatalogueAliasStore
	statuses     CatalogueSyncStatusStore
	pool         *pgxpool.Pool
	client       *river.Client[pgx.Tx]
	reader       catalogueReader
	capabilities analyzer.CapabilityProvider
	periodicMu   sync.Mutex
	periodicIDs  map[string]struct{}
}

func NewService(deps StoreDependencies, client *river.Client[pgx.Tx], reader catalogueReader, capabilities analyzer.CapabilityProvider) *Service {
	return &Service{connections: deps.Connections, catalogue: deps.Catalogue, aliases: deps.Aliases, statuses: deps.Statuses, pool: deps.Pool, client: client, reader: reader, capabilities: capabilities, periodicIDs: make(map[string]struct{})}
}

// BackfillCatalogueEntryAliases resolves each legacy catalogue-entry alias
// against all language feeds of every owner connection. It deliberately does
// not guess around feed errors, missing entries, or cross-connection matches.
// The operation is explicit so an operator can run it after the expand
// migration and retry it after correcting catalogue data or credentials.
func (s *Service) BackfillCatalogueEntryAliases(ctx context.Context) (AliasBackfillResult, error) {
	if s == nil || s.aliases == nil || s.connections == nil || s.reader == nil {
		return AliasBackfillResult{}, errors.New("catalogue alias backfill service is unavailable")
	}
	aliases, err := s.aliases.ListUnscopedCatalogueEntryAliases(ctx)
	if err != nil {
		return AliasBackfillResult{}, fmt.Errorf("list unscoped catalogue aliases: %w", err)
	}
	result := AliasBackfillResult{}
	for _, alias := range aliases {
		if alias.ConnectionID != "" || alias.AliasType != domain.AliasCatalogEntry || alias.Namespace != domain.NamespaceSourceIdentifier {
			continue
		}
		result.Examined++
		connectionID, err := s.findCatalogueEntryAliasConnection(ctx, alias)
		if err != nil {
			return result, err
		}
		if err := s.aliases.SetCatalogueEntryAliasConnection(ctx, alias.OwnerID, alias.ID, connectionID); err != nil {
			return result, fmt.Errorf("assign connection %s to alias %s: %w", connectionID, alias.ID, err)
		}
		result.Updated++
	}
	return result, nil
}

// findCatalogueEntryAliasConnection derives the one owner connection whose
// feeds advertise the legacy alias value, rejecting a value that is absent
// from every feed or present in more than one connection.
func (s *Service) findCatalogueEntryAliasConnection(ctx context.Context, alias domain.BookAlias) (string, error) {
	connections, err := s.connections.ListOpdsConnections(ctx, alias.OwnerID)
	if err != nil {
		return "", fmt.Errorf("list connections for alias %s: %w", alias.ID, err)
	}
	sort.SliceStable(connections, func(i, j int) bool {
		return connections[i].ID < connections[j].ID
	})
	var matches []string
	for _, connection := range connections {
		languages, err := s.reader.Languages(ctx, alias.OwnerID, connection.ID)
		if err != nil {
			return "", fmt.Errorf("read languages for alias %s from connection %s: %w", alias.ID, connection.ID, err)
		}
		advertises, err := s.connectionAdvertisesAlias(ctx, alias, connection, languages)
		if err != nil {
			return "", err
		}
		if advertises {
			matches = append(matches, connection.ID)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("catalogue alias %s value %q is absent from every owner connection feed", alias.ID, alias.Value)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("catalogue alias %s value %q is present in %d owner connections", alias.ID, alias.Value, len(matches))
	}
	return matches[0], nil
}

// connectionAdvertisesAlias walks every language feed of one connection for
// the alias's source identifier.
func (s *Service) connectionAdvertisesAlias(ctx context.Context, alias domain.BookAlias, connection domain.OpdsConnection, languages opds.Feed) (bool, error) {
	found := false
	for _, languageID := range opds.LanguageIDs(languages) {
		feed, err := s.reader.BrowseLanguageUnfiltered(ctx, alias.OwnerID, connection.ID, languageID)
		if err != nil {
			return false, fmt.Errorf("read feed %s for alias %s from connection %s: %w", languageID, alias.ID, connection.ID, err)
		}
		for _, entry := range feed.Entries {
			if strings.TrimSpace(entry.ID) == strings.TrimSpace(alias.Value) {
				found = true
				break
			}
		}
	}
	return found, nil
}

// PeriodicJobID is stable across process restarts and unique per owner and
// connection, so removing a connection can remove exactly its schedule.
func PeriodicJobID(owner, connectionID string) string {
	return periodicJobIDPrefix + owner + ":" + connectionID
}

func insertOpts() *river.InsertOpts {
	return &river.InsertOpts{Queue: Queue, MaxAttempts: defaultMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveJobStates}}
}

// Enqueue submits the same SyncArgs used by periodic jobs. The status is moved
// to syncing only after River has accepted a viable owner-scoped job.
func (s *Service) Enqueue(ctx context.Context, owner, connectionID string) (Handle, error) {
	if s == nil || s.connections == nil || s.statuses == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(connectionID) == "" {
		return Handle{}, ErrNotFound
	}
	exists, err := s.connections.OpdsConnectionExists(ctx, owner, connectionID)
	if err != nil {
		return Handle{}, err
	}
	if !exists {
		return Handle{}, ErrNotFound
	}
	inserted, err := s.client.Insert(ctx, SyncArgs{OwnerID: owner, ConnectionID: connectionID}, insertOpts())
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue catalogue sync: %w", err)
	}
	if inserted == nil || inserted.Job == nil {
		return Handle{}, errors.New("catalogue sync enqueue returned no River job")
	}
	if err = s.statuses.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner, ConnectionID: connectionID, State: domain.CatalogueSyncSyncing}); err != nil {
		return Handle{}, fmt.Errorf("record catalogue sync status: %w", err)
	}
	return Handle{ID: inserted.Job.ID, DisplayNumber: inserted.Job.ID}, nil
}

// EnqueueCover schedules optional cover retrieval independently of metadata
// reconciliation. The River payload remains safe to inspect and replay.
func (s *Service) EnqueueCover(ctx context.Context, owner, bookID, connectionID, sourceIdentifier string, advertisedAt time.Time) (Handle, error) {
	if s == nil || s.connections == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" || strings.TrimSpace(connectionID) == "" || strings.TrimSpace(sourceIdentifier) == "" {
		return Handle{}, ErrNotFound
	}
	exists, err := s.connections.OpdsConnectionExists(ctx, owner, connectionID)
	if err != nil {
		return Handle{}, err
	}
	if !exists {
		return Handle{}, ErrNotFound
	}
	inserted, err := s.client.Insert(ctx, CoverArgs{OwnerID: owner, BookID: bookID, ConnectionID: connectionID, SourceIdentifier: sourceIdentifier, AdvertisedAt: advertisedAt}, &river.InsertOpts{
		Queue: Queue, MaxAttempts: defaultMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveJobStates},
	})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue book cover retrieval: %w", err)
	}
	if inserted == nil || inserted.Job == nil {
		return Handle{}, errors.New("book cover enqueue returned no River job")
	}
	return Handle{ID: inserted.Job.ID, DisplayNumber: inserted.Job.ID}, nil
}

// RegisterConnection adds one in-memory River periodic job. The constructor
// performs no I/O and therefore cannot leak credentials or block the leader.
func (s *Service) RegisterConnection(_ context.Context, owner, connectionID string) error {
	if s == nil || s.connections == nil || s.client == nil {
		return errors.New("catalogue sync service is unavailable")
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(connectionID) == "" {
		return errors.New("catalogue sync connection identity is incomplete")
	}
	s.periodicMu.Lock()
	defer s.periodicMu.Unlock()
	if s.periodicIDs == nil {
		s.periodicIDs = make(map[string]struct{})
	}
	_, err := s.client.PeriodicJobs().AddSafely(river.NewPeriodicJob(
		river.PeriodicInterval(DefaultCadence),
		func() (river.JobArgs, *river.InsertOpts) {
			// Constructor must never block (River scheduler contract): no I/O
			// here. The in-memory active set is the only gate; a connection
			// deleted since registration is handled by the worker's no-op
			// when it cannot load the owner-scoped connection.
			s.periodicMu.Lock()
			_, active := s.periodicIDs[PeriodicJobID(owner, connectionID)]
			s.periodicMu.Unlock()
			if !active {
				return nil, nil
			}
			return SyncArgs{OwnerID: owner, ConnectionID: connectionID}, insertOpts()
		},
		&river.PeriodicJobOpts{ID: PeriodicJobID(owner, connectionID)},
	))
	if err == nil {
		s.periodicIDs[PeriodicJobID(owner, connectionID)] = struct{}{}
	}
	return err
}

func (s *Service) UnregisterConnection(owner, connectionID string) error {
	if s == nil || s.client == nil {
		return nil
	}
	s.periodicMu.Lock()
	defer s.periodicMu.Unlock()
	delete(s.periodicIDs, PeriodicJobID(owner, connectionID))
	s.client.PeriodicJobs().RemoveByID(PeriodicJobID(owner, connectionID))
	return nil
}

// RefreshEntry reloads a catalogue-backed Book in the owner's scope, finds its
// recorded entry through the existing catalogue reader, and applies the same
// metadata-only reconciliation used by catalogue sync.
func (s *Service) RefreshEntry(ctx context.Context, owner, bookID string) (RefreshResult, error) {
	if s == nil || s.catalogue == nil || s.connections == nil || s.reader == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return RefreshResult{}, ErrNotFound
	}
	book, err := s.catalogue.GetBook(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return RefreshResult{}, ErrNotFound
	}
	if err != nil {
		return RefreshResult{Failed: true}, err
	}
	alias, err := s.catalogue.GetBookCatalogEntryAlias(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return RefreshResult{}, ErrNotFound
	}
	if err != nil {
		return RefreshResult{Book: book, Failed: true}, err
	}
	if alias.BookID != book.ID || alias.AliasType != domain.AliasCatalogEntry || alias.Namespace != domain.NamespaceSourceIdentifier || strings.TrimSpace(alias.Value) == "" {
		return RefreshResult{}, ErrNotFound
	}
	resolved, err := s.resolveRefreshEntry(ctx, owner, book, alias)
	if err != nil {
		return RefreshResult{Book: book, Failed: true}, err
	}
	if !resolved.found {
		return RefreshResult{Book: book, Missing: true}, nil
	}
	return s.applyRefreshedEntry(ctx, owner, book, resolved)
}

// applyRefreshedEntry reconciles the matched catalogue entry, which compares
// the feed metadata against the Book and persists any change, then records and
// schedules the entry's cover advertisement. It returns the metadata-only
// refresh outcome.
func (s *Service) applyRefreshedEntry(ctx context.Context, owner string, book domain.Book, resolved refreshEntry) (RefreshResult, error) {
	reconciled, err := s.catalogue.ReconcileCatalogueEntry(ctx, owner, resolved.connectionID, resolved.entry.ID, resolved.entry.Title, resolved.entry.Author, languageTag(resolved.scope))
	if err != nil {
		return RefreshResult{Book: book, Failed: true}, err
	}
	coverPending, err := s.recordAndEnqueueCover(ctx, owner, reconciled.Book.ID, resolved.connectionID, resolved.entry)
	if err != nil {
		return RefreshResult{Book: reconciled.Book, Failed: true}, err
	}
	return RefreshResult{Book: reconciled.Book, Updated: reconciled.TitleChanged || reconciled.AuthorChanged || reconciled.LanguageChanged, Created: reconciled.Created, CoverPending: coverPending, CurrentReadingEnded: reconciled.CurrentReadingEnded}, nil
}

// refreshEntry is the catalogue entry a Book refresh resolved to, together
// with the connection and language scope that produced it.
type refreshEntry struct {
	connectionID string
	entry        opds.Entry
	scope        languageScope
	found        bool
}

// resolveRefreshEntry loads the alias's connection, derives the Book's
// eligible language scopes, and walks their EPUB feeds for the recorded source
// identifier. It never trusts a job-supplied URL.
func (s *Service) resolveRefreshEntry(ctx context.Context, owner string, book domain.Book, alias domain.BookAlias) (refreshEntry, error) {
	connection, err := s.connections.GetOpdsConnection(ctx, owner, alias.ConnectionID)
	if errors.Is(err, persistence.ErrNotFound) {
		return refreshEntry{}, fmt.Errorf("%w: %s", ErrConnectionNotFound, alias.ConnectionID)
	}
	if err != nil {
		return refreshEntry{}, err
	}
	scopes, err := s.refreshLanguageScopes(ctx, owner, connection.ID, book)
	if err != nil {
		return refreshEntry{}, err
	}
	for _, scope := range scopes {
		feed, feedErr := s.reader.BrowseLanguage(ctx, owner, connection.ID, scope.languageID)
		if feedErr != nil {
			return refreshEntry{}, feedErr
		}
		for _, entry := range opds.FilterEPUBEntries(feed).Entries {
			if strings.TrimSpace(entry.ID) != alias.Value || strings.TrimSpace(entry.Title) == "" {
				continue
			}
			return refreshEntry{connectionID: connection.ID, entry: entry, scope: scope, found: true}, nil
		}
	}
	return refreshEntry{}, nil
}

// refreshLanguageScopes mirrors the sync scope: ready non-English capabilities
// when available, otherwise the Book's advertised language feed.
func (s *Service) refreshLanguageScopes(ctx context.Context, owner, connectionID string, book domain.Book) ([]languageScope, error) {
	supported, err := s.catalogue.ListSupportedLanguages(ctx)
	if err != nil {
		return nil, err
	}
	languages, err := s.reader.Languages(ctx, owner, connectionID)
	if err != nil {
		return nil, err
	}
	if s.capabilities == nil {
		displayName := supportedLanguageDisplayName(supported, book.LanguageTag)
		if languageID := opds.LanguageID(languages, book.LanguageTag, displayName); languageID != "" {
			return []languageScope{{capability: analyzer.LanguageCapability{Language: book.LanguageTag}, languageID: languageID}}, nil
		}
		return nil, nil
	}
	capabilities, err := s.capabilities.GetCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	return eligibleLanguages(languages, capabilities), nil
}

func (s *Service) recordAndEnqueueCover(ctx context.Context, owner, bookID, connectionID string, entry opds.Entry) (bool, error) {
	covers, ok := s.catalogue.(coverStore)
	if !ok {
		return false, nil
	}
	advertised := opds.FindCoverImage(entry) != nil
	retrieve, advertisedAt, err := covers.RecordBookCoverAdvertisement(ctx, owner, bookID, connectionID, entry.ID, advertised)
	if err != nil {
		return false, err
	}
	if !retrieve || s.client == nil {
		return advertised, nil
	}
	if _, err := s.EnqueueCover(ctx, owner, bookID, connectionID, entry.ID, advertisedAt); err != nil {
		return advertised, err
	}
	return advertised, nil
}

// findEntryForConnection reloads the owner-scoped connection and walks its
// eligible language feeds for the requested Catalog entry. It never trusts a
// job-supplied URL: the source identifier is resolved against live state.
func (s *Service) findEntryForConnection(ctx context.Context, owner, connectionID, sourceIdentifier string) (opds.Entry, bool, error) {
	if _, err := s.connections.GetOpdsConnection(ctx, owner, connectionID); err != nil {
		return opds.Entry{}, false, err
	}
	languages, err := s.reader.Languages(ctx, owner, connectionID)
	if err != nil {
		return opds.Entry{}, false, err
	}
	scopes, err := s.entryLanguageScopes(ctx, languages)
	if err != nil {
		return opds.Entry{}, false, err
	}
	for _, scope := range scopes {
		feed, feedErr := s.reader.BrowseLanguage(ctx, owner, connectionID, scope.languageID)
		if feedErr != nil {
			return opds.Entry{}, false, feedErr
		}
		for _, entry := range opds.FilterEPUBEntries(feed).Entries {
			if strings.TrimSpace(entry.ID) == strings.TrimSpace(sourceIdentifier) {
				return entry, true, nil
			}
		}
	}
	return opds.Entry{}, false, nil
}

// entryLanguageScopes mirrors the sync scope: ready non-English capabilities
// when available, otherwise every advertised language feed.
func (s *Service) entryLanguageScopes(ctx context.Context, languages opds.Feed) ([]languageScope, error) {
	if s.capabilities != nil {
		capabilities, err := s.capabilities.GetCapabilities(ctx)
		if err != nil {
			return nil, err
		}
		return eligibleLanguages(languages, capabilities), nil
	}
	var scopes []languageScope
	for _, id := range opds.LanguageIDs(languages) {
		scopes = append(scopes, languageScope{languageID: id})
	}
	return scopes, nil
}

// FindAcquisitionTarget resolves the current EPUB link for a synced Book.
// The alias is stable across catalog feed changes, while the download Href is
// looked up again immediately before acquisition.
func (s *Service) FindAcquisitionTarget(ctx context.Context, owner, bookID string) (AcquisitionTarget, error) {
	if s == nil || s.catalogue == nil || s.connections == nil || s.reader == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return AcquisitionTarget{}, ErrNotFound
	}
	book, alias, err := s.loadAcquisitionIdentity(ctx, owner, bookID)
	if err != nil {
		return AcquisitionTarget{}, err
	}
	connectionID, entry, href, err := s.resolveAcquisitionEntry(ctx, owner, book, alias)
	if err != nil {
		return AcquisitionTarget{}, err
	}
	return AcquisitionTarget{ConnectionID: connectionID, Language: book.LanguageTag, Entry: entry, Href: href}, nil
}

// loadAcquisitionIdentity loads the owner-scoped Book and its catalog-entry
// alias, rejecting a missing or blank source identity.
func (s *Service) loadAcquisitionIdentity(ctx context.Context, owner, bookID string) (domain.Book, domain.BookAlias, error) {
	book, err := s.catalogue.GetBook(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return domain.Book{}, domain.BookAlias{}, ErrNotFound
	}
	if err != nil {
		return domain.Book{}, domain.BookAlias{}, err
	}
	alias, err := s.catalogue.GetBookCatalogEntryAlias(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return domain.Book{}, domain.BookAlias{}, ErrNotFound
	}
	if err != nil {
		return domain.Book{}, domain.BookAlias{}, err
	}
	if strings.TrimSpace(alias.Value) == "" {
		return domain.Book{}, domain.BookAlias{}, ErrNotFound
	}
	return book, alias, nil
}

// resolveAcquisitionEntry finds the current feed entry the alias identifies,
// through the alias connection's feed for the Book's language. An entry whose
// identifier matches but that advertises no usable EPUB link is skipped, so a
// later duplicate identity can still resolve.
func (s *Service) resolveAcquisitionEntry(ctx context.Context, owner string, book domain.Book, alias domain.BookAlias) (string, opds.Entry, string, error) {
	connection, err := s.connections.GetOpdsConnection(ctx, owner, alias.ConnectionID)
	if errors.Is(err, persistence.ErrNotFound) {
		return "", opds.Entry{}, "", fmt.Errorf("%w: %s", ErrConnectionNotFound, alias.ConnectionID)
	}
	if err != nil {
		return "", opds.Entry{}, "", err
	}
	supported, err := s.catalogue.ListSupportedLanguages(ctx)
	if err != nil {
		return "", opds.Entry{}, "", err
	}
	displayName := supportedLanguageDisplayName(supported, book.LanguageTag)
	languages, err := s.reader.Languages(ctx, owner, connection.ID)
	if err != nil {
		return "", opds.Entry{}, "", ErrNotFound
	}
	languageID := opds.LanguageID(languages, book.LanguageTag, displayName)
	if languageID == "" {
		return "", opds.Entry{}, "", ErrNotFound
	}
	feed, err := s.reader.BrowseLanguage(ctx, owner, connection.ID, languageID)
	if err != nil {
		return "", opds.Entry{}, "", ErrNotFound
	}
	for _, entry := range opds.FilterEPUBEntries(feed).Entries {
		if strings.TrimSpace(entry.ID) != alias.Value {
			continue
		}
		if href, ok := acquisitionHref(entry); ok {
			return connection.ID, entry, href, nil
		}
	}
	return "", opds.Entry{}, "", ErrNotFound
}

// acquisitionHref returns the first non-blank EPUB download target advertised
// by the entry.
func acquisitionHref(entry opds.Entry) (string, bool) {
	for _, link := range opds.FindEPUBs(entry) {
		if strings.TrimSpace(link.Href) != "" {
			return link.Href, true
		}
	}
	return "", false
}

// RegisterAll restores all schedules after a process start. It intentionally
// uses the repository's owner-scoped credential path only to enumerate rows;
// no credential is copied into the periodic constructor or River args.
func (s *Service) RegisterAll(ctx context.Context) error {
	if s == nil || s.connections == nil {
		return errors.New("catalogue sync service is unavailable")
	}
	connections, err := s.connections.ListAllOpdsConnectionIDs(ctx)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if err := s.RegisterConnection(ctx, connection.OwnerID, connection.ID); err != nil {
			return err
		}
	}
	return nil
}

// ListCatalogueSyncStatuses overlays durable status with River's live state.
// A worker or process can disappear after the durable row is marked syncing;
// that row must not permanently disable the Sync now action.
func (s *Service) ListCatalogueSyncStatuses(ctx context.Context, owner string) ([]domain.CatalogueSyncStatus, error) {
	if s == nil || s.pool == nil || s.statuses == nil || strings.TrimSpace(owner) == "" {
		return nil, nil
	}
	statuses, err := s.statuses.ListCatalogueSyncStatuses(ctx, owner)
	if err != nil {
		return nil, err
	}
	for i := range statuses {
		if statuses[i].State != domain.CatalogueSyncSyncing {
			continue
		}
		var live bool
		live, err = s.liveJobExists(ctx, owner, statuses[i].ConnectionID)
		if err != nil {
			return nil, err
		}
		if live {
			continue
		}
		if statuses[i].LastSyncedAt != nil {
			statuses[i].State = domain.CatalogueSyncSynced
			statuses[i].LastError = ""
		} else {
			statuses[i].State = domain.CatalogueSyncFailed
			statuses[i].LastError = statusCancelledReason
		}
	}
	return statuses, nil
}

func (s *Service) liveJobExists(ctx context.Context, owner, connectionID string) (bool, error) {
	var live bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM river_job
		WHERE kind=$1 AND args->>'owner_id'=$2
		  AND args->>'connection_id'=$3 AND state::text=ANY($4::text[])
	)`, Kind, owner, connectionID, liveRiverStates()).Scan(&live)
	if err != nil {
		return false, fmt.Errorf("check catalogue sync job state: %w", err)
	}
	return live, nil
}

func (s *Service) reconcileDurableStatus(ctx context.Context, owner, connectionID string, state domain.CatalogueSyncState) domain.CatalogueSyncState {
	if state != domain.CatalogueSyncSyncing || s.statuses == nil {
		return state
	}
	live, err := s.liveJobExists(ctx, owner, connectionID)
	if err != nil || live {
		return state
	}
	durable, err := s.statuses.GetCatalogueSyncStatus(ctx, owner, connectionID)
	if err != nil {
		return state
	}
	if durable.LastSyncedAt != nil {
		durable.State = domain.CatalogueSyncSynced
		durable.LastError = ""
	} else {
		durable.State = domain.CatalogueSyncFailed
		durable.LastError = statusCancelledReason
	}
	if err = s.statuses.SetCatalogueSyncStatus(context.WithoutCancel(ctx), durable); err != nil {
		return state
	}
	return durable.State
}

func liveRiverStates() []string {
	states := make([]string, len(liveJobStates))
	for i, state := range liveJobStates {
		states[i] = string(state)
	}
	return states
}

func (s *Service) Get(ctx context.Context, owner string, id int64) (Status, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(owner) == "" {
		return Status{}, ErrNotFound
	}
	var status Status
	var riverState string
	var connectionID, state, durableError string
	var syncedAt *time.Time
	err := s.pool.QueryRow(ctx, `SELECT j.id,j.attempt,j.created_at,j.finalized_at,j.state,COALESCE(j.args->>'connection_id',''),COALESCE(c.name,''),COALESCE(s.state,''),COALESCE(s.last_error,''),s.last_synced_at
FROM river_job j
LEFT JOIN opds_connections c ON c.owner_id=$1::uuid AND c.id::text=j.args->>'connection_id'
LEFT JOIN catalogue_sync_status s ON s.owner_id=$1::uuid AND s.connection_id::text=j.args->>'connection_id'
	WHERE j.id=$2 AND j.kind=$3 AND j.args->>'owner_id'=$1::text`, owner, id, Kind).Scan(&status.ID, &status.Attempt, &status.CreatedAt, &status.FinalizedAt, &riverState, &connectionID, &status.ConnectionName, &state, &durableError, &syncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, ErrNotFound
	}
	if err != nil {
		return Status{}, fmt.Errorf("get catalogue sync job: %w", err)
	}
	status.OwnerID = owner
	status.ConnectionID = connectionID
	state = string(s.reconcileDurableStatus(ctx, owner, connectionID, domain.CatalogueSyncState(state)))
	if state == string(domain.CatalogueSyncFailed) && durableError == "" {
		durableError = statusCancelledReason
	}
	status.LogicalState = logicalState(riverState)
	status.State = riverState
	riverMessage, riverErr := riverError(s.pool, ctx, id, owner)
	if riverErr != nil {
		return Status{}, fmt.Errorf("get catalogue sync error: %w", riverErr)
	}
	status.Error = firstError(durableError, riverMessage)
	status.Progress = 0
	if status.LogicalState == "completed" {
		status.Progress = 100
	}
	if state == string(domain.CatalogueSyncFailed) && durableError != "" && status.LogicalState != "cancelled" {
		status.LogicalState = "failed"
	}
	if syncedAt != nil {
		status.UpdatedAt = *syncedAt
	}
	_ = state
	return status, nil
}

func (s *Service) List(ctx context.Context, owner string) ([]Status, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(owner) == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT j.id,j.attempt,j.created_at,j.finalized_at,j.state,COALESCE(j.args->>'connection_id',''),COALESCE(c.name,''),COALESCE(s.state,''),COALESCE(s.last_error,''),COALESCE(j.errors[array_length(j.errors,1)]->>'error',''),s.last_synced_at
FROM river_job j
LEFT JOIN opds_connections c ON c.owner_id=$1::uuid AND c.id::text=j.args->>'connection_id'
LEFT JOIN catalogue_sync_status s ON s.owner_id=$1::uuid AND s.connection_id::text=j.args->>'connection_id'
WHERE j.kind=$2 AND j.args->>'owner_id'=$1::text ORDER BY j.created_at DESC,j.id DESC LIMIT 100`, owner, Kind)
	if err != nil {
		return nil, fmt.Errorf("list catalogue sync jobs: %w", err)
	}
	defer rows.Close()
	var out []Status
	for rows.Next() {
		item, err := s.listStatus(ctx, owner, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// listStatus projects one joined River and durable row into the owner-facing
// status, reconciling a stale durable syncing state before it is returned.
func (s *Service) listStatus(ctx context.Context, owner string, row pgx.Row) (Status, error) {
	var item Status
	var riverState, durableState, durableError, connectionID, riverErr string
	var syncedAt *time.Time
	if err := row.Scan(&item.ID, &item.Attempt, &item.CreatedAt, &item.FinalizedAt, &riverState, &connectionID, &item.ConnectionName, &durableState, &durableError, &riverErr, &syncedAt); err != nil {
		return Status{}, err
	}
	item.OwnerID, item.ConnectionID, item.State = owner, connectionID, riverState
	durableState = string(s.reconcileDurableStatus(ctx, owner, connectionID, domain.CatalogueSyncState(durableState)))
	if durableState == string(domain.CatalogueSyncFailed) && durableError == "" {
		durableError = statusCancelledReason
	}
	item.LogicalState, item.Error = logicalState(riverState), firstError(durableError, riverErr)
	if durableState == string(domain.CatalogueSyncFailed) && durableError != "" {
		item.LogicalState = "failed"
	}
	if item.LogicalState == "completed" {
		item.Progress = 100
	}
	if syncedAt != nil {
		item.UpdatedAt = *syncedAt
	}
	return item, nil
}

func (s *Service) Retry(ctx context.Context, owner string, id int64) (Handle, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return Handle{}, err
	}
	if status.LogicalState != "failed" && status.LogicalState != "cancelled" {
		return Handle{}, errors.New("catalogue sync job is not retryable")
	}
	return s.Enqueue(ctx, owner, status.ConnectionID)
}

func (s *Service) Cancel(ctx context.Context, owner string, id int64) (Status, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return Status{}, err
	}
	if status.LogicalState != "queued" && status.LogicalState != "running" {
		return status, nil
	}
	if _, err = s.client.JobCancel(ctx, id); err != nil {
		return Status{}, fmt.Errorf("cancel catalogue sync job: %w", err)
	}
	durable, durableErr := s.statuses.GetCatalogueSyncStatus(ctx, owner, status.ConnectionID)
	if durableErr != nil && !errors.Is(durableErr, persistence.ErrNotFound) {
		return Status{}, durableErr
	}
	if durable.LastSyncedAt != nil {
		durable.State = domain.CatalogueSyncSynced
		durable.LastError = ""
	} else {
		durable = domain.CatalogueSyncStatus{OwnerID: owner, ConnectionID: status.ConnectionID, State: domain.CatalogueSyncFailed, LastError: statusCancelledReason}
	}
	if err := s.statuses.SetCatalogueSyncStatus(context.WithoutCancel(ctx), durable); err != nil {
		return Status{}, err
	}
	return s.Get(ctx, owner, id)
}

type languageScope struct {
	capability analyzer.LanguageCapability
	languageID string
}

func eligibleLanguages(languages opds.Feed, capabilities analyzer.Capabilities) []languageScope {
	var out []languageScope
	for _, capability := range capabilities.Languages {
		if !capability.Ready || isEnglish(capability.Language) {
			continue
		}
		languageID := opds.LanguageID(languages, capability.Language, capability.DisplayName)
		if languageID == "" {
			continue
		}
		out = append(out, languageScope{capability: capability, languageID: languageID})
	}
	return out
}

func sameLanguage(left, right string) bool {
	left, right = canonicalization.NormalizeLanguage(left), canonicalization.NormalizeLanguage(right)
	return left == right || baseLanguage(left) == baseLanguage(right)
}

func baseLanguage(language string) string {
	if before, _, ok := strings.Cut(language, "-"); ok {
		return before
	}
	return language
}

func isEnglish(language string) bool {
	language = canonicalization.NormalizeLanguage(language)
	return language == "en" || language == "eng" || baseLanguage(language) == "en"
}

// languageTag returns the normalized language tag for newly created
// metadata-only Books. It comes from the capability that produced the feed
// being walked; it is never inferred from content.
func languageTag(scope languageScope) string {
	return canonicalization.NormalizeLanguage(scope.capability.Language)
}

func supportedLanguageDisplayName(languages []domain.SupportedLanguage, language string) string {
	for _, supported := range languages {
		if sameLanguage(supported.Language, language) {
			return supported.DisplayName
		}
	}
	return ""
}

func logicalState(state string) string {
	switch state {
	case "running":
		return "running"
	case "completed":
		return "completed"
	case "cancelled":
		return "cancelled"
	case "discarded":
		return "failed"
	default:
		return "queued"
	}
}

func firstError(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func riverError(pool *pgxpool.Pool, ctx context.Context, id int64, owner string) (string, error) {
	var value string
	err := pool.QueryRow(ctx, `SELECT COALESCE(errors[array_length(errors,1)]->>'error','') FROM river_job WHERE id=$1 AND args->>'owner_id'=$2`, id, owner).Scan(&value)
	return value, err
}

type coverAdvertisement struct {
	bookID           string
	sourceIdentifier string
	advertised       bool
}

func (s *Service) work(ctx context.Context, args SyncArgs) (int, error) {
	connection, found, err := s.loadSyncConnection(ctx, args)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, nil
	}
	if err := s.statuses.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: args.OwnerID, ConnectionID: args.ConnectionID, State: domain.CatalogueSyncSyncing}); err != nil {
		return 0, err
	}
	scope, err := s.syncLanguageScope(ctx, args, connection)
	if err != nil {
		return 0, err
	}
	upserted, advertisements, err := s.reconcileSyncFeeds(ctx, args, connection, scope)
	if err != nil {
		return upserted, err
	}
	// Only a complete, successful traversal may change retained cover state:
	// this is the point at which explicit upstream absence is conclusive.
	s.applyCoverAdvertisements(ctx, args.OwnerID, args.ConnectionID, advertisements)
	return upserted, nil
}

// loadSyncConnection reloads the owner-scoped connection for a job. A
// connection deleted since the job was scheduled is a no-op, not a failure.
func (s *Service) loadSyncConnection(ctx context.Context, args SyncArgs) (domain.OpdsConnection, bool, error) {
	connection, err := s.connections.GetOpdsConnection(ctx, args.OwnerID, args.ConnectionID)
	if errors.Is(err, persistence.ErrNotFound) {
		return domain.OpdsConnection{}, false, nil
	}
	if err != nil {
		return domain.OpdsConnection{}, false, errors.New("catalog connection could not be loaded")
	}
	return connection, true, nil
}

// syncLanguageScope refreshes the NLP language reference and returns the
// connection's eligible language feeds for reconciliation.
func (s *Service) syncLanguageScope(ctx context.Context, args SyncArgs, connection domain.OpdsConnection) ([]languageScope, error) {
	capabilities, err := s.capabilities.GetCapabilities(ctx)
	if err != nil {
		return nil, errors.New("NLP language readiness could not be checked")
	}
	readyLanguages := analyzer.ReadySupportedLanguages(capabilities)
	if err = s.catalogue.SyncSupportedLanguages(ctx, readyLanguages); err != nil {
		return nil, errors.New("NLP language reference could not be updated")
	}
	languages, err := s.reader.Languages(ctx, args.OwnerID, args.ConnectionID)
	if err != nil {
		return nil, safeSyncError(err, connection)
	}
	return eligibleLanguages(languages, capabilities), nil
}

// reconcileSyncFeeds walks each eligible language feed, reconciling its EPUB
// entries. A feed read failure aborts immediately; entry conflicts are
// collected so the first one is reported after the traversal completes.
func (s *Service) reconcileSyncFeeds(ctx context.Context, args SyncArgs, connection domain.OpdsConnection, scope []languageScope) (int, []coverAdvertisement, error) {
	upserted := 0
	var advertisements []coverAdvertisement
	var firstConflict error
	for _, language := range scope {
		feed, err := s.reader.BrowseLanguage(ctx, args.OwnerID, args.ConnectionID, language.languageID)
		if err != nil {
			return upserted, nil, safeSyncError(err, connection)
		}
		feedUpserted, feedAdvertisements, conflict := s.reconcileSyncEntries(ctx, args, languageTag(language), feed)
		upserted += feedUpserted
		advertisements = append(advertisements, feedAdvertisements...)
		if conflict != nil && firstConflict == nil {
			firstConflict = conflict
		}
	}
	if firstConflict != nil {
		return upserted, nil, firstConflict
	}
	return upserted, advertisements, nil
}

// reconcileSyncEntries reconciles the EPUB entries of one language feed,
// skipping entries the catalogue did not identify or title.
func (s *Service) reconcileSyncEntries(ctx context.Context, args SyncArgs, entryLanguage string, feed opds.Feed) (int, []coverAdvertisement, error) {
	upserted := 0
	var advertisements []coverAdvertisement
	var firstConflict error
	for _, entry := range opds.FilterEPUBEntries(feed).Entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Title) == "" {
			continue
		}
		result, err := s.catalogue.ReconcileCatalogueEntry(ctx, args.OwnerID, args.ConnectionID, entry.ID, entry.Title, entry.Author, entryLanguage)
		if err != nil {
			if firstConflict == nil {
				firstConflict = fmt.Errorf("catalog entry %q could not be reconciled: %w", entry.ID, err)
			}
			continue
		}
		if result.Upserted() {
			upserted++
		}
		advertisements = append(advertisements, coverAdvertisement{bookID: result.Book.ID, sourceIdentifier: entry.ID, advertised: opds.FindCoverImage(entry) != nil})
	}
	return upserted, advertisements, firstConflict
}

// applyCoverAdvertisements records advertised provenance and schedules
// deduplicated retrieval after reconciliation succeeds. Cover work is optional
// metadata: a scheduling failure is logged and never fails catalog sync.
func (s *Service) applyCoverAdvertisements(ctx context.Context, owner, connectionID string, advertisements []coverAdvertisement) {
	covers, ok := s.catalogue.(coverStore)
	if !ok {
		return
	}
	for _, advertisement := range advertisements {
		retrieve, advertisedAt, err := covers.RecordBookCoverAdvertisement(ctx, owner, advertisement.bookID, connectionID, advertisement.sourceIdentifier, advertisement.advertised)
		if err != nil {
			log.Printf("record book cover advertisement: %v", err)
			continue
		}
		if !retrieve || s.client == nil {
			continue
		}
		if _, err := s.EnqueueCover(ctx, owner, advertisement.bookID, connectionID, advertisement.sourceIdentifier, advertisedAt); err != nil {
			log.Printf("schedule book cover retrieval: %v", err)
		}
	}
}

func safeSyncError(err error, connection domain.OpdsConnection) error {
	if strings.HasPrefix(err.Error(), "catalog entry ") || strings.HasPrefix(err.Error(), "NLP language readiness") || strings.HasPrefix(err.Error(), "catalog connection could not be loaded") || strings.HasPrefix(err.Error(), "authentication failed") || strings.HasPrefix(err.Error(), "catalog returned an unsafe target") || strings.HasPrefix(err.Error(), "could not reach ") {
		return errors.New(err.Error())
	}
	host := "the catalog"
	if parsed, parseErr := url.Parse(connection.URL); parseErr == nil && parsed.Host != "" {
		host = parsed.Host
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "401"), strings.Contains(lower, "403"), strings.Contains(lower, "unauthorized"):
		return fmt.Errorf("authentication failed for connection %s", connection.Name)
	case strings.Contains(lower, "unsafe"), strings.Contains(lower, "outside the catalog origin"):
		return fmt.Errorf("catalog returned an unsafe target for connection %s", connection.Name)
	default:
		return fmt.Errorf("could not reach %s", host)
	}
}

type Worker struct {
	river.WorkerDefaults[SyncArgs]
	Connections  ConnectionStore
	Catalogue    CatalogueStore
	Statuses     CatalogueSyncStatusStore
	Reader       catalogueReader
	Capabilities analyzer.CapabilityProvider
	Client       *river.Client[pgx.Tx]
}

type CoverWorker struct {
	river.WorkerDefaults[CoverArgs]
	Connections  ConnectionStore
	Catalogue    coverStore
	Reader       coverReader
	Capabilities analyzer.CapabilityProvider
}

func (w *CoverWorker) Work(ctx context.Context, job *river.Job[CoverArgs]) error {
	if w == nil || w.Connections == nil || w.Catalogue == nil || w.Reader == nil || job == nil {
		return errors.New("book cover worker is unavailable")
	}
	args := job.Args
	if !coverArgsComplete(args) {
		return nil
	}
	eligible, err := w.coverRetrievalEligible(ctx, args)
	if err != nil {
		return err
	}
	if !eligible {
		return nil
	}
	service := &Service{connections: w.Connections, catalogue: w.Catalogue, reader: w.Reader, capabilities: w.Capabilities}
	entry, found, err := service.findEntryForConnection(ctx, args.OwnerID, args.ConnectionID, args.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found {
		return w.failCoverRetrieval(ctx, args)
	}
	links := opds.CoverImages(entry)
	if len(links) == 0 {
		// The entry no longer advertises an image; complete reconciliation owns
		// explicit absence, so leave retained state untouched.
		return nil
	}
	normalized, mediaType, width, height, contentHash, ok := w.normalizeCover(ctx, args, links)
	if !ok {
		return w.failCoverRetrieval(ctx, args)
	}
	return w.Catalogue.SaveBookCover(ctx, args.OwnerID, args.BookID, args.ConnectionID, args.SourceIdentifier, args.AdvertisedAt, mediaType, width, height, contentHash, normalized)
}

// coverArgsComplete reports whether a job carries the owner-scoped identity
// needed to reload mutable catalog state.
func coverArgsComplete(args CoverArgs) bool {
	return strings.TrimSpace(args.OwnerID) != "" && strings.TrimSpace(args.BookID) != "" &&
		strings.TrimSpace(args.ConnectionID) != "" && strings.TrimSpace(args.SourceIdentifier) != ""
}

// coverRetrievalEligible reloads the cover and alias state and reports whether
// this job may still attempt retrieval for its source generation.
func (w *CoverWorker) coverRetrievalEligible(ctx context.Context, args CoverArgs) (bool, error) {
	cover, err := w.Catalogue.GetBookCoverForRetrieval(ctx, args.OwnerID, args.BookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// A validated image is already selected from another source; a later
	// completion cannot displace the winner.
	if cover.State == domain.BookCoverAvailable && (cover.SelectedConnectionID != args.ConnectionID || cover.SelectedSourceIdentifier != args.SourceIdentifier) {
		return false, nil
	}
	alias, err := w.Catalogue.GetBookCatalogEntryAliasForConnection(ctx, args.OwnerID, args.BookID, args.ConnectionID)
	if errors.Is(err, persistence.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(alias.Value) != strings.TrimSpace(args.SourceIdentifier) {
		return false, nil
	}
	return true, nil
}

// normalizeCover fetches advertised images in preference order and returns the
// first one that normalises, skipping unreachable or invalid candidates.
func (w *CoverWorker) normalizeCover(ctx context.Context, args CoverArgs, links []opds.Link) ([]byte, string, int, int, string, bool) {
	for _, link := range links {
		raw, advertisedType, err := w.Reader.FetchCover(ctx, args.OwnerID, args.ConnectionID, link.Href)
		if err != nil {
			continue
		}
		normalized, mediaType, width, height, contentHash, err := bookcover.Normalize(raw, link.Type, advertisedType)
		if err != nil {
			continue
		}
		return normalized, mediaType, width, height, contentHash, true
	}
	return nil, "", 0, 0, "", false
}

// failCoverRetrieval preserves any retained image, records the failure for
// operators, and returns an error so River retries with existing diagnostics.
// It deliberately does not surface upstream URLs or credentials.
func (w *CoverWorker) failCoverRetrieval(ctx context.Context, args CoverArgs) error {
	if err := w.Catalogue.MarkBookCoverUnavailable(context.WithoutCancel(ctx), args.OwnerID, args.BookID, args.ConnectionID, args.SourceIdentifier, args.AdvertisedAt, "cover retrieval or validation failed"); err != nil {
		return err
	}
	return errors.New("book cover retrieval or validation failed")
}

func (w *Worker) Work(ctx context.Context, job *river.Job[SyncArgs]) (workErr error) {
	if w == nil || w.Connections == nil || w.Catalogue == nil || w.Statuses == nil || w.Reader == nil || w.Capabilities == nil || job == nil {
		return errors.New("catalogue sync worker is unavailable")
	}
	service := &Service{connections: w.Connections, catalogue: w.Catalogue, statuses: w.Statuses, reader: w.Reader, capabilities: w.Capabilities, client: w.Client}
	args := job.Args
	connection, loadErr := w.Connections.GetOpdsConnection(ctx, args.OwnerID, args.ConnectionID)
	if errors.Is(loadErr, persistence.ErrNotFound) {
		return nil
	}
	if loadErr != nil {
		return errors.New("catalog connection could not be loaded")
	}
	upserted, workErr := service.work(ctx, args)
	if workErr == nil {
		if err := w.Statuses.SetCatalogueSyncStatus(context.WithoutCancel(ctx), domain.CatalogueSyncStatus{OwnerID: args.OwnerID, ConnectionID: args.ConnectionID, State: domain.CatalogueSyncSynced, LastSyncedAt: new(time.Now()), LastUpsertedCount: upserted}); err != nil {
			return err
		}
		return nil
	}
	safe := safeSyncError(workErr, connection)
	if statusErr := w.Statuses.SetCatalogueSyncStatus(context.WithoutCancel(ctx), domain.CatalogueSyncStatus{OwnerID: args.OwnerID, ConnectionID: args.ConnectionID, State: domain.CatalogueSyncFailed, LastError: safe.Error()}); statusErr != nil {
		return errors.Join(safe, fmt.Errorf("record catalogue sync failure: %w", statusErr))
	}
	return safe
}

func AddWorker(workers *river.Workers, deps StoreDependencies, reader catalogueReader, capabilities analyzer.CapabilityProvider) *Worker {
	worker := &Worker{Connections: deps.Connections, Catalogue: deps.Catalogue, Statuses: deps.Statuses, Reader: reader, Capabilities: capabilities}
	river.AddWorker(workers, worker)
	if coverReader, readerOK := reader.(coverReader); readerOK {
		if covers, storeOK := deps.Catalogue.(coverStore); storeOK {
			river.AddWorker(workers, &CoverWorker{Connections: deps.Connections, Catalogue: covers, Reader: coverReader, Capabilities: capabilities})
		}
	}
	return worker
}
