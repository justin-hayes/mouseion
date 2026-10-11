package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

var fixtureCoverPNG = []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 31, 21, 196, 137, 0, 0, 0, 13, 73, 68, 65, 84, 120, 156, 99, 248, 207, 192, 240, 31, 0, 5, 0, 1, 255, 137, 153, 61, 29, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130}

func fixtureCover(bookID string) domain.BookCover {
	switch bookID {
	case BookID, SourceID, routeMatchBookID, ItalianGoalBookID:
		return domain.BookCover{State: domain.BookCoverAvailable, Width: 600, Height: 900}
	case "fixture-failed", routeTieBBookID:
		return domain.BookCover{State: domain.BookCoverPending}
	case routeTieABookID:
		return domain.BookCover{State: domain.BookCoverUnavailable}
	default:
		return domain.BookCover{State: domain.BookCoverNone}
	}
}

func (s *Store) ListSourceMaterials(_ context.Context, owner string) ([]domain.SourceMaterialSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.SourceMaterialSummary
	for _, book := range s.books {
		if book.Source.OwnerID == owner {
			if book.BookTitle == "" {
				book.BookTitle = book.Source.Title
			}
			result = append(result, book)
		}
	}
	return result, nil
}

func (s *Store) ListMyBooksWithEvidence(_ context.Context, owner string) ([]domain.MyBook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.myBooksForOwner(owner), nil
}

// GetBookDetailForMyBooksRefresh returns the same complete margin evidence as
// GetBookDetail: the in-memory fixture never defers the evidence the
// production refresh read loads lazily.
func (s *Store) GetBookDetailForMyBooksRefresh(ctx context.Context, owner, id string) (domain.MyBook, error) {
	return s.GetBookDetail(ctx, owner, id)
}

func (s *Store) GetBookDetail(_ context.Context, owner, id string) (domain.MyBook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	books := s.myBooksForOwner(owner)
	for _, book := range books {
		if book.Book.ID == id {
			return book, nil
		}
	}
	for _, book := range books {
		if book.Acquired != nil && book.Acquired.Source.ID == id {
			return book, nil
		}
	}
	return domain.MyBook{}, errNotFound
}

func (s *Store) myBooksForOwner(owner string) []domain.MyBook {
	out := make([]domain.MyBook, 0, len(s.books)+len(s.myBooks))
	for i := range s.books {
		source := s.books[i]
		if owner != "" && source.Source.OwnerID != owner {
			continue
		}
		languageState := domain.LanguageChosen
		languageTag := normalizeFixtureLanguage(source.Source.Language)
		if strings.TrimSpace(languageTag) == "" {
			languageState = domain.LanguageUnknown
			languageTag = ""
		}
		bookID := source.BookID
		if bookID == "" {
			bookID = source.Source.ID
		}
		if source.BookTitle == "" {
			source.BookTitle = source.Source.Title
		}
		out = append(out, domain.MyBook{Book: domain.Book{ID: bookID, OwnerID: source.Source.OwnerID, Title: source.BookTitle, Author: source.BookAuthor, LanguageState: languageState, LanguageTag: languageTag}, Cover: fixtureCover(bookID), Acquired: &source, Disposition: s.bookDispositionLocked(source.Source.OwnerID, bookID), DispositionRevision: s.bookDispositionRevisionLocked(source.Source.OwnerID, bookID)})
		s.applyVisibilityLocked(&out[len(out)-1])
	}
	for _, book := range s.myBooks {
		if owner == "" || book.Book.OwnerID == owner {
			book.Disposition = s.bookDispositionLocked(book.Book.OwnerID, book.Book.ID)
			book.DispositionRevision = s.bookDispositionRevisionLocked(book.Book.OwnerID, book.Book.ID)
			s.applyVisibilityLocked(&book)
			out = append(out, book)
		}
	}
	for i := range out {
		if out[i].Book.ID == BookID {
			out[i].CoverageKnownTokens = 974
			out[i].CoverageTotalTokens = 1000
			out[i].DeckState = "ready"
			out[i].DeckCardCount = 412
			preparedAt := fixtureJourneyTime
			out[i].DeckPreparedAt = &preparedAt
		}
		for _, reading := range s.currentReadings {
			if reading.OwnerID == out[i].Book.OwnerID && reading.BookID == out[i].Book.ID {
				out[i].IsCurrentReading = true
				break
			}
		}
		if imported, ok := s.importedHistory[fixtureDispositionKey(out[i].Book.OwnerID, out[i].Book.ID)]; ok {
			out[i].CompletionCount++
			out[i].LatestCompletionAt = &imported.CompletedAt
			out[i].LatestCompletionSource = domain.ReadingCompletionPreviouslyRead
		}
		for _, completion := range s.readingHistory {
			if completion.OwnerID != out[i].Book.OwnerID || completion.BookID != out[i].Book.ID {
				continue
			}
			out[i].CompletionCount++
			if out[i].LatestCompletionAt == nil || completion.CompletedAt.After(*out[i].LatestCompletionAt) {
				completedAt := completion.CompletedAt
				out[i].LatestCompletionAt = &completedAt
				out[i].LatestCompletionSource = completion.Source
			}
		}
	}
	return out
}

