// Package cataloguesync implements the owner-scoped metadata-only catalogue
// reconciliation path shared by explicit enqueueing and River periodic jobs.
package cataloguesync

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
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
	DefaultCadence        = 24 * time.Hour
	defaultMaxAttempts    = 3
	periodicJobIDPrefix   = "catalogue_sync:"
	statusCancelledReason = "Catalogue sync cancelled before completion. Retry when ready."
)

var ErrNotFound = errors.New("catalogue sync job: not found")

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
	Book    domain.Book
	Updated bool
	Created bool
	Missing bool
	Failed  bool
}

type connectionStore interface {
	GetBook(context.Context, string, string) (domain.Book, error)
	GetBookCatalogEntryAlias(context.Context, string, string) (domain.BookAlias, error)
	GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error)
	ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error)
	OpdsConnectionExists(context.Context, string, string) (bool, error)
	ListAllOpdsConnectionIDs(context.Context) ([]domain.OpdsConnection, error)
	ListSupportedLanguages(context.Context) ([]domain.SupportedLanguage, error)
	ReconcileCatalogueEntry(context.Context, string, string, string, string) (persistence.CatalogueEntryReconcileResult, error)
	SetCatalogueSyncStatus(context.Context, domain.CatalogueSyncStatus) error
	GetCatalogueSyncStatus(context.Context, string, string) (domain.CatalogueSyncStatus, error)
	ListCatalogueSyncStatuses(context.Context, string) ([]domain.CatalogueSyncStatus, error)
}

type catalogueReader interface {
	Languages(context.Context, string, string) (opds.Feed, error)
	BrowseLanguage(context.Context, string, string, string) (opds.Feed, error)
}

type Service struct {
	store        connectionStore
	pool         *pgxpool.Pool
	client       *river.Client[pgx.Tx]
	reader       catalogueReader
	capabilities analyzer.CapabilityProvider
	periodicMu   sync.Mutex
	periodicIDs  map[string]struct{}
}

func NewService(store *persistence.PostgresStore, client *river.Client[pgx.Tx], reader catalogueReader, capabilities analyzer.CapabilityProvider) *Service {
	var pool *pgxpool.Pool
	if store != nil {
		pool = store.Pool()
	}
	return &Service{store: store, pool: pool, client: client, reader: reader, capabilities: capabilities, periodicIDs: make(map[string]struct{})}
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
	if s == nil || s.store == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(connectionID) == "" {
		return Handle{}, ErrNotFound
	}
	exists, err := s.store.OpdsConnectionExists(ctx, owner, connectionID)
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
	if err = s.store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: owner, ConnectionID: connectionID, State: domain.CatalogueSyncSyncing}); err != nil {
		return Handle{}, fmt.Errorf("record catalogue sync status: %w", err)
	}
	return Handle{ID: inserted.Job.ID, DisplayNumber: inserted.Job.ID}, nil
}

