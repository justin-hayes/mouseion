package classical

import (
	"strings"
	"sync"
)

// SourceAttribution credits one source layer's content in the learner-facing
// catalog. Text is the layer's deployer-register row, which names the source
// and its licence.
type SourceAttribution struct {
	Layer string
	Text  string
}

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

// SourceAttribution returns the credit for one source layer. The layer's
// deployer-register row is the reviewed attribution data that names both the
// source and its licence.
func (l Layers) SourceAttribution(layerKey string) (SourceAttribution, bool) {
	layer, ok := l.Layer(layerKey)
	if !ok {
		return SourceAttribution{}, false
	}
	text := strings.TrimSpace(layer.DeployerRegisterRow)
	if text == "" {
		return SourceAttribution{}, false
	}
	return SourceAttribution{Layer: layer.Key, Text: text}, true
}

// EmbeddedSourceAttribution looks a source layer up in the release's
// layers.json.
func EmbeddedSourceAttribution(layerKey string) (SourceAttribution, bool) {
	layers, err := embeddedLayers()
	if err != nil {
		return SourceAttribution{}, false
	}
	return layers.SourceAttribution(layerKey)
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
