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
	units := domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "one"), Order: 0, SpineIndex: 0, ManifestID: "one", Text: "One", EndOffset: 3},
		{ID: domain.EPUBUnitID(1, "two"), Order: 1, SpineIndex: 1, ManifestID: "two", Text: "Two", StartOffset: 5, EndOffset: 8},
	}}
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "scope-book", Title: "Scope", MediaType: "application/epub+zip", ContentHash: "ignored", Content: []byte("One\n\nTwo"), FullText: "One\n\nTwo"}, units)
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.EPUBReviewedScopeSnapshot{SchemaVersion: 1, ScopeID: uuid.NewString(), OwnerID: alice.ID, SourceMaterialID: source.ID, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1}, SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: units.Units[1].ID, Order: 1}}}
	created, err := store.CreateEPUBReviewedScope(ctx, scope)
	if err != nil || created.CreatedAt.IsZero() {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	retried := scope
	retried.ScopeID = uuid.NewString()
	retried, err = store.CreateEPUBReviewedScope(ctx, retried)
	if err != nil || retried.ScopeID == scope.ScopeID {
		t.Fatalf("equivalent scope confirmation reused scope: %+v err=%v", retried, err)
	}
	loaded, err := store.GetEPUBReviewedScope(ctx, alice.ID, source.ID, scope.ScopeID)
	if err != nil || loaded.ScopeID != scope.ScopeID || len(loaded.SelectedUnits) != 1 || loaded.SelectedUnits[0] != scope.SelectedUnits[0] {
		t.Fatalf("loaded scope=%+v err=%v", loaded, err)
	}
	if _, err = store.GetEPUBReviewedScope(ctx, bob.ID, source.ID, scope.ScopeID); err == nil {
		t.Fatal("cross-owner scope read was accepted")
	}
	metadata := source
	metadata.Title = "Metadata-only rename"
	metadata.Language = "de"
	if _, err = store.PutSourceMaterial(ctx, metadata); err != nil {
		t.Fatalf("metadata-only update: %v", err)
	}
	if _, err = store.GetEPUBReviewedScope(ctx, alice.ID, source.ID, scope.ScopeID); err != nil {
		t.Fatalf("metadata-only update invalidated scope: %v", err)
	}
	historicalScope, err := store.GetEPUBReviewedScope(ctx, alice.ID, source.ID, scope.ScopeID)
	if err != nil || historicalScope.SourceUnitSnapshot != scope.SourceUnitSnapshot || len(historicalScope.SelectedUnits) != len(scope.SelectedUnits) || historicalScope.SelectedUnits[0] != scope.SelectedUnits[0] {
		t.Fatalf("historical scope changed: got=%+v want=%+v err=%v", historicalScope, scope, err)
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
	if _, err = store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: source.SourceIdentifier, Title: "Scope", MediaType: source.MediaType, ContentHash: "replacement", Content: []byte("One\n\nTwo"), FullText: "One\n\nTwo"}, units); err != nil {
		t.Fatalf("reimport after immutable review: %v", err)
	}
	var revisions int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, source.ID).Scan(&revisions); err != nil || revisions != 1 {
		t.Fatalf("same-content reimport created revisions=%d err=%v", revisions, err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM epub_reviewed_scopes WHERE scope_id=$1`, scope.ScopeID).Scan(&selected); err != nil || selected != 1 {
		t.Fatalf("historical scope count=%d err=%v", selected, err)
	}
	changedUnits := domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "changed"), Order: 0, SpineIndex: 0, ManifestID: "changed", Text: "Changed", EndOffset: 7}}}
	if _, err = store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: source.SourceIdentifier, Title: "Renamed Scope", MediaType: source.MediaType, ContentHash: "forged", Content: []byte("Changed"), FullText: "Changed"}, changedUnits); err != nil {
		t.Fatalf("changed-content reimport: %v", err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, source.ID).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatalf("changed-content revisions=%d err=%v", revisions, err)
	}
	if _, err = store.GetEPUBReviewedScope(ctx, alice.ID, source.ID, scope.ScopeID); err != nil {
		t.Fatalf("historical scope became unreadable after content revision: %v", err)
	}
}
