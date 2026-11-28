//go:build integration

package opds

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/download"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/stretchr/testify/require"
)

type acquisitionHistory struct {
	CatalogDigest   string                  `json:"catalog_digest"`
	DocumentDigest  string                  `json:"document_digest"`
	CatalogSize     int64                   `json:"catalog_size"`
	RetainedSize    int64                   `json:"retained_size"`
	SlimmingVersion int                     `json:"slimming_version"`
	RemovedItems    *[]epub.RemovedResource `json:"removed_items"`
}

func TestAcquireStoresTheRetainedEPUBEndToEnd(t *testing.T) {
	ctx := t.Context()
	store := openAcquisitionStore(t, ctx)
	owner := newAcquisitionOwner(t, ctx, store)
	catalog := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "illustrated"))
	server := newCatalogServer(t, catalog)
	service := newAcquisitionService(t, store, server)
	connection := newAcquisitionConnection(t, ctx, store, owner.ID, server.URL)

	result, err := service.Acquire(ctx, owner.ID, connection.ID, "de", catalogEntry(server.URL))
	require.NoError(t, err)
	require.False(t, result.AlreadyPresent)

	retained, err := epub.Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
	require.NoError(t, err)
	source, err := store.GetSourceMaterial(ctx, owner.ID, result.Source.ID)
	require.NoError(t, err)

	t.Run("stores the Retained EPUB and its digest", func(t *testing.T) {
		require.Equal(t, retained.Content, source.Content, "the stored EPUB is not the Retained EPUB")
		require.Equal(t, domain.EPUBContentDigest(retained.Content), source.ContentHash)
		require.NotEqual(t, domain.EPUBContentDigest(catalog), source.ContentHash, "the unslimmed Catalog file was stored")
	})
	t.Run("drops removed resources and keeps the SVG page", func(t *testing.T) {
		names := providerEntryNames(t, source.Content)
		for _, removed := range []string{"OPS/images/cover.jpg", "OPS/fonts/book.otf", "OPS/media/sample.mp4"} {
			require.NotContains(t, names, removed)
		}
		require.Contains(t, names, "OPS/Text/plate.svg", "the reading-order SVG page was removed")
	})
	t.Run("extracts the same text and units as the Catalog file", func(t *testing.T) {
		direct, err := epub.ExtractV2(catalog)
		require.NoError(t, err)
		units, err := store.GetExtractedUnits(ctx, owner.ID, source.ID)
		require.NoError(t, err)
		require.Equal(t, direct.Book.FullText, source.FullText)
		require.Equal(t, direct.Book.ExtractedUnits, units)
	})
	t.Run("records the Catalog file's Document digest, not the Retained EPUB's", func(t *testing.T) {
		want := download.PartialMD5Of(catalog)
		require.NotEqual(t, want, download.PartialMD5Of(retained.Content), "the fixture's slimming left the digest unchanged")
		require.Equal(t, want, revisionDocumentDigest(t, ctx, store, owner.ID, result.Source.ID))
		require.Equal(t, want, acquisitionHistoryFor(t, ctx, store, owner.ID, source.ID).DocumentDigest)
	})
	t.Run("records Catalog and slimming provenance", func(t *testing.T) {
		details := acquisitionHistoryFor(t, ctx, store, owner.ID, source.ID)
		require.Equal(t, domain.EPUBContentDigest(catalog), details.CatalogDigest)
		require.Equal(t, int64(len(catalog)), details.CatalogSize)
		require.Equal(t, int64(len(retained.Content)), details.RetainedSize)
		require.Equal(t, epub.RetainedSlimmingVersion, details.SlimmingVersion)
		require.NotNil(t, details.RemovedItems)
		require.Equal(t, retained.Removed, *details.RemovedItems)
		require.Len(t, *details.RemovedItems, 6)
	})
	t.Run("removes the temporary Catalog file", func(t *testing.T) {
		assertNoTemporaryFiles(t, service.acquisitionTempDir)
	})
}

