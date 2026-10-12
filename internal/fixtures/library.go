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
	out := s.fixtureBooksFromSources(owner)
	out = append(out, s.fixtureBooksFromMetadata(owner)...)
	s.applyFixtureBookEvidence(out)
	return out
}

// fixtureBooksFromSources builds the acquired-source entries of an owner's
// collection: one entry per acquired source, scoped to the owner when one is
// given. Visibility is applied here because a source entry is otherwise
// indistinguishable from a metadata entry.
func (s *Store) fixtureBooksFromSources(owner string) []domain.MyBook {
	out := make([]domain.MyBook, 0, len(s.books))
	for i := range s.books {
		source := s.books[i]
		if owner != "" && source.Source.OwnerID != owner {
			continue
		}
		book := s.fixtureMyBookFromSource(source)
		s.applyVisibilityLocked(&book)
		out = append(out, book)
	}
	return out
}

// fixtureMyBookFromSource derives one acquired-source entry. The Book identity
// prefers the resolved Book ID over the source ID, the title falls back to the
// source title, and a blank language is the unchosen state.
func (s *Store) fixtureMyBookFromSource(source domain.SourceMaterialSummary) domain.MyBook {
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
	return domain.MyBook{
		Book:                domain.Book{ID: bookID, OwnerID: source.Source.OwnerID, Title: source.BookTitle, Author: source.BookAuthor, LanguageState: languageState, LanguageTag: languageTag},
		Cover:               fixtureCover(bookID),
		Acquired:            &source,
		Disposition:         s.bookDispositionLocked(source.Source.OwnerID, bookID),
		DispositionRevision: s.bookDispositionRevisionLocked(source.Source.OwnerID, bookID),
	}
}

// fixtureBooksFromMetadata builds the metadata-only entries of an owner's
// collection, applying the same disposition and visibility projection as the
// acquired-source entries.
func (s *Store) fixtureBooksFromMetadata(owner string) []domain.MyBook {
	out := make([]domain.MyBook, 0, len(s.myBooks))
	for _, book := range s.myBooks {
		if owner != "" && book.Book.OwnerID != owner {
			continue
		}
		book.Disposition = s.bookDispositionLocked(book.Book.OwnerID, book.Book.ID)
		book.DispositionRevision = s.bookDispositionRevisionLocked(book.Book.OwnerID, book.Book.ID)
		s.applyVisibilityLocked(&book)
		out = append(out, book)
	}
	return out
}

// applyFixtureBookEvidence decorates the assembled collection with the evidence
// the fixture models per Book: deck coverage and state, the current reading, and
// reading completions from both the imported and current-reading histories. The
// latest completion across both histories supplies LatestCompletionAt and
// LatestCompletionSource.
func (s *Store) applyFixtureBookEvidence(books []domain.MyBook) {
	for i := range books {
		s.applyFixtureBookEvidenceLocked(&books[i])
	}
}

// applyFixtureBookEvidenceLocked decorates one assembled Book with the evidence
// the fixture models. Callers hold s.mu.
func (s *Store) applyFixtureBookEvidenceLocked(book *domain.MyBook) {
	if book.Book.ID == BookID {
		book.CoverageKnownTokens = 974
		book.CoverageTotalTokens = 1000
		book.DeckState = "ready"
		book.DeckCardCount = 412
		preparedAt := fixtureJourneyTime
		book.DeckPreparedAt = &preparedAt
	}
	if s.fixtureBookIsCurrentReading(book.Book.OwnerID, book.Book.ID) {
		book.IsCurrentReading = true
	}
	s.applyFixtureBookCompletions(book)
}

// fixtureBookIsCurrentReading reports whether the owner has an active reading of
// the Book. Callers hold s.mu.
func (s *Store) fixtureBookIsCurrentReading(owner, bookID string) bool {
	for _, reading := range s.currentReadings {
		if reading.OwnerID == owner && reading.BookID == bookID {
			return true
		}
	}
	return false
}