func fixtureDispositionKey(owner, bookID string) string { return owner + "\x00" + bookID }

// fixtureVisibility mirrors one book_visibility row; a missing row is a
// visible Book at revision 0.
type fixtureVisibility struct {
	hidden   bool
	revision int64
}

func (s *Store) applyVisibilityLocked(book *domain.MyBook) {
	state := s.visibility[fixtureDispositionKey(book.Book.OwnerID, book.Book.ID)]
	book.Hidden, book.VisibilityRevision = state.hidden, state.revision
}

func (s *Store) GetBookVisibility(_ context.Context, owner, bookID string) (bool, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.fixtureBookExists(owner, bookID) {
		return false, 0, errNotFound
	}
	state := s.visibility[fixtureDispositionKey(owner, bookID)]
	return state.hidden, state.revision, nil
}

// SetBookHidden mirrors the production expected-revision protocol and writes
// only the visibility state.
func (s *Store) SetBookHidden(_ context.Context, owner, bookID string, expectedRevision int64, hidden bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.fixtureBookExists(owner, bookID) {
		return false, errNotFound
	}
	key := fixtureDispositionKey(owner, bookID)
	state := s.visibility[key]
	if state.revision == expectedRevision+1 && state.hidden == hidden {
		return false, nil
	}
	if state.revision != expectedRevision {
		return false, persistence.ErrStaleBookVisibility
	}
	if state.hidden == hidden {
		return false, nil
	}
	s.visibility[key] = fixtureVisibility{hidden: hidden, revision: expectedRevision + 1}
	return true, nil
}

func (s *Store) bookDispositionLocked(owner, bookID string) domain.BookDisposition {
	if disposition, ok := s.dispositions[fixtureDispositionKey(owner, bookID)]; ok {
		return disposition
	}
	return domain.BookDispositionInbox
}

func (s *Store) bookDispositionRevisionLocked(owner, bookID string) int64 {
	if revision := s.dispositionRevisions[fixtureDispositionKey(owner, bookID)]; revision > 0 {
		return revision
	}
	return 1
}

// ListMyBooksBrowse mirrors the production collection browser in memory for
// the shared browser fixture store: literal case-insensitive title substring
// search, one language filter, lowercased deterministic title ordering, and
// counts over the complete active owner collection.
func (s *Store) ListMyBooksBrowse(ctx context.Context, owner, query, language, disposition string, history bool, offset, limit int) (persistence.MyBooksBrowseResult, error) {
	return s.ListMyBooksBrowseWithVisibility(ctx, owner, query, language, disposition, history, false, offset, limit)
}

