package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

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
	all := s.fixtureBooksFromSources(owner)
	all = append(all, s.fixtureBooksFromMetadata(owner)...)
	out := make([]domain.MyBook, 0, len(all))
	for _, book := range all {
		if !s.fixtureBookActiveLocked(book.Book.OwnerID, book.Book.ID) {
			continue
		}
		if s.unchosen[fixtureDispositionKey(book.Book.OwnerID, book.Book.ID)] {
			continue
		}
		out = append(out, book)
	}
	s.applyFixtureBookEvidence(out)
	return out
}

// fixtureBooksFromSources builds the acquired-source entries of an owner's
// collection: one entry per acquired source, scoped to the owner when one is
// given.
func (s *Store) fixtureBooksFromSources(owner string) []domain.MyBook {
	out := make([]domain.MyBook, 0, len(s.books))
	for i := range s.books {
		source := s.books[i]
		if owner != "" && source.Source.OwnerID != owner {
			continue
		}
		book := s.fixtureMyBookFromSource(source)
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
// collection, applying the same disposition projection as the acquired-source
// entries.
func (s *Store) fixtureBooksFromMetadata(owner string) []domain.MyBook {
	out := make([]domain.MyBook, 0, len(s.myBooks))
	for _, book := range s.myBooks {
		if owner != "" && book.Book.OwnerID != owner {
			continue
		}
		book.Disposition = s.bookDispositionLocked(book.Book.OwnerID, book.Book.ID)
		book.DispositionRevision = s.bookDispositionRevisionLocked(book.Book.OwnerID, book.Book.ID)
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
	s.applyEPUBAcquisitionFailure(book)
	if book.Book.ID == BookID {
		book.CoverageKnownTokens = 974
		book.CoverageTotalTokens = 1000
		book.DeckState = "ready"
		book.DeckCardCount = 412
		preparedAt := fixtureJourneyTime
		book.DeckPreparedAt = &preparedAt
	}
	// A classical Book represents completed preparation once it is To Read.
	// The evidence is a read-model projection; Add still stores a metadata-only
	// Book. Recorded preparation evidence takes precedence, so a seeded failure
	// is presented rather than hidden behind a synthetic analysis.
	if book.ReadingUnit != nil && book.Disposition == domain.BookDispositionToRead && book.Acquired == nil && book.ClassicalPreparation == nil {
		book.Acquired = fixtureClassicalSource(book.Book.OwnerID, book.Book.ID, *book.ReadingUnit)
	}
	book.IsCurrentReading = s.fixtureBookIsCurrentReading(book.Book.OwnerID, book.Book.ID)
	book.EverCurrentReading = s.fixtureBookEverCurrentReading(book.Book.OwnerID, book.Book.ID)
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

// fixtureBookEverCurrentReading reports whether the Book has held the Current
// reading role. An active reading counts, and a released snapshot records the
// role after the reading ends. Callers hold s.mu.
func (s *Store) fixtureBookEverCurrentReading(owner, bookID string) bool {
	if s.fixtureBookIsCurrentReading(owner, bookID) {
		return true
	}
	for _, lifecycle := range s.snapshotLifecycles {
		if lifecycle.BookID == bookID {
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

// fixtureRevisionCheck is the expected-revision protocol disposition writes
// use, mirroring the PostgreSQL adapter. A single-step replay of an accepted
// request (accepted reports that the stored state already equals the request)
// is done without writing; any other revision that is not the expected one is
// stale. Otherwise the caller decides whether to write.
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
// counts over the complete active owner collection. The projection is
// assembled from named steps that read like the PostgreSQL store's equivalent:
// normalize the request, count what the view offers, filter to the selected
// scope, order, and page.
func (s *Store) ListMyBooksBrowse(_ context.Context, owner, query, language, disposition string, history bool, offset, limit int) (domain.MyBooksBrowseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query, language, disposition = normalizeFixtureBrowseInputs(query, language, disposition)
	everything := s.myBooksForOwner(owner)
	result := domain.MyBooksBrowseResult{AllCount: len(everything)}
	result.ReadCount, result.DispositionCounts = fixtureWorkflowCounts(everything, language)
	filtered, scopeTotal := filterFixtureBooks(everything, query, language, disposition, history)
	result.ScopeTotal = scopeTotal
	sortFixtureBrowseBooks(filtered)
	result.Total = len(filtered)
	result.Items = fixtureBrowsePage(filtered, offset, limit)
	return result, nil
}

// normalizeFixtureBrowseInputs trims the search and the scope selectors and
// always canonicalizes the language: a Book matches a non-empty language only
// when its language is chosen and equal, and the retired unknown scope is
// gone.
func normalizeFixtureBrowseInputs(query, language, disposition string) (string, string, string) {
	query = strings.ToLower(strings.TrimSpace(query))
	language = strings.TrimSpace(language)
	disposition = strings.TrimSpace(disposition)
	language = normalizeFixtureLanguage(language)
	return query, language, disposition
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
		if bucket == domain.MyBookBucketNone {
			continue
		}
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
// language scope: every chosen-bucket member when the language is empty, and
// a chosen, canonical tag match otherwise. A Book whose language is not
// chosen matches no study language.
func fixtureBookInLanguage(book domain.MyBook, language string) bool {
	switch language {
	case "":
		return true
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

// CreateBook creates a To Read Book, as the PostgreSQL store does: My Books
// holds only chosen Books (ADR 0117).
func (s *Store) CreateBook(_ context.Context, book domain.Book) (domain.Book, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	book.ID = fmt.Sprintf("fixture-metadata-%d", len(s.myBooks)+1)
	s.myBooks = append(s.myBooks, domain.MyBook{Book: book})
	s.dispositions[fixtureDispositionKey(book.OwnerID, book.ID)] = domain.BookDispositionToRead
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
	if err := s.refuseUnchosenDispositionLocked(owner, bookID, disposition); err != nil {
		return err
	}
	key := fixtureDispositionKey(owner, bookID)
	if s.bookDispositionLocked(owner, bookID) != disposition {
		s.dispositionRevisions[key] = s.bookDispositionRevisionLocked(owner, bookID) + 1
	}
	s.dispositions[key] = disposition
	return nil
}

// refuseUnchosenDispositionLocked rejects the retired Inbox state for a Book
// that is neither the current reading nor has reading history. Callers hold
// s.mu.
func (s *Store) refuseUnchosenDispositionLocked(owner, bookID string, disposition domain.BookDisposition) error {
	if disposition != domain.BookDispositionInbox {
		return nil
	}
	if s.fixtureBookHasHistoryLocked(owner, bookID) || s.fixtureBookIsCurrentReading(owner, bookID) {
		return nil
	}
	return domain.ErrBookNotChosen
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
	if err := s.refuseUnchosenDispositionLocked(owner, bookID, disposition); err != nil {
		return false, err
	}
	s.dispositions[key] = disposition
	s.dispositionRevisions[key] = revision + 1
	return true, nil
}

func (s *Store) RemoveBookFromMyBooks(_ context.Context, owner, bookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bookID = s.fixtureBookID(owner, bookID)
	if bookID == "" {
		return domain.ErrNotFound
	}
	if s.fixtureBookIsCurrentReading(owner, bookID) {
		return domain.ErrBookIsCurrentReading
	}
	s.removedBooks[fixtureDispositionKey(owner, bookID)] = struct{}{}
	return nil
}

// fixtureBookActiveLocked reports whether the Book still has active My Books
// membership. Callers hold s.mu.
func (s *Store) fixtureBookActiveLocked(owner, bookID string) bool {
	_, removed := s.removedBooks[fixtureDispositionKey(owner, bookID)]
	return !removed
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

// ResolveOrCreateBookForAcquisition returns the Book a connectionless source
// alias names, restoring a removed one as To Read, or creates a To Read
// acquisition Book. Without an alias the identifier is not recorded, as in
// PostgreSQL, where ingest links the source after the Book exists.
func (s *Store) ResolveOrCreateBookForAcquisition(_ context.Context, owner, sourceIdentifier, language, title string) (string, error) {
	owner, sourceIdentifier, language, title = strings.TrimSpace(owner), strings.TrimSpace(sourceIdentifier), strings.TrimSpace(language), strings.TrimSpace(title)
	if owner == "" || sourceIdentifier == "" || language == "" || title == "" {
		return "", errors.New("fixtures: acquisition book identity is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, alias := range s.aliases {
		if alias.OwnerID == owner && alias.ConnectionID == "" && alias.Namespace == domain.NamespaceSourceIdentifier && alias.Value == sourceIdentifier {
			if !s.fixtureBookActiveLocked(owner, alias.BookID) {
				s.restoreFixtureBookMembershipLocked(owner, alias.BookID, domain.BookDispositionToRead)
			}
			return alias.BookID, nil
		}
	}
	book := domain.Book{
		ID:                 fmt.Sprintf("fixture-acquisition-%d", len(s.myBooks)+1),
		OwnerID:            owner,
		Title:              title,
		MetadataProvenance: domain.MetadataProvenanceAcquisition,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        language,
	}
	s.myBooks = append(s.myBooks, domain.MyBook{Book: book})
	s.dispositions[fixtureDispositionKey(owner, book.ID)] = domain.BookDispositionToRead
	return book.ID, nil
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
