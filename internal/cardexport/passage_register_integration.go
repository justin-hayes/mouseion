//go:build integration

package cardexport

import "github.com/justin-hayes/mouseion/internal/classical"

// UsePassageRegister replaces the release's translation register for an
// integration test, so a synthetic translation pin can be credited without
// shipping synthetic data in layers.json. It returns the function that restores
// the release register.
func UsePassageRegister(lookup func(pin string) (classical.PassageAttribution, bool, error)) (restore func()) {
	previous := embeddedPassageAttribution
	embeddedPassageAttribution = lookup
	return func() { embeddedPassageAttribution = previous }
}
