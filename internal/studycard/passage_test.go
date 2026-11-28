package studycard

import (
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const brownsonPin = "sha256:f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7"

func TestPassageAttributionComesFromTheTranslationPin(t *testing.T) {
	t.Run("the Brownson pin keeps its label and notice", func(t *testing.T) {
		label, err := PassageContextLabel(brownsonPin, "1.2.17")
		require.NoError(t, err)
		assert.Equal(t, "Passage context: Brownson (1922), Xen. Anab. 1.2.17. Not a translation of this sentence", label)
		notice, err := PassageNotice(brownsonPin)
		require.NoError(t, err)
		assert.Equal(t, dictionary.BrownsonSourceNotice, notice)
	})

	t.Run("an unlisted pin is an error", func(t *testing.T) {
		_, err := PassageContextLabel("sha256:"+strings.Repeat("0", 64), "1.2.17")
		require.ErrorIs(t, err, ErrInvalidInput)
		require.ErrorContains(t, err, "not listed in the translation register")
	})

	t.Run("a register that fails to parse is an error", func(t *testing.T) {
		previous := embeddedPassageAttribution
		embeddedPassageAttribution = func(string) (classical.PassageAttribution, bool, error) {
			return classical.PassageAttribution{}, false, errors.New("classical: layers.json: unexpected end of JSON input")
		}
		t.Cleanup(func() { embeddedPassageAttribution = previous })

		_, err := PassageNotice(brownsonPin)
		require.ErrorContains(t, err, "read translation register")
		require.ErrorContains(t, err, "layers.json")
	})
}
