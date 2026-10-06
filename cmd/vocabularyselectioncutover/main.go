// Command vocabularyselectioncutover removes the retired, saved Browse
// selections after the operator has captured and verified a pre-cutover backup.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
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
	apply := flag.Bool("apply", false, "apply the destructive data-only cutover (requires a verified pre-cutover backup)")
	flag.Parse()
	if !*apply {
		return errors.New("refusing to run without --apply; first capture and verify a pre-cutover backup (see doc/cutovers/2026-10-06-vocabulary-browse-selections.md)")
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
	report, err := store.CutoverVocabularyBrowseSelections(context.Background())
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write cutover report: %w", err)
	}
	return nil
}
