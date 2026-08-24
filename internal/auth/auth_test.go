package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
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
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword(h, "correct horse battery staple"); err != nil || !ok {
		t.Fatalf("verify=%v err=%v", ok, err)
	}
	if ok, err := VerifyPassword(h, "wrong"); err != nil || ok {
		t.Fatalf("wrong password verify=%v err=%v", ok, err)
	}
	if _, err = VerifyPassword("broken", "password"); !errors.Is(err, ErrInvalidPasswordHash) {
		t.Fatalf("malformed hash: %v", err)
	}
}
func TestServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	service := New(store, time.Hour)
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := store.createUser("alice", hash)
	if err != nil {
		t.Fatal(err)
	}
	token, err := service.Login(ctx, "alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.Authenticate(ctx, token); err != nil || got.ID != alice.ID {
		t.Fatalf("authenticate: %+v %v", got, err)
	}
	if err = service.LogoutEverywhere(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("session remains: %v", err)
	}
}
func TestCreateFirstAccountValidatesAndEstablishesSession(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	service := New(store, time.Hour)
	if _, _, err := service.CreateFirstAccount(ctx, " ", "password"); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("blank username: %v", err)
	}
	if _, _, err := service.CreateFirstAccount(ctx, "alice", "short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short password: %v", err)
	}
	u, token, err := service.CreateFirstAccount(ctx, " alice ", "password")
	if err != nil || u.Username != "alice" || token == "" {
		t.Fatalf("first account: %+v token=%q err=%v", u, token, err)
	}
	if got, err := service.Authenticate(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("initial session: %+v %v", got, err)
	}
	if _, _, err = service.CreateFirstAccount(ctx, "bob", "password"); !errors.Is(err, ErrFirstAccountExists) {
		t.Fatalf("second account: %v", err)
	}
}
func TestAuthorizeOwner(t *testing.T) {
	u := domain.User{ID: "alice"}
	if err := AuthorizeOwner(u, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeOwner(u, "bob"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-user access: %v", err)
	}
}
