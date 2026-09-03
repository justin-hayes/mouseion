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

func TestEligibleLanguagesIntersectsSavedReadyAndExcludesEnglish(t *testing.T) {
	profiles := []domain.LanguageProfile{
		{Language: "de-DE", DisplayName: "German"},
		{Language: "en", DisplayName: "English"},
		{Language: "it", DisplayName: "Italian"},
		{Language: "fr", DisplayName: "French"},
	}
	capabilities := analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", Ready: true},
		{Language: "en", Ready: true},
		{Language: "it", Ready: false},
		{Language: "fr", Ready: true},
	}}
	got := eligibleLanguages(profiles, capabilities)
	if len(got) != 2 || got[0].profile.Language != "de-DE" || got[1].profile.Language != "fr" {
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
	profiles    []domain.LanguageProfile
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
func (s *refreshStore) ListLanguageProfiles(context.Context, string) ([]domain.LanguageProfile, error) {
	return append([]domain.LanguageProfile(nil), s.profiles...), nil
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

type refreshReader struct {
	feed  opds.Feed
	err   error
	reads int
}

func (r *refreshReader) Languages(context.Context, string, string) (opds.Feed, error) {
	if r.err != nil {
		return opds.Feed{}, r.err
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
				profiles:    []domain.LanguageProfile{{Language: "de", DisplayName: "German"}},
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
		profiles:    []domain.LanguageProfile{{Language: "de", DisplayName: "German"}},
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
