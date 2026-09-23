package cataloguesync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
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
	args := CoverArgs{OwnerID: "owner", BookID: "book", ConnectionID: "connection"}
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	text := string(encoded)
	assert.NotContains(t, text, "password")
	assert.NotContains(t, text, "secret")
	assert.NotContains(t, text, "http")
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
}

func (s *refreshStore) ListUnscopedCatalogueEntryAliases(context.Context) ([]domain.BookAlias, error) {
	return append([]domain.BookAlias(nil), s.backfillAliases...), nil
}
func (s *refreshStore) SetCatalogueEntryAliasConnection(_ context.Context, owner, aliasID, connectionID string) error {
	if s.assigned == nil {
		s.assigned = make(map[string]string)
	}
	s.assigned[owner+":"+aliasID] = connectionID
	return nil
}

func (s *refreshStore) GetBook(_ context.Context, owner, bookID string) (domain.Book, error) {
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
func (s *refreshStore) GetOpdsConnection(_ context.Context, owner, id string) (domain.OpdsConnection, error) {
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
	return append([]domain.SupportedLanguage(nil), s.supported...), nil
}
func (s *refreshStore) SyncSupportedLanguages(context.Context, []domain.SupportedLanguage) error {
	return nil
}
func (s *refreshStore) ReconcileCatalogueEntry(_ context.Context, owner, connectionID, sourceIdentifier, title, author, language string) (persistence.CatalogueEntryReconcileResult, error) {
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
	languages map[string]opds.Feed
	feeds     map[string]opds.Feed
}

func (r *backfillReader) Languages(_ context.Context, owner, connection string) (opds.Feed, error) {
	return r.languages[owner+":"+connection], nil
}
func (r *backfillReader) BrowseLanguage(_ context.Context, owner, connection, language string) (opds.Feed, error) {
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
			if tc.readerErr != nil {
				assert.ErrorIs(t, err, tc.readerErr) //nolint:testifylint // Error classification and result flags are independent table expectations.
			} else if tc.browseErr != nil {
				assert.ErrorIs(t, err, tc.browseErr) //nolint:testifylint // Error classification and result flags are independent table expectations.
			} else {
				require.NoError(t, err)
			}
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
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Error and reader-call count are independent boundary expectations.
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
	assert.ErrorIs(t, refreshErr, ErrConnectionNotFound) //nolint:testifylint // Refresh and acquisition probe independent missing-connection behavior.
	assert.Empty(t, reader.connections)
	_, acquisitionErr := newRefreshService(store, reader).FindAcquisitionTarget(context.Background(), "alice", "book-1")
	assert.ErrorIs(t, acquisitionErr, ErrConnectionNotFound) //nolint:testifylint // Acquisition is an independent missing-connection behavior check.
	assert.Empty(t, reader.connections)
}
