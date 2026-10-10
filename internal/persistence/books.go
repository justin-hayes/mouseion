package persistence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// LanguageCount is one owner-scoped language pill count. Tag is "unknown"
// for books whose language state is unknown.
type LanguageCount struct {
	Tag   string
	Count int
}

type DispositionCount struct {
	Disposition domain.BookDisposition
	Count       int
}

type MyBooksBrowseResult struct {
	Items             []domain.MyBook
	ScopeTotal        int
	Total             int
	Counts            []LanguageCount
	DispositionCounts []DispositionCount
	AllCount          int
	HiddenCount       int
	ReadCount         int
}

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
	return books, err
}

// ListMyBooksBrowse returns one owner-scoped page of My Books. The domain's
// WorkflowBucket derivation is the source of truth for both filters and counts;
// SQL supplies the facts (disposition, current-reading role, and history) but
// does not independently classify a visible bucket.
func (s *PostgresStore) ListMyBooksBrowse(ctx context.Context, owner, query, language, disposition string, history bool, offset, limit int) (MyBooksBrowseResult, error) {
	return s.ListMyBooksBrowseWithVisibility(ctx, owner, query, language, disposition, history, false, offset, limit)
}

// ListMyBooksBrowseWithVisibility is ListMyBooksBrowse with an explicit
// visibility scope. Hidden Books are omitted unless showHidden is set, and
// then bucket, language, and total counts describe the same scope. AllCount
// stays the complete collection size and HiddenCount reports the Hidden Books
// in the language scope regardless of the selected visibility, so empty views
// can still offer recovery.
func (s *PostgresStore) ListMyBooksBrowseWithVisibility(ctx context.Context, owner, query, language, disposition string, history, showHidden bool, offset, limit int) (MyBooksBrowseResult, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	language = strings.TrimSpace(language)
	disposition = strings.TrimSpace(disposition)
	if disposition != "" {
		if err := domain.BookDisposition(disposition).Validate(); err != nil {
			return MyBooksBrowseResult{}, err
		}
	}
	if language != domain.LanguageUnknown {
		language = canonicalization.NormalizeLanguage(language)
	}
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	var result MyBooksBrowseResult
	pageEnd := offset + limit
	if pageEnd < offset { // Guard integer overflow from caller-supplied pagination.
		pageEnd = int(^uint(0) >> 1)
	}
	dispositionCounts := make(map[domain.MyBookBucket]int)
	var filteredCount int
	err := s.forEachMyBooksBrowseRow(ctx, owner, language, func(row sqlcgen.BrowseMyBooksEvidenceRow) error {
		book := myBookFromBrowseRow(row)
		book.Disposition = domain.BookDisposition(row.Disposition)
		book.DispositionRevision = row.DispositionRevision
		if book.Hidden && !showHidden {
			return nil
		}
		bucket := book.WorkflowBucket()
		dispositionCounts[bucket]++
		if bucket == domain.MyBookBucketCurrentReading {
			dispositionCounts[domain.MyBookBucketToRead]++
		}
		if bucket == domain.MyBookBucketRead {
			result.ReadCount++
		}
		if query != "" && !strings.Contains(strings.ToLower(book.Book.Title), query) && !strings.Contains(strings.ToLower(book.Book.Author), query) {
			return nil
		}
		if !bucket.MatchesBrowseFilter(domain.BookDisposition(disposition), history) {
			return nil
		}
		if filteredCount >= offset && filteredCount < pageEnd {
			result.Items = append(result.Items, book)
		}
		filteredCount++
		return nil
	})
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	if err := s.populateMyBooksCoverage(ctx, owner, result.Items); err != nil {
		return MyBooksBrowseResult{}, err
	}
	for _, item := range []struct {
		bucket domain.MyBookBucket
	}{
		{bucket: domain.MyBookBucketInbox},
		{bucket: domain.MyBookBucketToRead},
	} {
		if count := dispositionCounts[item.bucket]; count > 0 {
			persistedDisposition, ok := item.bucket.PersistedDisposition()
			if ok {
				result.DispositionCounts = append(result.DispositionCounts, DispositionCount{Disposition: persistedDisposition, Count: count})
			}
		}
	}
	result.Total = filteredCount
	q := s.queries()
	scopeTotal, err := q.CountMyBooksScope(ctx, sqlcgen.CountMyBooksScopeParams{Owner: owner, Language: language, ShowHidden: showHidden})
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	result.ScopeTotal, err = checked.IntFromInt64(scopeTotal)
	if err != nil {
		return MyBooksBrowseResult{}, fmt.Errorf("invalid scoped book count: %w", err)
	}
	hiddenCount, err := q.CountHiddenMyBooksScope(ctx, sqlcgen.CountHiddenMyBooksScopeParams{Owner: owner, Language: language})
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	result.HiddenCount, err = checked.IntFromInt64(hiddenCount)
	if err != nil {
		return MyBooksBrowseResult{}, fmt.Errorf("invalid hidden book count: %w", err)
	}
	allCount, err := q.CountMyBooksAll(ctx, owner)
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	result.AllCount, err = checked.IntFromInt64(allCount)
	if err != nil {
		return MyBooksBrowseResult{}, fmt.Errorf("invalid total book count: %w", err)
	}
	counts, err := q.CountMyBooksByLanguage(ctx, sqlcgen.CountMyBooksByLanguageParams{Owner: owner, ShowHidden: showHidden})
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	for _, count := range counts {
		bookCount, conversionErr := checked.IntFromInt64(count.BookCount)
		if conversionErr != nil {
			return MyBooksBrowseResult{}, fmt.Errorf("invalid book count for language %q: %w", count.LanguageTag, conversionErr)
		}
		result.Counts = append(result.Counts, LanguageCount{Tag: count.LanguageTag, Count: bookCount})
	}
	sort.Slice(result.Counts, func(i, j int) bool {
		if result.Counts[i].Tag == "unknown" {
			return false
		}
		if result.Counts[j].Tag == "unknown" {
			return true
		}
		return result.Counts[i].Tag < result.Counts[j].Tag
	})
	return result, nil
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
	book := myBookFromEvidence(row)
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
	if book.Hidden, book.VisibilityRevision, err = s.GetBookVisibility(ctx, owner, book.Book.ID); err != nil {
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
	return book, nil
}

// GetBookDetailForMyBooksRefresh loads the otherwise-expensive current margin
// evidence only for the HTMX row refresh path that renders the My Books row.
func (s *PostgresStore) GetBookDetailForMyBooksRefresh(ctx context.Context, owner, id string) (domain.MyBook, error) {
	book, err := s.GetBookDetail(ctx, owner, id)
	if err != nil {
		return domain.MyBook{}, err
	}
	margin, err := s.queries().GetMyBookMarginEvidence(ctx, sqlcgen.GetMyBookMarginEvidenceParams{Owner: owner, Book: book.Book.ID})
	if err != nil {
		return domain.MyBook{}, err
	}
	book.DeckState = margin.DeckState
	book.DeckCardCount = margin.DeckTotalCards
	book.IsCurrentReading = margin.IsCurrentReading
	if margin.DeckCompletedAt.Valid {
		book.DeckPreparedAt = &margin.DeckCompletedAt.Time
	}
	page := []domain.MyBook{book}
	if err := s.populateMyBooksCoverage(ctx, owner, page); err != nil {
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

// ListUnscopedCatalogueEntryAliases returns only legacy catalogue-entry aliases
// that still need the application-logic connection backfill. Strong
// bibliographic aliases are intentionally excluded.
func (s *PostgresStore) ListUnscopedCatalogueEntryAliases(ctx context.Context) ([]domain.BookAlias, error) {
	rows, err := s.queries().ListUnscopedCatalogueEntryAliases(ctx, sqlcgen.ListUnscopedCatalogueEntryAliasesParams{AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier})
	if err != nil {
		return nil, err
	}
	var aliases []domain.BookAlias
	for _, row := range rows {
		aliases = append(aliases, domain.BookAlias(row))
	}
	return aliases, nil
}

// SetCatalogueEntryAliasConnection assigns a legacy alias only while it is
// unscoped and only to a connection owned by the same learner.
func (s *PostgresStore) SetCatalogueEntryAliasConnection(ctx context.Context, owner, aliasID, connectionID string) error {
	tag, err := s.queries().SetCatalogueEntryAliasConnection(ctx, sqlcgen.SetCatalogueEntryAliasConnectionParams{OwnerID: owner, ID: aliasID, ConnectionID: uuidArg(connectionID), AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier})
	if err != nil {
		return aliasConflictError(err)
	}
	if tag == 0 {
		current, lookupErr := s.queries().GetBookAliasConnection(ctx, sqlcgen.GetBookAliasConnectionParams{OwnerID: owner, ID: aliasID})
		err = lookupErr
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if current == connectionID {
			return nil
		}
		return ErrAliasConflict
	}
	return nil
}

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
		return q.InsertInboxBookDisposition(ctx, sqlcgen.InsertInboxBookDispositionParams{OwnerID: created.OwnerID, BookID: created.ID})
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
	if err = candidate.Validate(); err != nil {
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
			_, err = removeIncompatibleBookGoals(ctx, q, owner, bookID, languageTag)
			return err
		}
		snapshotIDs, err := q.LockCurrentReadingsForAllLanguages(ctx, sqlcgen.LockCurrentReadingsForAllLanguagesParams{Owner: owner, Book: bookID})
		if err != nil {
			return err
		}
		if err := releaseCurrentReadingSnapshots(ctx, q, owner, snapshotIDs); err != nil {
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

func activateMembership(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	return sqlcgen.New(tx).ActivateBookMembership(ctx, sqlcgen.ActivateBookMembershipParams{OwnerID: owner, BookID: bookID})
}

func (s *PostgresStore) AddBookToMyBooks(ctx context.Context, owner, bookID string) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	if err = activateMembership(ctx, tx, owner, bookID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) RemoveBookFromMyBooks(ctx context.Context, owner, bookID string) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	if err = sqlcgen.New(tx).RemoveBookMembership(ctx, sqlcgen.RemoveBookMembershipParams{OwnerID: owner, BookID: bookID}); err != nil {
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
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
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
		if err = activateMembership(ctx, tx, owner, bookID); err != nil {
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
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
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
		if err = sqlcgen.New(tx).SetSourceMaterialBook(ctx, sqlcgen.SetSourceMaterialBookParams{OwnerID: owner, ID: sourceMaterialID, BookID: uuidArg(bookID)}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ResolveOrCreateBookForAcquisition(ctx context.Context, owner, sourceIdentifier, language, title string) (string, error) {
	return s.resolveOrCreateBookForAcquisition(ctx, owner, "", sourceIdentifier, language, title)
}

// CatalogueEntryReconcileResult reports what a metadata-only catalogue upsert
// actually changed, so connection status can distinguish "synced with changes"
// from "synced with no changes" without re-querying or implying content work.
type CatalogueEntryReconcileResult struct {
	Book            domain.Book
	Created         bool
	TitleChanged    bool
	LanguageChanged bool
	// CurrentReadingEnded reports that an incompatible Current reading was ended
	// (reservations released, no completion) by the language correction.
	CurrentReadingEnded bool
	AuthorChanged       bool
}

// Upserted reports whether the entry added or updated a Book.
func (r CatalogueEntryReconcileResult) Upserted() bool {
	return r.Created || r.TitleChanged || r.LanguageChanged || r.AuthorChanged
}

// ReconcileCatalogueEntry performs the metadata-only, owner-scoped catalogue
// upsert. New Books are created with a chosen language only when the catalogue
// sync scope determines that language deterministically from the catalogue
// entry (never inferred). Source materials and acquired content are never
// touched here: membership and identity are metadata-only.

func (s *PostgresStore) ReconcileCatalogueEntry(ctx context.Context, owner, connectionID, sourceIdentifier, title, author, language string) (result CatalogueEntryReconcileResult, err error) {
	owner, connectionID, sourceIdentifier, title, author, language = strings.TrimSpace(owner), strings.TrimSpace(connectionID), strings.TrimSpace(sourceIdentifier), strings.TrimSpace(title), strings.TrimSpace(author), strings.TrimSpace(language)
	if owner == "" || connectionID == "" || sourceIdentifier == "" || title == "" || language == "" {
		return CatalogueEntryReconcileResult{}, errors.New("persistence: catalogue entry identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	// Advisory lock serializes catalogue identity reconciliation; it is a
	// domain fence rather than a data query and therefore remains raw SQL.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,468))`, owner+":"+connectionID+":"+sourceIdentifier); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	aliasBook, err := q.GetCatalogueAliasBookForUpdate(ctx, sqlcgen.GetCatalogueAliasBookForUpdateParams{OwnerID: owner, ConnectionID: uuidArg(connectionID), Namespace: domain.NamespaceSourceIdentifier, Value: sourceIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		aliasBook = ""
	} else if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	bookID := ""
	if aliasBook != "" {
		bookID = aliasBook
	}
	created := bookID == ""
	var titleChanged, authorChanged, languageChanged, readingEnded bool
	if bookID == "" {
		createdBook, insertErr := q.InsertBook(ctx, sqlcgen.InsertBookParams{OwnerID: owner, Title: title, Author: author, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: textArg(language)})
		err = insertErr
		if err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
		bookID = createdBook.ID
	} else {
		if _, err = q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID}); err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
		current, getErr := q.GetBookMetadata(ctx, sqlcgen.GetBookMetadataParams{OwnerID: owner, ID: bookID})
		if err = getErr; err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
		titleChanged = strings.TrimSpace(current.Title) != title
		authorChanged = strings.TrimSpace(current.Author) != author
		languageChanged = current.LanguageState != domain.LanguageChosen || current.LanguageTag != language
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if _, err = q.UpdateBookMetadata(ctx, sqlcgen.UpdateBookMetadataParams{OwnerID: owner, ID: bookID, Title: title, Author: author, LanguageState: domain.LanguageChosen, LanguageTag: textArg(language)}); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if languageChanged {
		if readingEnded, err = removeIncompatibleBookGoals(ctx, q, owner, bookID, language); err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
	}
	if err = activateMembership(ctx, tx, owner, bookID); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if created {
		if err = q.InsertInboxBookDisposition(ctx, sqlcgen.InsertInboxBookDispositionParams{OwnerID: owner, BookID: bookID}); err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
	}
	if aliasBook == "" {
		if err = q.InsertCatalogueEntryAlias(ctx, sqlcgen.InsertCatalogueEntryAliasParams{OwnerID: owner, BookID: bookID, ConnectionID: uuidArg(connectionID), AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: sourceIdentifier}); err != nil {
			return CatalogueEntryReconcileResult{}, aliasConflictError(err)
		}
	}
	bookRow, err := q.GetBook(ctx, sqlcgen.GetBookParams{OwnerID: owner, ID: bookID})
	if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	book := domain.Book(bookRow)
	if err = tx.Commit(ctx); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	return CatalogueEntryReconcileResult{Book: book, Created: created, TitleChanged: titleChanged, AuthorChanged: authorChanged, LanguageChanged: languageChanged, CurrentReadingEnded: readingEnded}, nil
}

func removeIncompatibleBookGoals(ctx context.Context, q *sqlcgen.Queries, owner, bookID, language string) (bool, error) {
	snapshotIDs, err := q.LockCurrentReadingsExceptLanguage(ctx, sqlcgen.LockCurrentReadingsExceptLanguageParams{Owner: owner, Book: bookID, Language: language})
	if err != nil {
		return false, err
	}
	if err = releaseCurrentReadingSnapshots(ctx, q, owner, snapshotIDs); err != nil {
		return false, err
	}
	if err = q.DeleteBookGoalsExceptLanguage(ctx, sqlcgen.DeleteBookGoalsExceptLanguageParams{OwnerID: owner, BookID: bookID, Language: language}); err != nil {
		return false, err
	}
	return len(snapshotIDs) > 0, nil
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
	if requestedBookID != "" {
		if err = ensureBookExists(ctx, tx, owner, requestedBookID); err != nil {
			return "", err
		}
		linkedBook, getErr := q.GetSourceMaterialBookByIdentifierForUpdate(ctx, sqlcgen.GetSourceMaterialBookByIdentifierForUpdateParams{OwnerID: owner, SourceIdentifier: sourceIdentifier})
		err = getErr
		if errors.Is(err, pgx.ErrNoRows) {
			linkedBook = ""
		} else if err != nil {
			return "", err
		}
		if linkedBook != "" && linkedBook != requestedBookID {
			return "", ErrSourceBookConflict
		}
		aliasedBook, getErr := q.GetUnscopedAliasBookForUpdate(ctx, sqlcgen.GetUnscopedAliasBookForUpdateParams{OwnerID: owner, Namespace: domain.NamespaceSourceIdentifier, Value: sourceIdentifier})
		err = getErr
		if errors.Is(err, pgx.ErrNoRows) {
			aliasedBook = ""
		} else if err != nil {
			return "", err
		}
		if aliasedBook != "" && aliasedBook != requestedBookID {
			return "", ErrAliasConflict
		}
		if err = activateMembership(ctx, tx, owner, requestedBookID); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return requestedBookID, nil
	}
	linkedBook, getErr := q.GetSourceMaterialBookByIdentifierForUpdate(ctx, sqlcgen.GetSourceMaterialBookByIdentifierForUpdateParams{OwnerID: owner, SourceIdentifier: sourceIdentifier})
	err = getErr
	if errors.Is(err, pgx.ErrNoRows) {
		linkedBook = ""
	} else if err != nil {
		return "", err
	}
	if linkedBook != "" {
		if err = ensureBookExists(ctx, tx, owner, linkedBook); err != nil {
			return "", err
		}
		if err = activateMembership(ctx, tx, owner, linkedBook); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return linkedBook, nil
	}

	var bookID string
	bookRow, getErr := q.GetBookByUnscopedAliasForUpdate(ctx, sqlcgen.GetBookByUnscopedAliasForUpdateParams{OwnerID: owner, Namespace: domain.NamespaceSourceIdentifier, Value: sourceIdentifier})
	err = getErr
	if errors.Is(err, pgx.ErrNoRows) {
		bookID = ""
	} else if err != nil {
		return "", err
	} else {
		bookID = bookRow.BID
	}
	if bookID != "" {
		if err = activateMembership(ctx, tx, owner, bookID); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return bookID, nil
	}

	createdRow, err := q.InsertBook(ctx, sqlcgen.InsertBookParams{OwnerID: owner, Title: title, Author: "", MetadataProvenance: domain.MetadataProvenanceAcquisition, LanguageState: domain.LanguageChosen, LanguageTag: textArg(language)})
	if err != nil {
		return "", err
	}
	if err = activateMembership(ctx, tx, owner, createdRow.ID); err != nil {
		return "", err
	}
	if err = q.InsertInboxBookDisposition(ctx, sqlcgen.InsertInboxBookDispositionParams{OwnerID: owner, BookID: createdRow.ID}); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return createdRow.ID, nil
}
