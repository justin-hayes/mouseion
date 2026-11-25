package cataloguesync

import (
	"context"
	"encoding/json"
	"errors"
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

func TestEntryCoverArgsNeverSerializeCredentialsOrURLs(t *testing.T) {
	args := EntryCoverArgs{OwnerID: "owner", ConnectionID: "connection", SourceIdentifier: "entry-1"}
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
	book        domain.Book
	alias       domain.BookAlias
	connections []domain.OpdsConnection
	// updateDefaults are applied to the metadata update's result before it is
	// returned, so sync tests can express a linked Book and its changes.
	update         persistence.CatalogueBookMetadataResult
	updateCalls    int
	lastOwner      string
	lastConnection string
	aliasErr       error
	// catalogueEntry is the Catalog index row a Book's acquisition resolves.
	// A zero EntryIdentifier means the index no longer holds the entry.
	catalogueEntry    domain.CatalogueIndexEntry
	catalogueEntryErr error
	catalogueReads    int
	// Error injection pins the failure branches of each step.
	connectionsErr error
	bookErr        error
	updateErr      error
	indexErr       error
	indexRuns      int
	indexEntries   []domain.CatalogueIndexEntry
	indexObserved  time.Time
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
func (s *refreshStore) OpdsConnectionExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *refreshStore) ListAllOpdsConnectionIDs(context.Context) ([]domain.OpdsConnection, error) {
	return nil, nil
}
func (s *refreshStore) SyncSupportedLanguages(context.Context, []domain.SupportedLanguage) error {
	return nil
}
func (s *refreshStore) UpdateCatalogueBookMetadata(_ context.Context, owner, connectionID, sourceIdentifier, title, author string) (persistence.CatalogueBookMetadataResult, error) {
	s.updateCalls++
	s.lastOwner = owner
	s.lastConnection = connectionID
	if sourceIdentifier == "" {
		return persistence.CatalogueBookMetadataResult{}, errors.New("missing source identifier")
	}
	if s.updateErr != nil {
		return persistence.CatalogueBookMetadataResult{}, s.updateErr
	}
	result := s.update
	if result.Book.ID == "" {
		result.Book = s.book
	}
	if result.Book.Title == "" {
		result.Book.Title = title
	}
	if result.Book.Author == "" {
		result.Book.Author = author
	}
	return result, nil
}
func (s *refreshStore) ReconcileCatalogueIndex(_ context.Context, _ string, _ string, entries []domain.CatalogueIndexEntry, observedAt time.Time) error {
	if s.indexErr != nil {
		return s.indexErr
	}
	s.indexRuns++
	s.indexEntries = append([]domain.CatalogueIndexEntry(nil), entries...)
	s.indexObserved = observedAt
	return nil
}
func (s *refreshStore) GetCatalogueEntry(_ context.Context, owner, connectionID, entryIdentifier string) (domain.CatalogueIndexEntry, error) {
	s.catalogueReads++
	if s.catalogueEntryErr != nil {
		return domain.CatalogueIndexEntry{}, s.catalogueEntryErr
	}
	entry := s.catalogueEntry
	if entry.EntryIdentifier == "" || entry.ConnectionID != connectionID || entry.EntryIdentifier != entryIdentifier {
		return domain.CatalogueIndexEntry{}, persistence.ErrNotFound
	}
	return entry, nil
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

// conflictStore fails metadata reconciliation for the listed entry identifiers
// so the sync worker's per-entry conflict handling can be pinned through its
// public Work seam.
type conflictStore struct {
	*refreshStore
	failIDs map[string]bool
}

func (s *conflictStore) UpdateCatalogueBookMetadata(ctx context.Context, owner, connectionID, sourceIdentifier, title, author string) (persistence.CatalogueBookMetadataResult, error) {
	if s.failIDs[sourceIdentifier] {
		return persistence.CatalogueBookMetadataResult{}, errors.New("reconcile conflict")
	}
	return s.refreshStore.UpdateCatalogueBookMetadata(ctx, owner, connectionID, sourceIdentifier, title, author)
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

func newRefreshService(store *refreshStore, reader catalogueReader) *Service {
	return NewService(StoreDependencies{Connections: store, Catalogue: store, Statuses: store}, nil, reader, nil)
}

type stubCapabilities struct{ value analyzer.Capabilities }

func (s stubCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return s.value, nil
}

type erroringCapabilities struct{ err error }

func (c erroringCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return analyzer.Capabilities{}, c.err
}

func TestFindAcquisitionTargetReadsTheCatalogIndex(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		catalogueEntry: domain.CatalogueIndexEntry{
			ConnectionID: "connection-1", EntryIdentifier: "entry-1", Title: "Indexed title",
			LanguageTag: "de", EPUBHref: "https://catalog.example/book.epub",
		},
	}
	reader := &refreshReader{}

	target, err := newRefreshService(store, reader).FindAcquisitionTarget(context.Background(), "alice", "book-1")

	require.NoError(t, err)
	t.Run("resolves the stored EPUB link", func(t *testing.T) {
		require.Equal(t, "https://catalog.example/book.epub", target.Href)
		require.Equal(t, []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}, target.Entry.Links)
	})
	t.Run("reports the Book's connection and language", func(t *testing.T) {
		require.Equal(t, "connection-1", target.ConnectionID)
		require.Equal(t, "de", target.Language)
	})
	t.Run("projects the indexed entry identity", func(t *testing.T) {
		require.Equal(t, "entry-1", target.Entry.ID)
		require.Equal(t, "Indexed title", target.Entry.Title)
	})
	t.Run("reads only the Catalog index", func(t *testing.T) {
		require.Equal(t, 1, store.catalogueReads)
		require.Zero(t, reader.reads, "acquisition must not walk a language feed")
		require.Empty(t, reader.connections, "acquisition must not read the language navigation feed")
	})
}

func TestAcquisitionReportsMissingAliasConnection(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "deleted", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "other", Name: "Other"}},
	}
	reader := &refreshReader{}
	_, err := newRefreshService(store, reader).FindAcquisitionTarget(context.Background(), "alice", "book-1")
	require.ErrorIs(t, err, ErrConnectionNotFound)
	assert.Empty(t, reader.connections)
}

