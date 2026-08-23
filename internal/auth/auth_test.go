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
func (m *memoryStore) CreateUserWithPassword(_ context.Context, n, h string, a bool) (domain.User, error) {
	if _, ok := m.users[n]; ok {
		return domain.User{}, errors.New("duplicate")
	}
	u := domain.User{ID: n, Username: n, IsAdmin: a}
	m.users[n] = u
	m.hashes[n] = h
	return u, nil
}
func (m *memoryStore) GetUserByUsername(_ context.Context, n string) (domain.User, string, error) {
	u, ok := m.users[n]
	if !ok {
		return u, "", errors.New("missing")
	}
	return u, m.hashes[n], nil
}
func (m *memoryStore) GetUserByID(_ context.Context, id string) (domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return u, errors.New("missing")
	}
	return u, nil
}
func (m *memoryStore) SetUserPassword(_ context.Context, id, h string) error {
	m.hashes[id] = h
	return nil
}
func (m *memoryStore) BootstrapAdmin(ctx context.Context, n, h string) (domain.User, bool, error) {
	for _, u := range m.users {
		if u.IsAdmin {
			return domain.User{}, false, nil
		}
	}
	u, err := m.CreateUserWithPassword(ctx, n, h, true)
	return u, err == nil, err
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
func TestServiceLifecycleAndAuthorization(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	service := New(store, time.Hour)
	admin, err := service.BootstrapAdmin(ctx, "admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.BootstrapAdmin(ctx, "other", "secret"); !errors.Is(err, ErrBootstrapComplete) {
		t.Fatalf("second bootstrap: %v", err)
	}
	alice, err := service.CreateUser(ctx, admin.ID, "alice", "password", RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if alice.IsAdmin {
		t.Fatal("user role created an administrator")
	}
	operator, err := service.CreateUser(ctx, admin.ID, "operator", "password", RoleAdmin)
	if err != nil || !operator.IsAdmin {
		t.Fatalf("admin role: %+v %v", operator, err)
	}
	if _, err = service.CreateUser(ctx, admin.ID, "invalid", "password", Role("admin,user")); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("combined role: %v", err)
	}
	if _, err = service.CreateUser(ctx, alice.ID, "bob", "password", RoleUser); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin create: %v", err)
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
func TestAuthorizeOwner(t *testing.T) {
	u := domain.User{ID: "alice"}
	if err := AuthorizeOwner(u, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeOwner(u, "bob"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-user access: %v", err)
	}
}
