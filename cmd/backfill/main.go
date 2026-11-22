// Command backfill runs the data-only My Books backfills that need runtime
// state. It currently removes the retired Inbox memberships once every OPDS
// connection's Catalog index has completed a sync.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// backfillStore is the durable surface the command needs.
type backfillStore interface {
	BackfillChosenBooks(context.Context) (domain.MyBooksBackfillReport, error)
	Close() error
}

// openStore opens the configured database. Tests replace it with a fake.
type openStore func(context.Context, string) (backfillStore, error)

func main() {
	open := func(ctx context.Context, databaseURL string) (backfillStore, error) {
		return persistence.Open(ctx, databaseURL)
	}
	if err := run(context.Background(), os.Getenv, os.Stdout, open); err != nil {
		log.Fatal(err)
	}
}

// run applies the backfill, reports its result, and is idempotent so an
// operator can safely retry it.
func run(ctx context.Context, getenv func(string) string, output io.Writer, open openStore) error {
	databaseURL := getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MOUSEION_DATABASE_URL is required")
	}
	store, err := open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Printf("close database: %v", closeErr)
		}
	}()
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	report, err := store.BackfillChosenBooks(runCtx)
	if errors.Is(err, persistence.ErrMyBooksBackfillBlocked) {
		return errors.New("refusing to run: every catalog connection must complete a sync first")
	}
	if err != nil {
		return fmt.Errorf("backfill chosen books: %w", err)
	}
	if _, err := fmt.Fprintf(output, "removed %d unchosen Books from My Books\n", report.Removed); err != nil {
		return err
	}
	for _, book := range report.LanguageLessWithHistory {
		if _, err := fmt.Fprintf(output, "removed %s (%q) from My Books: no chosen language; its reading history is kept, and adding it again with a language restores it\n", book.ID, book.Title); err != nil {
			return err
		}
	}
	return nil
}
