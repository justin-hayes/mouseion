//go:build integration

package epub

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
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
	result, err := NewService(store).Import(ctx, alice.ID, "de", fixture(t))
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
	if _, err = store.GetExtractedUnits(ctx, bob.ID, result.Source.ID); !errors.Is(err, ErrExtractedUnitsUnavailable) {
		t.Fatalf("bob read alice units: %v", err)
	}
	second, err := NewService(store).Import(ctx, alice.ID, "de", fixture(t))
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
	replacementText := "Grüße 👋"
	replacementUnits := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{{
		ID: UnitID(7, "unicode"), Order: 0, SpineIndex: 7, Title: "Überschrift", TitleSource: UnitTitleHeading,
		Text: replacementText, StartOffset: 0, EndOffset: uint64(len([]rune(replacementText))), PackagePath: "OPS/package.opf",
		ManifestID: "unicode", SourceHref: "Text/ü.xhtml", ResolvedHref: "OPS/Text/ü.xhtml",
		MediaType: "application/xhtml+xml", Properties: []string{"scripted"}, Linear: true,
		NavigationLabels: []string{"Grüße"}, LandmarkTypes: []string{"bodymatter"},
	}}}
	replacementSource := result.Source
	replacementSource.FullText = replacementText
	replacementSource.ContentHash = "sha256:unicode-reimport"
	if _, err = store.PutSourceMaterialWithExtractedUnits(ctx, replacementSource, replacementUnits); err != nil {
		t.Fatalf("replace unit snapshot: %v", err)
	}
	units, err = store.GetExtractedUnits(ctx, alice.ID, result.Source.ID)
	if err != nil || !reflect.DeepEqual(units, replacementUnits) {
		t.Fatalf("replacement units=%+v err=%v, want %+v", units, err, replacementUnits)
	}
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, result.Source.ID).Scan(&unitCount); err != nil || unitCount != 1 {
		t.Fatalf("replacement unit count=%d err=%v", unitCount, err)
	}
	if _, err = admin.Exec(ctx, `INSERT INTO source_material_units(owner_id,source_material_id,unit_id,unit_order,spine_index,title,title_source,text,start_offset,end_offset,package_path,manifest_id,source_href,resolved_href,media_type,linear) SELECT $1,source_material_id,'cross-owner',99,99,'x','heading','x',0,1,'x','x','x','x','application/xhtml+xml',true FROM source_material_unit_snapshots WHERE owner_id=$2 AND source_material_id=$3`, bob.ID, alice.ID, result.Source.ID); err == nil {
		t.Fatal("cross-owner unit insert succeeded")
	}
	if _, err = store.GetSourceMaterial(ctx, bob.ID, result.Source.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("bob read alice source: %v", err)
	}
	var historyCount int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='epub.import' AND status='complete' AND details->>'source_material_id'=$2`, alice.ID, result.Source.ID).Scan(&historyCount); err != nil || historyCount != 1 {
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
