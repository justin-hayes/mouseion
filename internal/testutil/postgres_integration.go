//go:build integration

package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Postgres starts a disposable PostgreSQL instance unless an external test
// database was explicitly configured. If Testcontainers cannot reach Docker,
// it preserves the legacy localhost fallback. migrate must install every schema
// the test needs, including River's schema.
func Postgres(t *testing.T, ctx context.Context, migrate func(string) error) (string, *pgxpool.Pool) {
	t.Helper()

	databaseURL, explicit := externalDatabaseURL()
	var container *postgres.PostgresContainer
	if !explicit {
		// The named database is shared across package processes; leave it alive
		// for the next process instead of letting Ryuk reap it at process exit.
		t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
		var err error
		container, err = postgres.Run(ctx, "postgres:16-alpine",
			postgres.WithDatabase("mouseion_test"),
			postgres.WithUsername("postgres"),
			postgres.WithPassword("postgres"),
			postgres.BasicWaitStrategies(),
			testcontainers.WithReuseByName("mouseion-test-postgres"),
		)
		if err != nil {
			t.Logf("Testcontainers unavailable; using external PostgreSQL fallback %s: %v", databaseURL, err)
		} else {
			databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
			if err != nil {
				t.Fatalf("get PostgreSQL container connection string: %v", err)
			}
		}
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping integration database: %v", err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		t.Fatalf("acquire integration database lock connection: %v", err)
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("lock integration database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
		conn.Release()
		pool.Close()
	})
	if _, err = pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset integration database: %v", err)
	}
	if err = migrate(databaseURL); err != nil {
		t.Fatalf("migrate integration database: %v", err)
	}
	return databaseURL, pool
}
