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

func TestOpdsConnectionCRUDEncryptionAndOwnerIsolation(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	ctx := context.Background()
	databaseURL := integrationDatabase(t, ctx)
	store := openIntegrationStore(t, ctx, databaseURL)
	alice, err := store.CreateUser(ctx, "alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "bob", false)
	require.NoError(t, err)
	created, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Home library", URL: "https://books.example/opds", Username: "reader", Password: "plain-password-must-not-be-stored"})
	require.NoError(t, err)
	assert.Equal(t, "plain-password-must-not-be-stored", created.Password)
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "opds pool", func() error {
		pool.Close()
		return nil
	})
	var legacyID string
	err = pool.QueryRow(ctx, `INSERT INTO opds_connections(name,url,language) VALUES('Preserved legacy catalog','https://legacy.example/opds','de') RETURNING id`).Scan(&legacyID)
	require.NoError(t, err)
	listed, listErr := store.ListOpdsConnections(ctx, alice.ID)
	require.NoError(t, listErr)
	assert.Len(t, listed, 1, "legacy row exposed to owner")
	var legacyStillExists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM opds_connections WHERE id=$1 AND owner_id IS NULL)`, legacyID).Scan(&legacyStillExists)
	require.NoError(t, err)
	assert.True(t, legacyStillExists, "legacy row was not preserved")
	var encrypted []byte
	err = pool.QueryRow(ctx, `SELECT password_encrypted FROM opds_connections WHERE id=$1`, created.ID).Scan(&encrypted)
	require.NoError(t, err)
	assert.NotEqual(t, created.Password, string(encrypted), "credential not encrypted")
	assert.Greater(t, len(encrypted), len(created.Password), "credential not encrypted")
	got, err := store.GetOpdsConnection(ctx, alice.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Password, got.Password, "round trip")
	listed, err = store.ListOpdsConnections(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)
	assert.Equal(t, alice.ID, listed[0].OwnerID)
	listed, err = store.ListOpdsConnections(ctx, bob.ID)
	require.NoError(t, err)
	assert.Empty(t, listed)
	_, err = store.GetOpdsConnection(ctx, bob.ID, created.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Owner-isolation, update, delete, and post-delete checks are independent.
	created.Name = "Updated"
	created.URL = "https://books.example/new-opds"
	created.Password = "new-password"
	_, err = store.UpdateOpdsConnection(ctx, bob.ID, created)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Owner-isolation, update, delete, and post-delete checks are independent.
	err = store.DeleteOpdsConnection(ctx, bob.ID, created.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Owner-isolation, update, delete, and post-delete checks are independent.
	updated, err := store.UpdateOpdsConnection(ctx, alice.ID, created)
	require.NoError(t, err)
	assert.Equal(t, "Updated", updated.Name)
	assert.Equal(t, "new-password", updated.Password)
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt))
	err = store.DeleteOpdsConnection(ctx, alice.ID, created.ID)
	require.NoError(t, err)
	_, err = store.GetOpdsConnection(ctx, alice.ID, created.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}