// applyFixtureBookCompletions counts the imported and current-reading history of
// one Book and dates it from its latest completion. Callers hold s.mu.
func (s *Store) applyFixtureBookCompletions(book *domain.MyBook) {
	if imported, ok := s.importedHistory[fixtureDispositionKey(book.Book.OwnerID, book.Book.ID)]; ok {
		book.CompletionCount++
		book.LatestCompletionAt = &imported.CompletedAt
		book.LatestCompletionSource = domain.ReadingCompletionPreviouslyRead
	}
	for _, completion := range s.readingHistory {
		if completion.OwnerID != book.Book.OwnerID || completion.BookID != book.Book.ID {
			continue
		}
		book.CompletionCount++
		if book.LatestCompletionAt == nil || completion.CompletedAt.After(*book.LatestCompletionAt) {
			completedAt := completion.CompletedAt
			book.LatestCompletionAt = &completedAt
			book.LatestCompletionSource = completion.Source
		}
	}
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
	if done, err := fixtureRevisionCheck(state.revision, expectedRevision, state.hidden == hidden, domain.ErrStaleBookVisibility); done || err != nil {
		return false, err
	}
	if state.hidden == hidden {
		return false, nil
	}
	s.visibility[key] = fixtureVisibility{hidden: hidden, revision: expectedRevision + 1}
	return true, nil
}

