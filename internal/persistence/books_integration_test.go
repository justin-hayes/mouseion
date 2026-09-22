//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMyBooksPersistenceAndBackfill(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)

	alice, err := store.CreateUser(ctx, "books-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "books-bob", false)
	require.NoError(t, err)
	aliceConnection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Alice catalog", URL: "https://alice.example/opds"})
	require.NoError(t, err)
	bobConnection, err := store.CreateOpdsConnection(ctx, bob.ID, domain.OpdsConnection{Name: "Bob catalog", URL: "https://bob.example/opds"})
	require.NoError(t, err)

	legacySources := []struct {
		owner, language, identifier, title string
	}{
		{alice.ID, "de", "legacy-de", "Deutsches Buch"},
		{alice.ID, "it", "legacy-it", "Libro italiano"},
		{bob.ID, "fr", "legacy-fr", "Livre français"},
	}
	legacyIDs := make([]string, 0, len(legacySources))
	for i, source := range legacySources {
		id := insertLegacySource(t, ctx, pool, source.owner, source.language, source.identifier, source.title, []byte("legacy-epub-"+string(rune('a'+i))))
		legacyIDs = append(legacyIDs, id)
	}
	for i, source := range legacySources {
		connectionID := aliceConnection.ID
		if source.owner == bob.ID {
			connectionID = bobConnection.ID
		}
		_, err = store.ReconcileCatalogueEntry(ctx, source.owner, connectionID, source.identifier, source.title, "", source.language)
		require.NoError(t, err)
		var bookID string
		err = pool.QueryRow(ctx, `SELECT book_id::text FROM book_aliases WHERE owner_id=$1 AND connection_id=$2 AND value=$3`, source.owner, connectionID, source.identifier).Scan(&bookID)
		require.NoError(t, err)
		err = store.LinkSourceToBook(ctx, source.owner, bookID, legacyIDs[i])
		require.NoError(t, err)
	}

	books, err := store.ListMyBooks(ctx, alice.ID)
	require.NoError(t, err)
	assert.Len(t, books, 2, "alice backfill books")
	for _, source := range legacySources[:2] {
		book, found, resolveErr := store.ResolveBookByAlias(ctx, source.owner, domain.NamespaceSourceIdentifier, source.identifier)
		require.NoError(t, resolveErr)
		assert.True(t, found, "backfill alias %q", source.identifier)
		assert.Equal(t, domain.LanguageChosen, book.LanguageState, "backfill alias %q", source.identifier)
		assert.Equal(t, source.language, book.LanguageTag, "backfill alias %q", source.identifier)
	}
	var linkedCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM source_materials WHERE book_id IS NOT NULL`).Scan(&linkedCount)
	require.NoError(t, err)
	assert.Equal(t, len(legacySources), linkedCount)

	metadataOnly, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Unacquired book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	retriedMetadata, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Unacquired book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	assert.NotEqual(t, metadataOnly.ID, retriedMetadata.ID, "repeated metadata create reused book")
	books, err = store.ListMyBooks(ctx, alice.ID)
	require.NoError(t, err)
	assert.True(t, containsBook(books, metadataOnly.ID), "metadata-only book missing from My Books")
	view, err := store.ListMyBooksWithEvidence(ctx, alice.ID)
	require.NoError(t, err)
	var metadataView *domain.MyBook
	for i := range view {
		if view[i].Book.ID == metadataOnly.ID {
			metadataView = &view[i]
			break
		}
	}
	require.NotNil(t, metadataView, "metadata-only read model")
	assert.Nil(t, metadataView.Acquired)
	assert.Equal(t, domain.BookNotAcquired, metadataView.EvidenceState())
	assert.Equal(t, domain.LanguageUnknown, metadataView.Book.LanguageState)

	promotion, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Promote this book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	promotionSource := putBookSource(t, ctx, store, alice.ID, "promotion-source", "Promoted", []byte("promotion-epub"), "promoted")
	resolvedPromotion, err := store.ResolveOrCreateBookForAcquisitionForBook(ctx, alice.ID, promotion.ID, promotionSource.SourceIdentifier, promotionSource.Language, promotionSource.Title)
	require.NoError(t, err)
	assert.Equal(t, promotion.ID, resolvedPromotion)
	err = store.LinkSourceToBook(ctx, alice.ID, promotion.ID, promotionSource.ID)
	require.NoError(t, err)
	promotionRepeat, repeatErr := store.ResolveOrCreateBookForAcquisitionForBook(ctx, alice.ID, promotion.ID, promotionSource.SourceIdentifier, promotionSource.Language, promotionSource.Title)
	require.NoError(t, repeatErr)
	assert.Equal(t, promotion.ID, promotionRepeat)
	view, err = store.ListMyBooksWithEvidence(ctx, alice.ID)
	require.NoError(t, err)
	foundPromotion := false
	for i := range view {
		if view[i].Book.ID == promotion.ID {
			foundPromotion = true
			require.NotNil(t, view[i].Acquired, "promoted read model")
			assert.Equal(t, promotionSource.ID, view[i].Acquired.Source.ID, "promoted read model")
			assert.Equal(t, domain.LanguageUnknown, view[i].Book.LanguageState, "promoted read model")
			assert.Equal(t, "", view[i].Book.LanguageTag, "promoted read model")
			assert.Equal(t, domain.BookAcquiredUnassessed, view[i].EvidenceState(), "promoted read model")
			break
		}
	}
	assert.True(t, foundPromotion, "promoted book missing from My Books read model")

	first := putBookSource(t, ctx, store, alice.ID, "acquisition-source", "Acquired", []byte("epub-one"), "readable")
	bookID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, first.SourceIdentifier, first.Language, first.Title)
	require.NoError(t, err)
	err = store.LinkSourceToBook(ctx, alice.ID, bookID, first.ID)
	require.NoError(t, err)
	_, found, err := store.FindSourceMaterialForAcquisition(ctx, alice.ID, first.SourceIdentifier, first.ContentHash)
	require.NoError(t, err)
	assert.True(t, found, "same acquisition was not found")
	_, found, err = store.FindSourceMaterialForAcquisition(ctx, alice.ID, first.SourceIdentifier, domain.EPUBContentDigest([]byte("epub-two")))
	require.NoError(t, err)
	assert.False(t, found, "changed acquisition was reported present")
	repeated := putBookSource(t, ctx, store, alice.ID, "acquisition-source", "Acquired", []byte("epub-one"), "readable")
	repeatedBookID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, repeated.SourceIdentifier, repeated.Language, repeated.Title)
	require.NoError(t, err)
	assert.Equal(t, bookID, repeatedBookID, "repeated acquisition")
	assert.Equal(t, first.ID, repeated.ID, "repeated acquisition")
	changed := putBookSource(t, ctx, store, alice.ID, "acquisition-source", "Acquired", []byte("epub-two"), "readable")
	changedBookID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, changed.SourceIdentifier, changed.Language, changed.Title)
	require.NoError(t, err)
	assert.Equal(t, bookID, changedBookID, "changed acquisition")
	assert.Equal(t, first.ID, changed.ID, "changed acquisition")
	var revisions int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, first.ID).Scan(&revisions)
	require.NoError(t, err)
	assert.Equal(t, 2, revisions)

	otherSource := putBookSource(t, ctx, store, bob.ID, "acquisition-source", "Acquired", []byte("epub-one"), "readable")
	otherBookID, err := store.ResolveOrCreateBookForAcquisition(ctx, bob.ID, otherSource.SourceIdentifier, otherSource.Language, otherSource.Title)
	require.NoError(t, err)
	assert.NotEqual(t, bookID, otherBookID, "cross-owner acquisition reused a Book")
	_, err = store.GetBook(ctx, alice.ID, otherBookID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner lookup is independent of the following owner-scoped link.
	err = store.LinkSourceToBook(ctx, bob.ID, otherBookID, otherSource.ID)
	require.NoError(t, err)

	secondBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Second", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	err = store.AddBookAlias(ctx, alice.ID, bookID, domain.AliasStrongBibliographic, "isbn", "978-conflict")
	require.NoError(t, err)
	err = store.AddBookAlias(ctx, alice.ID, secondBook.ID, domain.AliasStrongBibliographic, "isbn", "978-conflict")
	assert.ErrorIs(t, err, ErrAliasConflict) //nolint:testifylint // Conflict rejection and alias resolution are independent assertions.
	resolved, found, err := store.ResolveBookByAlias(ctx, alice.ID, "isbn", "978-conflict")
	require.NoError(t, err)
	assert.True(t, found, "alias was reassigned")
	assert.Equal(t, bookID, resolved.ID, "alias was reassigned")
	err = store.LinkSourceToBook(ctx, alice.ID, secondBook.ID, first.ID)
	assert.ErrorIs(t, err, ErrSourceBookConflict) //nolint:testifylint // Conflict rejection is independent of the later membership lifecycle.

	err = store.RemoveBookFromMyBooks(ctx, alice.ID, bookID)
	require.NoError(t, err)
	books, err = store.ListMyBooks(ctx, alice.ID)
	require.NoError(t, err)
	assert.False(t, containsBook(books, bookID), "removed book still active")
	err = store.AddBookToMyBooks(ctx, alice.ID, bookID)
	require.NoError(t, err)
	restored, err := store.GetBook(ctx, alice.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, bookID, restored.ID, "restored book")
	_, found, err = store.ResolveBookByAlias(ctx, alice.ID, domain.NamespaceSourceIdentifier, "acquisition-source")
	require.NoError(t, err)
	assert.False(t, found, "connectionless acquisition source alias was persisted")

	err = store.AddBookAlias(ctx, alice.ID, secondBook.ID, "invalid", "failure", "must-not-commit")
	assert.Error(t, err, "invalid alias was accepted") //nolint:testifylint // Rejection and the rollback query are independent assertions.
	var invalidAliases int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND value='must-not-commit'`, alice.ID).Scan(&invalidAliases)
	require.NoError(t, err)
	assert.Zero(t, invalidAliases, "failed alias left partial row")

}

