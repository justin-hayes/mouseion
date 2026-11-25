package main

import (
	"flag"
	"io"
)

// commandFlags are the paths and switches one run uses.
type commandFlags struct {
	layers          *string
	overrides       *string
	manifest        *string
	upstream        *string
	notes           *string
	requireUpstream *bool
	lsIndex         *string
}

// newFlagSet defines the shared command line.
func newFlagSet(mode string) (*flag.FlagSet, *commandFlags) {
	flags := flag.NewFlagSet("classicalmanifest "+mode, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	command := &commandFlags{
		layers:          flags.String("layers", "internal/classical/layers.json", "path to layers.json"),
		overrides:       flags.String("overrides", "internal/classical/overrides.jsonl", "path to overrides.jsonl"),
		manifest:        flags.String("manifest", "internal/classical/manifest.jsonl", "path to the generated manifest.jsonl"),
		upstream:        flags.String("upstream", "", "directory holding the pinned upstream files"),
		notes:           flags.String("notes", "", "optional path for the maintainer notes report"),
		requireUpstream: flags.Bool("require-upstream", false, "verify mode fails when a pinned upstream file is absent"),
		lsIndex:         flags.String("ls-index", "", "derived dictionary index with Lewis & Short entries (make dictionary-index), read by generate for the Latin dictionary-match gate"),
	}
	return flags, command
}
