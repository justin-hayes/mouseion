// Command vocabularybackfill applies the active German canonicalization
// profile to mutable learner vocabulary projections. Run it explicitly after
// deploying the profile change and resolve any reported owner conflicts.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

func main() {
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("MOUSEION_DATABASE_URL is required")
	}
	store, err := persistence.Open(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	report, err := store.BackfillGermanVocabulary(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for _, conflict := range report.Conflicts {
		log.Printf("unresolved German vocabulary conflict: owner=%s table=%s lemma=%s upos=%s detail=%s", conflict.OwnerID, conflict.Table, conflict.CanonicalLemma, conflict.UPOS, conflict.Detail)
	}
	_, _ = fmt.Fprintf(os.Stdout, "German vocabulary backfill complete: owners=%d examined=%d updated=%d merged=%d conflicts=%d\n", report.Owners, report.Examined, report.Updated, report.Merged, len(report.Conflicts))
	if len(report.Conflicts) > 0 {
		os.Exit(1)
	}
}