func TestActiveStudyLanguagePersistenceResolvesLazily(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)

	alice, err := store.CreateUser(ctx, "active-language-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "active-language-bob", false)
	require.NoError(t, err)
	got, err := store.GetStoredActiveStudyLanguage(ctx, alice.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
	err = store.SetActiveStudyLanguage(ctx, alice.ID, "IT_it")
	require.NoError(t, err)
	got, err = store.GetStoredActiveStudyLanguage(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, "it", got)
	got, err = store.GetStoredActiveStudyLanguage(ctx, bob.ID)
	require.NoError(t, err)
	assert.Empty(t, got)

	deBook, err := domain.NewBook(alice.ID, "German", domain.MetadataProvenanceCatalogueSync, domain.LanguageChosen, "de")
	require.NoError(t, err)
	deBook, err = store.CreateBook(ctx, deBook)
	require.NoError(t, err)
	itBook, err := domain.NewBook(alice.ID, "Italian", domain.MetadataProvenanceCatalogueSync, domain.LanguageChosen, "it")
	require.NoError(t, err)
	itBook, err = store.CreateBook(ctx, itBook)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE book_membership SET activated_at=CASE book_id WHEN $2 THEN now() - interval '1 minute' WHEN $3 THEN now() ELSE activated_at END WHERE owner_id=$1`, alice.ID, deBook.ID, itBook.ID)
	require.NoError(t, err)

	languages, err := store.ListStudyLanguages(ctx, alice.ID)
	require.NoError(t, err)
	recent, err := store.MostRecentlyActivatedStudyLanguage(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, "it", recent)
	assert.Equal(t, "it", domain.ResolveActiveStudyLanguage(languages, "fr", recent))
	err = store.RemoveBookFromMyBooks(ctx, alice.ID, itBook.ID)
	require.NoError(t, err)
	recent, err = store.MostRecentlyActivatedStudyLanguage(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, "de", recent)
	got, err = store.GetStoredActiveStudyLanguage(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, "it", got)
	_, err = store.UpdateBookMetadata(ctx, alice.ID, deBook.ID, deBook.Title, deBook.Author, domain.LanguageUnknown, "")
	require.NoError(t, err)
	languages, err = store.ListStudyLanguages(ctx, alice.ID)
	require.NoError(t, err)
	assert.Empty(t, domain.ResolveActiveStudyLanguage(languages, "fr", "de"))
}

func TestGetBookDetailResolvesBookAndSourceIDsWithinOwner(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)

	alice, err := store.CreateUser(ctx, "book-detail-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "book-detail-bob", false)
	require.NoError(t, err)
	metadata, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Metadata", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	metadataDetail, err := store.GetBookDetail(ctx, alice.ID, metadata.ID)
	require.NoError(t, err)
	assert.Nil(t, metadataDetail.Acquired)
	assert.Equal(t, domain.BookNotAcquired, metadataDetail.EvidenceState())

	source := putBookSource(t, ctx, store, alice.ID, "detail-source", "Acquired", []byte("detail-epub"), "de")
	bookID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, source.SourceIdentifier, source.Language, source.Title)
	require.NoError(t, err)
	err = store.LinkSourceToBook(ctx, alice.ID, bookID, source.ID)
	require.NoError(t, err)
	for _, id := range []string{bookID, source.ID} {
		detail, detailErr := store.GetBookDetail(ctx, alice.ID, id)
		require.NoError(t, detailErr)
		assert.Equal(t, bookID, detail.Book.ID, "detail id=%q", id)
		require.NotNil(t, detail.Acquired, "detail id=%q", id)
		assert.Equal(t, source.ID, detail.Acquired.Source.ID, "detail id=%q", id)
		assert.Equal(t, domain.BookAcquiredUnassessed, detail.EvidenceState(), "detail id=%q", id)
	}
	_, err = store.GetBookDetail(ctx, bob.ID, bookID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner detail lookup is independent of the missing-ID lookup.
	_, err = store.GetBookDetail(ctx, alice.ID, "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestListStudyLanguagesDerivesActiveChosenBooks(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)

	alice, err := store.CreateUser(ctx, "study-languages-alice", false)
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "de", "German")
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "el", "Greek")
	require.NoError(t, err)
	inputs := []struct {
		title, language string
		state           string
	}{
		{title: "German one", language: "DE_de", state: domain.LanguageChosen},
		{title: "German two", language: "de-DE", state: domain.LanguageChosen},
		{title: "Greek", language: "el", state: domain.LanguageChosen},
		{title: "Fallback", language: "PT_br", state: domain.LanguageChosen},
		{title: "Unknown", state: domain.LanguageUnknown},
	}
	for _, input := range inputs {
		book, bookErr := domain.NewBook(alice.ID, input.title, domain.MetadataProvenanceCatalogueSync, input.state, input.language)
		require.NoError(t, bookErr)
		_, err = store.CreateBook(ctx, book)
		require.NoError(t, err)
	}
	books, err := store.ListMyBooks(ctx, alice.ID)
	require.NoError(t, err)
	var duplicateBookID string
	for _, book := range books {
		if book.Title == "German one" {
			duplicateBookID = book.ID
			break
		}
	}
	require.NotEmpty(t, duplicateBookID, "German one book missing")
	err = store.RemoveBookFromMyBooks(ctx, alice.ID, duplicateBookID)
	require.NoError(t, err)

	languages, err := store.ListStudyLanguages(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, languages, 3)
	assert.Equal(t, domain.StudyLanguage{Language: "de", DisplayName: "German"}, languages[0])
	assert.Equal(t, domain.StudyLanguage{Language: "el", DisplayName: "Greek"}, languages[1])
	assert.Equal(t, domain.StudyLanguage{Language: "pt", DisplayName: "pt"}, languages[2])
	err = store.SetActiveStudyLanguage(ctx, alice.ID, "el")
	require.NoError(t, err)
	active, err := store.GetStoredActiveStudyLanguage(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, "el", active)
}

func insertLegacySource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, language, identifier, title string, content []byte) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,'application/epub+zip',$5,$6,$7) RETURNING id::text`, owner, language, identifier, title, "legacy:"+identifier, content, string(content)).Scan(&id))
	return id
}

func putBookSource(t *testing.T, ctx context.Context, store *PostgresStore, owner, identifier, title string, content []byte, fullText string) domain.SourceMaterial {
	return putBookSourceInLanguage(t, ctx, store, owner, "de", identifier, title, content, fullText)
}

func putBookSourceInLanguage(t *testing.T, ctx context.Context, store *PostgresStore, owner, language, identifier, title string, content []byte, fullText string) domain.SourceMaterial {
	t.Helper()
	unit := domain.ExtractedUnit{ID: domain.EPUBUnitID(0, "item"), Order: 0, SpineIndex: 0, ManifestID: "item", Text: fullText, EndOffset: uint64(len([]rune(fullText))), MediaType: "application/xhtml+xml", Linear: true}
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner, Language: language, SourceIdentifier: identifier, Title: title, MediaType: "application/epub+zip", Content: content, FullText: fullText}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{unit}})
	require.NoError(t, err)
	return source
}

func containsBook(books []domain.Book, id string) bool {
	for _, book := range books {
		if book.ID == id {
			return true
		}
	}
	return false
}
