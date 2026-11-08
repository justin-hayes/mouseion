// Command glossreview runs a frozen contextual-Gloss or idiom-card review case
// set through the real codec and the configured OpenAI-compatible provider. It
// writes a dated Markdown report under doc/reviews/ and never runs in CI.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

const usage = "usage: glossreview [-review gloss|idiom] [-out <report.md>] [-date YYYY-MM-DD] [-book <book-id>] [-language <code>] [-seed <value>]"

// glossReviewCases and idiomReviewCases are the versioned repository case sets.
// Embedding them keeps a review tied to the code that produced it and makes the
// command independent of the caller's working directory outside the repository.
//
//go:embed testdata/gloss_review_cases.json
var glossReviewCases []byte

//go:embed testdata/idiom_review_cases.json
var idiomReviewCases []byte

// reviewKind selects the frozen case set and provider contract one run uses.
type reviewKind string

const (
	reviewKindGloss reviewKind = "gloss"
	reviewKindIdiom reviewKind = "idiom"
)

// reviewCommandOptions is the parsed command line one review run uses.
type reviewCommandOptions struct {
	Kind     reviewKind
	Now      time.Time
	OutPath  string
	BookID   string
	Language string
	Seed     string
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// reviewEnvironment is the configured provider and the metadata the report
// pins. Tests inject a real client whose HTTP transport is stubbed, so the
// command path never reaches the network in CI. Codec is the configured codec
// the idiom sample freezes with; it is nil when a test supplies the sample.
type reviewEnvironment struct {
	Client   translationClient
	Provider providerMetadata
	Codec    *enrichment.TranslationCodec
}

type reviewEnvironmentOpener func() (reviewEnvironment, error)

// run parses the command and writes the report. It refuses to run when the
// deployed provider is not configured.
func run(ctx context.Context, args []string, output io.Writer) error {
	options, err := parseReviewCommand(args)
	if err != nil {
		return err
	}
	if options.Kind == reviewKindIdiom {
		return executeIdiomCommand(ctx, options, output, openIdiomReviewEnvironment, openIdiomSampleSource)
	}
	return executeGlossCommand(ctx, options, output, openReviewEnvironment)
}

// openReviewEnvironment reads the provider configuration and builds the real
// contextual Gloss client. It is separate from run so a test can supply a stub
// without touching the environment or the network.
func openReviewEnvironment() (reviewEnvironment, error) {
	cfg, codec, client, err := openProvider()
	if err != nil {
		return reviewEnvironment{}, err
	}
	return newReviewEnvironment(cfg.Model, cfg.ReasoningEffort, codec, client, codec.ContextualGlossProviderVersion(), codec.ContextualGlossPromptContract()), nil
}

// openIdiomReviewEnvironment reads the same configured provider and pins the
// idiom-card contract and its reasoning effort instead of the lemma Gloss
// contract.
func openIdiomReviewEnvironment() (reviewEnvironment, error) {
	cfg, codec, client, err := openProvider()
	if err != nil {
		return reviewEnvironment{}, err
	}
	return newReviewEnvironment(cfg.Model, codec.IdiomCardReasoningEffort(), codec, client, codec.IdiomCardProviderVersion(), codec.IdiomCardPromptContract()), nil
}

// newReviewEnvironment builds the client and pinned metadata one report records.
func newReviewEnvironment(model, reasoningEffort string, codec *enrichment.TranslationCodec, client *enrichment.OpenAITranslationClient, providerVersion, promptContract string) reviewEnvironment {
	return reviewEnvironment{
		Client: client,
		Codec:  codec,
		Provider: providerMetadata{
			Model:           model,
			ReasoningEffort: reasoningEffort,
			ProviderName:    codec.ProviderName(),
			ProviderVersion: providerVersion,
			PromptContract:  promptContract,
		},
	}
}

// openProvider builds the configured client and codec shared by both reviews.
func openProvider() (enrichment.LLMConfig, *enrichment.TranslationCodec, *enrichment.OpenAITranslationClient, error) {
	cfg, err := enrichment.LLMConfigFromEnv()
	if err != nil {
		return enrichment.LLMConfig{}, nil, nil, err
	}
	if !cfg.Enabled {
		return enrichment.LLMConfig{}, nil, nil, errors.New("glossreview: no provider is configured; set MOUSEION_LLM_ENABLED=true, MOUSEION_LLM_API_KEY and MOUSEION_LLM_MODEL, then rerun")
	}
	client, err := enrichment.NewOpenAITranslationClient(cfg, nil)
	if err != nil {
		return enrichment.LLMConfig{}, nil, nil, err
	}
	codec, err := enrichment.NewTranslationCodec(cfg)
	if err != nil {
		return enrichment.LLMConfig{}, nil, nil, err
	}
	return cfg, codec, client, nil
}

// runWithEnvironment is run for the contextual Gloss review with an injectable
// environment for tests.
func runWithEnvironment(ctx context.Context, args []string, output io.Writer, open reviewEnvironmentOpener) error {
	options, err := parseReviewCommand(args)
	if err != nil {
		return err
	}
	if options.Kind != reviewKindGloss {
		return errors.New("glossreview: runWithEnvironment only supports the gloss review")
	}
	return executeGlossCommand(ctx, options, output, open)
}

// parseReviewCommand parses the shared flags and resolves the dated report
// header. An explicit date keeps a re-rendered report reproducible; otherwise
// the command uses UTC today.
func parseReviewCommand(args []string) (reviewCommandOptions, error) {
	flags := flag.NewFlagSet("glossreview", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kindValue := flags.String("review", string(reviewKindGloss), "review to run: gloss or idiom")
	outPath := flags.String("out", "", "report path under doc/reviews (default: dated report for the chosen review)")
	dateValue := flags.String("date", "", "report date in YYYY-MM-DD (default: today)")
	bookID := flags.String("book", "", "Book ID whose current deck preparation is re-frozen in memory for the idiom sample")
	language := flags.String("language", "it", "language of the idiom sample")
	seed := flags.String("seed", "", "deterministic idiom sample seed (default: report date)")
	if err := flags.Parse(args); err != nil {
		return reviewCommandOptions{}, fmt.Errorf("%s: %w", usage, err)
	}
	if flags.NArg() != 0 {
		return reviewCommandOptions{}, errors.New(usage)
	}
	now, err := reportTime(*dateValue)
	if err != nil {
		return reviewCommandOptions{}, err
	}
	kind := reviewKind(strings.TrimSpace(*kindValue))
	if kind != reviewKindGloss && kind != reviewKindIdiom {
		return reviewCommandOptions{}, fmt.Errorf("%s: unknown review %q", usage, *kindValue)
	}
	options := reviewCommandOptions{
		Kind:     kind,
		Now:      now,
		OutPath:  strings.TrimSpace(*outPath),
		BookID:   strings.TrimSpace(*bookID),
		Language: strings.TrimSpace(*language),
		Seed:     strings.TrimSpace(*seed),
	}
	if options.Seed == "" {
		options.Seed = now.Format("2006-01-02")
	}
	if kind == reviewKindIdiom && options.BookID != "" && options.Language == "" {
		return reviewCommandOptions{}, errors.New("glossreview: -language is required with -book")
	}
	return options, nil
}

// executeGlossCommand runs the contextual Gloss review.
func executeGlossCommand(ctx context.Context, options reviewCommandOptions, output io.Writer, open reviewEnvironmentOpener) error {
	environment, err := open()
	if err != nil {
		return err
	}
	cases, err := loadCaseSet(bytes.NewReader(glossReviewCases))
	if err != nil {
		return err
	}
	report := review(ctx, reviewOptions{
		Now:      options.Now,
		Cases:    cases,
		Client:   environment.Client,
		Provider: environment.Provider,
	})
	path := options.OutPath
	if path == "" {
		path = filepath.Join("doc", "reviews", fmt.Sprintf("contextual-gloss-%s.md", options.Now.Format("2006-01-02")))
	}
	return writeReviewFile(output, path, func(writer io.Writer) error { return writeReport(writer, report) }, fmt.Sprintf("gloss review report written: path=%s cases=%d\n", path, len(report.Cases)))
}

// writeReviewFile renders one report to its dated path and prints the summary.
func writeReviewFile(output io.Writer, path string, render func(io.Writer) error, summary string) error {
	var rendered bytes.Buffer
	if err := render(&rendered); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, rendered.Bytes(), 0o600); err != nil {
		return err
	}
	_, err := io.WriteString(output, summary)
	return err
}

// reportTime resolves the dated report header. An explicit date keeps a
// re-rendered report reproducible; otherwise the command uses UTC today.
func reportTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().UTC(), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("glossreview: -date must be YYYY-MM-DD: %w", err)
	}
	return parsed.UTC(), nil
}
