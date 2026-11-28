//go:build integration

package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bookPreparationFailureFixture seeds one Classical Book and one EPUB Book, the
// two kinds the shared failure record serves.
type bookPreparationFailureFixture struct {
	pool            *pgxpool.Pool
	store           *PostgresStore
	databaseURL     string
	owner           string
	classicalBookID string
	readingUnitKey  string
	revision        int
	epubBookID      string
	connectionID    string
}

// The structural migration that creates the shared failure record, and the
// separate data-only backfill that copies both old tables into it as its
// successor (ADR 0038 keeps the pair in distinct files).
const (
	bookPreparationFailuresMigration = 57
	bookPreparationFailuresBackfill  = 58
)

// migrateApplicationMigrationsThroughBackfill migrates a database only
// through the shared failure record and its backfill (000058), then installs
// River's schema so the store's preparation-status reads resolve. A fresh
// deployment runs the backfill while the two old failure tables still exist;
// the cleanup migration 000060 later drops them, so the latest template can
// no longer seed those tables. This capped migrate builds the template the
// backfill's own test still needs.
func migrateApplicationMigrationsThroughBackfill(databaseURL string) (err error) {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := m.Close()
		err = errors.Join(err, sourceErr, databaseErr)
	}()
	if err := m.Migrate(bookPreparationFailuresBackfill); err != nil {
		return fmt.Errorf("apply migrations through the backfill: %w", err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open postgres for River migrations: %w", err)
	}
	defer pool.Close()
	riverMigrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("initialize River migrations: %w", err)
	}
	if _, err = riverMigrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("apply River migrations: %w", err)
	}
	return nil
}

