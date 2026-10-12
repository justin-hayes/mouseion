package cataloguesync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEligibleLanguagesUsesReadyCatalogueLanguagesAndExcludesEnglish(t *testing.T) {
	languages := opds.Feed{Entries: []opds.Entry{
		{Title: "de", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}},
		{Title: "English", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/8"}}},
		{Title: "Greek", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/11"}}},
		{Title: "French", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/10"}}},
	}}
	capabilities := analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de-DE", DisplayName: "German", Ready: true},
		{Language: "en", DisplayName: "English", Ready: true},
		{Language: "it", Ready: false},
		{Language: "el", DisplayName: "Greek", Ready: true},
		{Language: "fr", DisplayName: "French", Ready: true},
		{Language: "es", DisplayName: "Spanish", Ready: true},
	}}
	got := eligibleLanguages(languages, capabilities)
	require.Len(t, got, 3)
	assert.Equal(t, "de-DE", got[0].capability.Language)
	assert.Equal(t, "7", got[0].languageID)
	assert.Equal(t, "el", got[1].capability.Language)
	assert.Equal(t, "11", got[1].languageID)
	assert.Equal(t, "fr", got[2].capability.Language)
	assert.Equal(t, "10", got[2].languageID)
}

func TestSyncArgsNeverSerializeCredentials(t *testing.T) {
	args := SyncArgs{OwnerID: "owner", ConnectionID: "connection"}
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	text := string(encoded)
	assert.NotContains(t, text, "password")
	assert.NotContains(t, text, "secret")
}

func TestCoverArgsNeverSerializeCredentialsOrURLs(t *testing.T) {
	args := CoverArgs{OwnerID: "owner", BookID: "book", ConnectionID: "connection", SourceIdentifier: "entry-1"}
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	text := string(encoded)
	assert.NotContains(t, text, "password")
	assert.NotContains(t, text, "secret")
	assert.NotContains(t, text, "http")
	assert.Contains(t, text, "entry-1")
}

func TestSafeSyncErrorIsActionableWithoutCredential(t *testing.T) {
	secret := "plain-password-must-not-escape"
	err := safeSyncError(errors.New("opds: HTTP 401 Unauthorized: "+secret), domain.OpdsConnection{Name: "Home", URL: "https://catalog.example/opds", Password: secret})
	assert.Contains(t, err.Error(), "authentication failed")
	assert.NotContains(t, err.Error(), secret)
}

func TestWorkerUnavailableDoesNotExposeInput(t *testing.T) {
	var worker *Worker
	err := worker.Work(context.Background(), nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "password")
}

type refreshStore struct {
	book            domain.Book
	alias           domain.BookAlias
	connections     []domain.OpdsConnection
	supported       []domain.SupportedLanguage
	reconciles      int
	lastOwner       string
	lastConnection  string
	reconcile       persistence.CatalogueEntryReconcileResult
	aliasErr        error
	backfillAliases []domain.BookAlias
	assigned        map[string]string
	// Error injection pins the failure branches of each step.
	listAliasesErr error
	connectionsErr error
	setErr         error
	bookErr        error
	supportedErr   error
	reconcileErr   error
}

func (s *refreshStore) ListUnscopedCatalogueEntryAliases(context.Context) ([]domain.BookAlias, error) {
	if s.listAliasesErr != nil {
		return nil, s.listAliasesErr
	}
	return append([]domain.BookAlias(nil), s.backfillAliases...), nil
}
func (s *refreshStore) SetCatalogueEntryAliasConnection(_ context.Context, owner, aliasID, connectionID string) error {
	if s.setErr != nil {
		return s.setErr
	}
	if s.assigned == nil {
		s.assigned = make(map[string]string)
	}
	s.assigned[owner+":"+aliasID] = connectionID
	return nil
}

