//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestOpdsConnectionCRUDEncryptionAndOwnership(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	ctx := context.Background()
	databaseURL := integrationURL()
	lockIntegrationDatabase(t, ctx, databaseURL)
	resetDatabase(t, ctx, databaseURL)
	if err := Migrate(databaseURL); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "opds-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "opds-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateOpdsConnection(ctx, domain.OpdsConnection{OwnerID: alice.ID, Name: "Home library", URL: "https://books.example/opds", Username: "reader", Password: "plain-password-must-not-be-stored", Language: "de"})
	if err != nil {
		t.Fatal(err)
	}
	if created.OwnerID != alice.ID || created.Password != "plain-password-must-not-be-stored" {
		t.Fatalf("created=%+v", created)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
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
	if _, err = store.GetOpdsConnection(ctx, bob.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Bob read Alice connection: %v", err)
	}
	if listed, err := store.ListOpdsConnections(ctx, bob.ID); err != nil || len(listed) != 0 {
		t.Fatalf("Bob list=%+v err=%v", listed, err)
	}
	created.Name = "Updated"
	created.URL = "https://books.example/new-opds"
	created.Password = "new-password"
	updated, err := store.UpdateOpdsConnection(ctx, alice.ID, created)
	if err != nil || updated.Name != "Updated" || updated.Password != "new-password" || !updated.UpdatedAt.After(updated.CreatedAt) {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	created.Name = "Stolen"
	if _, err = store.UpdateOpdsConnection(ctx, bob.ID, created); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Bob updated Alice connection: %v", err)
	}
	if err = store.DeleteOpdsConnection(ctx, bob.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Bob deleted Alice connection: %v", err)
	}
	if err = store.DeleteOpdsConnection(ctx, alice.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetOpdsConnection(ctx, alice.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted connection read: %v", err)
	}
}
