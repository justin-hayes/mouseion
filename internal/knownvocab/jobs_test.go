package knownvocab

import (
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobArgsRoundTripKeepsOwnerAndFileContents(t *testing.T) {
	want := JobArgs{OwnerID: "owner-1", Language: "de", FileContents: "Haus\ngehen\n"}
	encoded, err := json.Marshal(want)
	require.NoError(t, err)
	var got JobArgs
	require.NoError(t, json.Unmarshal(encoded, &got))
	assert.Equal(t, want, got)
	assert.Equal(t, "import_known_vocabulary", got.Kind())
}

func TestBackgroundImportUsesLanguageNeutralFallback(t *testing.T) {
	got, err := normalizeImportLemma("it", "CITTÀ")
	require.NoError(t, err)
	assert.Equal(t, "città", got.CanonicalLemma)
	assert.Equal(t, canonicalization.UnicodeCasefoldProfile, got.ProfileName)
	assert.Equal(t, canonicalization.UnicodeCasefoldVersion, got.ProfileVersion)
}
