// Command greekregister registers a compiled classical Reading unit for one
// named learner. Given the pinned source file, it also imports the annotation
// into an ordinary analysis run. It never calls the NLP service.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const usage = "usage: greekregister <learner-username> [pinned-source-file]"

// registrationStore is the persistence the command needs, plus the lifecycle
// run closes when it finishes so tests can supply a double.
type registrationStore interface {
	ancientgreek.Store
	Close() error
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, openStore, storeImportWriter); err != nil {
		log.Fatal(err)
	}
}

func openStore(ctx context.Context, databaseURL string) (registrationStore, error) {
	return persistence.Open(ctx, databaseURL)
}

// storeImportWriter builds the import writer from the PostgreSQL store.
func storeImportWriter(store registrationStore) (ancientgreek.ImportWriter, error) {
	postgres, ok := store.(*persistence.PostgresStore)
	if !ok {
		return nil, errors.New("greekregister: the import requires the PostgreSQL store")
	}
	return analysis.NewGreekImportWriter(postgres), nil
}

// run registers the Ancient Greek Book for one learner and, when a pinned
// source file is named, imports its annotation. It prints reports of codes and
// counts only, never any stored text.
func run(ctx context.Context, args []string, output io.Writer, open func(context.Context, string) (registrationStore, error), newWriter func(registrationStore) (ancientgreek.ImportWriter, error)) (err error) {
	if len(args) < 1 || len(args) > 2 {
		return errors.New(usage)
	}
	username := strings.TrimSpace(args[0])
	if username == "" {
		return errors.New(usage)
	}
	sourcePath := ""
	if len(args) == 2 {
		sourcePath = strings.TrimSpace(args[1])
		if sourcePath == "" {
			return errors.New(usage)
		}
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
	if _, err = fmt.Fprintf(output, "ancient greek registration complete: learner=%s book=%s created=%t\n", report.Username, report.BookID, report.Created); err != nil {
		return err
	}
	if sourcePath == "" {
		return nil
	}
	writer, err := newWriter(store)
	if err != nil {
		return err
	}
	importReport, err := ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID:    report.OwnerID,
		BookID:     report.BookID,
		SourcePath: sourcePath,
		Definition: ancientgreek.AnabasisBook1,
	}, writer)
	if err != nil {
		return err
	}
	return writeImportReport(output, importReport)
}

// writeImportReport prints the import's codes and counts, never any source text.
func writeImportReport(output io.Writer, report ancientgreek.ImportReport) error {
	if _, err := fmt.Fprintf(output, "ancient greek import complete: book=%s sentences=%d tokens=%d ellipsis_skipped=%d proper_names=%d numbered_lemmas=%d bare_numbered_lemmas=%d reimported=%t\n",
		report.BookID, report.Sentences, report.Tokens, report.EllipsisTokensSkipped, report.ProperNamesDerived, report.NumberedLemmaTypes, report.BareLemmasWithNumberedSibling, report.Reimported); err != nil {
		return err
	}
	for _, count := range report.UnmappedFeatureValues {
		if _, err := fmt.Fprintf(output, "ancient greek import unmapped: feature=%s value=%s count=%d\n", count.Feature, count.Value, count.Count); err != nil {
			return err
		}
	}
	return nil
}
