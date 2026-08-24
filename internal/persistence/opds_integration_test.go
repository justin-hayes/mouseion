//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestOpdsConnectionCRUDEncryptionAndOwnerIsolation(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	ctx := context.Background()
	databaseURL := integrationDatabase(t, ctx)
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "bob", false)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Home library", URL: "https://books.example/opds", Username: "reader", Password: "plain-password-must-not-be-stored"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Password != "plain-password-must-not-be-stored" {
		t.Fatalf("created=%+v", created)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var legacyID string
	if err = pool.QueryRow(ctx, `INSERT INTO opds_connections(name,url,language) VALUES('Preserved legacy catalog','https://legacy.example/opds','de') RETURNING id`).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if listed, listErr := store.ListOpdsConnections(ctx, alice.ID); listErr != nil || len(listed) != 1 {
		t.Fatalf("legacy row exposed to owner: %+v err=%v", listed, listErr)
	}
	var legacyStillExists bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM opds_connections WHERE id=$1 AND owner_id IS NULL)`, legacyID).Scan(&legacyStillExists); err != nil || !legacyStillExists {
		t.Fatalf("legacy row was not preserved: exists=%v err=%v", legacyStillExists, err)
	}
	var encrypted []byte
	if err = pool.QueryRow(ctx, `SELECT password_encrypted FROM opds_connections WHERE id=$1`, created.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if string(encrypted) == created.Password || len(encrypted) <= len(created.Password) {
		t.Fatalf("credential not encrypted: %q", encrypted)
	}
	got, err := store.GetOpdsConnection(ctx, alice.ID, created.ID)
	if err != nil || got.Password != created.Password {
		t.Fatalf("round trip=%+v err=%v", got, err)
	}
	if listed, err := store.ListOpdsConnections(ctx, alice.ID); err != nil || len(listed) != 1 || listed[0].ID != created.ID || listed[0].OwnerID != alice.ID {
		t.Fatalf("owner list=%+v err=%v", listed, err)
	}
	if listed, err := store.ListOpdsConnections(ctx, bob.ID); err != nil || len(listed) != 0 {
		t.Fatalf("cross-owner list=%+v err=%v", listed, err)
	}
	if _, err = store.GetOpdsConnection(ctx, bob.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner read: %v", err)
	}
	created.Name = "Updated"
	created.URL = "https://books.example/new-opds"
	created.Password = "new-password"
	if _, err = store.UpdateOpdsConnection(ctx, bob.ID, created); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner update: %v", err)
	}
	if err = store.DeleteOpdsConnection(ctx, bob.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner delete: %v", err)
	}
	updated, err := store.UpdateOpdsConnection(ctx, alice.ID, created)
	if err != nil || updated.Name != "Updated" || updated.Password != "new-password" || !updated.UpdatedAt.After(updated.CreatedAt) {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if err = store.DeleteOpdsConnection(ctx, alice.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetOpdsConnection(ctx, alice.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted connection read: %v", err)
	}
}
