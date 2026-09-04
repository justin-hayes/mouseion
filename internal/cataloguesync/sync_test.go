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
)

func TestEligibleLanguagesUsesReadyCatalogueLanguagesAndExcludesEnglish(t *testing.T) {
	languages := opds.Feed{Entries: []opds.Entry{
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}},
		{Title: "English", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/8"}}},
		{Title: "French", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/10"}}},
	}}
	capabilities := analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de-DE", DisplayName: "German", Ready: true},
		{Language: "en", DisplayName: "English", Ready: true},
		{Language: "it", Ready: false},
		{Language: "fr", DisplayName: "French", Ready: true},
		{Language: "es", DisplayName: "Spanish", Ready: true},
	}}
	got := eligibleLanguages(languages, capabilities)
	if len(got) != 2 || got[0].capability.Language != "de-DE" || got[0].languageID != "7" || got[1].capability.Language != "fr" || got[1].languageID != "10" {
		t.Fatalf("eligible languages=%+v", got)
	}
}

func TestSyncArgsNeverSerializeCredentials(t *testing.T) {
	args := SyncArgs{OwnerID: "owner", ConnectionID: "connection"}
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "password") || strings.Contains(text, "secret") {
		t.Fatalf("sync args contain credentials: %s", text)
	}
}

func TestSafeSyncErrorIsActionableWithoutCredential(t *testing.T) {
	secret := "plain-password-must-not-escape"
	err := safeSyncError(errors.New("opds: HTTP 401 Unauthorized: "+secret), domain.OpdsConnection{Name: "Home", URL: "https://catalog.example/opds", Password: secret})
	if !strings.Contains(err.Error(), "authentication failed") || strings.Contains(err.Error(), secret) {
		t.Fatalf("safe error=%q", err)
	}
}

func TestWorkerUnavailableDoesNotExposeInput(t *testing.T) {
	var worker *Worker
	if err := worker.Work(context.Background(), nil); err == nil || strings.Contains(err.Error(), "password") {
		t.Fatalf("worker error=%v", err)
	}
}

type refreshStore struct {
	book        domain.Book
	alias       domain.BookAlias
	connections []domain.OpdsConnection
	supported   []domain.SupportedLanguage
	reconciles  int
	lastOwner   string
	reconcile   persistence.CatalogueEntryReconcileResult
	aliasErr    error
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
func (s *refreshStore) GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error) {
	return domain.OpdsConnection{}, persistence.ErrNotFound
}
func (s *refreshStore) ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error) {
	return append([]domain.OpdsConnection(nil), s.connections...), nil
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
func (s *refreshStore) ReconcileCatalogueEntry(_ context.Context, owner, sourceIdentifier, title, language string) (persistence.CatalogueEntryReconcileResult, error) {
	s.reconciles++
	s.lastOwner = owner
	if s.reconcile.Book.ID == "" {
		s.reconcile.Book = s.book
	}
	if s.reconcile.Book.Title == "" {
		s.reconcile.Book.Title = title
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
	languages opds.Feed
	feed      opds.Feed
	err       error
	reads     int
}

func (r *refreshReader) Languages(context.Context, string, string) (opds.Feed, error) {
	if r.err != nil {
		return opds.Feed{}, r.err
	}
	if len(r.languages.Entries) > 0 {
		return r.languages, nil
	}
	return opds.Feed{Entries: []opds.Entry{{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}}, nil
}
func (r *refreshReader) BrowseLanguage(context.Context, string, string, string) (opds.Feed, error) {
	r.reads++
	if r.err != nil {
		return opds.Feed{}, r.err
	}
	return r.feed, nil
}

func newRefreshService(store *refreshStore, reader *refreshReader) *Service {
	return &Service{store: store, reader: reader}
}

func TestRefreshEntryOutcomesAreOwnerScopedAndMetadataOnly(t *testing.T) {
	entry := func(title string) opds.Feed {
		return opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: title, Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}
	}
	cases := []struct {
		name                                 string
		feed                                 opds.Feed
		readerErr                            error
		titleChange                          bool
		wantUpdated, wantMissing, wantFailed bool
		wantReconciles                       int
	}{
		{name: "updated", feed: entry("New title"), titleChange: true, wantUpdated: true, wantReconciles: 1},
		{name: "unchanged", feed: entry("Old title"), wantReconciles: 1},
		{name: "missing", feed: opds.Feed{}, wantMissing: true},
		{name: "failed", readerErr: errors.New("upstream secret details"), wantFailed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &refreshStore{
				book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
				alias:       domain.BookAlias{BookID: "book-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
				connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
				supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}},
				reconcile:   persistence.CatalogueEntryReconcileResult{TitleChanged: tc.titleChange, Book: domain.Book{ID: "book-1", OwnerID: "alice", Title: "New title", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
			}
			result, err := newRefreshService(store, &refreshReader{feed: tc.feed, err: tc.readerErr}).RefreshEntry(context.Background(), "alice", "book-1")
			if err != nil {
				t.Fatal(err)
			}
			if result.Updated != tc.wantUpdated || result.Missing != tc.wantMissing || result.Failed != tc.wantFailed || store.reconciles != tc.wantReconciles || (tc.wantReconciles > 0 && store.lastOwner != "alice") {
				t.Fatalf("result=%+v reconciles=%d owner=%q", result, store.reconciles, store.lastOwner)
			}
		})
	}
}

func TestRefreshEntryRejectsCrossOwnerBookWithoutReadingCatalogue(t *testing.T) {
	store := &refreshStore{book: domain.Book{ID: "book-1", OwnerID: "alice"}, alias: domain.BookAlias{BookID: "book-1", Value: "entry-1"}}
	reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "should not read"}}}}
	_, err := newRefreshService(store, reader).RefreshEntry(context.Background(), "bob", "book-1")
	if !errors.Is(err, ErrNotFound) || reader.reads != 0 {
		t.Fatalf("cross-owner refresh err=%v catalogue reads=%d", err, reader.reads)
	}
}

