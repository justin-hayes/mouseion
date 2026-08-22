//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestOpdsConnectionCRUDEncryptionAndSharedAccess(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "integration-test-secret-with-sufficient-entropy")
	ctx := context.Background()
	databaseURL := integrationDatabase(t, ctx)
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	created, err := store.CreateOpdsConnection(ctx, domain.OpdsConnection{Name: "Home library", URL: "https://books.example/opds", Username: "reader", Password: "plain-password-must-not-be-stored", Language: "de"})
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
	var encrypted []byte
	if err = pool.QueryRow(ctx, `SELECT password_encrypted FROM opds_connections WHERE id=$1`, created.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if string(encrypted) == created.Password || len(encrypted) <= len(created.Password) {
		t.Fatalf("credential not encrypted: %q", encrypted)
	}
	got, err := store.GetOpdsConnection(ctx, created.ID)
	if err != nil || got.Password != created.Password {
		t.Fatalf("round trip=%+v err=%v", got, err)
	}
	if listed, err := store.ListOpdsConnections(ctx); err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("shared list=%+v err=%v", listed, err)
	}
	created.Name = "Updated"
	created.URL = "https://books.example/new-opds"
	created.Password = "new-password"
	updated, err := store.UpdateOpdsConnection(ctx, created)
	if err != nil || updated.Name != "Updated" || updated.Password != "new-password" || !updated.UpdatedAt.After(updated.CreatedAt) {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if err = store.DeleteOpdsConnection(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetOpdsConnection(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted connection read: %v", err)
	}
}
