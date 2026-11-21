package classical

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmbeddedPassageAttributionReportsAnUnparseableRegister pins that a broken
// embedded layers.json is an error, never an unlisted pin that another caller
// could silently credit.
func TestEmbeddedPassageAttributionReportsAnUnparseableRegister(t *testing.T) {
	previous := embeddedLayers
	t.Cleanup(func() { embeddedLayers = previous })
	embeddedLayers = func() (Layers, error) {
		return Layers{}, errors.New("classical: layers.json: unexpected end of JSON input")
	}

	_, _, err := EmbeddedPassageAttribution("sha256:f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7")

	require.ErrorContains(t, err, "layers.json")
}