// TestFindAcquisitionTargetRejectsIncompleteOrUnreadableSources pins each
// rejection branch of the acquisition lookup through its public seam, so
// splitting the lookup into private steps cannot silently change an error.
func TestFindAcquisitionTargetRejectsIncompleteOrUnreadableSources(t *testing.T) {
	book := domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"}
	connection := []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}}
	validAlias := domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"}
	unavailable := errors.New("store unavailable")
	tests := []struct {
		name    string
		store   *refreshStore
		wantErr error
	}{
		{name: "book not found", store: &refreshStore{}, wantErr: ErrNotFound},
		{name: "book load fails", store: &refreshStore{bookErr: unavailable}, wantErr: unavailable},
		{name: "alias not found", store: &refreshStore{book: book, aliasErr: persistence.ErrNotFound}, wantErr: ErrCatalogEntryAliasMissing},
		{name: "alias load fails", store: &refreshStore{book: book, aliasErr: unavailable}, wantErr: unavailable},
		{name: "blank alias value", store: &refreshStore{book: book, alias: domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier}}, wantErr: ErrCatalogEntryAliasMissing},
		{name: "connection load fails", store: &refreshStore{book: book, alias: validAlias, connectionsErr: unavailable}, wantErr: unavailable},
		{name: "entry absent from the index", store: &refreshStore{book: book, alias: validAlias, connections: connection}, wantErr: ErrCatalogEntryMissing},
		{name: "entry load fails", store: &refreshStore{book: book, alias: validAlias, connections: connection, catalogueEntryErr: unavailable}, wantErr: unavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newRefreshService(tc.store, &refreshReader{}).FindAcquisitionTarget(context.Background(), "alice", "book-1")
			require.ErrorIs(t, err, tc.wantErr)
		})
	}

	t.Run("unavailable service", func(t *testing.T) {
		var service *Service
		_, err := service.FindAcquisitionTarget(context.Background(), "alice", "book-1")
		require.ErrorIs(t, err, ErrNotFound)
	})
}