func TestAcquireStoresACleanCatalogFileByteIdentical(t *testing.T) {
	ctx := t.Context()
	store := openAcquisitionStore(t, ctx)
	owner := newAcquisitionOwner(t, ctx, store)
	catalog := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "epub2-clean"))
	server := newCatalogServer(t, catalog)
	service := newAcquisitionService(t, store, server)
	connection := newAcquisitionConnection(t, ctx, store, owner.ID, server.URL)

	result, err := service.Acquire(ctx, owner.ID, connection.ID, "de", catalogEntry(server.URL))
	require.NoError(t, err)
	source, err := store.GetSourceMaterial(ctx, owner.ID, result.Source.ID)
	require.NoError(t, err)
	t.Run("stores the Catalog file byte-for-byte", func(t *testing.T) {
		require.Equal(t, catalog, source.Content)
		require.Equal(t, domain.EPUBContentDigest(catalog), source.ContentHash)
	})
	t.Run("records the Catalog file's Document digest", func(t *testing.T) {
		want := download.PartialMD5Of(catalog)
		require.Equal(t, want, revisionDocumentDigest(t, ctx, store, owner.ID, result.Source.ID))
		require.Equal(t, want, acquisitionHistoryFor(t, ctx, store, owner.ID, source.ID).DocumentDigest)
	})
	t.Run("records the no-op slimming report", func(t *testing.T) {
		details := acquisitionHistoryFor(t, ctx, store, owner.ID, source.ID)
		require.Equal(t, domain.EPUBContentDigest(catalog), details.CatalogDigest)
		require.Equal(t, int64(len(catalog)), details.RetainedSize)
		require.NotNil(t, details.RemovedItems)
		require.Empty(t, *details.RemovedItems)
	})
}

func TestReacquiringTheSameIllustratedCatalogReusesOneRevision(t *testing.T) {
	ctx := t.Context()
	store := openAcquisitionStore(t, ctx)
	owner := newAcquisitionOwner(t, ctx, store)
	catalog := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "illustrated"))
	server := newCatalogServer(t, catalog)
	service := newAcquisitionService(t, store, server)
	connection := newAcquisitionConnection(t, ctx, store, owner.ID, server.URL)

	first, err := service.Acquire(ctx, owner.ID, connection.ID, "de", catalogEntry(server.URL))
	require.NoError(t, err)
	second, err := service.Acquire(ctx, owner.ID, connection.ID, "de", catalogEntry(server.URL))
	require.NoError(t, err)
	t.Run("resolves to the same source and content revision", func(t *testing.T) {
		require.True(t, second.AlreadyPresent)
		require.Equal(t, first.Source.ID, second.Source.ID)
		require.Equal(t, first.Source.ContentRevisionID, second.Source.ContentRevisionID)
	})
	t.Run("keeps one content revision and one import history row", func(t *testing.T) {
		require.Equal(t, 1, acquisitionRowCount(t, ctx, store, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, first.Source.ID))
		require.Equal(t, 1, acquisitionRowCount(t, ctx, store, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='epub.import' AND details->>'source_material_id'=$2`, owner.ID, first.Source.ID))
	})
}

func TestReacquiringALegacyUnSlimmedRevisionReusesIt(t *testing.T) {
	ctx := t.Context()
	store := openAcquisitionStore(t, ctx)
	owner := newAcquisitionOwner(t, ctx, store)
	catalog := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "illustrated"))

	// A content revision stored before ADR 0109 holds the unslimmed Catalog
	// file and its digest, and its analysis must stay current.
	legacy, err := epub.NewService(store).Import(ctx, owner.ID, "de", catalog)
	require.NoError(t, err)
	legacySnapshot, _, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, legacy.Source.ID)
	require.NoError(t, err)

	server := newCatalogServer(t, catalog)
	service := newAcquisitionService(t, store, server)
	connection := newAcquisitionConnection(t, ctx, store, owner.ID, server.URL)
	result, err := service.Acquire(ctx, owner.ID, connection.ID, "de", catalogEntry(server.URL))
	require.NoError(t, err)
	t.Run("reuses the legacy source and content revision", func(t *testing.T) {
		require.True(t, result.AlreadyPresent, "a pre-ADR revision was duplicated on re-acquisition")
		require.Equal(t, legacy.Source.ID, result.Source.ID)
		require.Equal(t, legacy.Source.ContentRevisionID, result.Source.ContentRevisionID)
	})

	source, err := store.GetSourceMaterial(ctx, owner.ID, legacy.Source.ID)
	require.NoError(t, err)
	snapshot, _, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, legacy.Source.ID)
	require.NoError(t, err)
	t.Run("leaves the stored bytes and snapshot untouched", func(t *testing.T) {
		require.Equal(t, catalog, source.Content, "re-acquisition rewrote the legacy revision's bytes")
		require.Equal(t, domain.EPUBContentDigest(catalog), source.ContentHash)
		require.Equal(t, legacySnapshot, snapshot, "re-acquisition replaced the snapshot that the current analysis uses")
	})
	t.Run("adds no content revision", func(t *testing.T) {
		require.Equal(t, 1, acquisitionRowCount(t, ctx, store, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, legacy.Source.ID))
	})
}

func TestReacquiringFillsAMissingDocumentDigestWithoutANewRevision(t *testing.T) {
	ctx := t.Context()
	store := openAcquisitionStore(t, ctx)
	owner := newAcquisitionOwner(t, ctx, store)
	catalog := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "illustrated"))
	legacy, err := epub.NewService(store).Import(ctx, owner.ID, "de", catalog)
	require.NoError(t, err)
	clearDocumentDigest(t, ctx, store, legacy.Source.ContentRevisionID)
	require.Empty(t, revisionDocumentDigest(t, ctx, store, owner.ID, legacy.Source.ID))

	server := newCatalogServer(t, catalog)
	service := newAcquisitionService(t, store, server)
	connection := newAcquisitionConnection(t, ctx, store, owner.ID, server.URL)
	result, err := service.Acquire(ctx, owner.ID, connection.ID, "de", catalogEntry(server.URL))
	require.NoError(t, err)

	t.Run("fills the digest in on the existing revision", func(t *testing.T) {
		require.True(t, result.AlreadyPresent)
		require.Equal(t, legacy.Source.ContentRevisionID, result.Source.ContentRevisionID)
		require.Equal(t, download.PartialMD5Of(catalog), revisionDocumentDigest(t, ctx, store, owner.ID, legacy.Source.ID))
	})
	t.Run("adds no content revision", func(t *testing.T) {
		require.Equal(t, 1, acquisitionRowCount(t, ctx, store, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, legacy.Source.ID))
	})
	t.Run("never overwrites a different digest", func(t *testing.T) {
		const other = "00000000000000000000000000000000"
		require.NoError(t, store.FillContentRevisionDocumentDigest(ctx, owner.ID, legacy.Source.ContentRevisionID, other))
		require.Equal(t, download.PartialMD5Of(catalog), revisionDocumentDigest(t, ctx, store, owner.ID, legacy.Source.ID))
		_, err := store.Pool().Exec(ctx, `UPDATE source_content_revisions SET document_digest=$1 WHERE revision_id=$2`, other, legacy.Source.ContentRevisionID)
		require.ErrorContains(t, err, "immutable")
	})
}

func revisionDocumentDigest(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, sourceID string) string {
	t.Helper()
	var digest *string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT r.document_digest FROM source_materials s JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.revision_id=s.current_content_revision_id WHERE s.owner_id=$1 AND s.id=$2`, ownerID, sourceID).Scan(&digest))
	if digest == nil {
		return ""
	}
	return *digest
}

// clearDocumentDigest recreates a revision stored before the digest existed. The
// immutability trigger forbids clearing a digest, so the test lifts it.
func clearDocumentDigest(t *testing.T, ctx context.Context, store *persistence.PostgresStore, revisionID string) {
	t.Helper()
	for _, statement := range []string{
		`ALTER TABLE source_content_revisions DISABLE TRIGGER source_content_revisions_immutable`,
		`UPDATE source_content_revisions SET document_digest = NULL WHERE revision_id = $1`,
		`ALTER TABLE source_content_revisions ENABLE TRIGGER source_content_revisions_immutable`,
	} {
		args := []any{}
		if strings.Contains(statement, "$1") {
			args = append(args, revisionID)
		}
		_, err := store.Pool().Exec(ctx, statement, args...)
		require.NoError(t, err)
	}
}

func openAcquisitionStore(t *testing.T, ctx context.Context) *persistence.PostgresStore {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "opds acquisition store", store.Close)
	return store
}

