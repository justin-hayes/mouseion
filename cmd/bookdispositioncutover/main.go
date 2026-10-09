// Command bookdispositioncutover runs the operator-controlled stages of the
// Set Aside retirement (ADR 0086) while the application and its workers are
// stopped: migrate, plan, apply, verify, and guarded forward correct. See
// doc/cutovers/2026-10-set-aside-retirement.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const usage = `usage: bookdispositioncutover <stage> [flags]

stages (run in this order, application and workers stopped):
  migrate                      apply structural migrations (includes 000033)
  plan                         write the read-only cutover manifest JSON to stdout
  apply   --manifest FILE --apply   convert in one bounded transaction
  verify                       read-only pre-reopen validation; exits non-zero on failure
  correct --cutover ID --owner ID --book ID --to inbox|to_read --apply
                               guarded forward correction after reopening`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, out io.Writer) (err error) {
	if len(args) == 0 {
		return errors.New(usage)
	}
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MOUSEION_DATABASE_URL is required")
	}
	ctx := context.Background()
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	manifestPath := flags.String("manifest", "", "reviewed manifest produced by plan")
	apply := flags.Bool("apply", false, "confirm a verified backup exists and writers are stopped")
	cutoverID := flags.String("cutover", "", "cutover id from the apply report")
	owner := flags.String("owner", "", "owner id")
	book := flags.String("book", "", "book id")
	to := flags.String("to", "", "inbox or to_read")
	if err = flags.Parse(args[1:]); err != nil {
		return err
	}
	if args[0] == "migrate" {
		if !*apply {
			return errors.New("refusing to migrate without --apply; first capture and verify a pre-cutover backup")
		}
		return persistence.Migrate(databaseURL)
	}
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	encode := func(v any) error {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(v)
	}
	switch args[0] {
	case "plan":
		manifest, planErr := store.PlanBookDispositionCutover(ctx)
		if planErr != nil {
			return planErr
		}
		if err = encode(manifest); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
		if len(manifest.Blockers) > 0 {
			return fmt.Errorf("manifest has %d blockers; review before continuing", len(manifest.Blockers))
		}
		return nil
	case "apply":
		if !*apply || *manifestPath == "" {
			return errors.New("apply requires --manifest FILE and --apply (verified backup, writers stopped)")
		}
		raw, readErr := os.ReadFile(filepath.Clean(*manifestPath)) // #nosec G304 G703 -- operator-supplied path to their own reviewed manifest
		if readErr != nil {
			return readErr
		}
		var manifest persistence.DispositionCutoverManifest
		if err = json.Unmarshal(raw, &manifest); err != nil {
			return fmt.Errorf("read manifest: %w", err)
		}
		report, applyErr := store.ApplyBookDispositionCutover(ctx, manifest)
		if applyErr != nil {
			return applyErr
		}
		return encode(report)
	case "verify":
		verification, verifyErr := store.VerifyBookDispositionCutover(ctx)
		if verifyErr != nil {
			return verifyErr
		}
		if err = encode(verification); err != nil {
			return err
		}
		if len(verification.Failures) > 0 {
			return fmt.Errorf("verification failed: %v", verification.Failures)
		}
		return nil
	case "correct":
		if !*apply || *cutoverID == "" || *owner == "" || *book == "" || *to == "" {
			return errors.New("correct requires --cutover, --owner, --book, --to and --apply")
		}
		applied, correctErr := store.CorrectBookDispositionCutover(ctx, *cutoverID, *owner, *book, domain.BookDisposition(*to))
		if correctErr != nil {
			return correctErr
		}
		return encode(map[string]bool{"applied": applied})
	default:
		return errors.New(usage)
	}
}