// TestFindAcquisitionTargetNamesTheMissingEntry pins the identity carried by a
// missing-index failure, which becomes the durable `entry` failure record.
func TestFindAcquisitionTargetNamesTheMissingEntry(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", ConnectionID: "connection-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
	}

	target, err := newRefreshService(store, &refreshReader{}).FindAcquisitionTarget(context.Background(), "alice", "book-1")

	require.ErrorIs(t, err, ErrCatalogEntryMissing)
	assert.Equal(t, "connection-1", target.ConnectionID)
	assert.Equal(t, "de", target.Language)
	assert.Equal(t, "entry-1", target.Entry.ID)
}

// TestSyncWorkReconcilesEntriesAndReportsFirstConflict pins the sync worker's
// index and per-entry metadata phases through its public Work seam: unusable
// entries are skipped, the index is written after a complete traversal, the
// first metadata conflict is reported, and a feed, index, or capability failure
// is surfaced.
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
				update:      persistence.CatalogueBookMetadataResult{Book: book},
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
		require.Equal(t, 1, store.updateCalls)
		require.Equal(t, 1, store.indexRuns)
		require.Len(t, store.indexEntries, 1)
		assert.Equal(t, "good", store.indexEntries[0].EntryIdentifier)
	})

	t.Run("writes the index only after a complete run", func(t *testing.T) {
		store := newStore(nil)
		indexed := entry("entry-1", "First title")
		indexed.Author = "First author"
		indexed.Links = append(indexed.Links, opds.Link{Rel: opds.CoverImageRel, Type: "image/png", Href: "https://catalog.example/cover.png"})
		reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{indexed}}}
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
		require.NoError(t, worker.Work(context.Background(), job()))
		require.Equal(t, 1, store.indexRuns)
		require.Len(t, store.indexEntries, 1)
		got := store.indexEntries[0]
		assert.Equal(t, "entry-1", got.EntryIdentifier)
		assert.Equal(t, "First title", got.Title)
		assert.Equal(t, "First author", got.Author)
		assert.Equal(t, "de", got.LanguageTag)
		assert.Equal(t, "https://catalog.example/cover.png", got.CoverHref)
		assert.Equal(t, "image/png", got.CoverMediaType)
		assert.Equal(t, "https://catalog.example/entry-1.epub", got.EPUBHref)
		assert.False(t, store.indexObserved.IsZero())
	})

	t.Run("reports the first conflict and keeps reconciling", func(t *testing.T) {
		store := newStore(map[string]bool{"bad-1": true, "bad-2": true})
		reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{entry("bad-1", "First bad"), entry("good", "Good"), entry("bad-2", "Second bad")}}}
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
		err := worker.Work(context.Background(), job())
		require.Error(t, err)
		require.Contains(t, err.Error(), `catalog entry "bad-1" could not be reconciled`)
		require.Equal(t, 1, store.updateCalls)
		assert.Equal(t, 1, store.indexRuns, "a complete traversal did not write the index before metadata propagation")
	})

	t.Run("surfaces a feed read failure", func(t *testing.T) {
		store := newStore(nil)
		reader := &refreshReader{languages: opds.Feed{Entries: []opds.Entry{{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}}, browseErr: errors.New("feed unavailable")}
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
		err := worker.Work(context.Background(), job())
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not reach")
		assert.Zero(t, store.indexRuns, "a failed run reconciled the Catalog index")
	})

	t.Run("surfaces an index failure", func(t *testing.T) {
		store := newStore(nil)
		store.indexErr = errors.New("index unavailable")
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: &refreshReader{feed: opds.Feed{Entries: []opds.Entry{entry("good", "Good")}}}, Capabilities: capabilities}
		err := worker.Work(context.Background(), job())
		require.Error(t, err)
		require.Contains(t, err.Error(), "catalog index could not be reconciled")
	})

	t.Run("surfaces a capability failure", func(t *testing.T) {
		store := newStore(nil)
		worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: &refreshReader{}, Capabilities: erroringCapabilities{err: errors.New("capabilities unavailable")}}
		err := worker.Work(context.Background(), job())
		require.Error(t, err)
		require.Contains(t, err.Error(), "NLP language readiness could not be checked")
	})
}
