package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// CreateDevice stores a Device and maps a taken username to
// auth.ErrDeviceUsernameTaken so the auth service can retry.
func (s *PostgresStore) CreateDevice(ctx context.Context, ownerID, name, username, keyHash string) (domain.Device, error) {
	row, err := s.queries().InsertDevice(ctx, sqlcgen.InsertDeviceParams{Owner: ownerID, Name: name, Username: username, KeyHash: keyHash})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "devices_username_key" {
			return domain.Device{}, auth.ErrDeviceUsernameTaken
		}
		return domain.Device{}, err
	}
	return domain.Device{ID: row.ID, Name: row.Name, Username: row.Username, CreatedAt: row.CreatedAt, LastSeenAt: pgTimePtr(row.LastSeenAt), RevokedAt: pgTimePtr(row.RevokedAt)}, nil
}

// ListDevices returns the owner's Devices, revoked ones included, oldest first.
func (s *PostgresStore) ListDevices(ctx context.Context, ownerID string) ([]domain.Device, error) {
	rows, err := s.queries().ListDevices(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	devices := make([]domain.Device, 0, len(rows))
	for _, row := range rows {
		devices = append(devices, domain.Device{ID: row.ID, Name: row.Name, Username: row.Username, CreatedAt: row.CreatedAt, LastSeenAt: pgTimePtr(row.LastSeenAt), RevokedAt: pgTimePtr(row.RevokedAt)})
	}
	return devices, nil
}
