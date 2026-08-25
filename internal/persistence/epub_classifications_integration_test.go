//go:build integration

package persistence

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestEPUBClassificationPersistenceOwnershipReplacementAndConstraints(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, _ := store.CreateUser(ctx, "classification-alice", false)
	bob, _ := store.CreateUser(ctx, "classification-bob", false)

	units := classificationTestUnits()
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "classified-book", Title: "Book", MediaType: "application/epub+zip", ContentHash: "sha256:classified", Content: []byte("epub"), FullText: "One\n\nTwo"}, units)
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, gotUnits, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, source.ID)
	if err != nil || !reflect.DeepEqual(gotUnits, units) || snapshotID == "" {
		t.Fatalf("snapshot id=%q units=%+v err=%v", snapshotID, gotUnits, err)
	}
	classifications := classificationTestResults(snapshotID, units)
	if err = store.ReplaceEPUBUnitClassifications(ctx, alice.ID, source.ID, classifications); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetEPUBUnitClassifications(ctx, alice.ID, source.ID, "deterministic", "1")
	if err != nil || len(got) != 2 {
		t.Fatalf("get classifications=%+v err=%v", got, err)
	}
	for i := range got {
		if got[i].CreatedAt.IsZero() || got[i].UpdatedAt.IsZero() {
			t.Fatalf("classification %d timestamps are missing", i)
		}
		got[i].CreatedAt, got[i].UpdatedAt = classifications[i].CreatedAt, classifications[i].UpdatedAt
	}
	if !reflect.DeepEqual(got, classifications) {
		t.Fatalf("round trip=%+v want=%+v", got, classifications)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO source_material_unit_classification_reasons(owner_id,source_material_id,snapshot_id,classifier_name,classifier_version,unit_id,reason_order,signal,message) VALUES($1,$2,$3,'deterministic','1',$4,0,'another','Duplicate order.')`, alice.ID, source.ID, snapshotID, units.Units[0].ID); err == nil {
		t.Fatal("duplicate reason order was accepted")
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO source_material_unit_classification_reasons(owner_id,source_material_id,snapshot_id,classifier_name,classifier_version,unit_id,reason_order,signal,message) VALUES($1,$2,$3,'deterministic','1',$4,99,'first','Duplicate signal.')`, alice.ID, source.ID, snapshotID, units.Units[0].ID); err == nil {
		t.Fatal("duplicate reason signal was accepted")
	}
	firstRead, err := store.GetEPUBUnitClassifications(ctx, alice.ID, source.ID, "deterministic", "1")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ReplaceEPUBUnitClassifications(ctx, alice.ID, source.ID, classifications); err != nil {
		t.Fatal(err)
	}
	secondRead, err := store.GetEPUBUnitClassifications(ctx, alice.ID, source.ID, "deterministic", "1")
	if err != nil || !reflect.DeepEqual(firstRead, secondRead) {
		t.Fatalf("deterministic reclassification changed persisted output: before=%+v after=%+v err=%v", firstRead, secondRead, err)
	}
	if _, err = store.GetEPUBUnitClassifications(ctx, bob.ID, source.ID, "deterministic", "1"); !errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		t.Fatalf("cross-owner read: %v", err)
	}
	if err = store.ReplaceEPUBUnitClassifications(ctx, bob.ID, source.ID, classifications); !errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		t.Fatalf("cross-owner write: %v", err)
	}
	duplicate := append([]domain.EPUBUnitClassification(nil), classifications...)
	duplicate[1].SourceUnitSnapshot.UnitID = duplicate[0].SourceUnitSnapshot.UnitID
	if err = store.ReplaceEPUBUnitClassifications(ctx, alice.ID, source.ID, duplicate); err == nil {
		t.Fatal("duplicate classification identity was accepted")
	}
	for _, malformed := range []struct {
		name       string
		category   string
		confidence int
	}{
		{"category", "invalid", 20},
		{"confidence", "main_matter", 101},
		{"unknown confidence", "unknown", 50},
	} {
		if _, err = store.Pool().Exec(ctx, `INSERT INTO source_material_unit_classifications(owner_id,source_material_id,snapshot_id,unit_id,schema_version,extracted_units_schema_version,classifier_name,classifier_version,category,confidence,recommended_inclusion) VALUES($1,$2,$3,$4,1,1,'malformed',$5,$6,$7,false)`, alice.ID, source.ID, snapshotID, units.Units[0].ID, malformed.name, malformed.category, malformed.confidence); err == nil {
			t.Fatalf("malformed %s was accepted", malformed.name)
		}
	}

	replacementUnits := domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "replacement"), Order: 0, SpineIndex: 0, ManifestID: "replacement", Text: "Replacement", EndOffset: 11}}}
	if _, err = store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: source.SourceIdentifier, Title: "Book", MediaType: source.MediaType, ContentHash: "sha256:replacement", Content: []byte("new epub"), FullText: "Replacement"}, replacementUnits); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetEPUBUnitClassifications(ctx, alice.ID, source.ID, "deterministic", "1"); !errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		t.Fatalf("stale classifications survived snapshot replacement: %v", err)
	}
	newSnapshotID, _, _ := store.GetExtractedUnitSnapshot(ctx, alice.ID, source.ID)
	if newSnapshotID == snapshotID {
		t.Fatal("snapshot replacement reused immutable identity")
	}
	if err = store.ReplaceEPUBUnitClassifications(ctx, alice.ID, source.ID, classifications); !errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		t.Fatalf("stale snapshot write: %v", err)
	}

	legacy, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "legacy-classification", Title: "Legacy", MediaType: "application/epub+zip", ContentHash: "sha256:legacy-classification", FullText: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetEPUBUnitClassifications(ctx, alice.ID, legacy.ID, "deterministic", "1"); !errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		t.Fatalf("legacy classification availability: %v", err)
	}
}

func classificationTestUnits() domain.ExtractedUnits {
	return domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "one"), Order: 0, SpineIndex: 0, ManifestID: "one", Text: "One", EndOffset: 3},
		{ID: domain.EPUBUnitID(1, "two"), Order: 1, SpineIndex: 1, ManifestID: "two", Text: "Two", StartOffset: 5, EndOffset: 8},
	}}
}

func classificationTestResults(snapshotID string, units domain.ExtractedUnits) []domain.EPUBUnitClassification {
	result := make([]domain.EPUBUnitClassification, len(units.Units))
	for i, unit := range units.Units {
		result[i] = domain.EPUBUnitClassification{SchemaVersion: 1, Classifier: domain.EPUBClassifierIdentity{Name: "deterministic", Version: "1"}, SourceUnitSnapshot: domain.EPUBSourceUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1, UnitID: unit.ID}, Category: domain.EPUBCategoryUnknown, Confidence: 20, Reasons: []domain.EPUBClassificationReason{{Signal: "first", Message: "First ordered reason."}, {Signal: "second", Message: "Second ordered reason."}}, RecommendedInclusion: i == 0}
	}
	return result
}