func (s *refreshStore) GetBook(_ context.Context, owner, bookID string) (domain.Book, error) {
	if s.bookErr != nil {
		return domain.Book{}, s.bookErr
	}
	if s.book.OwnerID != owner || s.book.ID != bookID {
		return domain.Book{}, persistence.ErrNotFound
	}
	return s.book, nil
}
func (s *refreshStore) GetBookCatalogEntryAlias(_ context.Context, owner, bookID string) (domain.BookAlias, error) {
	if s.aliasErr != nil {
		return domain.BookAlias{}, s.aliasErr
	}
	if s.alias.OwnerID != "" && (s.alias.OwnerID != owner || s.alias.BookID != bookID) {
		return domain.BookAlias{}, persistence.ErrNotFound
	}
	return s.alias, nil
}
func (s *refreshStore) GetBookCatalogEntryAliasForConnection(_ context.Context, owner, bookID, connectionID string) (domain.BookAlias, error) {
	if s.aliasErr != nil {
		return domain.BookAlias{}, s.aliasErr
	}
	if s.alias.OwnerID != "" && (s.alias.OwnerID != owner || s.alias.BookID != bookID || s.alias.ConnectionID != connectionID) {
		return domain.BookAlias{}, persistence.ErrNotFound
	}
	return s.alias, nil
}
func (s *refreshStore) GetOpdsConnection(_ context.Context, owner, id string) (domain.OpdsConnection, error) {
	if s.connectionsErr != nil {
		return domain.OpdsConnection{}, s.connectionsErr
	}
	for _, connection := range s.connections {
		if connection.OwnerID == "" || connection.OwnerID == owner {
			if connection.ID == id {
				return connection, nil
			}
		}
	}
	return domain.OpdsConnection{}, persistence.ErrNotFound
}
func (s *refreshStore) ListOpdsConnections(_ context.Context, owner string) ([]domain.OpdsConnection, error) {
	if s.connectionsErr != nil {
		return nil, s.connectionsErr
	}
	connections := make([]domain.OpdsConnection, 0, len(s.connections))
	for _, connection := range s.connections {
		if connection.OwnerID == "" || connection.OwnerID == owner {
			connections = append(connections, connection)
		}
	}
	return connections, nil
}
func (s *refreshStore) OpdsConnectionExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *refreshStore) ListAllOpdsConnectionIDs(context.Context) ([]domain.OpdsConnection, error) {
	return nil, nil
}
func (s *refreshStore) ListSupportedLanguages(context.Context) ([]domain.SupportedLanguage, error) {
	if s.supportedErr != nil {
		return nil, s.supportedErr
	}
	return append([]domain.SupportedLanguage(nil), s.supported...), nil
}
func (s *refreshStore) SyncSupportedLanguages(context.Context, []domain.SupportedLanguage) error {
	return nil
}
func (s *refreshStore) ReconcileCatalogueEntry(_ context.Context, owner, connectionID, sourceIdentifier, title, author, language string) (persistence.CatalogueEntryReconcileResult, error) {
	if s.reconcileErr != nil {
		return persistence.CatalogueEntryReconcileResult{}, s.reconcileErr
	}
	s.reconciles++
	s.lastOwner = owner
	s.lastConnection = connectionID
	if s.reconcile.Book.ID == "" {
		s.reconcile.Book = s.book
	}
	if s.reconcile.Book.Title == "" {
		s.reconcile.Book.Title = title
	}
	if s.reconcile.Book.Author == "" {
		s.reconcile.Book.Author = author
	}
	if s.reconcile.Book.LanguageTag == "" {
		s.reconcile.Book.LanguageTag = language
	}
	if sourceIdentifier == "" {
		return persistence.CatalogueEntryReconcileResult{}, errors.New("missing source identifier")
	}
	return s.reconcile, nil
}
func (s *refreshStore) SetCatalogueSyncStatus(context.Context, domain.CatalogueSyncStatus) error {
	return nil
}
func (s *refreshStore) GetCatalogueSyncStatus(context.Context, string, string) (domain.CatalogueSyncStatus, error) {
	return domain.CatalogueSyncStatus{}, persistence.ErrNotFound
}
func (s *refreshStore) ListCatalogueSyncStatuses(context.Context, string) ([]domain.CatalogueSyncStatus, error) {
	return nil, nil
}

// conflictStore fails reconciliation for the listed entry identifiers so the
// sync worker's per-entry conflict handling can be pinned through its public
// Work seam.
type conflictStore struct {
	*refreshStore
	failIDs map[string]bool
}

func (s *conflictStore) ReconcileCatalogueEntry(ctx context.Context, owner, connectionID, sourceIdentifier, title, author, language string) (persistence.CatalogueEntryReconcileResult, error) {
	if s.failIDs[sourceIdentifier] {
		return persistence.CatalogueEntryReconcileResult{}, errors.New("reconcile conflict")
	}
	return s.refreshStore.ReconcileCatalogueEntry(ctx, owner, connectionID, sourceIdentifier, title, author, language)
}

type refreshReader struct {
	languages   opds.Feed
	feed        opds.Feed
	err         error
	browseErr   error
	reads       int
	connections []string
}