// RegisterConnection adds one in-memory River periodic job. The constructor
// performs no I/O and therefore cannot leak credentials or block the leader.
func (s *Service) RegisterConnection(_ context.Context, owner, connectionID string) error {
	if s == nil || s.store == nil || s.client == nil {
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
	if s == nil || s.store == nil || s.reader == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return RefreshResult{}, ErrNotFound
	}
	book, err := s.store.GetBook(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return RefreshResult{}, ErrNotFound
	}
	if err != nil {
		return RefreshResult{Failed: true}, err
	}
	alias, err := s.store.GetBookCatalogEntryAlias(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return RefreshResult{}, ErrNotFound
	}
	if err != nil {
		return RefreshResult{Book: book, Failed: true}, err
	}
	if alias.BookID != book.ID || alias.AliasType != domain.AliasCatalogEntry || alias.Namespace != domain.NamespaceSourceIdentifier || strings.TrimSpace(alias.Value) == "" {
		return RefreshResult{}, ErrNotFound
	}
	connections, err := s.store.ListOpdsConnections(ctx, owner)
	if err != nil {
		return RefreshResult{Book: book, Failed: true}, err
	}
	if len(connections) == 0 {
		return RefreshResult{}, ErrNotFound
	}
	supported, err := s.store.ListSupportedLanguages(ctx)
	if err != nil {
		return RefreshResult{Book: book, Failed: true}, err
	}
	displayName := supportedLanguageDisplayName(supported, book.LanguageTag)
	sort.SliceStable(connections, func(i, j int) bool {
		if connections[i].Name != connections[j].Name {
			return connections[i].Name < connections[j].Name
		}
		return connections[i].ID < connections[j].ID
	})
	failedConnections := 0
	for _, connection := range connections {
		languages, readErr := s.reader.Languages(ctx, owner, connection.ID)
		if readErr != nil {
			failedConnections++
			continue
		}
		languageID := opds.LanguageID(languages, book.LanguageTag, displayName)
		if languageID == "" {
			continue
		}
		feed, readErr := s.reader.BrowseLanguage(ctx, owner, connection.ID, languageID)
		if readErr != nil {
			failedConnections++
			continue
		}
		for _, entry := range opds.FilterEPUBEntries(feed).Entries {
			if strings.TrimSpace(entry.ID) != alias.Value || strings.TrimSpace(entry.Title) == "" {
				continue
			}
			reconciled, reconcileErr := s.store.ReconcileCatalogueEntry(ctx, owner, entry.ID, entry.Title, book.LanguageTag)
			if reconcileErr != nil {
				return RefreshResult{Book: book, Failed: true}, reconcileErr
			}
			return RefreshResult{Book: reconciled.Book, Updated: reconciled.TitleChanged, Created: reconciled.Created}, nil
		}
	}
	if failedConnections == len(connections) {
		return RefreshResult{Book: book, Failed: true}, nil
	}
	return RefreshResult{Book: book, Missing: true}, nil
}

// FindAcquisitionTarget resolves the current EPUB link for a synced Book.
// The alias is stable across catalog feed changes, while the download Href is
// looked up again immediately before acquisition.
func (s *Service) FindAcquisitionTarget(ctx context.Context, owner, bookID string) (AcquisitionTarget, error) {
	if s == nil || s.store == nil || s.reader == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return AcquisitionTarget{}, ErrNotFound
	}
	book, err := s.store.GetBook(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return AcquisitionTarget{}, ErrNotFound
	}
	if err != nil {
		return AcquisitionTarget{}, err
	}
	alias, err := s.store.GetBookCatalogEntryAlias(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return AcquisitionTarget{}, ErrNotFound
	}
	if err != nil {
		return AcquisitionTarget{}, err
	}
	if strings.TrimSpace(alias.Value) == "" {
		return AcquisitionTarget{}, ErrNotFound
	}
	connections, err := s.store.ListOpdsConnections(ctx, owner)
	if err != nil {
		return AcquisitionTarget{}, err
	}
	supported, err := s.store.ListSupportedLanguages(ctx)
	if err != nil {
		return AcquisitionTarget{}, err
	}
	displayName := supportedLanguageDisplayName(supported, book.LanguageTag)
	for _, connection := range connections {
		languages, readErr := s.reader.Languages(ctx, owner, connection.ID)
		if readErr != nil {
			continue
		}
		languageID := opds.LanguageID(languages, book.LanguageTag, displayName)
		if languageID == "" {
			continue
		}
		feed, readErr := s.reader.BrowseLanguage(ctx, owner, connection.ID, languageID)
		if readErr != nil {
			continue
		}
		for _, entry := range opds.FilterEPUBEntries(feed).Entries {
			if strings.TrimSpace(entry.ID) != alias.Value {
				continue
			}
			for _, link := range opds.FindEPUBs(entry) {
				if strings.TrimSpace(link.Href) != "" {
					return AcquisitionTarget{ConnectionID: connection.ID, Language: book.LanguageTag, Entry: entry, Href: link.Href}, nil
				}
			}
		}
	}
	return AcquisitionTarget{}, ErrNotFound
}

