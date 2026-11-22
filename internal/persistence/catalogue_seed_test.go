//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// seedCatalogueBook seeds one indexed Catalog entry and chooses it through the
// production Add path. Sync itself never creates Books, so tests that need a
// linked Book must create it explicitly.
func seedCatalogueBook(t *testing.T, ctx context.Context, store *PostgresStore, owner, connectionID, identifier, title, author, language string) (domain.CatalogueAddResult, error) {
	t.Helper()
	err := store.ReconcileCatalogueIndex(ctx, owner, connectionID, []domain.CatalogueIndexEntry{{
		ConnectionID: connectionID, EntryIdentifier: identifier, Title: title, Author: author,
		LanguageTag: language, EPUBHref: "https://catalog.example/" + identifier + ".epub",
	}}, time.Now())
	if err != nil {
		return domain.CatalogueAddResult{}, err
	}
	added, err := store.AddCatalogueEntry(ctx, owner, connectionID, identifier)
	if err != nil {
		return domain.CatalogueAddResult{}, err
	}
	require.NotEmpty(t, added.Book.ID)
	return added, nil
}