func (r *refreshReader) Languages(_ context.Context, _, connection string) (opds.Feed, error) {
	r.connections = append(r.connections, connection)
	if r.err != nil {
		return opds.Feed{}, r.err
	}
	if len(r.languages.Entries) > 0 {
		return r.languages, nil
	}
	return opds.Feed{Entries: []opds.Entry{{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}}, nil
}
func (r *refreshReader) BrowseLanguage(_ context.Context, _, connection, _ string) (opds.Feed, error) {
	r.connections = append(r.connections, connection)
	r.reads++
	if r.browseErr != nil {
		return opds.Feed{}, r.browseErr
	}
	if r.err != nil {
		return opds.Feed{}, r.err
	}
	return r.feed, nil
}
func (r *refreshReader) BrowseLanguageUnfiltered(ctx context.Context, owner, connection, language string) (opds.Feed, error) {
	return r.BrowseLanguage(ctx, owner, connection, language)
}

func newRefreshService(store *refreshStore, reader catalogueReader) *Service {
	return NewService(StoreDependencies{Connections: store, Catalogue: store, Aliases: store, Statuses: store}, nil, reader, nil)
}

type backfillReader struct {
	languages    map[string]opds.Feed
	feeds        map[string]opds.Feed
	languagesErr error
	feedErr      error
}

func (r *backfillReader) Languages(_ context.Context, owner, connection string) (opds.Feed, error) {
	if r.languagesErr != nil {
		return opds.Feed{}, r.languagesErr
	}
	return r.languages[owner+":"+connection], nil
}
func (r *backfillReader) BrowseLanguage(_ context.Context, owner, connection, language string) (opds.Feed, error) {
	if r.feedErr != nil {
		return opds.Feed{}, r.feedErr
	}
	return r.feeds[owner+":"+connection+":"+language], nil
}
func (r *backfillReader) BrowseLanguageUnfiltered(ctx context.Context, owner, connection, language string) (opds.Feed, error) {
	return r.BrowseLanguage(ctx, owner, connection, language)
}

func TestBackfillCatalogueEntryAliasesIsStrictAndIdempotent(t *testing.T) {
	languages := func(id string) opds.Feed {
		return opds.Feed{Entries: []opds.Entry{{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/" + id}}}}}
	}
	entry := func(id string) opds.Feed { return opds.Feed{Entries: []opds.Entry{{ID: id}}} }
	tests := []struct {
		name        string
		aliases     []domain.BookAlias
		connections []domain.OpdsConnection
		feeds       map[string]opds.Feed
		want        string
		wantErr     string
	}{
		{name: "unique owner match", aliases: []domain.BookAlias{{ID: "a", OwnerID: "alice", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry"}}, connections: []domain.OpdsConnection{{ID: "one", OwnerID: "alice"}}, feeds: map[string]opds.Feed{"alice:one:7": entry("entry")}, want: "alice:a=one"},
		{name: "missing fails", aliases: []domain.BookAlias{{ID: "a", OwnerID: "alice", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "missing"}}, connections: []domain.OpdsConnection{{ID: "one", OwnerID: "alice"}}, feeds: map[string]opds.Feed{}, wantErr: "absent"},
		{name: "ambiguous fails", aliases: []domain.BookAlias{{ID: "a", OwnerID: "alice", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry"}}, connections: []domain.OpdsConnection{{ID: "one", OwnerID: "alice"}, {ID: "two", OwnerID: "alice"}}, feeds: map[string]opds.Feed{"alice:one:7": entry("entry"), "alice:two:7": entry("entry")}, wantErr: "present in 2"},
		{name: "owner isolation", aliases: []domain.BookAlias{{ID: "a", OwnerID: "alice", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry"}}, connections: []domain.OpdsConnection{{ID: "one", OwnerID: "bob"}}, feeds: map[string]opds.Feed{"alice:one:7": entry("entry")}, wantErr: "absent"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &refreshStore{backfillAliases: tc.aliases, connections: tc.connections}
			reader := &backfillReader{languages: map[string]opds.Feed{"alice:one": languages("7"), "alice:two": languages("7")}, feeds: tc.feeds}
			result, err := newRefreshService(store, reader).BackfillCatalogueEntryAliases(context.Background())
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, result.Examined)
			assert.Equal(t, 1, result.Updated)
			assert.Equal(t, "one", store.assigned["alice:a"])
		})
	}

	store := &refreshStore{backfillAliases: []domain.BookAlias{{ID: "a", OwnerID: "alice", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry", ConnectionID: "one"}}, connections: []domain.OpdsConnection{{ID: "one", OwnerID: "alice"}}}
	reader := &backfillReader{languages: map[string]opds.Feed{"alice:one": languages("7")}, feeds: map[string]opds.Feed{"alice:one:7": entry("entry")}}
	result, err := newRefreshService(store, reader).BackfillCatalogueEntryAliases(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result.Examined)
	assert.Equal(t, 0, result.Updated)
}

func TestBackfillLeavesStrongBibliographicAliasesUnselected(t *testing.T) {
	store := &refreshStore{backfillAliases: []domain.BookAlias{{ID: "isbn", OwnerID: "alice", AliasType: domain.AliasStrongBibliographic, Namespace: "isbn", Value: "978"}}}
	result, err := newRefreshService(store, &backfillReader{}).BackfillCatalogueEntryAliases(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result.Examined)
	assert.Empty(t, store.assigned)
}

func TestBackfillCatalogueEntryAliasesRejectsUnavailableService(t *testing.T) {
	store := &refreshStore{}
	tests := []struct {
		name    string
		service *Service
	}{
		{name: "nil service", service: nil},
		{name: "missing aliases", service: NewService(StoreDependencies{Connections: store, Catalogue: store, Statuses: store}, nil, &backfillReader{}, nil)},
		{name: "missing connections", service: NewService(StoreDependencies{Catalogue: store, Aliases: store, Statuses: store}, nil, &backfillReader{}, nil)},
		{name: "missing reader", service: NewService(StoreDependencies{Connections: store, Catalogue: store, Aliases: store, Statuses: store}, nil, nil, nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.service.BackfillCatalogueEntryAliases(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "catalogue alias backfill service is unavailable")
		})
	}
}

// TestBackfillCatalogueEntryAliasesSurfacesStepFailures pins the failure
// branches of paging, deriving the owning connection, and writing the result
// through the public seam, so the split into private steps cannot silently
// change an error.
func TestBackfillCatalogueEntryAliasesSurfacesStepFailures(t *testing.T) {
	alias := domain.BookAlias{ID: "a", OwnerID: "alice", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry"}
	ownerConnection := []domain.OpdsConnection{{ID: "one", OwnerID: "alice"}}
	languages := map[string]opds.Feed{"alice:one": {Entries: []opds.Entry{{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}}}
	matchedFeed := map[string]opds.Feed{"alice:one:7": {Entries: []opds.Entry{{ID: "entry"}}}}
	tests := []struct {
		name         string
		store        *refreshStore
		reader       *backfillReader
		wantErr      string
		wantExamined int
	}{
		{name: "list aliases fails", store: &refreshStore{listAliasesErr: errors.New("aliases unavailable")}, reader: &backfillReader{}, wantErr: "list unscoped catalogue aliases", wantExamined: 0},
		{name: "list connections fails", store: &refreshStore{backfillAliases: []domain.BookAlias{alias}, connectionsErr: errors.New("connections unavailable")}, reader: &backfillReader{}, wantErr: "list connections for alias a", wantExamined: 1},
		{name: "languages read fails", store: &refreshStore{backfillAliases: []domain.BookAlias{alias}, connections: ownerConnection}, reader: &backfillReader{languagesErr: errors.New("languages unavailable")}, wantErr: "read languages for alias a from connection one", wantExamined: 1},
		{name: "feed read fails", store: &refreshStore{backfillAliases: []domain.BookAlias{alias}, connections: ownerConnection}, reader: &backfillReader{languages: languages, feedErr: errors.New("feed unavailable")}, wantErr: "read feed 7 for alias a from connection one", wantExamined: 1},
		{name: "assign fails", store: &refreshStore{backfillAliases: []domain.BookAlias{alias}, connections: ownerConnection, setErr: errors.New("write unavailable")}, reader: &backfillReader{languages: languages, feeds: matchedFeed}, wantErr: "assign connection one to alias a", wantExamined: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := newRefreshService(tc.store, tc.reader).BackfillCatalogueEntryAliases(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Equal(t, tc.wantExamined, result.Examined)
			assert.Zero(t, result.Updated)
			assert.Empty(t, tc.store.assigned)
		})
	}
}

func TestRefreshEntryOutcomesAreOwnerScopedAndMetadataOnly(t *testing.T) {
	entry := func(title string) opds.Feed {
		return opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: title, Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}
	}
	cases := []struct {
		name                                 string
		feed                                 opds.Feed
		readerErr                            error
		browseErr                            error
		titleChange                          bool
		wantUpdated, wantMissing, wantFailed bool
		wantReconciles                       int
	}{
		{name: "updated", feed: entry("New title"), titleChange: true, wantUpdated: true, wantReconciles: 1},
		{name: "unchanged", feed: entry("Old title"), wantReconciles: 1},
		{name: "missing", feed: opds.Feed{}, wantMissing: true},
		{name: "failed", readerErr: errors.New("upstream secret details"), wantFailed: true},
		{name: "browse failed", browseErr: errors.New("browse failed"), wantFailed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &refreshStore{
				book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
				alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
				connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
				supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}},
				reconcile:   persistence.CatalogueEntryReconcileResult{TitleChanged: tc.titleChange, Book: domain.Book{ID: "book-1", OwnerID: "alice", Title: "New title", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
			}
			reader := &refreshReader{feed: tc.feed, err: tc.readerErr, browseErr: tc.browseErr}
			result, err := newRefreshService(store, reader).RefreshEntry(context.Background(), "alice", "book-1")
			t.Run("returns the expected error", func(t *testing.T) {
				if tc.readerErr != nil {
					require.ErrorIs(t, err, tc.readerErr)
				} else if tc.browseErr != nil {
					require.ErrorIs(t, err, tc.browseErr)
				} else {
					require.NoError(t, err)
				}
			})
			assert.Equal(t, tc.wantUpdated, result.Updated)
			assert.Equal(t, tc.wantMissing, result.Missing)
			assert.Equal(t, tc.wantFailed, result.Failed)
			assert.Equal(t, tc.wantReconciles, store.reconciles)
			if tc.wantReconciles > 0 {
				assert.Equal(t, "alice", store.lastOwner)
				assert.Equal(t, "connection-1", store.lastConnection)
			}
		})
	}
}

func TestRefreshEntryRejectsCrossOwnerBookWithoutReadingCatalogue(t *testing.T) {
	store := &refreshStore{book: domain.Book{ID: "book-1", OwnerID: "alice"}, alias: domain.BookAlias{BookID: "book-1", Value: "entry-1"}}
	reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "should not read"}}}}
	_, err := newRefreshService(store, reader).RefreshEntry(context.Background(), "bob", "book-1")
	t.Run("cross-owner refresh is not found", func(t *testing.T) {
		require.ErrorIs(t, err, ErrNotFound)
	})
	assert.Equal(t, 0, reader.reads, "cross-owner refresh catalogue reads")
}

func TestRefreshEntryRepeatedUnchangedMetadataIsIdempotent(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Same title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}},
		reconcile:   persistence.CatalogueEntryReconcileResult{Book: domain.Book{ID: "book-1", OwnerID: "alice", Title: "Same title", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
	}
	reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Same title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType}}}}}}
	service := newRefreshService(store, reader)
	first, err := service.RefreshEntry(context.Background(), "alice", "book-1")
	require.NoError(t, err)
	second, err := service.RefreshEntry(context.Background(), "alice", "book-1")
	require.NoError(t, err)
	assert.False(t, first.Updated)
	assert.False(t, second.Updated)
	assert.False(t, first.Created)
	assert.False(t, second.Created)
	assert.Equal(t, 2, store.reconciles)
}

func TestRefreshEntryUsesSupportedLanguageDisplayName(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "Deutsch"}},
		reconcile:   persistence.CatalogueEntryReconcileResult{Book: domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
	}
	reader := &refreshReader{
		languages: opds.Feed{Entries: []opds.Entry{{Title: "Deutsch", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}},
		feed:      opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Old title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}},
	}
	result, err := newRefreshService(store, reader).RefreshEntry(context.Background(), "alice", "book-1")
	require.NoError(t, err)
	assert.False(t, result.Missing)
	assert.False(t, result.Failed)
	assert.Equal(t, 1, store.reconciles)
}

