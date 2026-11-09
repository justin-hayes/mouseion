package classical_test

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedManifestCarriesTheAnabasisPilot(t *testing.T) {
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	require.NoError(t, err)
	require.Len(t, definitions, 1)
	definition := definitions[0]
	assert.Equal(t, "xenophon-anabasis-1", definition.Key)
	assert.Equal(t, "xenophon-anabasis", definition.WorkKey)
	assert.Equal(t, "xenophon-anabasis-marchant-1904", definition.EditionKey)
	assert.Equal(t, "Anabasis", definition.WorkTitle)
	assert.Equal(t, "Xenophon", definition.WorkAuthor)
	assert.Equal(t, "grc", definition.Language)
	require.NotNil(t, definition.ConstituentBook)
	assert.Equal(t, 1, *definition.ConstituentBook)
	assert.Equal(t, 7, definition.ConstituentBooks)
	assert.Equal(t, "1.1.1", definition.CitationStart)
	assert.Equal(t, "1.10.19", definition.CitationEnd)
	assert.Positive(t, definition.WordCount)
	assert.False(t, definition.Withdrawn)

	assert.Equal(t, int64(7438037), definition.Source.Size)
	assert.Equal(t, "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61", definition.Source.SHA256)
	assert.Equal(t, "tlg0032.tlg006.perseus-grc2_oga020_src.conllu", definition.Source.FileName)
	assert.Equal(t, "oga-agdt-upos/5", definition.InterpretationVersion)
	require.NotNil(t, definition.Translation)
	assert.Equal(t, int64(1101217), definition.Translation.Size)
	assert.Equal(t, "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7", definition.Translation.SHA256)
	assert.Equal(t, "tlg0032.tlg006.perseus-eng2.xml", definition.Translation.FileName)
	assert.Equal(t, "1.1.1", definition.Translation.CitationStart)
	assert.Equal(t, "1.10.19", definition.Translation.CitationEnd)
}

func TestEmbeddedInputsParse(t *testing.T) {
	_, err := classical.ParseOverrides(classical.OverridesJSONL)
	require.NoError(t, err)
	layers, err := classical.ParseLayers(classical.LayersJSON)
	require.NoError(t, err)
	_, ok := layers.Layer("oga-src-v0.2.0-text")
	assert.True(t, ok)
	_, ok = layers.Layer("oga-src-v0.2.0-annotations")
	assert.True(t, ok)
	_, ok = layers.Layer("perseus-eng2-brownson")
	assert.True(t, ok)
}

func TestEmbeddedManifestIsSortedByKey(t *testing.T) {
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	require.NoError(t, err)
	keys := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		keys = append(keys, definition.Key)
	}
	for index := 1; index < len(keys); index++ {
		assert.Less(t, keys[index-1], keys[index])
	}
}
