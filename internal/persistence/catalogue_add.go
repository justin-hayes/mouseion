package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// catalogueAddLockSalt namespaces the per-entry advisory lock that Add takes,
// so concurrent Adds of one catalog entry serialize before either writes an
// alias.
const catalogueAddLockSalt = 470

// AddCatalogueEntry creates the learner's To Read Book for one indexed Catalog
// entry, or restores the Book its catalog-entry alias already names. It reuses
// the Book's identity and evidence, clears Hidden, reactivates membership and
// sets To Read; the Add route submits preparation after this transaction
// commits. Replaying Add on an already active To Read Book changes nothing.
func (s *PostgresStore) AddCatalogueEntry(ctx context.Context, owner, connectionID, entryIdentifier string) (result domain.CatalogueAddResult, err error) {
	owner, connectionID, entryIdentifier = strings.TrimSpace(owner), strings.TrimSpace(connectionID), strings.TrimSpace(entryIdentifier)
	if owner == "" || connectionID == "" || entryIdentifier == "" {
		return domain.CatalogueAddResult{}, errors.New("persistence: catalog Add identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CatalogueAddResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	// The connection row lock serializes Add with a complete index sync, so an
	// entry cannot leave the index between the read and the Book write.
	if _, err := q.LockCatalogueIndexConnection(ctx, sqlcgen.LockCatalogueIndexConnectionParams{OwnerID: uuidArg(owner), ConnectionID: connectionID}); err != nil {
		return domain.CatalogueAddResult{}, missing(err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, $2))`, owner+":"+connectionID+":"+entryIdentifier, catalogueAddLockSalt); err != nil {
		return domain.CatalogueAddResult{}, err
	}
	entry, err := q.GetCatalogueEntry(ctx, sqlcgen.GetCatalogueEntryParams{OwnerID: uuidArg(owner), ConnectionID: connectionID, EntryIdentifier: entryIdentifier})
	if err != nil {
		return domain.CatalogueAddResult{}, missing(err)
	}
	language := canonicalization.NormalizeLanguage(entry.LanguageTag)
	if language == "" {
		return domain.CatalogueAddResult{}, errors.New("persistence: catalog entry has no language")
	}
	if err := q.PutSupportedLanguageOrIgnore(ctx, language); err != nil {
		return domain.CatalogueAddResult{}, err
	}

	bookID, err := q.GetCatalogueAliasBookForUpdate(ctx, sqlcgen.GetCatalogueAliasBookForUpdateParams{
		OwnerID: owner, ConnectionID: uuidArg(connectionID), Namespace: domain.NamespaceSourceIdentifier, Value: entryIdentifier,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		result, err = createCatalogueBookTx(ctx, q, owner, connectionID, entryIdentifier, entry.Title, entry.Author, language)
	case err != nil:
		return domain.CatalogueAddResult{}, err
	default:
		result, err = restoreCatalogueBookTx(ctx, q, owner, bookID, entry.Title, entry.Author, language)
	}
	if err != nil {
		return domain.CatalogueAddResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CatalogueAddResult{}, err
	}
	return result, nil
}

// createCatalogueBookTx creates the metadata-only Book, its active membership,
// its To Read intent and the catalog-entry alias in one transaction.
func createCatalogueBookTx(ctx context.Context, q *sqlcgen.Queries, owner, connectionID, entryIdentifier, title, author, language string) (domain.CatalogueAddResult, error) {
	book, err := q.InsertBook(ctx, sqlcgen.InsertBookParams{
		OwnerID:            owner,
		Title:              title,
		Author:             author,
		MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        textArg(language),
	})
	if err != nil {
		return domain.CatalogueAddResult{}, err
	}
	if err := q.InsertBookMembership(ctx, sqlcgen.InsertBookMembershipParams{OwnerID: owner, BookID: book.ID}); err != nil {
		return domain.CatalogueAddResult{}, err
	}
	if err := upsertBookDisposition(ctx, q, owner, book.ID, domain.BookDispositionToRead); err != nil {
		return domain.CatalogueAddResult{}, err
	}
	if err := q.InsertCatalogueEntryAlias(ctx, sqlcgen.InsertCatalogueEntryAliasParams{
		OwnerID: owner, BookID: book.ID, ConnectionID: uuidArg(connectionID),
		AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: entryIdentifier,
	}); err != nil {
		return domain.CatalogueAddResult{}, aliasConflictError(err)
	}
	return domain.CatalogueAddResult{Book: domain.Book(book), Created: true, Changed: true}, nil
}

// restoreCatalogueBookTx makes an existing Book reachable again and restores
// the To Read intent Add expresses. Analysis, corrections, exclusions, reading
// history and every other piece of evidence stay untouched. The entry's current
// title and author refresh the Book, exactly as the next sync would.
func restoreCatalogueBookTx(ctx context.Context, q *sqlcgen.Queries, owner, bookID, title, author, language string) (domain.CatalogueAddResult, error) {
	changed := false
	membership, err := q.GetBookMembershipForUpdate(ctx, sqlcgen.GetBookMembershipForUpdateParams{Owner: owner, Book: bookID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		changed = true
	case err != nil:
		return domain.CatalogueAddResult{}, err
	case membership != "active":
		changed = true
	}
	disposition, err := q.GetBookDispositionStateForUpdate(ctx, sqlcgen.GetBookDispositionStateForUpdateParams{OwnerID: owner, BookID: bookID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		changed = true
	case err != nil:
		return domain.CatalogueAddResult{}, err
	case domain.BookDisposition(disposition.Disposition) != domain.BookDispositionToRead:
		changed = true
	}
	visibility, err := q.GetBookVisibilityForUpdate(ctx, sqlcgen.GetBookVisibilityForUpdateParams{Owner: owner, Book: bookID})
	if err != nil {
		return domain.CatalogueAddResult{}, missing(err)
	}
	if visibility.Hidden {
		changed = true
		if err := q.WriteBookVisibility(ctx, sqlcgen.WriteBookVisibilityParams{
			Owner: owner, Book: bookID, Hidden: false, Revision: visibility.Revision + 1,
		}); err != nil {
			return domain.CatalogueAddResult{}, err
		}
	}

	current, err := q.GetBookMetadata(ctx, sqlcgen.GetBookMetadataParams{OwnerID: owner, ID: bookID})
	if err != nil {
		return domain.CatalogueAddResult{}, missing(err)
	}
	if current.LanguageState != domain.LanguageChosen || canonicalization.NormalizeLanguage(current.LanguageTag) != language {
		if err := removeIncompatibleBookGoals(ctx, q, owner, bookID, language); err != nil {
			return domain.CatalogueAddResult{}, err
		}
	}
	book, err := q.UpdateBookMetadata(ctx, sqlcgen.UpdateBookMetadataParams{
		OwnerID: owner, ID: bookID, Title: title, Author: author,
		LanguageState: domain.LanguageChosen, LanguageTag: textArg(language),
	})
	if err != nil {
		return domain.CatalogueAddResult{}, missing(err)
	}
	if err := upsertBookDisposition(ctx, q, owner, bookID, domain.BookDispositionToRead); err != nil {
		return domain.CatalogueAddResult{}, err
	}
	if err := q.ActivateBookMembership(ctx, sqlcgen.ActivateBookMembershipParams{OwnerID: owner, BookID: bookID}); err != nil {
		return domain.CatalogueAddResult{}, err
	}
	return domain.CatalogueAddResult{Book: domain.Book(book), Changed: changed}, nil
}
