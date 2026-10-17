//go:build ignore

// THROWAWAY: exercise the real ancient-greek/1 normalizer, not Python casefold.
// Run by prepare_inputs.py from the repository root. No database or provider.
package main

import (
	"encoding/json"
	"os"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

func main() {
	if unicode.Version != canonicalization.AncientGreekUnicodeVersion || norm.Version != canonicalization.AncientGreekUnicodeVersion {
		panic("prototype normalization Unicode version mismatch")
	}
	var words []string
	if err := json.NewDecoder(os.Stdin).Decode(&words); err != nil {
		panic(err)
	}
	keys := make(map[string]string, len(words))
	for _, word := range words {
		keys[word] = canonicalization.AncientGreek().Canonical(word)
	}
	if err := json.NewEncoder(os.Stdout).Encode(keys); err != nil {
		panic(err)
	}
}
