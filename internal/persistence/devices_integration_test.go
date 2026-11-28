//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevicesStoreEnforcesUniqueUsernameOwnerScopeAndCascade(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	hash, err := auth.HashPassword("password-123")
	require.NoError(t, err)
	alice, err := store.CreateUserWithPassword(ctx, "alice", hash, false)
	require.NoError(t, err)
	bob, err := store.CreateUserWithPassword(ctx, "bob", hash, false)
	require.NoError(t, err)

	device, err := store.CreateDevice(ctx, alice.ID, "Palma", "palma-k3f9", "keyhash")
	require.NoError(t, err)
	assert.Equal(t, "Palma", device.Name)
	assert.Nil(t, device.LastSeenAt)
	assert.Nil(t, device.RevokedAt)

	_, err = store.CreateDevice(ctx, bob.ID, "Other", "palma-k3f9", "keyhash")
	require.ErrorIs(t, err, auth.ErrDeviceUsernameTaken)

	aliceDevices, err := store.ListDevices(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, aliceDevices, 1)
	assert.Equal(t, device.ID, aliceDevices[0].ID)
	bobDevices, err := store.ListDevices(ctx, bob.ID)
	require.NoError(t, err)
	assert.Empty(t, bobDevices)

	_, err = store.Pool().Exec(ctx, `DELETE FROM users WHERE id = $1`, alice.ID)
	require.NoError(t, err)
	var count int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM devices`).Scan(&count))
	assert.Zero(t, count, "deleting the account removes its Devices")
}
