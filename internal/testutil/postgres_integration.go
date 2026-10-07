//go:build integration

package testutil

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// integrationLockKey serializes template builds across package processes. It
// is taken in the server's maintenance database, never in a test database.
const integrationLockKey = 90420009

var serverURL struct {
	sync.Mutex
	url string
}

// Postgres returns a private database for t, cloned from a template database
// that migrate built once per migration set. Tests therefore neither share
// state nor wait for one another, and package processes run concurrently.
//
// It starts (or reuses) a named PostgreSQL container unless an external test
// server was explicitly configured. If Testcontainers cannot reach Docker, it
// preserves the legacy localhost fallback. An external server's role needs the
// CREATEDB privilege. migrate must install every schema the test needs,
// including River's schema.
func Postgres(t *testing.T, ctx context.Context, migrate func(string) error) (string, *pgxpool.Pool) {
	t.Helper()

	adminURL := integrationServerURL(t, ctx)
	name := "mouseion_t_" + randomHex(t)
	if err := createDatabase(ctx, adminURL, name, migrate); err != nil {
		t.Fatalf("create integration database: %v", err)
	}
	Cleanup(t, "integration database", func() error {
		return dropDatabase(context.Background(), adminURL, name)
	})

	databaseURL, err := withDatabase(adminURL, name)
	if err != nil {
		t.Fatalf("integration database URL: %v", err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	// Registered after the drop so the pool closes first.
	t.Cleanup(pool.Close)
	if err = pool.Ping(ctx); err != nil {
		t.Fatalf("ping integration database: %v", err)
	}
	return databaseURL, pool
}

// integrationServerURL resolves the server once per process; asking
// Testcontainers for the reused container on every test costs a Docker lookup
// and readiness wait each time.
func integrationServerURL(t *testing.T, ctx context.Context) string {
	t.Helper()
	serverURL.Lock()
	defer serverURL.Unlock()
	if serverURL.url != "" {
		return serverURL.url
	}

	databaseURL, explicit := externalDatabaseURL()
	if !explicit {
		// The named server is shared across package processes; leave it alive
		// for the next process instead of letting Ryuk reap it at process exit.
		t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
		container, err := postgres.Run(ctx, "postgres:16-alpine",
			postgres.WithDatabase("mouseion_test"),
			postgres.WithUsername("postgres"),
			postgres.WithPassword("postgres"),
			postgres.BasicWaitStrategies(),
			// Concurrent package processes each hold pools and River clients.
			testcontainers.WithCmdArgs("-c", "max_connections=300"),
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
	serverURL.url = databaseURL
	return databaseURL
}

// createDatabase clones name from the template database for migrate.
func createDatabase(ctx context.Context, adminURL, name string, migrate func(string) error) (err error) {
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("open integration server: %w", err)
	}
	defer func() { err = errors.Join(err, admin.Close(context.Background())) }()

	template := templateDatabaseName(migrate)
	if err = ensureTemplateDatabase(ctx, admin, adminURL, template, migrate); err != nil {
		return fmt.Errorf("prepare template database: %w", err)
	}
	_, err = admin.Exec(ctx, "CREATE DATABASE "+quoteIdent(name)+" TEMPLATE "+quoteIdent(template))
	return err
}

// ensureTemplateDatabase builds template with migrate unless a completed build
// exists. is_template marks completion, so an interrupted build is rebuilt.
func ensureTemplateDatabase(ctx context.Context, admin *pgx.Conn, adminURL, template string, migrate func(string) error) (err error) {
	if _, err = admin.Exec(ctx, `SELECT pg_advisory_lock($1)`, integrationLockKey); err != nil {
		return fmt.Errorf("lock template build: %w", err)
	}
	defer func() {
		_, unlockErr := admin.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, integrationLockKey)
		err = errors.Join(err, unlockErr)
	}()

	var complete bool
	err = admin.QueryRow(ctx, `SELECT datistemplate FROM pg_database WHERE datname=$1`, template).Scan(&complete)
	if err == nil && complete {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(template)+" WITH (FORCE)"); err != nil {
		return err
	}
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoteIdent(template)); err != nil {
		return err
	}
	templateURL, err := withDatabase(adminURL, template)
	if err != nil {
		return err
	}
	if err = migrate(templateURL); err != nil {
		return fmt.Errorf("migrate template database: %w", err)
	}
	_, err = admin.Exec(ctx, "ALTER DATABASE "+quoteIdent(template)+" WITH is_template true")
	return err
}

// templateDatabaseName identifies the schema migrate installs: the migrate
// function, the embedded migrations, and the migration library versions
// (River's schema comes from its module version).
func templateDatabaseName(migrate func(string) error) string {
	h := sha256.New()
	h.Write([]byte(runtime.FuncForPC(reflect.ValueOf(migrate).Pointer()).Name() + "\n"))
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if strings.HasPrefix(dep.Path, "github.com/riverqueue/") || strings.HasPrefix(dep.Path, "github.com/golang-migrate/") {
				h.Write([]byte(dep.Path + " " + dep.Version + "\n"))
			}
		}
	}
	err := fs.WalkDir(migrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := fs.ReadFile(migrations.FS, path)
		h.Write([]byte(fmt.Sprintf("%s %d\n", path, len(content))))
		h.Write(content)
		return err
	})
	if err != nil {
		panic(fmt.Sprintf("hash embedded migrations: %v", err))
	}
	return "mouseion_tmpl_" + hex.EncodeToString(h.Sum(nil))[:16]
}

func dropDatabase(ctx context.Context, adminURL, name string) (err error) {
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, admin.Close(context.Background())) }()
	_, err = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(name)+" WITH (FORCE)")
	return err
}

func withDatabase(rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}

func quoteIdent(name string) string { return pgx.Identifier{name}.Sanitize() }

func randomHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random integration database name: %v", err)
	}
	return hex.EncodeToString(b)
}
