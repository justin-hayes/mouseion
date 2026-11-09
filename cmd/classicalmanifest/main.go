// Command classicalmanifest generates the embedded Classical catalog manifest
// from reviewed overrides, layer pins and pinned upstream files. Verify mode
// reproduces the committed manifest byte-for-byte and is the CI gate.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/justin-hayes/mouseion/internal/classicalmanifest"
)

const usage = "usage: classicalmanifest <generate|verify> [-layers <path>] [-overrides <path>] [-manifest <path>] [-upstream <dir>] [-notes <path>] [-require-upstream]"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// run parses the mode and flags, then generates or verifies the manifest.
func run(args []string, output io.Writer) error {
	mode, command, err := parseCommand(args)
	if err != nil {
		return err
	}
	input, err := readInput(mode, command)
	if err != nil {
		return err
	}
	result, err := classicalmanifest.Generate(classicalmanifest.Options{
		Layers:               input.layers,
		Overrides:            input.overrides,
		Manifest:             input.manifest,
		UpstreamDir:          *command.upstream,
		AllowMissingUpstream: mode == "verify" && !*command.requireUpstream,
	})
	if err != nil {
		return err
	}
	return emit(mode, command, input.manifest, result, output)
}

// parseCommand reads the mode and its flags.
func parseCommand(args []string) (string, *commandFlags, error) {
	if len(args) == 0 {
		return "", nil, errors.New(usage)
	}
	mode := args[0]
	if mode != "generate" && mode != "verify" {
		return "", nil, errors.New(usage)
	}
	flags, command := newFlagSet(mode)
	if err := flags.Parse(args[1:]); err != nil {
		return "", nil, err
	}
	return mode, command, nil
}

// input is the reviewed data one run reads.
type input struct {
	layers    []byte
	overrides []byte
	manifest  []byte
}

// readInput reads the layers, overrides and committed manifest.
func readInput(mode string, command *commandFlags) (input, error) {
	//nolint:gosec // the maintainer names these paths on the command line.
	layers, err := os.ReadFile(*command.layers)
	if err != nil {
		return input{}, fmt.Errorf("classicalmanifest: read layers: %w", err)
	}
	//nolint:gosec // the maintainer names these paths on the command line.
	overrides, err := os.ReadFile(*command.overrides)
	if err != nil {
		return input{}, fmt.Errorf("classicalmanifest: read overrides: %w", err)
	}
	//nolint:gosec // the maintainer names these paths on the command line.
	manifest, err := os.ReadFile(*command.manifest)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return input{}, fmt.Errorf("classicalmanifest: read manifest: %w", err)
	}
	if mode == "verify" && errors.Is(err, os.ErrNotExist) {
		return input{}, fmt.Errorf("classicalmanifest: %s does not exist", *command.manifest)
	}
	return input{layers: layers, overrides: overrides, manifest: manifest}, nil
}

// emit compares verify output or writes the generated manifest and notes.
func emit(mode string, command *commandFlags, committed []byte, result classicalmanifest.Result, output io.Writer) error {
	if mode == "verify" {
		if !bytes.Equal(result.Manifest, committed) {
			return fmt.Errorf("classicalmanifest: %s is stale; run make classical-manifest", *command.manifest)
		}
		_, err := fmt.Fprintf(output, "classical manifest verified: %d definitions\n", len(result.Definitions))
		return err
	}
	//nolint:gosec // the maintainer names the output path and it contains no learner data.
	if err := os.WriteFile(*command.manifest, result.Manifest, 0o600); err != nil {
		return fmt.Errorf("classicalmanifest: write manifest: %w", err)
	}
	if _, err := fmt.Fprintf(output, "classical manifest generated: %d definitions\n", len(result.Definitions)); err != nil {
		return err
	}
	if *command.notes == "" {
		return nil
	}
	//nolint:gosec // the maintainer names the output path and it contains no learner data.
	if err := os.WriteFile(*command.notes, []byte(result.Notes), 0o600); err != nil {
		return fmt.Errorf("classicalmanifest: write notes: %w", err)
	}
	return nil
}