// fixtureRevisionCheck is the expected-revision protocol the visibility and
// disposition writes share, mirroring the PostgreSQL adapters. A single-step
// replay of an accepted request (accepted reports that the stored state already
// equals the request) is done without writing; any other revision that is not
// the expected one is stale. Otherwise the caller decides whether to write.
func fixtureRevisionCheck(revision, expected int64, accepted bool, stale error) (done bool, err error) {
	if revision == expected+1 && accepted {
		return true, nil
	}
	if revision != expected {
		return false, stale
	}
	return false, nil
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
func (s *Store) ListMyBooksBrowse(ctx context.Context, owner, query, language, disposition string, history bool, offset, limit int) (domain.MyBooksBrowseResult, error) {
	return s.ListMyBooksBrowseWithVisibility(ctx, owner, query, language, disposition, history, false, offset, limit)
}

// ListMyBooksBrowseWithVisibility mirrors the production visibility scope:
// Hidden Books are omitted unless showHidden is set and all counts follow it.
// The projection is assembled from named steps that read like the PostgreSQL
// store's equivalent: normalize the request, drop the invisible Books, count
// what the view offers, filter to the selected scope, order, and page.
func (s *Store) ListMyBooksBrowseWithVisibility(_ context.Context, owner, query, language, disposition string, history, showHidden bool, offset, limit int) (domain.MyBooksBrowseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query, language, disposition = normalizeFixtureBrowseInputs(query, language, disposition)
	everything := s.myBooksForOwner(owner)
	result := domain.MyBooksBrowseResult{AllCount: len(everything)}
	visible, hiddenCount := fixtureVisibleBooks(everything, language, showHidden)
	result.HiddenCount = hiddenCount
	result.Counts = fixtureLanguageCounts(visible)
	result.ReadCount, result.DispositionCounts = fixtureWorkflowCounts(visible, language)
	filtered, scopeTotal := filterFixtureBooks(visible, query, language, disposition, history)
	result.ScopeTotal = scopeTotal
	sortFixtureBrowseBooks(filtered)
	result.Total = len(filtered)
	result.Items = fixtureBrowsePage(filtered, offset, limit)
	return result, nil
}

// normalizeFixtureBrowseInputs trims the search and the scope selectors and
// canonicalizes the language, except for the explicit unknown scope.
func normalizeFixtureBrowseInputs(query, language, disposition string) (string, string, string) {
	query = strings.ToLower(strings.TrimSpace(query))
	language = strings.TrimSpace(language)
	disposition = strings.TrimSpace(disposition)
	if language != domain.LanguageUnknown {
		language = normalizeFixtureLanguage(language)
	}
	return query, language, disposition
}

// fixtureVisibleBooks drops Hidden Books unless showHidden is set. It also
// reports the Hidden Books in the language scope, which stays independent of
// that choice so an empty view can still offer recovery.
func fixtureVisibleBooks(everything []domain.MyBook, language string, showHidden bool) (visible []domain.MyBook, hiddenCount int) {
	visible = make([]domain.MyBook, 0, len(everything))
	for _, book := range everything {
		if book.Hidden {
			if fixtureBookInLanguage(book, language) {
				hiddenCount++
			}
			if !showHidden {
				continue
			}
		}
		visible = append(visible, book)
	}
	return visible, hiddenCount
}

// fixtureLanguageCounts counts the visible Books per language pill, with the
// unknown pill last. The pills describe the whole visible collection rather
// than the selected language.
func fixtureLanguageCounts(books []domain.MyBook) []domain.LanguageCount {
	counts := map[string]int{}
	for _, book := range books {
		tag := domain.LanguageUnknown
		if book.Book.LanguageState == domain.LanguageChosen {
			tag = normalizeFixtureLanguage(book.Book.LanguageTag)
		}
		counts[tag]++
	}
	var result []domain.LanguageCount
	for tag, count := range counts {
		result = append(result, domain.LanguageCount{Tag: tag, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Tag == domain.LanguageUnknown {
			return false
		}
		if result[j].Tag == domain.LanguageUnknown {
			return true
		}
		return result[i].Tag < result[j].Tag
	})
	return result
}

// fixtureWorkflowCounts counts the visible workflow buckets in the language
// scope: the Read projection and the persisted dispositions. A current reading
// carries its persisted To Read disposition, so it also counts as To Read.
func fixtureWorkflowCounts(books []domain.MyBook, language string) (readCount int, counts []domain.DispositionCount) {
	dispositionCounts := map[domain.BookDisposition]int{}
	for _, book := range books {
		if !fixtureBookInLanguage(book, language) {
			continue
		}
		bucket := book.WorkflowBucket()
		if bucket == domain.MyBookBucketRead {
			readCount++
		}
		if disposition, ok := bucket.PersistedDisposition(); ok {
			dispositionCounts[disposition]++
		}
		if bucket == domain.MyBookBucketCurrentReading {
			dispositionCounts[domain.BookDispositionToRead]++
		}
	}
	for disposition, count := range dispositionCounts {
		counts = append(counts, domain.DispositionCount{Disposition: disposition, Count: count})
	}
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].Disposition < counts[j].Disposition
	})
	return readCount, counts
}

// filterFixtureBooks applies the search, language, and workflow filters to the
// visible Books and returns the selected scope size alongside the matches.
func filterFixtureBooks(books []domain.MyBook, query, language, disposition string, history bool) ([]domain.MyBook, int) {
	filtered := make([]domain.MyBook, 0, len(books))
	var scopeTotal int
	for _, book := range books {
		inLanguage := fixtureBookInLanguage(book, language)
		if inLanguage {
			scopeTotal++
		}
		if query != "" && !strings.Contains(strings.ToLower(book.Book.Title), query) {
			continue
		}
		if !inLanguage {
			continue
		}
		if !book.WorkflowBucket().MatchesBrowseFilter(domain.BookDisposition(disposition), history) {
			continue
		}
		filtered = append(filtered, book)
	}
	return filtered, scopeTotal
}

// fixtureBookInLanguage reports whether a Book belongs to the selected
// language scope: every language when empty, only unchosen languages for the
// explicit unknown scope, and a canonical tag match otherwise.
func fixtureBookInLanguage(book domain.MyBook, language string) bool {
	switch language {
	case "":
		return true
	case domain.LanguageUnknown:
		return book.Book.LanguageState == domain.LanguageUnknown
	default:
		return book.Book.LanguageState == domain.LanguageChosen && normalizeFixtureLanguage(book.Book.LanguageTag) == language
	}
}

