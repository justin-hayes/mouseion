package study

import "github.com/justin-hayes/mouseion/internal/cardpolicy"

// Available reports whether Study serves a study language: its card policy's
// Study switch is on (ADR 0138). Modern languages are on; Ancient Greek and
// Latin were switched on once their evidence runs were recorded.
func Available(language string) bool {
	return cardpolicy.For(language).Study
}
