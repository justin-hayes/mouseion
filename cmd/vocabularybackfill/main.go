// Command vocabularybackfill applies the active German canonicalization
// profile to mutable learner vocabulary projections. Run it explicitly after
// deploying the profile change and resolve any reported owner conflicts.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (err error) {
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MOUSEION_DATABASE_URL is required")
	}
	store, err := persistence.Open(context.Background(), databaseURL)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	report, err := store.BackfillGermanVocabulary(context.Background())
	if err != nil {
		return err
	}
	for _, conflict := range report.Conflicts {
		log.Printf("unresolved German vocabulary conflict: owner=%s table=%s lemma=%s upos=%s detail=%s", conflict.OwnerID, conflict.Table, conflict.CanonicalLemma, conflict.UPOS, conflict.Detail)
	}
	if _, err = fmt.Fprintf(os.Stdout, "German vocabulary backfill complete: owners=%d examined=%d updated=%d merged=%d conflicts=%d\n", report.Owners, report.Examined, report.Updated, report.Merged, len(report.Conflicts)); err != nil {
		return err
	}
	if len(report.Conflicts) > 0 {
		return errors.New("German vocabulary backfill found unresolved conflicts")
	}
	return nil
}
