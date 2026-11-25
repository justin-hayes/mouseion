package study

import "github.com/justin-hayes/mouseion/internal/cardpolicy"

// Available reports whether Study serves a study language: only those on the
// modern-language provider route, whose card policy has a contextual Gloss to
// anchor a learned meaning (ADR 0133, ADR 0134 §1).
func Available(language string) bool {
	return !cardpolicy.For(language).SourceEvidence
}
