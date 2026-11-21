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

// connectionContractValidationMigration is the migration that validates
// book_aliases_connection_contract. The version before it is the schema the
// test seeds.
const connectionContractValidationMigration uint = 54

// TestBookAliasesConnectionContractMigrationValidatesExistingAliases exercises
// the forward-only validation over rows that predate it: the constraint is
// NOT VALID beforehand, both alias kinds are seeded, and the migration must
// leave the constraint validated.
func TestBookAliasesConnectionContractMigrationValidatesExistingAliases(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	require.True(t, bookAliasesConnectionContractValidated(ctx, t, pool),
		"a fresh database must report the constraint validated")

	// Rewind to the migration that precedes the validation so the check runs
	// against an existing alias population instead of a freshly created schema.
	moveApplicationMigrationsTo(t, databaseURL, connectionContractValidationMigration-1)
	require.False(t, bookAliasesConnectionContractValidated(ctx, t, pool))

	owner, err := store.CreateUser(ctx, "alias-contract-owner", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID:            owner.ID,
		Title:              "Catalog book",
		MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        "de",
	})
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value)
VALUES ($1, $2, $3, 'catalog_entry', 'source_identifier', 'catalog-entry-1')`, owner.ID, book.ID, connection.ID)
	require.NoError(t, err)
	err = store.AddBookAlias(ctx, owner.ID, book.ID, domain.AliasStrongBibliographic, "isbn", "978-3-16-148410-0")
	require.NoError(t, err)

	migrateApplicationMigrationsToLatest(t, databaseURL)

	assert.True(t, bookAliasesConnectionContractValidated(ctx, t, pool))
}

func bookAliasesConnectionContractValidated(ctx context.Context, t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var validated bool
	err := pool.QueryRow(ctx,
		`SELECT convalidated FROM pg_constraint WHERE conname = 'book_aliases_connection_contract'`).Scan(&validated)
	require.NoError(t, err)
	return validated
}
