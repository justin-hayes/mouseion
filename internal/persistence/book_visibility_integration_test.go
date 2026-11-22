//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetiredBookVisibilityRowsDoNotAffectMyBooks proves the retired Hidden
// choice has no effect while its rows stay in place for a later cleanup. The
// row is written directly because no application path can write one any more.
func TestRetiredBookVisibilityRowsDoNotAffectMyBooks(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	owner, err := store.CreateUser(ctx, "visibility-retired", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Retired visibility catalog", URL: "https://retired-visibility.example/opds"})
	require.NoError(t, err)
	created, err := seedCatalogueBook(t, ctx, store, owner.ID, connection.ID, "retired-hidden", "Previously hidden", "Author", "de")
	require.NoError(t, err)
	bookID := created.Book.ID

	_, err = store.Pool().Exec(ctx, `INSERT INTO book_visibility(owner_id, book_id, hidden, revision) VALUES ($1, $2, true, 1)`, owner.ID, bookID)
	require.NoError(t, err)

	browse, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, browse.Items, 1, "a previously hidden Book is an ordinary Book")
	assert.Equal(t, bookID, browse.Items[0].Book.ID)
	assert.Equal(t, 1, browse.ScopeTotal)
	assert.Equal(t, 1, browse.AllCount)
	assert.Equal(t, domain.BookDispositionToRead, browse.Items[0].Disposition, "the hidden row never changed the To Read choice")

	require.NoError(t, store.RemoveBookFromMyBooks(ctx, owner.ID, bookID))
	_, err = store.AddCatalogueEntry(ctx, owner.ID, connection.ID, "retired-hidden")
	require.NoError(t, err)

	var hidden bool
	var revision int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT hidden, revision FROM book_visibility WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&hidden, &revision))
	assert.True(t, hidden, "no application path deletes or rewrites the retired row")
	assert.EqualValues(t, 1, revision)
}