func newAcquisitionOwner(t *testing.T, ctx context.Context, store *persistence.PostgresStore) domain.User {
	t.Helper()
	owner, err := store.CreateUser(ctx, "opds-acquisition", false)
	require.NoError(t, err)
	return owner
}

func newAcquisitionService(t *testing.T, store *persistence.PostgresStore, server *httptest.Server) *Service {
	t.Helper()
	return NewService(store, epub.NewService(store), server.Client()).WithAcquisitionTempDir(t.TempDir())
}

func newAcquisitionConnection(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, catalogURL string) domain.OpdsConnection {
	t.Helper()
	connection, err := store.CreateOpdsConnection(ctx, ownerID, domain.OpdsConnection{Name: "Integration catalog", URL: catalogURL})
	require.NoError(t, err)
	return connection
}

func newCatalogServer(t *testing.T, catalog []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.Bytes(t, w, catalog)
	}))
	t.Cleanup(server.Close)
	return server
}

func catalogEntry(target string) Entry {
	return Entry{Links: []Link{{Rel: AcquisitionRel, Type: EPUBMediaType, Href: target}}}
}

func acquisitionHistoryFor(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, sourceID string) acquisitionHistory {
	t.Helper()
	var details []byte
	err := store.Pool().QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND operation='epub.import' AND details->>'source_material_id'=$2`, ownerID, sourceID).Scan(&details)
	require.NoError(t, err)
	var history acquisitionHistory
	require.NoError(t, json.Unmarshal(details, &history))
	return history
}

func acquisitionRowCount(t *testing.T, ctx context.Context, store *persistence.PostgresStore, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, store.Pool().QueryRow(ctx, query, args...).Scan(&count))
	return count
}

func providerEntryNames(t *testing.T, data []byte) []string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, path.Clean(file.Name))
	}
	return names
}
