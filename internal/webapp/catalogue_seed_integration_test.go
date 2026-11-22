//go:build integration

package webapp

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/require"
)

// seedWebappCatalogueBook seeds one indexed Catalog entry and chooses it through
// the production Add path. Sync never creates Books, so a test that needs a
// linked Book must create it explicitly.
func seedWebappCatalogueBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, connectionID, identifier, title, author, language string) (domain.CatalogueAddResult, error) {
	t.Helper()
	err := store.ReconcileCatalogueIndex(ctx, owner, connectionID, []domain.CatalogueIndexEntry{{
		ConnectionID: connectionID, EntryIdentifier: identifier, Title: title, Author: author,
		LanguageTag: language, EPUBHref: "https://catalog.example/" + identifier + ".epub",
	}}, time.Now())
	if err != nil {
		return domain.CatalogueAddResult{}, err
	}
	result, err := store.AddCatalogueEntry(ctx, owner, connectionID, identifier)
	if err != nil {
		return domain.CatalogueAddResult{}, err
	}
	require.NotEmpty(t, result.Book.ID)
	return result, nil
}