func TestFindAcquisitionTargetUsesSupportedLanguageDisplayName(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "Deutsch"}},
	}
	reader := &refreshReader{
		languages: opds.Feed{Entries: []opds.Entry{{Title: "Deutsch", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}},
		feed:      opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Old title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}},
	}
	target, err := newRefreshService(store, reader).FindAcquisitionTarget(context.Background(), "alice", "book-1")
	require.NoError(t, err)
	assert.Equal(t, "https://catalog.example/book.epub", target.Href)
	assert.Equal(t, "connection-1", target.ConnectionID)
	assert.Equal(t, "de", target.Language)
	assert.Equal(t, "connection-1,connection-1", strings.Join(reader.connections, ","))
}

func TestRefreshEntryUsesOnlyAliasConnection(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-2", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "First"}, {ID: "connection-2", Name: "Second"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}},
		reconcile:   persistence.CatalogueEntryReconcileResult{Book: domain.Book{ID: "book-1", OwnerID: "alice", LanguageTag: "de"}},
	}
	reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Title"}}}}
	_, err := newRefreshService(store, reader).RefreshEntry(context.Background(), "alice", "book-1")
	require.NoError(t, err)
	assert.Equal(t, "connection-2,connection-2", strings.Join(reader.connections, ","))
}

func TestRefreshAndAcquisitionReportMissingAliasConnection(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "deleted", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "other", Name: "Other"}},
	}
	reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Wrong connection"}}}}
	_, refreshErr := newRefreshService(store, reader).RefreshEntry(context.Background(), "alice", "book-1")
	t.Run("refresh reports missing alias connection", func(t *testing.T) {
		require.ErrorIs(t, refreshErr, ErrConnectionNotFound)
	})
	assert.Empty(t, reader.connections)
	_, acquisitionErr := newRefreshService(store, reader).FindAcquisitionTarget(context.Background(), "alice", "book-1")
	t.Run("acquisition reports missing alias connection", func(t *testing.T) {
		require.ErrorIs(t, acquisitionErr, ErrConnectionNotFound)
	})
	assert.Empty(t, reader.connections)
}

