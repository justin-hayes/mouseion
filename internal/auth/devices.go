package auth

import (
	"context"
	"crypto/md5" //nolint:gosec // KOReader's kosync wire key is the MD5 hex of the password; it is not a security boundary here.
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/domain"
)

const (
	// MaxDeviceNameLength is the longest Device name, in characters.
	MaxDeviceNameLength = 60

	deviceAlphabet      = "23456789abcdefghjkmnpqrstuvwxyz" // lower-case letters and digits without 0, 1, i, l, o
	devicePasswordGroup = 4
	devicePasswordParts = 4
	deviceSuffixLength  = 4
	deviceSlugLength    = 20
	deviceIssueAttempts = 8
)

var (
	// ErrInvalidDeviceName reports an empty or over-long Device name.
	ErrInvalidDeviceName = errors.New("auth: device name must be 1 to 60 characters")
	// ErrDeviceUsernameTaken is returned by a DeviceStore when the username exists.
	ErrDeviceUsernameTaken = errors.New("auth: device username already exists")
)

// DeviceStore persists Devices. It is separate from Store so the session
// store implementations stay unchanged.
type DeviceStore interface {
	// CreateDevice stores a Device and returns ErrDeviceUsernameTaken when the
	// username already exists.
	CreateDevice(ctx context.Context, ownerID, name, username, keyHash string) (domain.Device, error)
	// ListDevices returns the owner's Devices that are not revoked, oldest first.
	ListDevices(ctx context.Context, ownerID string) ([]domain.Device, error)
}

// DeviceService issues and lists Device credentials.
type DeviceService struct {
	store DeviceStore
}

// NewDeviceService returns a DeviceService over store.
func NewDeviceService(store DeviceStore) *DeviceService { return &DeviceService{store: store} }

// DeviceCredential is a newly issued Device with the password shown once.
type DeviceCredential struct {
	Device   domain.Device
	Password string
}

// ValidateDeviceName trims name and reports whether it is a valid Device name.
func ValidateDeviceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxDeviceNameLength {
		return name, ErrInvalidDeviceName
	}
	return name, nil
}

// IssueDevice adds a Device for ownerID and returns its credential. The
// username is a slug of the name plus a random suffix, retried while taken.
// The password is returned once; only SHA-256 of its MD5 hex key is stored.
func (s *DeviceService) IssueDevice(ctx context.Context, ownerID, name string) (DeviceCredential, error) {
	name, err := ValidateDeviceName(name)
	if err != nil {
		return DeviceCredential{}, err
	}
	password, err := generateDevicePassword()
	if err != nil {
		return DeviceCredential{}, err
	}
	keyHash := DeviceKeyHash(DeviceKey(password))
	slug := deviceSlug(name)
	for range deviceIssueAttempts {
		suffix, err := randomDeviceString(deviceSuffixLength)
		if err != nil {
			return DeviceCredential{}, err
		}
		device, err := s.store.CreateDevice(ctx, ownerID, name, slug+"-"+suffix, keyHash)
		if errors.Is(err, ErrDeviceUsernameTaken) {
			continue
		}
		if err != nil {
			return DeviceCredential{}, err
		}
		return DeviceCredential{Device: device, Password: password}, nil
	}
	return DeviceCredential{}, fmt.Errorf("auth: no free device username for %q after %d attempts", slug, deviceIssueAttempts)
}

// ListDevices returns the owner's Devices.
func (s *DeviceService) ListDevices(ctx context.Context, ownerID string) ([]domain.Device, error) {
	return s.store.ListDevices(ctx, ownerID)
}

// DeviceKey is the lower-case MD5 hex of the password, which KOReader sends as
// its x-auth-key header.
func DeviceKey(password string) string {
	sum := md5.Sum([]byte(password)) //nolint:gosec // see the crypto/md5 import.
	return hex.EncodeToString(sum[:])
}

// DeviceKeyHash is the value stored for a Device: SHA-256 of the MD5 hex key.
func DeviceKeyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// deviceSlug lower-cases name to ASCII letters and digits joined by single
// hyphens, at most deviceSlugLength characters, falling back to "device".
func deviceSlug(name string) string {
	var b strings.Builder
	pendingHyphen := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingHyphen = false
			b.WriteRune(r)
			continue
		}
		pendingHyphen = true
	}
	slug := b.String()
	if len(slug) > deviceSlugLength {
		slug = strings.TrimRight(slug[:deviceSlugLength], "-")
	}
	if slug == "" {
		return "device"
	}
	return slug
}

// generateDevicePassword returns four groups of four characters joined by
// hyphens, for example "k3f9-m2xq-7bnd-hw4t".
func generateDevicePassword() (string, error) {
	groups := make([]string, devicePasswordParts)
	for i := range groups {
		group, err := randomDeviceString(devicePasswordGroup)
		if err != nil {
			return "", err
		}
		groups[i] = group
	}
	return strings.Join(groups, "-"), nil
}

// randomDeviceString draws n characters of deviceAlphabet uniformly from
// crypto/rand by rejecting bytes outside the largest multiple of the alphabet.
func randomDeviceString(n int) (string, error) {
	limit := 256 - 256%len(deviceAlphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n*2)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate device credential: %w", err)
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, deviceAlphabet[int(b)%len(deviceAlphabet)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}
