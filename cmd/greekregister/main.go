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
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const usage = "usage: greekregister <learner-username> [pinned-source-file] [brownson-translation-file]"

// registrationStore is the persistence the command needs, plus the lifecycle
// run closes when it finishes so tests can supply a double.
type registrationStore interface {
	ancientgreek.Store
	Close() error
}

// pinnedSources names the reviewed annotation definition and the Brownson
// reference source the operator imports. They are compiled data supplied by
// main and injected in tests.
type pinnedSources struct {
	Annotation  domain.ReadingUnitDefinition
	Translation ancientgreek.BrownsonSource
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, openStore, storeImportWriter, storeReferenceWriter); err != nil {
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

// storeReferenceWriter builds the Brownson translation writer from the
// PostgreSQL store.
func storeReferenceWriter(store registrationStore) (ancientgreek.BrownsonImportWriter, error) {
	postgres, ok := store.(*persistence.PostgresStore)
	if !ok {
		return nil, errors.New("greekregister: the import requires the PostgreSQL store")
	}
	return analysis.NewReferenceTranslationWriter(postgres), nil
}

// run registers the Ancient Greek Book for one learner and, when pinned source
// files are named, imports the annotation and the Brownson reference
// translation. It prints reports of codes and counts only, never any stored
// text.
func run(ctx context.Context, args []string, output io.Writer, open func(context.Context, string) (registrationStore, error), newWriter func(registrationStore) (ancientgreek.ImportWriter, error), newReferenceWriter func(registrationStore) (ancientgreek.BrownsonImportWriter, error)) (err error) {
	if len(args) < 1 || len(args) > 3 {
		return errors.New(usage)
	}
	username := strings.TrimSpace(args[0])
	if username == "" {
		return errors.New(usage)
	}
	sourcePath := ""
	if len(args) >= 2 {
		sourcePath = strings.TrimSpace(args[1])
		if sourcePath == "" {
			return errors.New(usage)
		}
	}
	brownsonPath := ""
	if len(args) == 3 {
		brownsonPath = strings.TrimSpace(args[2])
		if brownsonPath == "" {
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
	return importPinnedSources(ctx, store, report, sourcePath, brownsonPath, pinnedSources{Annotation: ancientgreek.AnabasisBook1, Translation: ancientgreek.BrownsonAnabasisBook1}, output, newWriter, newReferenceWriter)
}

// importPinnedSources imports the pinned annotation and, when named, the pinned
// Brownson translation. It prints reports of codes and counts only.
func importPinnedSources(ctx context.Context, store registrationStore, report ancientgreek.Report, sourcePath, brownsonPath string, sources pinnedSources, output io.Writer, newWriter func(registrationStore) (ancientgreek.ImportWriter, error), newReferenceWriter func(registrationStore) (ancientgreek.BrownsonImportWriter, error)) error {
	writer, err := newWriter(store)
	if err != nil {
		return err
	}
	importReport, err := ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID:    report.OwnerID,
		BookID:     report.BookID,
		SourcePath: sourcePath,
		Definition: sources.Annotation,
	}, writer)
	if err != nil {
		return err
	}
	if err := writeImportReport(output, importReport); err != nil {
		return err
	}
	if brownsonPath == "" {
		return nil
	}
	referenceWriter, err := newReferenceWriter(store)
	if err != nil {
		return err
	}
	translationReport, err := ancientgreek.ImportBrownson(ctx, ancientgreek.BrownsonImportRequest{
		OwnerID:    report.OwnerID,
		SourcePath: brownsonPath,
		Source:     sources.Translation,
	}, referenceWriter)
	if err != nil {
		return err
	}
	return writeTranslationReport(output, translationReport)
}

// writeTranslationReport prints the Brownson import's pin and counts, never any
// source text.
func writeTranslationReport(output io.Writer, report ancientgreek.BrownsonImportReport) error {
	_, err := fmt.Fprintf(output, "ancient greek translation import complete: pin=%s sections=%d bytes=%d\n", report.SourcePin, report.Sections, report.Bytes)
	return err
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