// TestFindAcquisitionTargetRejectsIncompleteOrUnreadableSources pins each
// rejection branch of the acquisition lookup through its public seam, so
// splitting the lookup into private steps cannot silently change an error.
func TestFindAcquisitionTargetRejectsIncompleteOrUnreadableSources(t *testing.T) {
	book := domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"}
	connection := []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}}
	validAlias := domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"}
	german := opds.Feed{Entries: []opds.Entry{{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}}
	entry := func(id, href string) opds.Entry {
		return opds.Entry{ID: id, Title: "Title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: href}}}
	}
	unavailable := errors.New("store unavailable")
	readyStore := func() *refreshStore {
		return &refreshStore{book: book, alias: validAlias, connections: connection, supported: []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}}}
	}
	tests := []struct {
		name    string
		store   *refreshStore
		reader  *refreshReader
		wantErr error
	}{
		{name: "book not found", store: &refreshStore{}, reader: &refreshReader{}, wantErr: ErrNotFound},
		{name: "book load fails", store: &refreshStore{bookErr: unavailable}, reader: &refreshReader{}, wantErr: unavailable},
		{name: "alias not found", store: &refreshStore{book: book, aliasErr: persistence.ErrNotFound}, reader: &refreshReader{}, wantErr: ErrNotFound},
		{name: "alias load fails", store: &refreshStore{book: book, aliasErr: unavailable}, reader: &refreshReader{}, wantErr: unavailable},
		{name: "blank alias value", store: &refreshStore{book: book, alias: domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier}}, reader: &refreshReader{}, wantErr: ErrNotFound},
		{name: "connection load fails", store: &refreshStore{book: book, alias: validAlias, connectionsErr: unavailable}, reader: &refreshReader{}, wantErr: unavailable},
		{name: "supported languages fail", store: &refreshStore{book: book, alias: validAlias, connections: connection, supportedErr: unavailable}, reader: &refreshReader{}, wantErr: unavailable},
		{name: "languages read fails", store: readyStore(), reader: &refreshReader{err: unavailable}, wantErr: ErrNotFound},
		{name: "language absent from feed", store: &refreshStore{book: book, alias: validAlias, connections: connection}, reader: &refreshReader{languages: opds.Feed{Entries: []opds.Entry{{Title: "Esperanto"}}}}, wantErr: ErrNotFound},
		{name: "language feed read fails", store: readyStore(), reader: &refreshReader{languages: german, browseErr: unavailable}, wantErr: ErrNotFound},
		{name: "entry absent from feed", store: readyStore(), reader: &refreshReader{languages: german, feed: opds.Feed{Entries: []opds.Entry{entry("other", "https://catalog.example/other.epub")}}}, wantErr: ErrNotFound},
		{name: "entry advertises no usable href", store: readyStore(), reader: &refreshReader{languages: german, feed: opds.Feed{Entries: []opds.Entry{entry("entry-1", "")}}}, wantErr: ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newRefreshService(tc.store, tc.reader).FindAcquisitionTarget(context.Background(), "alice", "book-1")
			require.ErrorIs(t, err, tc.wantErr)
		})
	}

	t.Run("unavailable service", func(t *testing.T) {
		var service *Service
		_, err := service.FindAcquisitionTarget(context.Background(), "alice", "book-1")
		require.ErrorIs(t, err, ErrNotFound)
	})
}

