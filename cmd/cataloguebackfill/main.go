// Command cataloguebackfill assigns legacy catalogue-entry aliases to their
// owning OPDS connection. Run it explicitly after the expand migration.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func main() {
	if err := persistence.ValidateSecret(os.Getenv("MOUSEION_SECRET")); err != nil {
		log.Fatal(err)
	}
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("MOUSEION_DATABASE_URL is required")
	}
	store, err := persistence.Open(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	service := cataloguesync.NewService(store, nil, opds.NewService(store, nil, nil), nil)
	result, err := service.BackfillCatalogueEntryAliases(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	_, _ = fmt.Fprintf(os.Stdout, "catalogue alias backfill complete: examined=%d updated=%d\n", result.Examined, result.Updated)
}