func TestRefreshEntryRepeatedUnchangedMetadataIsIdempotent(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Same title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}},
		reconcile:   persistence.CatalogueEntryReconcileResult{Book: domain.Book{ID: "book-1", OwnerID: "alice", Title: "Same title", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
	}
	reader := &refreshReader{feed: opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Same title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType}}}}}}
	service := newRefreshService(store, reader)
	first, err := service.RefreshEntry(context.Background(), "alice", "book-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RefreshEntry(context.Background(), "alice", "book-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Updated || second.Updated || first.Created || second.Created || store.reconciles != 2 {
		t.Fatalf("repeated refresh first=%+v second=%+v reconciles=%d", first, second, store.reconciles)
	}
}

func TestRefreshEntryUsesSupportedLanguageDisplayName(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "Deutsch"}},
		reconcile:   persistence.CatalogueEntryReconcileResult{Book: domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
	}
	reader := &refreshReader{
		languages: opds.Feed{Entries: []opds.Entry{{Title: "Deutsch", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}},
		feed:      opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Old title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}},
	}
	result, err := newRefreshService(store, reader).RefreshEntry(context.Background(), "alice", "book-1")
	if err != nil || result.Missing || result.Failed || store.reconciles != 1 {
		t.Fatalf("result=%+v err=%v reconciles=%d", result, err, store.reconciles)
	}
}

func TestFindAcquisitionTargetUsesSupportedLanguageDisplayName(t *testing.T) {
	store := &refreshStore{
		book:        domain.Book{ID: "book-1", OwnerID: "alice", Title: "Old title", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		alias:       domain.BookAlias{BookID: "book-1", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "entry-1"},
		connections: []domain.OpdsConnection{{ID: "connection-1", Name: "Home"}},
		supported:   []domain.SupportedLanguage{{Language: "de", DisplayName: "Deutsch"}},
	}
	reader := &refreshReader{
		languages: opds.Feed{Entries: []opds.Entry{{Title: "Deutsch", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}}}},
		feed:      opds.Feed{Entries: []opds.Entry{{ID: "entry-1", Title: "Old title", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}},
	}
	target, err := newRefreshService(store, reader).FindAcquisitionTarget(context.Background(), "alice", "book-1")
	if err != nil || target.Href != "https://catalog.example/book.epub" || target.Language != "de" {
		t.Fatalf("target=%+v err=%v", target, err)
	}
}