// TestFindAcquisitionTargetSkipsEntriesWithoutAUsableHref pins the scan
// continuing past a matching identifier with no usable EPUB link, so a later
// duplicate identity can still resolve.
func TestFindAcquisitionTargetSkipsEntriesWithoutAUsableHref(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}},
	}
	entry := func(href string) opds.Entry {
		return opds.Entry{ID: "entry-1", Title: "Title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: href}}}
	}
	reader := &refreshReader{
		languages: opds.Feed{Entries: []opds.Entry{{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}},
		feed:      opds.Feed{Entries: []opds.Entry{entry(""), entry("https://catalog.example/book.epub")}},
	}
	target, err := newRefreshService(store, reader).FindAcquisitionTarget(context.Background(), "alice", "book-1")
	require.NoError(t, err)
	assert.Equal(t, "https://catalog.example/book.epub", target.Href)
}

// TestSyncWorkReconcilesEntriesAndReportsFirstConflict pins the sync worker's
// per-entry reconcile phase through its public Work seam: unusable entries are
// skipped, the first conflict is reported after the traversal completes, and a
// feed or capability failure is surfaced.
func TestSyncWorkReconcilesEntriesAndReportsFirstConflict(t *testing.T) {
	entry := func(id, title string) opds.Entry {
		return opds.Entry{ID: id, Title: title, Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/" + id + ".epub"}}}
	}
	book := domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"}
	newStore := func(failIDs map[string]bool) *conflictStore {
		return &conflictStore{
			refreshStore: &refreshStore{
				book:        book,
				connections: []domain.OpdsConnection{{ID: "connection-1", OwnerID: "alice"}},
				reconcile:   persistence.CatalogueEntryReconcileResult{Book: book},
			},
			failIDs: failIDs,
		}
	}
	capabilities := stubCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
	job := func() *river.Job[SyncArgs] {
		return &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: "alice", ConnectionID: "connection-1"}}
	}

	t.Run("skips unusable entries", func(t *testing.T) {
		store := newStore(nil)
		reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{entry("", "Blank id"), entry("blank-title", ""), entry("good", "Good")}}}
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
		require.NoError(t, worker.Work(context.Background(), job()))
		require.Equal(t, 1, store.reconciles)
	})

	t.Run("reports the first conflict and keeps reconciling", func(t *testing.T) {
		store := newStore(map[string]bool{"bad-1": true, "bad-2": true})
		reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{entry("bad-1", "First bad"), entry("good", "Good"), entry("bad-2", "Second bad")}}}
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
		err := worker.Work(context.Background(), job())
		require.Error(t, err)
		require.Contains(t, err.Error(), `catalog entry "bad-1" could not be reconciled`)
		require.Equal(t, 1, store.reconciles)
	})

	t.Run("surfaces a feed read failure", func(t *testing.T) {
		store := newStore(nil)
		reader := &refreshReader{languages: opds.Feed{Entries: []opds.Entry{{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}}, browseErr: errors.New("feed unavailable")}
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
		err := worker.Work(context.Background(), job())
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not reach")
	})

	t.Run("surfaces a capability failure", func(t *testing.T) {
		store := newStore(nil)
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: &refreshReader{}, Capabilities: erroringCapabilities{err: errors.New("capabilities unavailable")}}
		err := worker.Work(context.Background(), job())
		require.Error(t, err)
		require.Contains(t, err.Error(), "NLP language readiness could not be checked")
	})
}