// RegisterAll restores all schedules after a process start. It intentionally
// uses the repository's owner-scoped credential path only to enumerate rows;
// no credential is copied into the periodic constructor or River args.
func (s *Service) RegisterAll(ctx context.Context) error {
	if s == nil || s.store == nil {
		return errors.New("catalogue sync service is unavailable")
	}
	connections, err := s.store.ListAllOpdsConnectionIDs(ctx)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if err = s.RegisterConnection(ctx, connection.OwnerID, connection.ID); err != nil {
			return err
		}
	}
	return nil
}

// ListCatalogueSyncStatuses overlays durable status with River's live state.
// A worker or process can disappear after the durable row is marked syncing;
// that row must not permanently disable the Sync now action.
func (s *Service) ListCatalogueSyncStatuses(ctx context.Context, owner string) ([]domain.CatalogueSyncStatus, error) {
	if s == nil || s.pool == nil || s.store == nil || strings.TrimSpace(owner) == "" {
		return nil, nil
	}
	statuses, err := s.store.ListCatalogueSyncStatuses(ctx, owner)
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
	if state != domain.CatalogueSyncSyncing || s.store == nil {
		return state
	}
	live, err := s.liveJobExists(ctx, owner, connectionID)
	if err != nil || live {
		return state
	}
	durable, err := s.store.GetCatalogueSyncStatus(ctx, owner, connectionID)
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
	if err = s.store.SetCatalogueSyncStatus(context.WithoutCancel(ctx), durable); err != nil {
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
	status.Error = firstError(durableError, riverError(s.pool, ctx, id, owner))
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
		var item Status
		var riverState, durableState, durableError, connectionID, riverErr string
		var syncedAt *time.Time
		if err = rows.Scan(&item.ID, &item.Attempt, &item.CreatedAt, &item.FinalizedAt, &riverState, &connectionID, &item.ConnectionName, &durableState, &durableError, &riverErr, &syncedAt); err != nil {
			return nil, err
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
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) Retry(ctx context.Context, owner string, id int64) (Handle, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return Handle{}, err
	}
	if status.LogicalState != "failed" && status.LogicalState != "cancelled" {
		return Handle{}, fmt.Errorf("catalogue sync job is not retryable")
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
	durable, durableErr := s.store.GetCatalogueSyncStatus(ctx, owner, status.ConnectionID)
	if durableErr != nil && !errors.Is(durableErr, persistence.ErrNotFound) {
		return Status{}, durableErr
	}
	if durable.LastSyncedAt != nil {
		durable.State = domain.CatalogueSyncSynced
		durable.LastError = ""
	} else {
		durable = domain.CatalogueSyncStatus{OwnerID: owner, ConnectionID: status.ConnectionID, State: domain.CatalogueSyncFailed, LastError: statusCancelledReason}
	}
	if err = s.store.SetCatalogueSyncStatus(context.WithoutCancel(ctx), durable); err != nil {
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
	if index := strings.IndexByte(language, '-'); index >= 0 {
		return language[:index]
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

func riverError(pool *pgxpool.Pool, ctx context.Context, id int64, owner string) string {
	var value string
	_ = pool.QueryRow(ctx, `SELECT COALESCE(errors[array_length(errors,1)]->>'error','') FROM river_job WHERE id=$1 AND args->>'owner_id'=$2`, id, owner).Scan(&value)
	return value
}

func (s *Service) work(ctx context.Context, args SyncArgs) (int, error) {
	connection, err := s.store.GetOpdsConnection(ctx, args.OwnerID, args.ConnectionID)
	if errors.Is(err, persistence.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, errors.New("catalogue connection could not be loaded")
	}
	if err = s.store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{OwnerID: args.OwnerID, ConnectionID: args.ConnectionID, State: domain.CatalogueSyncSyncing}); err != nil {
		return 0, err
	}
	capabilities, err := s.capabilities.GetCapabilities(ctx)
	if err != nil {
		return 0, errors.New("NLP language readiness could not be checked")
	}
	languages, err := s.reader.Languages(ctx, args.OwnerID, args.ConnectionID)
	if err != nil {
		return 0, safeSyncError(err, connection)
	}
	scope := eligibleLanguages(languages, capabilities)
	var firstConflict error
	upserted := 0
	for _, language := range scope {
		feed, feedErr := s.reader.BrowseLanguage(ctx, args.OwnerID, args.ConnectionID, language.languageID)
		if feedErr != nil {
			return upserted, safeSyncError(feedErr, connection)
		}
		entryLanguage := languageTag(language)
		for _, entry := range opds.FilterEPUBEntries(feed).Entries {
			if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Title) == "" {
				continue
			}
			if result, reconcileErr := s.store.ReconcileCatalogueEntry(ctx, args.OwnerID, entry.ID, entry.Title, entryLanguage); reconcileErr != nil {
				if firstConflict == nil {
					firstConflict = fmt.Errorf("catalog entry %q could not be reconciled: %w", entry.ID, reconcileErr)
				}
			} else if result.Upserted() {
				upserted++
			}
		}
	}
	if firstConflict != nil {
		return upserted, firstConflict
	}
	return upserted, nil
}

func safeSyncError(err error, connection domain.OpdsConnection) error {
	if strings.HasPrefix(err.Error(), "catalog entry ") || strings.HasPrefix(err.Error(), "NLP language readiness") || strings.HasPrefix(err.Error(), "catalogue connection could not be loaded") || strings.HasPrefix(err.Error(), "authentication failed") || strings.HasPrefix(err.Error(), "catalogue returned an unsafe target") || strings.HasPrefix(err.Error(), "could not reach ") {
		return errors.New(err.Error())
	}
	host := "the catalogue"
	if parsed, parseErr := url.Parse(connection.URL); parseErr == nil && parsed.Host != "" {
		host = parsed.Host
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "401"), strings.Contains(lower, "403"), strings.Contains(lower, "unauthorized"):
		return fmt.Errorf("authentication failed for connection %s", connection.Name)
	case strings.Contains(lower, "unsafe"), strings.Contains(lower, "outside the catalog origin"):
		return fmt.Errorf("catalogue returned an unsafe target for connection %s", connection.Name)
	default:
		return fmt.Errorf("could not reach %s", host)
	}
}

type Worker struct {
	river.WorkerDefaults[SyncArgs]
	Store        *persistence.PostgresStore
	Reader       catalogueReader
	Capabilities analyzer.CapabilityProvider
}

func (w *Worker) Work(ctx context.Context, job *river.Job[SyncArgs]) (workErr error) {
	if w == nil || w.Store == nil || w.Reader == nil || w.Capabilities == nil || job == nil {
		return errors.New("catalogue sync worker is unavailable")
	}
	service := &Service{store: w.Store, reader: w.Reader, capabilities: w.Capabilities}
	args := job.Args
	connection, loadErr := w.Store.GetOpdsConnection(ctx, args.OwnerID, args.ConnectionID)
	if errors.Is(loadErr, persistence.ErrNotFound) {
		return nil
	}
	if loadErr != nil {
		return errors.New("catalogue connection could not be loaded")
	}
	upserted, workErr := service.work(ctx, args)
	if workErr == nil {
		if err := w.Store.SetCatalogueSyncStatus(context.WithoutCancel(ctx), domain.CatalogueSyncStatus{OwnerID: args.OwnerID, ConnectionID: args.ConnectionID, State: domain.CatalogueSyncSynced, LastSyncedAt: timePtr(time.Now()), LastUpsertedCount: upserted}); err != nil {
			return err
		}
		return nil
	}
	safe := safeSyncError(workErr, connection)
	_ = w.Store.SetCatalogueSyncStatus(context.WithoutCancel(ctx), domain.CatalogueSyncStatus{OwnerID: args.OwnerID, ConnectionID: args.ConnectionID, State: domain.CatalogueSyncFailed, LastError: safe.Error()})
	return safe
}

func timePtr(value time.Time) *time.Time { return &value }

func AddWorker(workers *river.Workers, store *persistence.PostgresStore, reader catalogueReader, capabilities analyzer.CapabilityProvider) {
	river.AddWorker(workers, &Worker{Store: store, Reader: reader, Capabilities: capabilities})
}