// ListMyBooksBrowseWithVisibility mirrors the production visibility scope:
// Hidden Books are omitted unless showHidden is set and all counts follow it.
func (s *Store) ListMyBooksBrowseWithVisibility(_ context.Context, owner, query, language, disposition string, history, showHidden bool, offset, limit int) (persistence.MyBooksBrowseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query = strings.ToLower(strings.TrimSpace(query))
	language = strings.TrimSpace(language)
	disposition = strings.TrimSpace(disposition)
	if language != domain.LanguageUnknown {
		language = normalizeFixtureLanguage(language)
	}
	everything := s.myBooksForOwner(owner)
	result := persistence.MyBooksBrowseResult{AllCount: len(everything)}
	all := make([]domain.MyBook, 0, len(everything))
	for _, book := range everything {
		inLanguage := language == "" || (language == domain.LanguageUnknown && book.Book.LanguageState == domain.LanguageUnknown) || (language != domain.LanguageUnknown && book.Book.LanguageState == domain.LanguageChosen && normalizeFixtureLanguage(book.Book.LanguageTag) == language)
		if book.Hidden && inLanguage {
			result.HiddenCount++
		}
		if book.Hidden && !showHidden {
			continue
		}
		all = append(all, book)
	}
	counts := map[string]int{}
	for _, book := range all {
		tag := domain.LanguageUnknown
		if book.Book.LanguageState == domain.LanguageChosen {
			tag = normalizeFixtureLanguage(book.Book.LanguageTag)
		}
		counts[tag]++
	}
	for tag, count := range counts {
		result.Counts = append(result.Counts, persistence.LanguageCount{Tag: tag, Count: count})
	}
	dispositionCounts := map[domain.BookDisposition]int{}
	for _, book := range all {
		if language == "" || (language == domain.LanguageUnknown && book.Book.LanguageState == domain.LanguageUnknown) || (language != domain.LanguageUnknown && book.Book.LanguageState == domain.LanguageChosen && normalizeFixtureLanguage(book.Book.LanguageTag) == language) {
			bucket := book.WorkflowBucket()
			if bucket == domain.MyBookBucketRead {
				result.ReadCount++
			}
			if disposition, ok := bucket.PersistedDisposition(); ok {
				dispositionCounts[disposition]++
			}
			if bucket == domain.MyBookBucketCurrentReading {
				dispositionCounts[domain.BookDispositionToRead]++
			}
		}
	}
	for disposition, count := range dispositionCounts {
		result.DispositionCounts = append(result.DispositionCounts, persistence.DispositionCount{Disposition: disposition, Count: count})
	}
	sort.Slice(result.DispositionCounts, func(i, j int) bool {
		return result.DispositionCounts[i].Disposition < result.DispositionCounts[j].Disposition
	})
	sort.Slice(result.Counts, func(i, j int) bool {
		if result.Counts[i].Tag == domain.LanguageUnknown {
			return false
		}
		if result.Counts[j].Tag == domain.LanguageUnknown {
			return true
		}
		return result.Counts[i].Tag < result.Counts[j].Tag
	})

	filtered := make([]domain.MyBook, 0, len(all))
	for _, book := range all {
		if language == "" || (language == domain.LanguageUnknown && book.Book.LanguageState == domain.LanguageUnknown) || (language != domain.LanguageUnknown && book.Book.LanguageState == domain.LanguageChosen && normalizeFixtureLanguage(book.Book.LanguageTag) == language) {
			result.ScopeTotal++
		}
		if query != "" && !strings.Contains(strings.ToLower(book.Book.Title), query) {
			continue
		}
		if language == domain.LanguageUnknown {
			if book.Book.LanguageState != domain.LanguageUnknown {
				continue
			}
		} else if language != "" && (book.Book.LanguageState != domain.LanguageChosen || normalizeFixtureLanguage(book.Book.LanguageTag) != language) {
			continue
		}
		if !book.WorkflowBucket().MatchesBrowseFilter(domain.BookDisposition(disposition), history) {
			continue
		}
		filtered = append(filtered, book)
	}
	sort.Slice(filtered, func(i, j int) bool {
		left, right := filtered[i].Book, filtered[j].Book
		leftTitle, rightTitle := strings.ToLower(left.Title), strings.ToLower(right.Title)
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}
		if left.Title != right.Title {
			return left.Title < right.Title
		}
		return left.ID < right.ID
	})
	result.Total = len(filtered)
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	if offset < len(filtered) {
		end := offset + limit
		if end < offset || end > len(filtered) {
			end = len(filtered)
		}
		result.Items = filtered[offset:end]
	}
	return result, nil
}