// sortFixtureBrowseBooks orders a page the way the PostgreSQL browse query
// does: lowercased title, then exact title, then Book ID.
func sortFixtureBrowseBooks(books []domain.MyBook) {
	sort.Slice(books, func(i, j int) bool {
		left, right := books[i].Book, books[j].Book
		leftTitle, rightTitle := strings.ToLower(left.Title), strings.ToLower(right.Title)
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}
		if left.Title != right.Title {
			return left.Title < right.Title
		}
		return left.ID < right.ID
	})
}

// fixtureBrowsePage clamps caller-supplied pagination and returns the selected
// page, guarding the integer overflow the PostgreSQL store also guards.
func fixtureBrowsePage(books []domain.MyBook, offset, limit int) []domain.MyBook {
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	if offset >= len(books) {
		return nil
	}
	end := offset + limit
	if end < offset || end > len(books) {
		end = len(books)
	}
	return books[offset:end]
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

// ListMyBooks returns no Books. My Books persistence is intentionally small in
// the browser fixture; these methods cover the learner-facing metadata controls
// without a database.
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
			return fixtureBookFromSource(owner, source), nil
		}
	}
	return domain.Book{}, errNotFound
}

// fixtureBookFromSource derives the Book a source entry identifies: the
// resolved Book ID prefers BookID over the source ID, a blank language is
// unchosen, and the title falls back to the source title.
func fixtureBookFromSource(owner string, source domain.SourceMaterialSummary) domain.Book {
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
	return domain.Book{ID: resolvedBookID, OwnerID: owner, Title: title, Author: source.BookAuthor, LanguageState: state, LanguageTag: source.Source.Language}
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
	if done, err := fixtureRevisionCheck(revision, expectedRevision, current == disposition, domain.ErrStaleBookDisposition); done || err != nil {
		return false, err
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
			return s.updateFixtureSourceMetadataLocked(i, owner, title, author, languageState, languageTag), nil
		}
	}
	for i := range s.myBooks {
		if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == bookID {
			return s.updateFixtureMyBookMetadataLocked(i, owner, title, author, languageState, languageTag), nil
		}
	}
	return domain.Book{}, errNotFound
}

// updateFixtureSourceMetadataLocked writes metadata to the source entry at
// index, resolving its Book ID, and clears goals the retag invalidates. Callers
// hold s.mu.
func (s *Store) updateFixtureSourceMetadataLocked(index int, owner, title, author, languageState, languageTag string) domain.Book {
	source := &s.books[index]
	resolvedBookID := source.BookID
	if resolvedBookID == "" {
		resolvedBookID = source.Source.ID
	}
	source.BookTitle = title
	source.BookAuthor = strings.TrimSpace(author)
	if languageState == domain.LanguageUnknown {
		languageTag = ""
	}
	source.Source.Language = languageTag
	s.clearFixtureGoalsForRetaggedBook(owner, resolvedBookID, languageState, languageTag)
	return domain.Book{ID: resolvedBookID, OwnerID: owner, Title: title, Author: strings.TrimSpace(author), LanguageState: languageState, LanguageTag: languageTag}
}

// updateFixtureMyBookMetadataLocked writes metadata to the metadata-only entry
// at index and clears goals the retag invalidates. Callers hold s.mu.
func (s *Store) updateFixtureMyBookMetadataLocked(index int, owner, title, author, languageState, languageTag string) domain.Book {
	book := &s.myBooks[index].Book
	book.Title = title
	book.Author = strings.TrimSpace(author)
	book.LanguageState = languageState
	book.LanguageTag = languageTag
	s.clearFixtureGoalsForRetaggedBook(owner, book.ID, languageState, languageTag)
	return *book
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
