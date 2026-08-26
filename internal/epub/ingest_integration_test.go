//go:build integration

package epub

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestImportPostgresOwnerIsolationHistoryAndDeletion(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("MOUSEION_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
	}
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	conn, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
	if _, err = admin.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = persistence.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "epub-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "epub-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewService(store).Import(ctx, alice.ID, "de", fixtureDirectory(t, "testfixtures/epub3-edge-cases"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSourceMaterial(ctx, alice.ID, result.Source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerID != alice.ID || got.FullText != result.Book.FullText || len(got.Content) == 0 {
		t.Fatalf("stored source: %+v", got)
	}
	units, err := store.GetExtractedUnits(ctx, alice.ID, result.Source.ID)
	if err != nil || !reflect.DeepEqual(units, result.Book.ExtractedUnits) {
		t.Fatalf("stored units=%+v err=%v, want %+v", units, err, result.Book.ExtractedUnits)
	}
	snapshotID, persistedUnits, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, result.Source.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantClassifications, err := ClassifyUnits(snapshotID, persistedUnits)
	if err != nil {
		t.Fatal(err)
	}
	classifications, err := store.GetEPUBUnitClassifications(ctx, alice.ID, result.Source.ID, ClassifierName, ClassifierVersion)
	if err != nil {
		t.Fatal(err)
	}
	for i := range classifications {
		if classifications[i].CreatedAt.IsZero() || classifications[i].UpdatedAt.IsZero() {
			t.Fatalf("classification %d timestamps are missing", i)
		}
		classifications[i].CreatedAt = wantClassifications[i].CreatedAt
		classifications[i].UpdatedAt = wantClassifications[i].UpdatedAt
	}
	if !reflect.DeepEqual(classifications, wantClassifications) {
		t.Fatalf("persisted classifications=%+v\nwant=%+v", classifications, wantClassifications)
	}
	if got.FullText != result.Book.FullText || !strings.Contains(got.FullText, "Bibliographie.") || classifications[len(classifications)-1].RecommendedInclusion {
		t.Fatalf("classification changed analysis scope: full_text=%q classifications=%+v", got.FullText, classifications)
	}
	if _, err = store.GetExtractedUnits(ctx, bob.ID, result.Source.ID); !errors.Is(err, ErrExtractedUnitsUnavailable) {
		t.Fatalf("bob read alice units: %v", err)
	}
	if _, err = store.GetEPUBUnitClassifications(ctx, bob.ID, result.Source.ID, ClassifierName, ClassifierVersion); !errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		t.Fatalf("bob read alice classifications: %v", err)
	}
	second, err := NewService(store).Import(ctx, alice.ID, "de", fixtureDirectory(t, "testfixtures/epub3-edge-cases"))
	if err != nil || second.Source.ID != result.Source.ID {
		t.Fatalf("idempotent reimport: source=%+v err=%v", second.Source, err)
	}
	var snapshotCount, unitCount int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, result.Source.ID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, result.Source.ID).Scan(&unitCount); err != nil {
		t.Fatal(err)
	}
	if snapshotCount != 1 || unitCount != len(result.Book.ExtractedUnits.Units) {
		t.Fatalf("reimport snapshots=%d units=%d", snapshotCount, unitCount)
	}
	reimportSnapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, result.Source.ID)
	if err != nil || reimportSnapshotID != snapshotID {
		t.Fatalf("idempotent reimport snapshot=%q want=%q err=%v", reimportSnapshotID, snapshotID, err)
	}
	replacement, err := NewService(store).Import(ctx, alice.ID, "de", fixtureDirectory(t, "testfixtures/epub3-reimport"))
	if err != nil || replacement.Source.ID != result.Source.ID {
		t.Fatalf("replace imported snapshot: source=%+v err=%v", replacement.Source, err)
	}
	units, err = store.GetExtractedUnits(ctx, alice.ID, result.Source.ID)
	if err != nil || !reflect.DeepEqual(units, replacement.Book.ExtractedUnits) {
		t.Fatalf("replacement units=%+v err=%v, want %+v", units, err, replacement.Book.ExtractedUnits)
	}
	got, err = store.GetSourceMaterial(ctx, alice.ID, result.Source.ID)
	if err != nil || got.FullText != replacement.Book.FullText || got.ContentHash != replacement.Source.ContentHash {
		t.Fatalf("replacement source=%+v err=%v", got, err)
	}
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, result.Source.ID).Scan(&unitCount); err != nil || unitCount != len(result.Book.ExtractedUnits.Units)+len(replacement.Book.ExtractedUnits.Units) {
		t.Fatalf("replacement unit count=%d err=%v", unitCount, err)
	}
	replacementSnapshotID, replacementUnits, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, result.Source.ID)
	if err != nil || replacementSnapshotID == snapshotID {
		t.Fatalf("replacement snapshot=%q old=%q err=%v", replacementSnapshotID, snapshotID, err)
	}
	replacementClassifications, err := store.GetEPUBUnitClassifications(ctx, alice.ID, result.Source.ID, ClassifierName, ClassifierVersion)
	if err != nil || len(replacementClassifications) != len(replacementUnits.Units) || replacementClassifications[0].SourceUnitSnapshot.SnapshotID != replacementSnapshotID {
		t.Fatalf("replacement classifications=%+v units=%+v err=%v", replacementClassifications, replacementUnits, err)
	}
	if _, err = admin.Exec(ctx, `INSERT INTO source_material_units(owner_id,source_material_id,snapshot_id,unit_id,unit_order,spine_index,title,title_source,text,start_offset,end_offset,package_path,manifest_id,source_href,resolved_href,media_type,linear) SELECT $1,source_material_id,snapshot_id,'cross-owner',99,99,'x','heading','x',0,1,'x','x','x','x','application/xhtml+xml',true FROM source_material_unit_snapshots WHERE owner_id=$2 AND source_material_id=$3`, bob.ID, alice.ID, result.Source.ID); err == nil {
		t.Fatal("cross-owner unit insert succeeded")
	}
	if _, err = store.GetSourceMaterial(ctx, bob.ID, result.Source.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("bob read alice source: %v", err)
	}
	legacy, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "legacy-epub", Title: "Legacy", MediaType: MediaType(), ContentHash: "sha256:legacy", Content: []byte("retained epub"), FullText: "Legacy full text."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetExtractedUnits(ctx, alice.ID, legacy.ID); !errors.Is(err, ErrExtractedUnitsUnavailable) {
		t.Fatalf("legacy source units: %v", err)
	}
	if _, err = store.GetEPUBUnitClassifications(ctx, alice.ID, legacy.ID, ClassifierName, ClassifierVersion); !errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		t.Fatalf("legacy source classifications: %v", err)
	}
	var historyCount int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='epub.import' AND status='complete' AND details->>'source_material_id'=$2`, alice.ID, result.Source.ID).Scan(&historyCount); err != nil || historyCount != 3 {
		t.Fatalf("history count=%d err=%v", historyCount, err)
	}
	if _, err = admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, alice.ID); err != nil {
		t.Fatal(err)
	}
	var sourceCount, remainingHistory, remainingUnits int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM source_materials WHERE id=$1`, result.Source.ID).Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1`, alice.ID).Scan(&remainingHistory); err != nil {
		t.Fatal(err)
	}
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1`, alice.ID).Scan(&remainingUnits); err != nil {
		t.Fatal(err)
	}
	if sourceCount != 0 || remainingHistory != 0 || remainingUnits != 0 {
		t.Fatalf("private artifacts survived deletion: sources=%d history=%d units=%d", sourceCount, remainingHistory, remainingUnits)
	}
}