func (s *Store) IsMetadataOnlyMyBook(_ context.Context, owner, bookID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, source := range s.books {
		if source.Source.OwnerID == owner && (source.Source.ID == bookID || source.BookID == bookID) {
			return false, nil
		}
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == bookID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) GetSourceMaterial(_ context.Context, o, id string) (domain.SourceMaterial, error) {
	for _, b := range s.books {
		if b.Source.ID == id && b.Source.OwnerID == o {
			return b.Source, nil
		}
	}
	return domain.SourceMaterial{}, errNotFound
}

// My Books persistence is intentionally small in the browser fixture; these
// methods cover the learner-facing metadata controls without a database.
func (s *Store) ListMyBooks(context.Context, string) ([]domain.Book, error) { return nil, nil }

func (s *Store) GetBookCoverResource(_ context.Context, owner, bookID string) (domain.BookCoverResource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, book := range s.myBooksForOwner(owner) {
		if book.Book.ID == bookID && book.Cover.State == domain.BookCoverAvailable {
			return domain.BookCoverResource{OwnerID: owner, BookID: bookID, MediaType: "image/png", ContentHash: "fixture-cover-v1", Bytes: append([]byte(nil), fixtureCoverPNG...), Width: book.Cover.Width, Height: book.Cover.Height}, nil
		}
	}
	return domain.BookCoverResource{}, errNotFound
}

func (s *Store) GetActiveBookCoverResource(ctx context.Context, owner, bookID string) (domain.BookCoverResource, error) {
	return s.GetBookCoverResource(ctx, owner, bookID)
}

func (s *Store) GetBook(_ context.Context, owner, bookID string) (domain.Book, error) {
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == bookID {
			return book.Book, nil
		}
	}
	for _, source := range s.books {
		if source.Source.OwnerID == owner && (source.Source.ID == bookID || source.BookID == bookID) {
			state := domain.LanguageChosen
			if strings.TrimSpace(source.Source.Language) == "" {
				state = domain.LanguageUnknown
			}
			resolvedBookID := source.BookID
			if resolvedBookID == "" {
				resolvedBookID = source.Source.ID
			}
			title := source.BookTitle
			if title == "" {
				title = source.Source.Title
			}
			return domain.Book{ID: resolvedBookID, OwnerID: owner, Title: title, Author: source.BookAuthor, LanguageState: state, LanguageTag: source.Source.Language}, nil
		}
	}
	return domain.Book{}, errNotFound
}

func (s *Store) CreateBook(_ context.Context, book domain.Book) (domain.Book, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	book.ID = fmt.Sprintf("fixture-metadata-%d", len(s.myBooks)+1)
	s.myBooks = append(s.myBooks, domain.MyBook{Book: book})
	s.dispositions[fixtureDispositionKey(book.OwnerID, book.ID)] = domain.BookDispositionInbox
	return book, nil
}

func (s *Store) GetBookDisposition(_ context.Context, owner, bookID string) (domain.BookDisposition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.fixtureBookExists(owner, bookID) {
		return "", errNotFound
	}
	return s.bookDispositionLocked(owner, bookID), nil
}

func (s *Store) SetBookDisposition(_ context.Context, owner, bookID string, disposition domain.BookDisposition) error {
	if err := disposition.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.fixtureBookExists(owner, bookID) {
		return errNotFound
	}
	key := fixtureDispositionKey(owner, bookID)
	if s.bookDispositionLocked(owner, bookID) != disposition {
		s.dispositionRevisions[key] = s.bookDispositionRevisionLocked(owner, bookID) + 1
	}
	s.dispositions[key] = disposition
	return nil
}