func newBookPreparationFailureFixture(t *testing.T, migrateDB func(string) error) bookPreparationFailureFixture {
	t.Helper()
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, migrateDB)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "preparation-failure-owner", false)
	require.NoError(t, err)
	require.NoError(t, store.ReconcileClassicalManifest(ctx, classicalAddDefinitions()))
	report, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"work-1"})
	require.NoError(t, err)
	require.Len(t, report.Books, 1)
	// Plain SQL, not the Book detail read: this database is capped at an older
	// schema that has no Position reports, which the detail read queries.
	var readingUnitKey string
	var revision int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT ru.definition_key, ru.revision FROM book_reading_units bru
		JOIN reading_units ru ON ru.id = bru.reading_unit_id
		WHERE bru.owner_id = $1 AND bru.book_id = $2`, owner.ID, report.Books[0].BookID).Scan(&readingUnitKey, &revision))

	epubBook, err := store.CreateBook(ctx, domain.Book{
		OwnerID:            owner.ID,
		Title:              "An EPUB book",
		MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        "de",
	})
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{
		Name: "Catalog", URL: "https://catalog.example/opds",
	})
	require.NoError(t, err)
	return bookPreparationFailureFixture{
		pool: pool, store: store, databaseURL: databaseURL, owner: owner.ID,
		classicalBookID: report.Books[0].BookID,
		readingUnitKey:  readingUnitKey,
		revision:        revision,
		epubBookID:      epubBook.ID,
		connectionID:    connection.ID,
	}
}

// TestBookPreparationFailuresBackfillCopiesBothKindsUnchanged exercises the
// data-only migration as its own step: rows from both old tables arrive with
// unchanged values, and a second application changes nothing.
func TestBookPreparationFailuresBackfillCopiesBothKindsUnchanged(t *testing.T) {
	// The old failure tables exist only through the backfill, so this test
	// builds its database against the capped migrate, not the latest schema
	// the cleanup migration leaves behind.
	f := newBookPreparationFailureFixture(t, migrateApplicationMigrationsThroughBackfill)
	ctx := context.Background()

	classicalFailedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	epubFailedAt := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
	_, err := f.pool.Exec(ctx, `
		INSERT INTO classical_preparation_failures(
			owner_id, book_id, reading_unit_key, definition_revision, stage, code, failed_at)
		VALUES ($1, $2, $3, $4, 'source', 'source_pin_mismatch', $5)`,
		f.owner, f.classicalBookID, f.readingUnitKey, f.revision, classicalFailedAt)
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, `
		INSERT INTO epub_acquisition_failures(
			owner_id, book_id, connection_id, entry_identifier, stage, code, failed_at)
		VALUES ($1, $2, $3, 'entry-1', 'fetch', 'source_download_failed', $4)`,
		f.owner, f.epubBookID, f.connectionID, epubFailedAt)
	require.NoError(t, err)

	// The capped migrate built this database at the backfill's version, so
	// force the version to the structural migration and let the backfill run
	// as a real successor data migration. Stop at the backfill's version so
	// later structural migrations do not run against schema that already
	// has them.
	forceApplicationMigration(t, f.databaseURL, bookPreparationFailuresMigration)
	moveApplicationMigrationsTo(t, f.databaseURL, bookPreparationFailuresBackfill)

	type failureRow struct {
		owner, book, unitKey, entry string
		revision                    int
		connection                  string
		stage, code                 string
		failedAt                    time.Time
	}
	readRows := func(t *testing.T) []failureRow {
		t.Helper()
		rows, err := f.pool.Query(ctx, `
			SELECT owner_id::text, book_id::text,
			       COALESCE(reading_unit_key, ''), COALESCE(definition_revision, 0),
			       COALESCE(connection_id::text, ''), COALESCE(entry_identifier, ''),
			       stage, code, failed_at
			FROM book_preparation_failures
			WHERE owner_id = $1
			ORDER BY book_id`, f.owner)
		require.NoError(t, err)
		defer rows.Close()
		var got []failureRow
		for rows.Next() {
			var row failureRow
			require.NoError(t, rows.Scan(&row.owner, &row.book, &row.unitKey, &row.revision,
				&row.connection, &row.entry, &row.stage, &row.code, &row.failedAt))
			row.failedAt = row.failedAt.UTC()
			got = append(got, row)
		}
		require.NoError(t, rows.Err())
		return got
	}

	first := readRows(t)
	require.Len(t, first, 2)
	byBook := map[string]failureRow{}
	for _, row := range first {
		byBook[row.book] = row
	}
	t.Run("Classical row is copied unchanged", func(t *testing.T) {
		assert.Equal(t, failureRow{
			owner: f.owner, book: f.classicalBookID,
			unitKey: f.readingUnitKey, revision: f.revision,
			stage: "source", code: "source_pin_mismatch", failedAt: classicalFailedAt,
		}, byBook[f.classicalBookID])
	})
	t.Run("EPUB row is copied unchanged", func(t *testing.T) {
		assert.Equal(t, failureRow{
			owner: f.owner, book: f.epubBookID,
			connection: f.connectionID, entry: "entry-1",
			stage: "fetch", code: "source_download_failed", failedAt: epubFailedAt,
		}, byBook[f.epubBookID])
	})
	t.Run("a repeated backfill changes nothing", func(t *testing.T) {
		forceApplicationMigration(t, f.databaseURL, 57)
		moveApplicationMigrationsTo(t, f.databaseURL, 58)
		assert.Equal(t, first, readRows(t))
	})
}

// TestBookPreparationFailureConstraintsRejectProvenanceAmbiguity pins the
// provenance rule: exactly one kind, and never both or neither.
func TestBookPreparationFailureConstraintsRejectProvenanceAmbiguity(t *testing.T) {
	f := newBookPreparationFailureFixture(t, Migrate)
	ctx := context.Background()
	insert := func(stage, unitKey string, revision int, connection, entry, book string) error {
		_, err := f.pool.Exec(ctx, `
			INSERT INTO book_preparation_failures(
				owner_id, book_id, reading_unit_key, definition_revision,
				connection_id, entry_identifier, stage, code)
			VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, 0),
			        NULLIF($5, '')::uuid, NULLIF($6, ''), $7, 'source_download_failed')`,
			f.owner, book, unitKey, revision, connection, entry, stage)
		return err
	}

	cases := []struct {
		name              string
		stage, unitKey    string
		revision          int
		connection, entry string
		book              string
		wantErr           bool
	}{
		{name: "both kinds' provenance", stage: "source", unitKey: f.readingUnitKey, revision: f.revision,
			connection: f.connectionID, entry: "entry-1", book: f.epubBookID, wantErr: true},
		{name: "neither kind's provenance outside the entry stage", stage: "fetch",
			book: f.epubBookID, wantErr: true},
		{name: "an incomplete Classical provenance", stage: "source", unitKey: f.readingUnitKey,
			book: f.classicalBookID, wantErr: true},
		{name: "an entry without its connection", stage: "fetch", entry: "entry-1",
			book: f.epubBookID, wantErr: true},
		{name: "an unresolved entry-stage EPUB failure", stage: "entry",
			book: f.epubBookID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := insert(tc.stage, tc.unitKey, tc.revision, tc.connection, tc.entry, tc.book)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestBookPreparationFailuresWriteOnlyTheNewTable proves both kinds write the
// shared record. The old tables no longer exist at the latest schema: the
// cleanup migration drops them, and the cleanup test pins that.
func TestBookPreparationFailuresWriteOnlyTheNewTable(t *testing.T) {
	f := newBookPreparationFailureFixture(t, Migrate)
	ctx := context.Background()

	require.NoError(t, f.store.PutClassicalPreparationFailure(ctx, domain.ClassicalPreparationFailureRecord{
		OwnerID: f.owner, BookID: f.classicalBookID,
		ReadingUnitKey: f.readingUnitKey, DefinitionRevision: f.revision,
		Stage: domain.BookPreparationStageSource, Code: domain.BookPreparationCodeSourcePinMismatch,
	}))
	require.NoError(t, f.store.RecordEPUBAcquisitionFailure(ctx, domain.EPUBAcquisitionFailureRecord{
		OwnerID: f.owner, BookID: f.epubBookID,
		ConnectionID: f.connectionID, EntryIdentifier: "entry-1",
		Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
	}))

	var count int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM book_preparation_failures WHERE owner_id = $1`, f.owner).Scan(&count))
	assert.Equal(t, 2, count, "both kinds share the new record")
}
