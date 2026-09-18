package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	users    map[string]domain.User
	hashes   map[string]string
	sessions map[string]string
}

func newMemoryStore() *memoryStore {
	return &memoryStore{users: map[string]domain.User{}, hashes: map[string]string{}, sessions: map[string]string{}}
}
func (m *memoryStore) createUser(n, h string) (domain.User, error) {
	if _, ok := m.users[n]; ok {
		return domain.User{}, errors.New("duplicate")
	}
	u := domain.User{ID: n, Username: n}
	m.users[n] = u
	m.hashes[n] = h
	return u, nil
}
func (m *memoryStore) HasUsers(context.Context) (bool, error) { return len(m.users) > 0, nil }
func (m *memoryStore) CreateFirstUserAndSession(ctx context.Context, n, h, token string, _ time.Time) (domain.User, bool, error) {
	if len(m.users) > 0 {
		return domain.User{}, false, nil
	}
	u, err := m.createUser(n, h)
	if err != nil {
		return domain.User{}, false, err
	}
	m.sessions[token] = u.ID
	return u, true, nil
}
func (m *memoryStore) GetUserByUsername(_ context.Context, n string) (domain.User, string, error) {
	u, ok := m.users[n]
	if !ok {
		return u, "", errors.New("missing")
	}
	return u, m.hashes[n], nil
}
func (m *memoryStore) CreateSession(_ context.Context, id, h string, _ time.Time) error {
	m.sessions[h] = id
	return nil
}
func (m *memoryStore) GetSession(_ context.Context, h string) (domain.User, time.Time, error) {
	id, ok := m.sessions[h]
	if !ok {
		return domain.User{}, time.Time{}, errors.New("missing")
	}
	return m.users[id], time.Now().Add(time.Hour), nil
}
func (m *memoryStore) DeleteSession(_ context.Context, h string) error {
	delete(m.sessions, h)
	return nil
}
func (m *memoryStore) DeleteUserSessions(_ context.Context, id string) error {
	for h, v := range m.sessions {
		if v == id {
			delete(m.sessions, h)
		}
	}
	return nil
}

func TestPasswordHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	require.NoError(t, err)
	ok, err := VerifyPassword(h, "correct horse battery staple")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = VerifyPassword(h, "wrong")
	require.NoError(t, err)
	assert.False(t, ok)
	_, err = VerifyPassword("broken", "password")
	assert.ErrorIs(t, err, ErrInvalidPasswordHash)
}
func TestServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	service := New(store, time.Hour)
	hash, err := HashPassword("password")
	require.NoError(t, err)
	alice, err := store.createUser("alice", hash)
	require.NoError(t, err)
	token, err := service.Login(ctx, "alice", "password")
	require.NoError(t, err)
	got, err := service.Authenticate(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, alice.ID, got.ID)
	require.NoError(t, service.LogoutEverywhere(ctx, alice.ID))
	_, err = service.Authenticate(ctx, token)
	assert.ErrorIs(t, err, ErrUnauthenticated)
}
func TestCreateFirstAccountValidatesAndEstablishesSession(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	service := New(store, time.Hour)
	_, _, err := service.CreateFirstAccount(ctx, " ", "password")
	assert.ErrorIs(t, err, ErrInvalidUsername) //nolint:testifylint // Independent validation case; the following call tests a different input.
	_, _, err = service.CreateFirstAccount(ctx, "alice", "short")
	assert.ErrorIs(t, err, ErrInvalidPassword) //nolint:testifylint // Independent validation case; later assertions cover successful creation.
	u, token, err := service.CreateFirstAccount(ctx, " alice ", "password")
	require.NoError(t, err)
	assert.Equal(t, "alice", u.Username)
	assert.NotEmpty(t, token)
	got, err := service.Authenticate(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	_, _, err = service.CreateFirstAccount(ctx, "bob", "password")
	assert.ErrorIs(t, err, ErrFirstAccountExists)
}
func TestAuthorizeOwner(t *testing.T) {
	u := domain.User{ID: "alice"}
	require.NoError(t, AuthorizeOwner(u, "alice"))
	assert.ErrorIs(t, AuthorizeOwner(u, "bob"), ErrForbidden)
}