func (s *Store) TransitionBookDisposition(_ context.Context, owner, bookID string, expectedRevision int64, disposition domain.BookDisposition) (bool, error) {
	if err := disposition.Validate(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.fixtureBookExists(owner, bookID) {
		return false, errNotFound
	}
	key := fixtureDispositionKey(owner, bookID)
	revision := s.bookDispositionRevisionLocked(owner, bookID)
	current := s.bookDispositionLocked(owner, bookID)
	if revision == expectedRevision+1 && current == disposition {
		return false, nil
	}
	if revision != expectedRevision {
		return false, persistence.ErrStaleBookDisposition
	}
	s.dispositions[key] = disposition
	s.dispositionRevisions[key] = revision + 1
	return true, nil
}

func (s *Store) UpdateBookMetadata(_ context.Context, owner, bookID, title, author, languageState, languageTag string) (domain.Book, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if languageState == domain.LanguageChosen {
		languageTag = canonicalization.NormalizeLanguage(languageTag)
	}
	for i := range s.books {
		if s.books[i].Source.OwnerID == owner && (s.books[i].Source.ID == bookID || s.books[i].BookID == bookID) {
			resolvedBookID := s.books[i].BookID
			if resolvedBookID == "" {
				resolvedBookID = s.books[i].Source.ID
			}
			s.books[i].BookTitle = title
			s.books[i].BookAuthor = strings.TrimSpace(author)
			if languageState == domain.LanguageUnknown {
				languageTag = ""
			}
			s.books[i].Source.Language = languageTag
			s.clearFixtureGoalsForRetaggedBook(owner, resolvedBookID, languageState, languageTag)
			return domain.Book{ID: resolvedBookID, OwnerID: owner, Title: title, Author: strings.TrimSpace(author), LanguageState: languageState, LanguageTag: languageTag}, nil
		}
	}
	for i := range s.myBooks {
		if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == bookID {
			s.myBooks[i].Book.Title = title
			s.myBooks[i].Book.Author = strings.TrimSpace(author)
			s.myBooks[i].Book.LanguageState = languageState
			s.myBooks[i].Book.LanguageTag = languageTag
			s.clearFixtureGoalsForRetaggedBook(owner, bookID, languageState, languageTag)
			return s.myBooks[i].Book, nil
		}
	}
	return domain.Book{}, errNotFound
}

func (s *Store) clearFixtureGoalsForRetaggedBook(owner, bookID, languageState, language string) {
	language = normalizeFixtureLanguage(language)
	for key, goal := range s.currentReadings {
		if goal.OwnerID != owner || goal.BookID != bookID {
			continue
		}
		if languageState != domain.LanguageChosen || goal.Language != language {
			delete(s.currentReadings, key)
		}
	}
}

func (s *Store) AddBookToMyBooks(context.Context, string, string) error { return nil }

func (s *Store) RemoveBookFromMyBooks(_ context.Context, owner, bookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.myBooks {
		if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == bookID {
			s.myBooks = append(s.myBooks[:i], s.myBooks[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *Store) ResolveBookByAlias(ctx context.Context, owner, namespace, value string) (domain.Book, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, alias := range s.aliases {
		if alias.OwnerID == owner && alias.Namespace == namespace && alias.Value == value {
			for _, book := range s.myBooksForOwner(owner) {
				if book.Book.ID == alias.BookID {
					return book.Book, true, nil
				}
			}
		}
	}
	return domain.Book{}, false, nil
}

func (s *Store) AddBookAlias(context.Context, string, string, string, string, string) error {
	return nil
}

func (s *Store) GetBookCatalogEntryAlias(_ context.Context, owner, bookID string) (domain.BookAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, alias := range s.aliases {
		if alias.OwnerID == owner && alias.BookID == bookID && alias.AliasType == domain.AliasCatalogEntry && alias.Namespace == domain.NamespaceSourceIdentifier {
			return alias, nil
		}
	}
	return domain.BookAlias{}, errNotFound
}

func (s *Store) LinkSourceToBook(context.Context, string, string, string) error { return nil }

func (s *Store) ResolveOrCreateBookForAcquisition(context.Context, string, string, string, string) (string, error) {
	return "", errNotFound
}

func (s *Store) ResolveBookID(_ context.Context, owner, id string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bookID := s.fixtureBookID(owner, id)
	return bookID, bookID != "", nil
}

func (s *Store) fixtureBookExists(owner, bookID string) bool {
	return s.fixtureBookID(owner, bookID) != ""
}

func (s *Store) fixtureBookID(owner, id string) string {
	for _, source := range s.books {
		if source.Source.OwnerID == owner && (source.Source.ID == id || source.BookID == id) {
			if source.BookID != "" {
				return source.BookID
			}
			return source.Source.ID
		}
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == id {
			return book.Book.ID
		}
	}
	return ""
}
