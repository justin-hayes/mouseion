//go:build integration

package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// SeedLegacyInboxDisposition writes the retired Inbox state onto an existing
// Book: the stored "not To Read" disposition with no reading history. Both
// stores refuse that write (ADR 0117), so only a direct row update can seed
// the shape that existed before My Books held only chosen Books.
func SeedLegacyInboxDisposition(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID, bookID string) {
	t.Helper()
	tag, err := pool.Exec(ctx, `UPDATE book_dispositions SET disposition = 'inbox' WHERE owner_id = $1 AND book_id = $2`, ownerID, bookID)
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected(), "the Book has a disposition row")
}