// TestRefreshEntrySurfacesLoadFailures pins the load/rejection branches of the
// refresh fetch step through the public seam, so splitting it into private
// functions cannot silently change an error or result shape.
func TestRefreshEntrySurfacesLoadFailures(t *testing.T) {
	book := domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"}
	connection := []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}}
	validAlias := domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"}
	unavailable := errors.New("store unavailable")
	tests := []struct {
		name       string
		store      *refreshStore
		wantErr    error
		wantFailed bool
		wantBookID string
	}{
		{name: "book load fails", store: &refreshStore{bookErr: unavailable}, wantErr: unavailable, wantFailed: true},
		{name: "alias not found", store: &refreshStore{book: book, aliasErr: persistence.ErrNotFound}, wantErr: ErrNotFound},
		{name: "alias load fails", store: &refreshStore{book: book, aliasErr: unavailable}, wantErr: unavailable, wantFailed: true, wantBookID: "book-1"},
		{name: "alias shape rejects", store: &refreshStore{book: book, alias: domain.BookAlias{BookID: "other", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"}}, wantErr: ErrNotFound},
		{name: "connection load fails", store: &refreshStore{book: book, alias: validAlias, connectionsErr: unavailable}, wantErr: unavailable, wantFailed: true, wantBookID: "book-1"},
		{name: "supported languages fail", store: &refreshStore{book: book, alias: validAlias, connections: connection, supportedErr: unavailable}, wantErr: unavailable, wantFailed: true, wantBookID: "book-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Title"}}}}
			result, err := newRefreshService(tc.store, reader).RefreshEntry(context.Background(), "alice", "book-1")
			require.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, tc.wantFailed, result.Failed)
			assert.False(t, result.Missing)
			assert.Equal(t, tc.wantBookID, result.Book.ID)
			assert.Zero(t, reader.reads, "a rejected refresh read catalogue feeds")
		})
	}
}

