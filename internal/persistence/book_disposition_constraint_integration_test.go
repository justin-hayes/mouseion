//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestBookDispositionConstraintRejectsRetiredSetAside(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "set-aside-retired-owner", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Retired", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='set_aside' WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.Error(t, err)
	require.Error(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDisposition("set_aside")))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
}
