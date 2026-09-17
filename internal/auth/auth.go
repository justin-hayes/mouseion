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

	"github.com/justin-hayes/mouseion/internal/checked"
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
	ErrInvalidUsername     = errors.New("auth: username must not be empty")
	ErrInvalidPassword     = errors.New("auth: password must be at least 8 characters")
	ErrFirstAccountExists  = errors.New("auth: first account already exists")
	ErrUnauthenticated     = errors.New("auth: unauthenticated")
	ErrForbidden           = errors.New("auth: forbidden")
	ErrInvalidPasswordHash = errors.New("auth: invalid password hash")
)

type Store interface {
	HasUsers(context.Context) (bool, error)
	CreateFirstUserAndSession(context.Context, string, string, string, time.Time) (domain.User, bool, error)
	GetUserByUsername(context.Context, string) (domain.User, string, error)
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
	keyLength, err := checked.Uint32FromInt(len(want))
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, keyLength)
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

func (s *Service) HasUsers(ctx context.Context) (bool, error) {
	return s.store.HasUsers(ctx)
}

// CreateFirstAccount atomically creates the installation's sole bootstrap
// learner and its initial session. The store arbitrates concurrent attempts.
func (s *Service) CreateFirstAccount(ctx context.Context, username, password string) (domain.User, string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return domain.User{}, "", ErrInvalidUsername
	}
	if len(password) < 8 {
		return domain.User{}, "", ErrInvalidPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return domain.User{}, "", err
	}
	raw, tokenHash, err := GenerateSessionToken()
	if err != nil {
		return domain.User{}, "", err
	}
	u, created, err := s.store.CreateFirstUserAndSession(ctx, username, hash, tokenHash, s.now().Add(s.lifetime))
	if err != nil {
		return domain.User{}, "", err
	}
	if !created {
		return domain.User{}, "", ErrFirstAccountExists
	}
	return u, raw, nil
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

func AuthorizeOwner(user domain.User, ownerID string) error {
	if user.ID != ownerID {
		return ErrForbidden
	}
	return nil
}
