package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// The My Books browse result types are defined in domain so store contracts can
// read them without importing an adapter; these names are the same types.
type (
	DispositionCount    = domain.DispositionCount
	MyBooksBrowseResult = domain.MyBooksBrowseResult
)

func (s *PostgresStore) ListMyBooks(ctx context.Context, owner string) ([]domain.Book, error) {
	rows, err := s.queries().ListActiveBooks(ctx, owner)
	if err != nil {
		return nil, err
	}
	var books []domain.Book
	for _, row := range rows {
		books = append(books, domain.Book{
			ID:                 row.BID,
			OwnerID:            row.BOwnerID,
			Title:              row.Title,
			Author:             row.Author,
			MetadataProvenance: row.MetadataProvenance,
			LanguageState:      row.LanguageState,
			LanguageTag:        row.LanguageTag,
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return books, nil
}

// ListStudyLanguages derives the learner's study languages from active Books
// with a chosen canonical language. Reference names are optional so newly
// discovered tags remain importable.
func (s *PostgresStore) ListStudyLanguages(ctx context.Context, owner string) ([]domain.StudyLanguage, error) {
	rows, err := s.queries().ListStudyLanguages(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []domain.StudyLanguage
	for _, row := range rows {
		out = append(out, domain.StudyLanguage{Language: pgText(row.Language), DisplayName: pgText(row.DisplayName)})
	}
	return out, nil
}

// ListMyBooksWithEvidence is the My Books collection read model. Membership
// is the driving table, so metadata-only Books remain visible; the current
// acquired source and analysis projection are optional evidence on each row.
func (s *PostgresStore) ListMyBooksWithEvidence(ctx context.Context, owner string) ([]domain.MyBook, error) {
	var books []domain.MyBook
	err := s.forEachMyBooksBrowseRow(ctx, owner, "", func(row sqlcgen.BrowseMyBooksEvidenceRow) error {
		book := myBookFromBrowseRow(row)
		book.Disposition = domain.BookDisposition(row.Disposition)
		book.DispositionRevision = row.DispositionRevision
		books = append(books, book)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Reading-unit association is part of the MyBook read model: it carries the
	// reviewed Work and constituent position the Reading chooser and Reading
	// page render as plain facts, plus the preparation state of a classical Book.
	if err := s.populateMyBooksReadingUnits(ctx, owner, books); err != nil {
		return nil, err
	}
	if err := s.populateClassicalPreparationStatuses(ctx, owner, books); err != nil {
		return nil, err
	}
	pointers := make([]*domain.MyBook, len(books))
	for i := range books {
		pointers[i] = &books[i]
	}
	if err := s.populateEPUBAcquisitionStatuses(ctx, owner, pointers); err != nil {
		return nil, err
	}
	return books, nil
}

// BackfillChosenBooks removes the retired Inbox state from My Books. It ends
// the membership of every active Book that is not To Read, has no reading
// history and is not the current reading. Membership is the only thing it
// changes, so Add restores the same Book. The gate refuses to run until every
// connection's Catalog index has completed a sync, so no entry becomes
// unfindable during the upgrade. A repeated run changes nothing.
func (s *PostgresStore) BackfillChosenBooks(ctx context.Context) (report domain.MyBooksBackfillReport, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.MyBooksBackfillReport{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	blocked, err := q.CatalogueIndexBackfillBlocked(ctx)
	if err != nil {
		return domain.MyBooksBackfillReport{}, err
	}
	if blocked {
		return domain.MyBooksBackfillReport{}, domain.ErrMyBooksBackfillBlocked
	}
	rows, err := q.ListLanguageLessReadBooks(ctx)
	if err != nil {
		return domain.MyBooksBackfillReport{}, err
	}
	for _, row := range rows {
		report.LanguageLessWithHistory = append(report.LanguageLessWithHistory, domain.Book{
			ID: row.ID, OwnerID: row.OwnerID, Title: row.Title, Author: row.Author,
			MetadataProvenance: row.MetadataProvenance, LanguageState: row.LanguageState,
			LanguageTag: row.LanguageTag, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	removed, err := q.RemoveUnchosenBookMembership(ctx)
	if err != nil {
		return domain.MyBooksBackfillReport{}, err
	}
	report.Removed, err = checked.IntFromInt64(removed)
	if err != nil {
		return domain.MyBooksBackfillReport{}, fmt.Errorf("invalid removed book count: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MyBooksBackfillReport{}, err
	}
	return report, nil
}

// ListMyBooksBrowse returns one owner-scoped page of My Books. The domain's
// WorkflowBucket derivation is the source of truth for both filters and counts;
// SQL supplies the facts (disposition, current-reading role, and history) but
// does not independently classify a visible bucket.
func (s *PostgresStore) ListMyBooksBrowse(ctx context.Context, owner, query, language, disposition string, history bool, offset, limit int) (MyBooksBrowseResult, error) {
	query, language, disposition, err := normalizeMyBooksBrowseRequest(query, language, disposition)
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	accumulator := &myBooksBrowseAccumulator{
		query:             query,
		disposition:       domain.BookDisposition(disposition),
		history:           history,
		window:            myBooksBrowsePageWindow(offset, limit),
		dispositionCounts: make(map[domain.MyBookBucket]int),
	}
	if err := s.collectMyBooksBrowsePage(ctx, owner, language, accumulator); err != nil {
		return MyBooksBrowseResult{}, err
	}
	counts, err := s.collectMyBooksBrowseCounts(ctx, owner, language)
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	result := accumulator.result
	result.DispositionCounts = myBooksDispositionCounts(accumulator.dispositionCounts)
	result.ScopeTotal = counts.scopeTotal
	result.AllCount = counts.allCount
	return result, nil
}

// normalizeMyBooksBrowseRequest trims and lowercases the search, trims the
// scope selectors, validates the disposition before any read, and always
// canonicalizes the language: a Book matches a non-empty language only when
// its language is chosen and equal, and the retired unknown scope is gone.
func normalizeMyBooksBrowseRequest(query, language, disposition string) (string, string, string, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	language = strings.TrimSpace(language)
	disposition = strings.TrimSpace(disposition)
	if disposition != "" {
		if err := domain.BookDisposition(disposition).Validate(); err != nil {
			return "", "", "", err
		}
	}
	language = canonicalization.NormalizeLanguage(language)
	return query, language, disposition, nil
}

// myBooksBrowseWindow is the clamped requested page: offset is the first
// matching row to keep and pageEnd is the exclusive upper bound.
type myBooksBrowseWindow struct {
	offset  int
	pageEnd int
}

// myBooksBrowsePageWindow clamps caller-supplied pagination and guards the
// integer overflow a caller-supplied limit can produce.
func myBooksBrowsePageWindow(offset, limit int) myBooksBrowseWindow {
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	pageEnd := offset + limit
	if pageEnd < offset { // Guard integer overflow from caller-supplied pagination.
		pageEnd = int(^uint(0) >> 1)
	}
	return myBooksBrowseWindow{offset: offset, pageEnd: pageEnd}
}

// myBooksBrowseAccumulator folds the ordered browse rows into the
// workflow-bucket counts and the matching-scope size. Classification stays in
// the domain; this only applies the search, filter and page steps. Every Book
// is one row.
type myBooksBrowseAccumulator struct {
	query             string
	disposition       domain.BookDisposition
	history           bool
	window            myBooksBrowseWindow
	dispositionCounts map[domain.MyBookBucket]int
	result            MyBooksBrowseResult
	books             []domain.MyBook
}

// visit applies every step to one row, in the same order the SQL projection and
// the domain classifier expect: tally the workflow buckets, then keep the Book
// for search, filter and paging once the whole projection is known.
func (a *myBooksBrowseAccumulator) visit(row sqlcgen.BrowseMyBooksEvidenceRow) error {
	book := myBookFromBrowseRow(row)
	book.Disposition = domain.BookDisposition(row.Disposition)
	book.DispositionRevision = row.DispositionRevision
	bucket := book.WorkflowBucket()
	if bucket == domain.MyBookBucketNone {
		// The backfill owns removing the retired Inbox members. Until it runs,
		// they have no visible My Books role and are not listed.
		return nil
	}
	a.dispositionCounts[bucket]++
	if bucket == domain.MyBookBucketCurrentReading {
		a.dispositionCounts[domain.MyBookBucketToRead]++
	}
	if bucket == domain.MyBookBucketRead {
		a.result.ReadCount++
	}
	a.books = append(a.books, book)
	return nil
}

// finish applies the search and the selected filter to every Book and pages the
// matches.
func (a *myBooksBrowseAccumulator) finish() {
	matching := make([]domain.MyBook, 0, len(a.books))
	for _, book := range a.books {
		if myBooksBrowseMatchesQuery(book, a.query) && book.WorkflowBucket().MatchesBrowseFilter(a.disposition, a.history) {
			matching = append(matching, book)
		}
	}
	a.result.Total = len(matching)
	a.result.Items = myBooksPageItems(matching, a.window)
}

// myBooksBrowseMatchesQuery applies the trimmed, lowercased search to a Book's
// title or author. A Book from the classical Catalog also matches its Work's
// author, English title or Greek title, ignoring accents; the Greek title uses
// the Go search key, never SQL lower().
func myBooksBrowseMatchesQuery(book domain.MyBook, query string) bool {
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(book.Book.Title), query) || strings.Contains(strings.ToLower(book.Book.Author), query) {
		return true
	}
	unit := book.ReadingUnit
	return unit != nil && myBooksWorkTextMatches(unit, query)
}

// myBooksWorkTextMatches compares accent-insensitive search keys, so "ιλιασ"
// finds Ἰλιάς and "homer" finds Hómēr.
func myBooksWorkTextMatches(unit *domain.BookReadingUnit, text string) bool {
	key := canonicalization.SearchKey(text)
	if key == "" {
		return false
	}
	if strings.Contains(canonicalization.SearchKey(unit.WorkTitle), key) {
		return true
	}
	if strings.Contains(canonicalization.SearchKey(unit.WorkAuthor), key) {
		return true
	}
	return unit.OriginalTitle != "" && strings.Contains(canonicalization.SearchKey(unit.OriginalTitle), key)
}

// myBooksPageItems slices the matching Books for the requested page.
func myBooksPageItems(books []domain.MyBook, window myBooksBrowseWindow) []domain.MyBook {
	if window.offset >= len(books) || window.pageEnd <= window.offset {
		return nil
	}
	end := min(window.pageEnd, len(books))
	return books[window.offset:end]
}

// collectMyBooksBrowsePage walks the ordered browse projection and folds it
// into the accumulator, then loads coverage for the selected page.
func (s *PostgresStore) collectMyBooksBrowsePage(ctx context.Context, owner, language string, accumulator *myBooksBrowseAccumulator) error {
	if err := s.forEachMyBooksBrowseRow(ctx, owner, language, accumulator.visit); err != nil {
		return err
	}
	accumulator.finish()
	pointers := make([]*domain.MyBook, 0, len(accumulator.result.Items))
	for i := range accumulator.result.Items {
		pointers = append(pointers, &accumulator.result.Items[i])
	}
	if err := s.populateMyBooksCoverage(ctx, owner, pointers); err != nil {
		return err
	}
	if err := s.populateClassicalPreparationPointers(ctx, owner, pointers); err != nil {
		return err
	}
	if err := s.populateEPUBAcquisitionStatuses(ctx, owner, pointers); err != nil {
		return err
	}
	return nil
}

// myBooksDispositionCounts projects the workflow-bucket tallies onto the
// persisted To Read disposition, omitting an empty bucket. The current reading
// keeps its To Read disposition and is counted there.
func myBooksDispositionCounts(counts map[domain.MyBookBucket]int) []DispositionCount {
	count := counts[domain.MyBookBucketToRead]
	if count == 0 {
		return nil
	}
	return []DispositionCount{{Disposition: domain.BookDispositionToRead, Count: count}}
}

// myBooksBrowseCounts is the collection size evidence around the page.
type myBooksBrowseCounts struct {
	scopeTotal int
	allCount   int
}

// collectMyBooksBrowseCounts reads the count queries in the same order and with
// the same scopes the page was built from.
func (s *PostgresStore) collectMyBooksBrowseCounts(ctx context.Context, owner, language string) (myBooksBrowseCounts, error) {
	q := s.queries()
	var counts myBooksBrowseCounts
	scopeTotal, err := q.CountMyBooksScope(ctx, sqlcgen.CountMyBooksScopeParams{Owner: owner, Language: language})
	if err != nil {
		return myBooksBrowseCounts{}, err
	}
	counts.scopeTotal, err = checked.IntFromInt64(scopeTotal)
	if err != nil {
		return myBooksBrowseCounts{}, fmt.Errorf("invalid scoped book count: %w", err)
	}
	allCount, err := q.CountMyBooksAll(ctx, owner)
	if err != nil {
		return myBooksBrowseCounts{}, err
	}
	counts.allCount, err = checked.IntFromInt64(allCount)
	if err != nil {
		return myBooksBrowseCounts{}, fmt.Errorf("invalid total book count: %w", err)
	}
	return counts, nil
}

const myBooksBrowseBatchSize = 256

// forEachMyBooksBrowseRow walks the ordered browse projection in bounded
// keyset batches. Classification remains in the domain; SQL only pages the
// facts needed by the projection.
func (s *PostgresStore) forEachMyBooksBrowseRow(ctx context.Context, owner, language string, visit func(sqlcgen.BrowseMyBooksEvidenceRow) error) error {
	q := s.queries()
	hasCursor := false
	var afterSortTitle, afterBookTitle, afterBookID string
	for {
		rows, err := q.BrowseMyBooksEvidence(ctx, sqlcgen.BrowseMyBooksEvidenceParams{
			Owner: owner, Language: language, HasCursor: hasCursor,
			AfterSortTitle: afterSortTitle, AfterBookTitle: afterBookTitle,
			AfterBookID: afterBookID, Limit: myBooksBrowseBatchSize,
		})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := visit(row); err != nil {
				return err
			}
		}
		if len(rows) < myBooksBrowseBatchSize {
			return nil
		}
		last := rows[len(rows)-1]
		hasCursor = true
		afterSortTitle, afterBookTitle, afterBookID = last.SortTitle, last.BookTitle, last.BookID
	}
}

// GetBookDetail resolves either the canonical Book ID or a historical source
// material ID within the owner's active My Books membership.
func (s *PostgresStore) GetBookDetail(ctx context.Context, owner, id string) (domain.MyBook, error) {
	row, err := s.queries().GetMyBookDetail(ctx, sqlcgen.GetMyBookDetailParams{Owner: owner, ID: id})
	if err != nil {
		return domain.MyBook{}, missing(err)
	}
	book := myBookFromEvidence(row, "")
	if book.Acquired != nil {
		failureCode, codeErr := s.queries().GetLatestAnalysisFailureCode(ctx, sqlcgen.GetLatestAnalysisFailureCodeParams{Owner: owner, Source: book.Acquired.Source.ID})
		if codeErr != nil {
			return domain.MyBook{}, codeErr
		}
		book.Acquired.Signals.FailureCode = domain.BookPreparationCode(failureCode)
	}
	states, err := s.bookDispositionMap(ctx, owner)
	if err != nil {
		return domain.MyBook{}, err
	}
	state, ok := states[book.Book.ID]
	if !ok {
		return domain.MyBook{}, ErrNotFound
	}
	book.Disposition = state.disposition
	book.DispositionRevision = state.revision
	// The current-reading role is part of the detail read model, so a started
	// Book classifies as Currently reading exactly as the My Books browse row
	// and the fixtures adapter do; the corpus-sized margin evidence stays in
	// the browse projection.
	book.IsCurrentReading, err = s.queries().BookHasActiveCurrentReading(ctx, sqlcgen.BookHasActiveCurrentReadingParams{Owner: owner, Book: book.Book.ID})
	if err != nil {
		return domain.MyBook{}, err
	}
	book.EverCurrentReading, err = s.queries().BookEverCurrentReading(ctx, sqlcgen.BookEverCurrentReadingParams{Owner: owner, Book: book.Book.ID})
	if err != nil {
		return domain.MyBook{}, err
	}
	completion, err := s.queries().GetMyBookCompletionSummary(ctx, sqlcgen.GetMyBookCompletionSummaryParams{Owner: owner, Book: book.Book.ID})
	if err != nil {
		return domain.MyBook{}, err
	}
	book.CompletionCount, err = checked.IntFromInt64(completion.CompletionCount)
	if err != nil {
		return domain.MyBook{}, fmt.Errorf("invalid reading completion count: %w", err)
	}
	if book.CompletionCount > 0 {
		book.LatestCompletionAt = completionTime(completion.LatestCompletedAt)
	}
	book.LatestCompletionSource = domain.ReadingCompletionSource(completion.LatestCompletionSource)
	page := []domain.MyBook{book}
	if err := s.populateMyBooksReadingUnits(ctx, owner, page); err != nil {
		return domain.MyBook{}, err
	}
	if err := s.populateClassicalPreparationStatuses(ctx, owner, page); err != nil {
		return domain.MyBook{}, err
	}
	if err := s.populateEPUBAcquisitionStatuses(ctx, owner, []*domain.MyBook{&page[0]}); err != nil {
		return domain.MyBook{}, err
	}
	return page[0], nil
}

type bookDispositionState struct {
	disposition domain.BookDisposition
	revision    int64
}

func (s *PostgresStore) bookDispositionMap(ctx context.Context, owner string) (map[string]bookDispositionState, error) {
	rows, err := s.queries().ListBookDispositions(ctx, owner)
	if err != nil {
		return nil, err
	}
	dispositions := make(map[string]bookDispositionState, len(rows))
	for _, row := range rows {
		dispositions[row.BookID] = bookDispositionState{disposition: domain.BookDisposition(row.Disposition), revision: row.Revision}
	}
	return dispositions, nil
}

func (s *PostgresStore) GetBook(ctx context.Context, owner, bookID string) (domain.Book, error) {
	row, err := s.queries().GetBook(ctx, sqlcgen.GetBookParams{OwnerID: owner, ID: bookID})
	if err != nil {
		return domain.Book{}, missing(err)
	}
	return domain.Book(row), nil
}

// GetBookCatalogEntryAlias returns the owner's recorded catalogue identity for
// an active Book. Missing and cross-owner identities are deliberately the same
// not-found result.
func (s *PostgresStore) GetBookCatalogEntryAlias(ctx context.Context, owner, bookID string) (domain.BookAlias, error) {
	row, err := s.queries().GetBookCatalogEntryAlias(ctx, sqlcgen.GetBookCatalogEntryAliasParams{OwnerID: owner, BookID: bookID, AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier})
	if err != nil {
		return domain.BookAlias{}, missing(err)
	}
	return domain.BookAlias{ID: row.AID, OwnerID: row.AOwnerID, BookID: row.ABookID, ConnectionID: row.ConnectionID, AliasType: row.AliasType, Namespace: row.Namespace, Value: row.Value, CreatedAt: row.CreatedAt}, nil
}

// GetBookCatalogEntryAliasForConnection returns the owner's recorded catalogue
// identity for a Book on one specific connection. The cover worker uses it to
// reload the current entry without trusting job-supplied URLs or credentials.
func (s *PostgresStore) GetBookCatalogEntryAliasForConnection(ctx context.Context, owner, bookID, connectionID string) (domain.BookAlias, error) {
	row, err := s.queries().GetBookCatalogEntryAliasForConnection(ctx, sqlcgen.GetBookCatalogEntryAliasForConnectionParams{OwnerID: owner, BookID: bookID, ConnectionID: uuidArg(connectionID), AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier})
	if err != nil {
		return domain.BookAlias{}, missing(err)
	}
	return domain.BookAlias{ID: row.AID, OwnerID: row.AOwnerID, BookID: row.ABookID, ConnectionID: row.ConnectionID, AliasType: row.AliasType, Namespace: row.Namespace, Value: row.Value, CreatedAt: row.CreatedAt}, nil
}

// CreateBook creates a Book with active My Books membership. My Books holds
// only chosen Books (ADR 0117), so the new Book is To Read.
func (s *PostgresStore) CreateBook(ctx context.Context, b domain.Book) (domain.Book, error) {
	b.Title = strings.TrimSpace(b.Title)
	b.Author = strings.TrimSpace(b.Author)
	b.LanguageTag = strings.TrimSpace(b.LanguageTag)
	if err := b.Validate(); err != nil {
		return domain.Book{}, err
	}
	var created domain.Book
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		row, err := q.InsertBook(ctx, sqlcgen.InsertBookParams{OwnerID: b.OwnerID, Title: b.Title, Author: b.Author, MetadataProvenance: b.MetadataProvenance, LanguageState: b.LanguageState, LanguageTag: nullableTextArg(nullableLanguageTag(b.LanguageState, b.LanguageTag))})
		if err != nil {
			return err
		}
		created = domain.Book(row)
		if err := q.InsertBookMembership(ctx, sqlcgen.InsertBookMembershipParams{OwnerID: created.OwnerID, BookID: created.ID}); err != nil {
			return err
		}
		return upsertBookDisposition(ctx, q, created.OwnerID, created.ID, domain.BookDispositionToRead)
	})
	return created, err
}

func nullableLanguageTag(state, tag string) string {
	if state == domain.LanguageUnknown {
		return ""
	}
	return tag
}

func (s *PostgresStore) UpdateBookMetadata(ctx context.Context, owner, bookID, title, author, languageState, languageTag string) (domain.Book, error) {
	current, err := s.GetBook(ctx, owner, bookID)
	if err != nil {
		return domain.Book{}, err
	}
	candidate := current
	if languageState == domain.LanguageChosen {
		languageTag = canonicalization.NormalizeLanguage(languageTag)
	}
	candidate.Title, candidate.Author, candidate.LanguageState, candidate.LanguageTag = strings.TrimSpace(title), strings.TrimSpace(author), languageState, languageTag
	if err := candidate.Validate(); err != nil {
		return domain.Book{}, err
	}
	var updated domain.Book
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		row, err := q.UpdateBookMetadata(ctx, sqlcgen.UpdateBookMetadataParams{OwnerID: owner, ID: bookID, Title: candidate.Title, Author: candidate.Author, LanguageState: languageState, LanguageTag: nullableTextArg(nullableLanguageTag(languageState, languageTag))})
		if err != nil {
			return err
		}
		updated = domain.Book(row)
		if languageState == domain.LanguageChosen {
			err = removeIncompatibleBookGoals(ctx, q, owner, bookID, languageTag)
			return err
		}
		return q.DeleteBookGoals(ctx, sqlcgen.DeleteBookGoalsParams{OwnerID: owner, BookID: bookID})
	})
	return updated, err
}

func ensureBookExists(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	found, err := sqlcgen.New(tx).BookExists(ctx, sqlcgen.BookExistsParams{Owner: owner, Book: bookID})
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

// LockBookForUpdateTx locks the owner's Book row for update and reports a
// missing Book as ErrNotFound. It is the first lock every Book deck writer
// takes, so the Book fences the Current reading and preparation rows that
// follow.
func (s *PostgresStore) LockBookForUpdateTx(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	return lockBookForUpdate(ctx, sqlcgen.New(tx), owner, bookID)
}

// activateMembership makes the Book an active My Books member. A Book that was
// not active (removed, or never a member) returns as To Read, so activation
// never leaves a member that is not Currently reading, To Read or Read (ADR
// 0117). An already active Book keeps its disposition.
func activateMembership(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	q := sqlcgen.New(tx)
	state, err := q.GetBookMembershipForUpdate(ctx, sqlcgen.GetBookMembershipForUpdateParams{Owner: owner, Book: bookID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && state == "active" {
		return nil
	}
	if err := upsertBookDisposition(ctx, q, owner, bookID, domain.BookDispositionToRead); err != nil {
		return err
	}
	return q.ActivateBookMembership(ctx, sqlcgen.ActivateBookMembershipParams{OwnerID: owner, BookID: bookID})
}

func (s *PostgresStore) RemoveBookFromMyBooks(ctx context.Context, owner, bookID string) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	// The Book lock is the same first lock a Current reading Start takes, so a
	// Start and this refusal cannot interleave.
	if err := lockBookForUpdate(ctx, q, owner, bookID); err != nil {
		return err
	}
	current, err := q.BookIsCurrentReading(ctx, sqlcgen.BookIsCurrentReadingParams{Owner: owner, Book: bookID})
	if err != nil {
		return err
	}
	if current {
		return domain.ErrBookIsCurrentReading
	}
	if err := q.RemoveBookMembership(ctx, sqlcgen.RemoveBookMembershipParams{OwnerID: owner, BookID: bookID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveBookID maps an owner-scoped Book or source-material identity to its
// canonical Book ID. Source materials may link directly to a Book or through
// the legacy source-identifier alias.
func (s *PostgresStore) ResolveBookID(ctx context.Context, owner, id string) (string, bool, error) {
	exists, err := s.queries().BookExists(ctx, sqlcgen.BookExistsParams{Owner: owner, Book: id})
	if err != nil {
		return "", false, err
	}
	if exists {
		return id, true, nil
	}
	linked, err := s.queries().ResolveLinkedSourceBook(ctx, sqlcgen.ResolveLinkedSourceBookParams{
		Owner: owner, Source: id, Namespace: domain.NamespaceSourceIdentifier,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return linked, linked != "", nil
}

func (s *PostgresStore) ResolveBookByAlias(ctx context.Context, owner, namespace, value string) (domain.Book, bool, error) {
	namespace, value = strings.TrimSpace(namespace), strings.TrimSpace(value)
	row, err := s.queries().GetBookByAlias(ctx, sqlcgen.GetBookByAliasParams{OwnerID: owner, Namespace: namespace, Value: value})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Book{}, false, nil
	}
	if err != nil {
		return domain.Book{}, false, err
	}
	return domain.Book{ID: row.BID, OwnerID: row.BOwnerID, Title: row.Title, Author: row.Author, MetadataProvenance: row.MetadataProvenance, LanguageState: row.LanguageState, LanguageTag: row.LanguageTag, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, true, nil
}

func aliasConflictError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAliasConflict
	}
	return err
}

func (s *PostgresStore) AddBookAlias(ctx context.Context, owner, bookID, aliasType, namespace, value string) (err error) {
	alias := domain.BookAlias{OwnerID: owner, BookID: bookID, AliasType: aliasType, Namespace: strings.TrimSpace(namespace), Value: strings.TrimSpace(value)}
	if err := alias.Validate(); err != nil {
		return err
	}
	if aliasType == domain.AliasCatalogEntry {
		return errors.New("persistence: catalogue entry alias requires a connection")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err := ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	_, err = sqlcgen.New(tx).InsertBookAlias(ctx, sqlcgen.InsertBookAliasParams{OwnerID: owner, BookID: bookID, AliasType: aliasType, Namespace: alias.Namespace, Value: alias.Value})
	if errors.Is(err, pgx.ErrNoRows) {
		var existingBook string
		existingBook, err = sqlcgen.New(tx).GetUnscopedAliasBookForUpdate(ctx, sqlcgen.GetUnscopedAliasBookForUpdateParams{OwnerID: owner, Namespace: alias.Namespace, Value: alias.Value})
		if err != nil {
			return err
		}
		if existingBook != bookID {
			return ErrAliasConflict
		}
		if err := activateMembership(ctx, tx, owner, bookID); err != nil {
			return err
		}
	} else if err != nil {
		return aliasConflictError(err)
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) LinkSourceToBook(ctx context.Context, owner, bookID, sourceMaterialID string) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err := ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	current, err := sqlcgen.New(tx).GetSourceMaterialBookForUpdate(ctx, sqlcgen.GetSourceMaterialBookForUpdateParams{OwnerID: owner, ID: sourceMaterialID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current != "" && current != bookID {
		return ErrSourceBookConflict
	}
	if current == "" {
		if err := sqlcgen.New(tx).SetSourceMaterialBook(ctx, sqlcgen.SetSourceMaterialBookParams{OwnerID: owner, ID: sourceMaterialID, BookID: uuidArg(bookID)}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ResolveOrCreateBookForAcquisition(ctx context.Context, owner, sourceIdentifier, language, title string) (string, error) {
	return s.resolveOrCreateBookForAcquisition(ctx, owner, "", sourceIdentifier, language, title)
}

// CatalogueBookMetadataResult reports whether a sync updated a Book linked to
// a Catalog index entry. A result with an empty Book means the entry is not
// linked to any Book, so sync leaves My Books untouched.
type CatalogueBookMetadataResult struct {
	Book    domain.Book
	Updated bool
}

// UpdateCatalogueBookMetadata propagates one Catalog entry's title and author
// to the learner's Book linked to that entry by catalog-entry alias. It never
// creates a Book, activates membership, writes a disposition, or changes the
// Book's language: entering My Books remains the learner's Add action. Source
// materials and acquired content are never touched.
func (s *PostgresStore) UpdateCatalogueBookMetadata(ctx context.Context, owner, connectionID, sourceIdentifier, title, author string) (result CatalogueBookMetadataResult, err error) {
	owner, connectionID, sourceIdentifier, title, author = strings.TrimSpace(owner), strings.TrimSpace(connectionID), strings.TrimSpace(sourceIdentifier), strings.TrimSpace(title), strings.TrimSpace(author)
	if owner == "" || connectionID == "" || sourceIdentifier == "" || title == "" {
		return CatalogueBookMetadataResult{}, errors.New("persistence: catalogue entry identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogueBookMetadataResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	// Advisory lock serializes catalogue identity reconciliation; it is a
	// domain fence rather than a data query and therefore remains raw SQL.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,468))`, owner+":"+connectionID+":"+sourceIdentifier); err != nil {
		return CatalogueBookMetadataResult{}, err
	}
	bookID, err := q.GetCatalogueAliasBookForUpdate(ctx, sqlcgen.GetCatalogueAliasBookForUpdateParams{OwnerID: owner, ConnectionID: uuidArg(connectionID), Namespace: domain.NamespaceSourceIdentifier, Value: sourceIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		return CatalogueBookMetadataResult{}, nil
	}
	if err != nil {
		return CatalogueBookMetadataResult{}, err
	}
	if _, err = q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID}); err != nil {
		return CatalogueBookMetadataResult{}, err
	}
	current, err := q.GetBookMetadata(ctx, sqlcgen.GetBookMetadataParams{OwnerID: owner, ID: bookID})
	if err != nil {
		return CatalogueBookMetadataResult{}, err
	}
	book, err := q.GetBook(ctx, sqlcgen.GetBookParams{OwnerID: owner, ID: bookID})
	if err != nil {
		return CatalogueBookMetadataResult{}, err
	}
	result.Book = domain.Book(book)
	if strings.TrimSpace(current.Title) == title && strings.TrimSpace(current.Author) == author {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return CatalogueBookMetadataResult{}, commitErr
		}
		return result, nil
	}
	updated, err := q.UpdateBookMetadata(ctx, sqlcgen.UpdateBookMetadataParams{
		OwnerID: owner, ID: bookID, Title: title, Author: author,
		LanguageState: current.LanguageState, LanguageTag: nullableTextArg(nullableLanguageTag(current.LanguageState, current.LanguageTag)),
	})
	if err != nil {
		return CatalogueBookMetadataResult{}, err
	}
	result.Book, result.Updated = domain.Book(updated), true
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return CatalogueBookMetadataResult{}, commitErr
	}
	return result, nil
}

func removeIncompatibleBookGoals(ctx context.Context, q *sqlcgen.Queries, owner, bookID, language string) error {
	return q.DeleteBookGoalsExceptLanguage(ctx, sqlcgen.DeleteBookGoalsExceptLanguageParams{OwnerID: owner, BookID: bookID, Language: language})
}

// ResolveOrCreateBookForAcquisitionForBook promotes an explicitly selected
// metadata-only Book. The Book ID is still checked in the same owner scope as
// the source identifier and alias, so a stale or cross-owner form cannot
// redirect acquired evidence.
func (s *PostgresStore) ResolveOrCreateBookForAcquisitionForBook(ctx context.Context, owner, bookID, sourceIdentifier, language, title string) (string, error) {
	return s.resolveOrCreateBookForAcquisition(ctx, owner, bookID, sourceIdentifier, language, title)
}

func (s *PostgresStore) resolveOrCreateBookForAcquisition(ctx context.Context, owner, requestedBookID, sourceIdentifier, language, title string) (result string, err error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(sourceIdentifier) == "" || strings.TrimSpace(language) == "" || strings.TrimSpace(title) == "" {
		return "", errors.New("persistence: acquisition book identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	// Advisory lock serializes acquisition identity resolution; it is a domain
	// fence rather than a data query and therefore remains raw SQL.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,466))`, owner+":"+sourceIdentifier); err != nil {
		return "", err
	}
	bookID := requestedBookID
	if requestedBookID != "" {
		if err := confirmAcquisitionBook(ctx, tx, q, owner, requestedBookID, sourceIdentifier); err != nil {
			return "", err
		}
	} else {
		bookID, err = matchAcquisitionBook(ctx, tx, q, owner, sourceIdentifier)
		if err != nil {
			return "", err
		}
		if bookID == "" {
			bookID, err = createAcquisitionBook(ctx, tx, q, owner, language, title)
			if err != nil {
				return "", err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return bookID, nil
}

// confirmAcquisitionBook validates an explicitly selected Book against the
// source identifier's linked material and connectionless alias, activating its
// membership when both agree. A disagreement is a conflict.
func confirmAcquisitionBook(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, requestedBookID, sourceIdentifier string) error {
	if err := ensureBookExists(ctx, tx, owner, requestedBookID); err != nil {
		return err
	}
	linkedBook, err := q.GetSourceMaterialBookByIdentifierForUpdate(ctx, sqlcgen.GetSourceMaterialBookByIdentifierForUpdateParams{OwnerID: owner, SourceIdentifier: sourceIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		linkedBook = ""
	} else if err != nil {
		return err
	}
	if linkedBook != "" && linkedBook != requestedBookID {
		return ErrSourceBookConflict
	}
	aliasedBook, err := q.GetUnscopedAliasBookForUpdate(ctx, sqlcgen.GetUnscopedAliasBookForUpdateParams{OwnerID: owner, Namespace: domain.NamespaceSourceIdentifier, Value: sourceIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		aliasedBook = ""
	} else if err != nil {
		return err
	}
	if aliasedBook != "" && aliasedBook != requestedBookID {
		return ErrAliasConflict
	}
	return activateMembership(ctx, tx, owner, requestedBookID)
}

// matchAcquisitionBook returns the Book already linked to or connectionless-
// aliased by the source identifier, activating its membership. It returns ""
// when no Book matches and the caller must create one.
func matchAcquisitionBook(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, sourceIdentifier string) (string, error) {
	linkedBook, err := q.GetSourceMaterialBookByIdentifierForUpdate(ctx, sqlcgen.GetSourceMaterialBookByIdentifierForUpdateParams{OwnerID: owner, SourceIdentifier: sourceIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		linkedBook = ""
	} else if err != nil {
		return "", err
	}
	if linkedBook != "" {
		if err := ensureBookExists(ctx, tx, owner, linkedBook); err != nil {
			return "", err
		}
		if err := activateMembership(ctx, tx, owner, linkedBook); err != nil {
			return "", err
		}
		return linkedBook, nil
	}
	bookRow, err := q.GetBookByUnscopedAliasForUpdate(ctx, sqlcgen.GetBookByUnscopedAliasForUpdateParams{OwnerID: owner, Namespace: domain.NamespaceSourceIdentifier, Value: sourceIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := activateMembership(ctx, tx, owner, bookRow.BID); err != nil {
		return "", err
	}
	return bookRow.BID, nil
}

// createAcquisitionBook creates a new acquisition-provenance Book, activates
// its membership, and makes it To Read.
func createAcquisitionBook(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language, title string) (string, error) {
	createdRow, err := q.InsertBook(ctx, sqlcgen.InsertBookParams{OwnerID: owner, Title: title, Author: "", MetadataProvenance: domain.MetadataProvenanceAcquisition, LanguageState: domain.LanguageChosen, LanguageTag: textArg(language)})
	if err != nil {
		return "", err
	}
	if err := activateMembership(ctx, tx, owner, createdRow.ID); err != nil {
		return "", err
	}
	return createdRow.ID, nil
}
