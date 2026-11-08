package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// idiomSampleSource reads confirmed idiom cards from one Book's prepared deck.
// It is an interface so the command tests never need PostgreSQL.
type idiomSampleSource interface {
	ListConfirmedIdiomCardsForBook(context.Context, string, string) ([]persistence.ConfirmedIdiomCard, error)
	Close() error
}

type idiomSampleSourceOpener func(context.Context) (idiomSampleSource, error)

// openIdiomSampleSource opens the installation database only when a sample was
// requested.
func openIdiomSampleSource(ctx context.Context) (idiomSampleSource, error) {
	databaseURL := strings.TrimSpace(os.Getenv("MOUSEION_DATABASE_URL"))
	if databaseURL == "" {
		return nil, errors.New("glossreview: MOUSEION_DATABASE_URL is required when -book is set")
	}
	return persistence.Open(ctx, databaseURL)
}

// executeIdiomCommand runs the idiom-card review and, when a Book is named,
// appends the deterministic sample of confirmed cards.
func executeIdiomCommand(ctx context.Context, options reviewCommandOptions, output io.Writer, open reviewEnvironmentOpener, openSample idiomSampleSourceOpener) (err error) {
	environment, err := open()
	if err != nil {
		return err
	}
	cases, err := loadIdiomCaseSet(bytes.NewReader(idiomReviewCases))
	if err != nil {
		return err
	}
	sample := idiomSampleOptions{
		Requested: options.BookID != "",
		BookID:    options.BookID,
		Language:  options.Language,
		Seed:      options.Seed,
	}
	if sample.Requested {
		source, sourceErr := openSample(ctx)
		if sourceErr != nil {
			return sourceErr
		}
		defer func() { err = errors.Join(err, source.Close()) }()
		sample.Cards, err = source.ListConfirmedIdiomCardsForBook(ctx, options.BookID, options.Language)
		if err != nil {
			return err
		}
	}
	report := reviewIdiom(ctx, idiomReviewOptions{
		Now:            options.Now,
		Cases:          cases,
		Client:         environment.Client,
		Provider:       environment.Provider,
		MatcherVersion: idiom.MatcherVersion,
		GapBound:       idiom.DefaultGapBound,
		Sample:         sample,
	})
	path := options.OutPath
	if path == "" {
		path = filepath.Join("doc", "reviews", fmt.Sprintf("idiom-review-%s.md", options.Now.Format("2006-01-02")))
	}
	return writeReviewFile(output, path, func(writer io.Writer) error { return writeIdiomReport(writer, report) },
		fmt.Sprintf("idiom review report written: path=%s cases=%d sample=%d\n", path, len(report.Cases), len(report.Sample.Cards)))
}
