package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type memoryDeviceStore struct {
	devices []domain.Device
	hashes  map[string]string
	taken   int // usernames to refuse before accepting one
	err     error
}

func (s *memoryDeviceStore) CreateDevice(_ context.Context, _, name, username, keyHash string) (domain.Device, error) {
	if s.err != nil {
		return domain.Device{}, s.err
	}
	if s.taken > 0 {
		s.taken--
		return domain.Device{}, ErrDeviceUsernameTaken
	}
	for _, d := range s.devices {
		if d.Username == username {
			return domain.Device{}, ErrDeviceUsernameTaken
		}
	}
	d := domain.Device{ID: username, Name: name, Username: username, CreatedAt: time.Now()}
	s.devices = append(s.devices, d)
	if s.hashes == nil {
		s.hashes = map[string]string{}
	}
	s.hashes[username] = keyHash
	return d, nil
}

func (s *memoryDeviceStore) ListDevices(context.Context, string) ([]domain.Device, error) {
	return s.devices, nil
}

var (
	devicePasswordShape = regexp.MustCompile(`^[2-9a-hjkmnp-z]{4}(-[2-9a-hjkmnp-z]{4}){3}$`)
	deviceUsernameShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*-[2-9a-hjkmnp-z]{4}$`)
)

func TestIssueDeviceCredentialShapeAndStorage(t *testing.T) {
	store := &memoryDeviceStore{}
	cred, err := NewDeviceService(store).IssueDevice(context.Background(), "owner", "  Palma  ")
	if err != nil {
		t.Fatal(err)
	}
	if cred.Device.Name != "Palma" || !strings.HasPrefix(cred.Device.Username, "palma-") || !deviceUsernameShape.MatchString(cred.Device.Username) {
		t.Fatalf("device = %+v", cred.Device)
	}
	if !devicePasswordShape.MatchString(cred.Password) {
		t.Fatalf("password %q has the wrong shape", cred.Password)
	}
	key := DeviceKey(cred.Password)
	sum := sha256.Sum256([]byte(key))
	if got := store.hashes[cred.Device.Username]; got != hex.EncodeToString(sum[:]) || got != DeviceKeyHash(key) {
		t.Fatalf("stored hash = %q", got)
	}
	if strings.Contains(store.hashes[cred.Device.Username], cred.Password) {
		t.Fatal("stored value contains the password")
	}
}

func TestDeviceKeyIsLowercaseMD5Hex(t *testing.T) {
	if got := DeviceKey("abc"); got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("DeviceKey = %q", got)
	}
}

func TestIssueDeviceUsernamesAreUniqueForCollidingSlugs(t *testing.T) {
	store := &memoryDeviceStore{}
	svc := NewDeviceService(store)
	seen := map[string]bool{}
	for range 50 {
		cred, err := svc.IssueDevice(context.Background(), "owner", "Palma!")
		if err != nil {
			t.Fatal(err)
		}
		if seen[cred.Device.Username] {
			t.Fatalf("duplicate username %q", cred.Device.Username)
		}
		seen[cred.Device.Username] = true
	}
}

func TestIssueDeviceRetriesTakenUsernameAndGivesUp(t *testing.T) {
	store := &memoryDeviceStore{taken: deviceIssueAttempts - 1}
	if _, err := NewDeviceService(store).IssueDevice(context.Background(), "owner", "Palma"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	store = &memoryDeviceStore{taken: deviceIssueAttempts}
	if _, err := NewDeviceService(store).IssueDevice(context.Background(), "owner", "Palma"); err == nil || errors.Is(err, ErrDeviceUsernameTaken) {
		t.Fatalf("exhausted retries = %v", err)
	}
	boom := errors.New("boom")
	store = &memoryDeviceStore{err: boom}
	if _, err := NewDeviceService(store).IssueDevice(context.Background(), "owner", "Palma"); !errors.Is(err, boom) {
		t.Fatalf("store error = %v", err)
	}
}

func TestDeviceNameValidation(t *testing.T) {
	for name, wantErr := range map[string]bool{
		"":                                    true,
		"   ":                                 true,
		strings.Repeat("a", 60):               false,
		strings.Repeat("a", 61):               true,
		strings.Repeat("é", 60):               false,
		"  " + strings.Repeat("a", 60) + "  ": false,
	} {
		got, err := ValidateDeviceName(name)
		if (err != nil) != wantErr || (err != nil && !errors.Is(err, ErrInvalidDeviceName)) {
			t.Errorf("ValidateDeviceName(%d chars) err = %v", len(name), err)
		}
		if got != strings.TrimSpace(name) {
			t.Errorf("name not trimmed: %q", got)
		}
	}
	if _, err := NewDeviceService(&memoryDeviceStore{}).IssueDevice(context.Background(), "owner", " "); !errors.Is(err, ErrInvalidDeviceName) {
		t.Fatalf("IssueDevice empty = %v", err)
	}
}

func TestDeviceSlug(t *testing.T) {
	for name, want := range map[string]string{
		"Palma":                      "palma",
		"Kobo  Libra #2":             "kobo-libra-2",
		"--Pocket--":                 "pocket",
		"Ελληνικά":                   "device",
		"a very long device name ok": "a-very-long-device-n",
	} {
		if got := deviceSlug(name); got != want {
			t.Errorf("deviceSlug(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestRandomDeviceStringUsesOnlyTheAlphabet(t *testing.T) {
	s, err := randomDeviceString(500)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range s {
		if !strings.ContainsRune(deviceAlphabet, r) {
			t.Fatalf("unexpected %q", r)
		}
	}
	if strings.ContainsAny(deviceAlphabet, "01ilo") {
		t.Fatal("ambiguous characters in the alphabet")
	}
}
