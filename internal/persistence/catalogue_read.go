package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// MarkCatalogueEntryPreviouslyRead creates or restores the learner's Book for
// one indexed Catalog entry as Read. It records one Previously read assertion,
// acquires nothing, starts no job and changes no Known vocabulary. The catalog
// entry's current metadata refreshes the restored Book, exactly as a sync
// would.
func (s *PostgresStore) MarkCatalogueEntryPreviouslyRead(ctx context.Context, owner, connectionID, entryIdentifier string) (result domain.CatalogueReadResult, err error) {
	if owner == "" || connectionID == "" || entryIdentifier == "" {
		return domain.CatalogueReadResult{}, errors.New("persistence: catalog previously read identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	// The connection row lock serializes this assertion with a complete index
	// sync, so an entry cannot leave the index between the read and the write.
	if _, err := q.LockCatalogueIndexConnection(ctx, sqlcgen.LockCatalogueIndexConnectionParams{OwnerID: uuidArg(owner), ConnectionID: connectionID}); err != nil {
		return domain.CatalogueReadResult{}, missing(err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, $2))`, owner+":"+connectionID+":"+entryIdentifier, catalogueAddLockSalt); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	entry, err := q.GetCatalogueEntry(ctx, sqlcgen.GetCatalogueEntryParams{OwnerID: uuidArg(owner), ConnectionID: connectionID, EntryIdentifier: entryIdentifier})
	if err != nil {
		return domain.CatalogueReadResult{}, missing(err)
	}
	language := canonicalization.NormalizeLanguage(entry.LanguageTag)
	if language == "" {
		return domain.CatalogueReadResult{}, errors.New("persistence: catalog entry has no language")
	}
	if err := q.PutSupportedLanguageOrIgnore(ctx, language); err != nil {
		return domain.CatalogueReadResult{}, err
	}

	bookID, err := q.GetCatalogueAliasBookForUpdate(ctx, sqlcgen.GetCatalogueAliasBookForUpdateParams{
		OwnerID: owner, ConnectionID: uuidArg(connectionID), Namespace: domain.NamespaceSourceIdentifier, Value: entryIdentifier,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		result, err = createReadCatalogueBookTx(ctx, q, owner, connectionID, entryIdentifier, entry.Title, entry.Author, language)
	case err != nil:
		return domain.CatalogueReadResult{}, err
	default:
		result, err = restoreReadCatalogueBookTx(ctx, q, owner, bookID, entry.Title, entry.Author, language)
	}
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	return result, nil
}

// createReadCatalogueBookTx creates the metadata-only Book, its active
// membership, its not-To-Read disposition and its Previously read record in one
// transaction.
func createReadCatalogueBookTx(ctx context.Context, q *sqlcgen.Queries, owner, connectionID, entryIdentifier, title, author, language string) (domain.CatalogueReadResult, error) {
	book, err := q.InsertBook(ctx, sqlcgen.InsertBookParams{
		OwnerID:            owner,
		Title:              title,
		Author:             author,
		MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        textArg(language),
	})
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := q.InsertBookMembership(ctx, sqlcgen.InsertBookMembershipParams{OwnerID: owner, BookID: book.ID}); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := upsertBookDisposition(ctx, q, owner, book.ID, domain.BookDispositionInbox); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if err := q.InsertCatalogueEntryAlias(ctx, sqlcgen.InsertCatalogueEntryAliasParams{
		OwnerID: owner, BookID: book.ID, ConnectionID: uuidArg(connectionID),
		AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: entryIdentifier,
	}); err != nil {
		return domain.CatalogueReadResult{}, aliasConflictError(err)
	}
	// An entry whose cover is already retained shows it at once (ADR 0130).
	if err := selectBookCoversFromEntry(ctx, q, owner, connectionID, entryIdentifier); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	if _, err := insertPreviouslyReadTx(ctx, q, owner, book.ID); err != nil {
		return domain.CatalogueReadResult{}, err
	}
	return domain.CatalogueReadResult{Book: domain.Book(book), Created: true, Changed: true}, nil
}

// restoreReadCatalogueBookTx makes an existing Book reachable again and records
// it as Read. Analysis, corrections, exclusions and every other piece of
// evidence stay untouched, and a Book already read keeps its existing history.
func restoreReadCatalogueBookTx(ctx context.Context, q *sqlcgen.Queries, owner, bookID, title, author, language string) (domain.CatalogueReadResult, error) {
	changed, err := restoreReadBookMembershipTx(ctx, q, owner, bookID)
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	book, err := q.GetBookMetadata(ctx, sqlcgen.GetBookMetadataParams{OwnerID: owner, ID: bookID})
	if err != nil {
		return domain.CatalogueReadResult{}, missing(err)
	}
	if book.LanguageState != domain.LanguageChosen || canonicalization.NormalizeLanguage(book.LanguageTag) != language {
		changed = true
		if err := removeIncompatibleBookGoals(ctx, q, owner, bookID, language); err != nil {
			return domain.CatalogueReadResult{}, err
		}
	}
	updated, err := q.UpdateBookMetadata(ctx, sqlcgen.UpdateBookMetadataParams{
		OwnerID: owner, ID: bookID, Title: title, Author: author,
		LanguageState: domain.LanguageChosen, LanguageTag: textArg(language),
	})
	if err != nil {
		return domain.CatalogueReadResult{}, missing(err)
	}
	recorded, err := insertPreviouslyReadTx(ctx, q, owner, bookID)
	if err != nil {
		return domain.CatalogueReadResult{}, err
	}
	return domain.CatalogueReadResult{Book: domain.Book(updated), Changed: changed || recorded}, nil
}

// restoreReadBookMembershipTx locks an existing Book, refuses the Current
// reading, sets the not-To-Read disposition and reactivates membership. It
// reports whether any of those facts changed.
func restoreReadBookMembershipTx(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) (bool, error) {
	if err := lockBookForUpdate(ctx, q, owner, bookID); err != nil {
		return false, err
	}
	current, err := q.BookHasActiveCurrentReading(ctx, sqlcgen.BookHasActiveCurrentReadingParams{Owner: owner, Book: bookID})
	if err != nil {
		return false, err
	}
	if current {
		return false, domain.ErrBookIsCurrentReading
	}
	changed, err := membershipOrDispositionChanged(ctx, q, owner, bookID)
	if err != nil {
		return false, err
	}
	if err := upsertBookDisposition(ctx, q, owner, bookID, domain.BookDispositionInbox); err != nil {
		return false, err
	}
	if err := q.ActivateBookMembership(ctx, sqlcgen.ActivateBookMembershipParams{OwnerID: owner, BookID: bookID}); err != nil {
		return false, err
	}
	return changed, nil
}

// membershipOrDispositionChanged reports whether reactivation or the not-To-Read
// disposition would change an existing Book.
func membershipOrDispositionChanged(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) (bool, error) {
	membership, err := q.GetBookMembershipForUpdate(ctx, sqlcgen.GetBookMembershipForUpdateParams{Owner: owner, Book: bookID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return true, nil
	case err != nil:
		return false, err
	}
	disposition, err := q.GetBookDispositionStateForUpdate(ctx, sqlcgen.GetBookDispositionStateForUpdateParams{OwnerID: owner, BookID: bookID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return true, nil
	case err != nil:
		return false, err
	}
	return membership != "active" || domain.BookDisposition(disposition.Disposition) != domain.BookDispositionInbox, nil
}

// insertPreviouslyReadTx records one Previously read assertion for an active,
// chosen-language Book without history. A Book that already has history, or is
// the Current reading, keeps it: the assertion is idempotent.
func insertPreviouslyReadTx(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) (bool, error) {
	_, err := q.InsertPreviouslyReadImport(ctx, sqlcgen.InsertPreviouslyReadImportParams{
		Owner: owner, Book: bookID, CompletedAt: time.Now().UTC(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
