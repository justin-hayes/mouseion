// Command glossreview runs a frozen contextual-Gloss review case set through
// the real translation codec and the configured OpenAI-compatible provider. It
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

const usage = "usage: glossreview [-out <report.md>] [-date YYYY-MM-DD]"

// glossReviewCases is the versioned repository case set. Embedding it keeps a
// review tied to the code that produced it and makes the command independent of
// the caller's working directory outside the repository.
//
//go:embed testdata/gloss_review_cases.json
var glossReviewCases []byte

// lemmaMismatchOutcomeSupported reports whether the current contextual-Gloss
// response contract can return a lemma_mismatch outcome. It becomes true when
// the mismatch-outcome ticket adds the enumerated unresolved kind; until then
// the review reports the check as unsupported instead of failing every case.
const lemmaMismatchOutcomeSupported = false

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// reviewEnvironment is the configured provider and the metadata the report
// pins. Tests inject a real client whose HTTP transport is stubbed, so the
// command path never reaches the network in CI.
type reviewEnvironment struct {
	Client   translationClient
	Provider providerMetadata
}

type reviewEnvironmentOpener func() (reviewEnvironment, error)

// run parses the command and writes the report. It refuses to run when the
// deployed provider is not configured.
func run(ctx context.Context, args []string, output io.Writer) error {
	return runWithEnvironment(ctx, args, output, openReviewEnvironment)
}

// openReviewEnvironment reads the provider configuration and builds the real
// client. It is separate from run so a test can supply a stub without touching
// the environment or the network.
func openReviewEnvironment() (reviewEnvironment, error) {
	cfg, err := enrichment.LLMConfigFromEnv()
	if err != nil {
		return reviewEnvironment{}, err
	}
	if !cfg.Enabled {
		return reviewEnvironment{}, errors.New("glossreview: no provider is configured; set MOUSEION_LLM_ENABLED=true, MOUSEION_LLM_API_KEY and MOUSEION_LLM_MODEL, then rerun")
	}
	client, err := enrichment.NewOpenAITranslationClient(cfg, nil)
	if err != nil {
		return reviewEnvironment{}, err
	}
	codec, err := enrichment.NewTranslationCodec(cfg)
	if err != nil {
		return reviewEnvironment{}, err
	}
	return reviewEnvironment{
		Client: client,
		Provider: providerMetadata{
			Model:           cfg.Model,
			ReasoningEffort: cfg.ReasoningEffort,
			ProviderName:    codec.ProviderName(),
			ProviderVersion: codec.ContextualGlossProviderVersion(),
			PromptContract:  codec.ContextualGlossPromptContract(),
		},
	}, nil
}

// runWithEnvironment is run with an injectable environment for tests.
func runWithEnvironment(ctx context.Context, args []string, output io.Writer, open reviewEnvironmentOpener) error {
	flags := flag.NewFlagSet("glossreview", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	outPath := flags.String("out", "", "report path under doc/reviews (default: dated contextual-gloss report)")
	dateValue := flags.String("date", "", "report date in YYYY-MM-DD (default: today)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%s: %w", usage, err)
	}
	if flags.NArg() != 0 {
		return errors.New(usage)
	}
	now, err := reportTime(*dateValue)
	if err != nil {
		return err
	}
	environment, err := open()
	if err != nil {
		return err
	}
	cases, err := loadCaseSet(bytes.NewReader(glossReviewCases))
	if err != nil {
		return err
	}
	report := review(ctx, reviewOptions{
		Now:                      now,
		Cases:                    cases,
		Client:                   environment.Client,
		Provider:                 environment.Provider,
		MismatchOutcomeSupported: lemmaMismatchOutcomeSupported,
	})
	path := strings.TrimSpace(*outPath)
	if path == "" {
		path = filepath.Join("doc", "reviews", fmt.Sprintf("contextual-gloss-%s.md", now.Format("2006-01-02")))
	}
	var rendered bytes.Buffer
	if err := writeReport(&rendered, report); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, rendered.Bytes(), 0o600); err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "gloss review report written: path=%s cases=%d\n", path, len(report.Cases))
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
