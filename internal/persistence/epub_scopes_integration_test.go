//go:build integration

package persistence

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestReviewedScopePersistenceIsOwnerScopedAndImmutable(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, _ := store.CreateUser(ctx, "scope-alice", false)
	bob, _ := store.CreateUser(ctx, "scope-bob", false)
	units := classificationTestUnits()
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "scope-book", Title: "Scope", MediaType: "application/epub+zip", ContentHash: "scope", FullText: "One\n\nTwo"}, units)
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.EPUBReviewedScopeSnapshot{SchemaVersion: 1, ScopeID: uuid.NewString(), OwnerID: alice.ID, SourceMaterialID: source.ID, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1}, Classifier: domain.EPUBClassifierIdentity{Name: "deterministic", Version: "1"}, SelectionMode: domain.EPUBScopeSelectionOverridden, SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: units.Units[1].ID, Order: 1}}}
	created, err := store.CreateEPUBReviewedScope(ctx, scope)
	if err != nil || created.CreatedAt.IsZero() {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	if _, err = store.CreateEPUBReviewedScope(ctx, scope); err == nil {
		t.Fatal("immutable scope identity was overwritten")
	}
	loaded, err := store.GetEPUBReviewedScope(ctx, alice.ID, source.ID, scope.ScopeID)
	if err != nil || loaded.ScopeID != scope.ScopeID || len(loaded.SelectedUnits) != 1 || loaded.SelectedUnits[0] != scope.SelectedUnits[0] {
		t.Fatalf("loaded scope=%+v err=%v", loaded, err)
	}
	if _, err = store.GetEPUBReviewedScope(ctx, bob.ID, source.ID, scope.ScopeID); err == nil {
		t.Fatal("cross-owner scope read was accepted")
	}
	classificationsV1 := classificationTestResults(snapshotID, units)
	if err = store.ReplaceEPUBUnitClassifications(ctx, alice.ID, source.ID, classificationsV1); err != nil {
		t.Fatal(err)
	}
	classificationsV2 := classificationTestResults(snapshotID, units)
	for i := range classificationsV2 {
		classificationsV2[i].Classifier.Version = "2"
		classificationsV2[i].Reasons = []domain.EPUBClassificationReason{{Signal: "refined", Message: "A refined deterministic rule matched."}}
		classificationsV2[i].Category = domain.EPUBCategoryMainMatter
		classificationsV2[i].Confidence = 95
		classificationsV2[i].RecommendedInclusion = true
	}
	if err = store.ReplaceEPUBUnitClassifications(ctx, alice.ID, source.ID, classificationsV2); err != nil {
		t.Fatal(err)
	}
	historicalClassifications, err := store.GetEPUBUnitClassifications(ctx, alice.ID, source.ID, "deterministic", "1")
	if err != nil {
		t.Fatal(err)
	}
	for i := range historicalClassifications {
		historicalClassifications[i].CreatedAt = classificationsV1[i].CreatedAt
		historicalClassifications[i].UpdatedAt = classificationsV1[i].UpdatedAt
	}
	if !reflect.DeepEqual(historicalClassifications, classificationsV1) {
		t.Fatalf("classifier refinement mutated historical classifications: got=%+v want=%+v", historicalClassifications, classificationsV1)
	}
	historicalScope, err := store.GetEPUBReviewedScope(ctx, alice.ID, source.ID, scope.ScopeID)
	if err != nil || historicalScope.Classifier != scope.Classifier || historicalScope.SourceUnitSnapshot != scope.SourceUnitSnapshot || !reflect.DeepEqual(historicalScope.SelectedUnits, scope.SelectedUnits) {
		t.Fatalf("classifier refinement mutated historical scope: got=%+v want=%+v err=%v", historicalScope, scope, err)
	}
	foreign := scope
	foreign.ScopeID = uuid.NewString()
	foreign.OwnerID = bob.ID
	if _, err = store.CreateEPUBReviewedScope(ctx, foreign); err == nil {
		t.Fatal("cross-owner scope was accepted")
	}
	var selected int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM epub_reviewed_scope_units WHERE scope_id=$1`, scope.ScopeID).Scan(&selected); err != nil || selected != 1 {
		t.Fatalf("selected=%d err=%v", selected, err)
	}
	if _, err = store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: source.SourceIdentifier, Title: "Scope", MediaType: source.MediaType, ContentHash: "replacement", FullText: "One\n\nTwo"}, units); err != nil {
		t.Fatalf("reimport after immutable review: %v", err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM epub_reviewed_scopes WHERE scope_id=$1`, scope.ScopeID).Scan(&selected); err != nil || selected != 1 {
		t.Fatalf("historical scope count=%d err=%v", selected, err)
	}
}
