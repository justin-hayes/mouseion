// Package canonicalization normalizes language-specific vocabulary identities.
package canonicalization

import "strings"

// Lemma applies the baseline, language-neutral normalization.
func Lemma(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
