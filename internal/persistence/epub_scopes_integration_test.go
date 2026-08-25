//go:build integration

package persistence

import (
	"context"
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
