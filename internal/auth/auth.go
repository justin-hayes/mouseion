// Package auth implements local credentials, sessions, and authorization.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"golang.org/x/crypto/argon2"
)

const (
	memory                 = 64 * 1024
	iterations             = 3
	parallelism            = 2
	saltLength             = 16
	keyLength              = 32
	DefaultSessionLifetime = 24 * time.Hour
)

var (
	ErrInvalidCredentials  = errors.New("auth: invalid credentials")
	ErrUnauthenticated     = errors.New("auth: unauthenticated")
	ErrForbidden           = errors.New("auth: forbidden")
	ErrBootstrapComplete   = errors.New("auth: an administrator already exists")
	ErrInvalidPasswordHash = errors.New("auth: invalid password hash")
	ErrInvalidRole         = errors.New("auth: invalid account role")
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type Store interface {
	CreateUserWithPassword(context.Context, string, string, bool) (domain.User, error)
	BootstrapAdmin(context.Context, string, string) (domain.User, bool, error)
	GetUserByUsername(context.Context, string) (domain.User, string, error)
	GetUserByID(context.Context, string) (domain.User, error)
	SetUserPassword(context.Context, string, string) error
	CreateSession(context.Context, string, string, time.Time) error
	GetSession(context.Context, string) (domain.User, time.Time, error)
	DeleteSession(context.Context, string) error
	DeleteUserSessions(context.Context, string) error
}

type Service struct {
	store    Store
	lifetime time.Duration
	now      func() time.Time
}

func New(store Store, lifetime time.Duration) *Service {
	if lifetime <= 0 {
		lifetime = DefaultSessionLifetime
	}
	return &Service{store: store, lifetime: lifetime, now: time.Now}
}

// HashPassword uses argon2id with 64 MiB memory, three iterations, and two lanes.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("auth: password must not be empty")
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	var m, t uint32
	var p uint8
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, ErrInvalidPasswordHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m == 0 || t == 0 || p == 0 {
		return false, ErrInvalidPasswordHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, ErrInvalidPasswordHash
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func GenerateSessionToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashSessionToken(raw), nil
}
func HashSessionToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	u, encoded, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	ok, err := VerifyPassword(encoded, password)
	if err != nil || !ok {
		return "", ErrInvalidCredentials
	}
	raw, hash, err := GenerateSessionToken()
	if err != nil {
		return "", err
	}
	if err = s.store.CreateSession(ctx, u.ID, hash, s.now().Add(s.lifetime)); err != nil {
		return "", err
	}
	return raw, nil
}
func (s *Service) Authenticate(ctx context.Context, raw string) (domain.User, error) {
	if raw == "" {
		return domain.User{}, ErrUnauthenticated
	}
	u, _, err := s.store.GetSession(ctx, HashSessionToken(raw))
	if err != nil {
		return domain.User{}, ErrUnauthenticated
	}
	return u, nil
}
func (s *Service) Logout(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, HashSessionToken(raw))
}
func (s *Service) LogoutEverywhere(ctx context.Context, userID string) error {
	return s.store.DeleteUserSessions(ctx, userID)
}

func (s *Service) BootstrapAdmin(ctx context.Context, username, password string) (domain.User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return domain.User{}, err
	}
	u, created, err := s.store.BootstrapAdmin(ctx, username, hash)
	if err != nil {
		return domain.User{}, err
	}
	if !created {
		return domain.User{}, ErrBootstrapComplete
	}
	return u, nil
}
func (s *Service) CreateUser(ctx context.Context, adminID, username, password string, role Role) (domain.User, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return domain.User{}, err
	}
	if role != RoleUser && role != RoleAdmin {
		return domain.User{}, ErrInvalidRole
	}
	hash, err := HashPassword(password)
	if err != nil {
		return domain.User{}, err
	}
	return s.store.CreateUserWithPassword(ctx, username, hash, role == RoleAdmin)
}
func (s *Service) ResetPassword(ctx context.Context, adminID, targetID, newPassword string) error {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return err
	}
	if _, err := s.store.GetUserByID(ctx, targetID); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err = s.store.SetUserPassword(ctx, targetID, hash); err != nil {
		return err
	}
	return s.store.DeleteUserSessions(ctx, targetID)
}
func (s *Service) requireAdmin(ctx context.Context, id string) error {
	u, err := s.store.GetUserByID(ctx, id)
	if err != nil || !u.IsAdmin {
		return ErrForbidden
	}
	return nil
}
func AuthorizeOwner(user domain.User, ownerID string) error {
	if user.ID != ownerID {
		return ErrForbidden
	}
	return nil
}
