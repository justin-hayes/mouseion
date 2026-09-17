// Command cataloguebackfill assigns legacy catalogue-entry aliases to their
// owning OPDS connection. Run it explicitly after the expand migration.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (err error) {
	if err := persistence.ValidateSecret(os.Getenv("MOUSEION_SECRET")); err != nil {
		return err
	}
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MOUSEION_DATABASE_URL is required")
	}
	store, err := persistence.Open(context.Background(), databaseURL)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	service := cataloguesync.NewService(store, nil, opds.NewService(store, nil, nil), nil)
	result, err := service.BackfillCatalogueEntryAliases(context.Background())
	if err != nil {
		return err
	}
	if _, err = store.Pool().Exec(context.Background(), `ALTER TABLE book_aliases VALIDATE CONSTRAINT book_aliases_connection_contract`); err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "catalogue alias backfill complete: examined=%d updated=%d\n", result.Examined, result.Updated)
	return err
}
