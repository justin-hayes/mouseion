package migrations

import (
	"testing"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func TestEmbeddedMigrationsHaveUniqueVersions(t *testing.T) {
	if _, err := iofs.New(FS, "."); err != nil {
		t.Fatalf("embedded migrations are not a valid migration source: %v", err)
	}
}
