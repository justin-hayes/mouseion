package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"context"
	"sync"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// DeviceStore keeps Devices in memory for the fixture server.
type DeviceStore struct {
	mu      sync.Mutex
	devices []fixtureDevice
}

type fixtureDevice struct {
	owner  string
	device domain.Device
}

func NewDeviceStore() *DeviceStore { return &DeviceStore{} }

func (s *DeviceStore) CreateDevice(_ context.Context, ownerID, name, username, _ string) (domain.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.devices {
		if d.device.Username == username {
			return domain.Device{}, auth.ErrDeviceUsernameTaken
		}
	}
	device := domain.Device{ID: username, Name: name, Username: username, CreatedAt: time.Now().UTC()}
	s.devices = append(s.devices, fixtureDevice{owner: ownerID, device: device})
	return device, nil
}

func (s *DeviceStore) ListDevices(_ context.Context, ownerID string) ([]domain.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Device
	for _, d := range s.devices {
		if d.owner == ownerID && d.device.RevokedAt == nil {
			out = append(out, d.device)
		}
	}
	return out, nil
}
