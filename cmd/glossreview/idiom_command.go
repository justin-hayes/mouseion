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

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
)

// idiomSampleSource re-freezes one Book's current deck preparation in memory
// and returns its idiom items. It is an interface so the command tests never
// need PostgreSQL or a dictionary index.
type idiomSampleSource interface {
	FreezeIdiomReviewItems(ctx context.Context, bookID, language string) (prepareddeck.IdiomReviewFreeze, error)
	Close() error
}

type idiomSampleSourceOpener func(context.Context, *enrichment.TranslationCodec) (idiomSampleSource, error)

// databaseIdiomSampleSource is the installation database and dictionary index
// one sample reads. Neither is written.
type databaseIdiomSampleSource struct {
	prepareddeck.IdiomReviewFreezer
	index *dictionary.Index
}

func (s databaseIdiomSampleSource) Close() error {
	return errors.Join(s.Store.Close(), s.index.Close())
}

// openIdiomSampleSource opens the installation database and dictionary index
// only when a sample was requested. An index without the idiom lookup tables
// can match no idiom, so it is refused rather than reported as an empty sample.
func openIdiomSampleSource(ctx context.Context, codec *enrichment.TranslationCodec) (idiomSampleSource, error) {
	databaseURL := strings.TrimSpace(os.Getenv("MOUSEION_DATABASE_URL"))
	if databaseURL == "" {
		return nil, errors.New("glossreview: MOUSEION_DATABASE_URL is required when -book is set")
	}
	indexPath := strings.TrimSpace(os.Getenv("MOUSEION_DICTIONARY_INDEX"))
	if indexPath == "" {
		return nil, errors.New("glossreview: MOUSEION_DICTIONARY_INDEX is required when -book is set")
	}
	if codec == nil {
		return nil, errors.New("glossreview: the idiom sample needs the configured provider codec")
	}
	index, err := dictionary.OpenIndex(ctx, indexPath)
	if err != nil {
		return nil, fmt.Errorf("glossreview: open dictionary index %s: %w", indexPath, err)
	}
	if !index.HasIdiomEntries() {
		return nil, errors.Join(fmt.Errorf("glossreview: dictionary index %s has no idiom lookup tables; rebuild it with the current derivation", indexPath), index.Close())
	}
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		return nil, errors.Join(err, index.Close())
	}
	return databaseIdiomSampleSource{Store: store, Presentation: cardexport.NewPresentation(index), Codec: codec, index: index}, nil
}

// executeIdiomCommand runs the idiom-card review and, when a Book is named,
// appends the deterministic sample from an in-memory freeze of the Book.
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
		source, sourceErr := openSample(ctx, environment.Codec)
		if sourceErr != nil {
			return sourceErr
		}
		defer func() { err = errors.Join(err, source.Close()) }()
		freeze, freezeErr := source.FreezeIdiomReviewItems(ctx, options.BookID, options.Language)
		if freezeErr != nil {
			return freezeErr
		}
		sample.Items, sample.Ranking = freeze.Items, freeze.Ranking
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
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("glossreview: idiom review interrupted; no report written: %w", err)
	}
	path := options.OutPath
	if path == "" {
		path = filepath.Join("doc", "reviews", fmt.Sprintf("idiom-review-%s.md", options.Now.Format("2006-01-02")))
	}
	return writeReviewFile(output, path, func(writer io.Writer) error { return writeIdiomReport(writer, report) },
		fmt.Sprintf("idiom review report written: path=%s cases=%d sample=%d\n", path, len(report.Cases), len(report.Sample.Cards)))
}
