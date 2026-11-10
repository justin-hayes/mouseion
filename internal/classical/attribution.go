package classical

import (
	"strings"
	"sync"
)

// PassageAttribution is how an exported card and deck credit the reference
// translation a frozen passage context came from.
type PassageAttribution struct {
	// Label names the translation and work on the card, before the citation.
	Label string
	// Notice is the deck-level attribution and licence notice.
	Notice string
}

// PassageAttribution returns the credit of the translation file whose SHA-256
// matches pin. The pin may carry the "sha256:" prefix that prepared decks
// freeze. A file is found by its bytes, not its layer, so a deck keeps its
// credit however the file is later organised into layers.
func (l Layers) PassageAttribution(pin string) (PassageAttribution, bool) {
	digest := strings.TrimPrefix(strings.TrimSpace(pin), "sha256:")
	if digest == "" {
		return PassageAttribution{}, false
	}
	for _, layer := range l.Layers {
		for _, file := range layer.Files {
			if file.SHA256 == digest && file.HasPassageAttribution() {
				return PassageAttribution{Label: file.PassageLabel, Notice: file.DeckNotice}, true
			}
		}
	}
	return PassageAttribution{}, false
}

// embeddedLayers parses the release's layers.json once.
var embeddedLayers = sync.OnceValues(func() (Layers, error) { return ParseLayers(LayersJSON) })

// EmbeddedPassageAttribution looks a frozen translation pin up in the
// release's layers.json.
func EmbeddedPassageAttribution(pin string) (PassageAttribution, bool) {
	layers, err := embeddedLayers()
	if err != nil {
		return PassageAttribution{}, false
	}
	return layers.PassageAttribution(pin)
}
