// Command greekregister registers a compiled classical Reading unit for one
// named learner. It creates the learner's unprepared Book and ensures the
// language appears in the drop-down; importing the pinned annotation is a
// separate step.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const usage = "usage: greekregister <learner-username>"

// registrationStore is the persistence the command needs, plus the lifecycle
// run closes when it finishes so tests can supply a double.
type registrationStore interface {
	ancientgreek.Store
	Close() error
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, openStore); err != nil {
		log.Fatal(err)
	}
}

func openStore(ctx context.Context, databaseURL string) (registrationStore, error) {
	return persistence.Open(ctx, databaseURL)
}

// run registers the Ancient Greek Book for one learner. It prints a report of
// codes and identifiers only, never any stored text.
func run(ctx context.Context, args []string, output io.Writer, open func(context.Context, string) (registrationStore, error)) (err error) {
	if len(args) != 1 {
		return errors.New(usage)
	}
	username := strings.TrimSpace(args[0])
	if username == "" {
		return errors.New(usage)
	}
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MOUSEION_DATABASE_URL is required")
	}
	store, err := open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	report, err := ancientgreek.NewService(store).Register(ctx, username, ancientgreek.AnabasisBook1)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "ancient greek registration complete: learner=%s book=%s created=%t\n", report.Username, report.BookID, report.Created)
	return err
}