// TestRefreshEntrySurfacesPersistFailures pins the capability-comparison and
// persist branches through the public seam, so splitting them into private
// functions cannot silently change an error or result shape.
func TestRefreshEntrySurfacesPersistFailures(t *testing.T) {
	book := domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"}
	connection := []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}}
	validAlias := domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"}
	feed := opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}

	t.Run("capabilities fail", func(t *testing.T) {
		unavailable := errors.New("capabilities unavailable")
		store := &refreshStore{book: book, alias: validAlias, connections: connection, supported: []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}}}
		reader := &refreshReader{feed: feed}
		result, err := newRefreshServiceWithCapabilities(store, reader, erroringCapabilities{err: unavailable}).RefreshEntry(context.Background(), "alice", "book-1")
		require.ErrorIs(t, err, unavailable)
		assert.True(t, result.Failed)
		assert.Equal(t, "book-1", result.Book.ID)
		assert.Zero(t, reader.reads)
	})

	t.Run("reconcile fails", func(t *testing.T) {
		unavailable := errors.New("reconcile unavailable")
		store := &refreshStore{book: book, alias: validAlias, connections: connection, supported: []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}}, reconcileErr: unavailable}
		reader := &refreshReader{feed: feed}
		result, err := newRefreshService(store, reader).RefreshEntry(context.Background(), "alice", "book-1")
		require.ErrorIs(t, err, unavailable)
		assert.True(t, result.Failed)
		assert.Equal(t, "book-1", result.Book.ID)
		assert.Equal(t, 1, reader.reads)
	})

	t.Run("cover fails", func(t *testing.T) {
		unavailable := errors.New("cover unavailable")
		store := &recordingCoverStore{
			book:        book,
			alias:       validAlias,
			connections: connection,
			supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}},
			reconcile:   persistence.CatalogueEntryReconcileResult{Book: domain.Book{ID: "book-1", OwnerID: "alice", LanguageTag: "de"}},
			recordErr:   unavailable,
		}
		reader := &refreshReader{feed: feed}
		service := NewService(StoreDependencies{Connections: store, Catalogue: store, Aliases: store, Statuses: store}, nil, reader, nil)
		result, err := service.RefreshEntry(context.Background(), "alice", "book-1")
		require.ErrorIs(t, err, unavailable)
		assert.True(t, result.Failed)
		assert.Equal(t, "book-1", result.Book.ID)
	})
}

// TestRefreshEntryUsesCapabilityScopes pins the eligible-language branch used
// when a capability provider is configured.
func TestRefreshEntryUsesCapabilityScopes(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}},
		reconcile:   persistence.CatalogueEntryReconcileResult{Book: domain.Book{ID: "book-1", OwnerID: "alice", Title: "New title", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, TitleChanged: true},
	}
	reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "New title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}}
	capabilities := stubCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
	result, err := newRefreshServiceWithCapabilities(store, reader, capabilities).RefreshEntry(context.Background(), "alice", "book-1")
	require.NoError(t, err)
	assert.True(t, result.Updated)
	assert.False(t, result.Missing)
	assert.False(t, result.Failed)
	assert.Equal(t, 1, store.reconciles)
}

type stubCapabilities struct{ value analyzer.Capabilities }

func (s stubCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return s.value, nil
}

type erroringCapabilities struct{ err error }

func (c erroringCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return analyzer.Capabilities{}, c.err
}

// newRefreshServiceWithCapabilities builds the service with a capability
// provider so the eligible-language branch is exercised through the public seam.
func newRefreshServiceWithCapabilities(store *refreshStore, reader catalogueReader, capabilities analyzer.CapabilityProvider) *Service {
	return NewService(StoreDependencies{Connections: store, Catalogue: store, Aliases: store, Statuses: store}, nil, reader, capabilities)
}

// recordingCoverStore captures advertisement decisions and can fail them to
// prove cover work never fails catalog sync.
type recordingCoverStore struct {
	refreshStore
	advertisements []string
	recordErr      error
}

func (s *recordingCoverStore) RecordBookCoverAdvertisement(_ context.Context, _, _, _, sourceIdentifier string, advertised bool) (bool, time.Time, error) {
	s.advertisements = append(s.advertisements, sourceIdentifier)
	if s.recordErr != nil {
		return false, time.Time{}, s.recordErr
	}
	return advertised, time.Now(), nil
}
func (s *recordingCoverStore) SaveBookCover(context.Context, string, string, string, string, time.Time, string, int, int, string, []byte) error {
	return nil
}
func (s *recordingCoverStore) MarkBookCoverUnavailable(context.Context, string, string, string, string, time.Time, string) error {
	return nil
}
func (s *recordingCoverStore) GetBookCoverForRetrieval(context.Context, string, string) (domain.BookCoverRetrieval, error) {
	return domain.BookCoverRetrieval{}, persistence.ErrNotFound
}

func TestSyncRecordsCoverAdvertisementsAfterSuccessAndNeverFailsOnCoverErrors(t *testing.T) {
	entry := opds.Entry{ID: "entry-1", Title: "Title", Links: []opds.Link{
		{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/one.epub"},
		{Rel: opds.CoverImageRel, Type: "image/png", Href: "https://catalog.example/one.png"},
	}}
	store := &recordingCoverStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		connections: []domain.OpdsConnection{{ID: "connection-1", OwnerID: "alice"}},
		reconcile:   persistence.CatalogueEntryReconcileResult{Book: domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"}}}
	reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{entry}}}
	worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: stubCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}}

	require.NoError(t, worker.Work(context.Background(), &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: "alice", ConnectionID: "connection-1"}}))
	assert.Equal(t, []string{"entry-1"}, store.advertisements)

	store.advertisements = nil
	store.recordErr = errors.New("cover store unavailable")
	require.NoError(t, worker.Work(context.Background(), &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: "alice", ConnectionID: "connection-1"}}), "cover recording failure failed catalog sync")
	assert.Equal(t, []string{"entry-1"}, store.advertisements)
}
