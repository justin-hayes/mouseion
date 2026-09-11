package knownvocab

import (
	"encoding/json"
	"testing"

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
