package classical_test

import (
	"fmt"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedManifestCarriesTheAnabasisAsOneWholeWorkUnit(t *testing.T) {
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	require.NoError(t, err)
	byKey := map[string]classical.Definition{}
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	definition, ok := byKey["xenophon-anabasis"]
	require.True(t, ok)
	assert.Equal(t, "xenophon-anabasis", definition.WorkKey)
	assert.Equal(t, "xenophon-anabasis-marchant-1904", definition.EditionKey)
	assert.Equal(t, "Anabasis", definition.WorkTitle)
	assert.Equal(t, "Xenophon", definition.WorkAuthor)
	assert.Equal(t, "grc", definition.Language)
	assert.Nil(t, definition.ConstituentBook, "the Anabasis is one whole-work unit")
	assert.Equal(t, 7, definition.ConstituentBooks)
	assert.Equal(t, "1.1.1", definition.CitationStart)
	assert.Equal(t, "7.8.24", definition.CitationEnd)
	assert.Equal(t, 56862, definition.WordCount)
	require.Len(t, definition.Contents, 7)
	assert.Equal(t, classical.ConstituentBookSpan{Book: 1, CitationStart: "1.1.1", CitationEnd: "1.10.19", WordCount: 9305}, definition.Contents[0])
	assert.Equal(t, classical.ConstituentBookSpan{Book: 7, CitationStart: "7.1.2", CitationEnd: "7.8.24", WordCount: 10915}, definition.Contents[6])
	assert.Equal(t, "Δαρείου καὶ Παρυσάτιδος γίγνονται παῖδες δύο, πρεσβύτερος μὲν Ἀρταξέρξης, νεώτερος δὲ Κῦρος·", definition.Opening)
	assert.False(t, definition.Withdrawn)

	assert.Equal(t, int64(7438037), definition.Source.Size)
	assert.Equal(t, "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61", definition.Source.SHA256)
	assert.Equal(t, "tlg0032.tlg006.perseus-grc2_oga020_src.conllu", definition.Source.FileName)
	assert.Equal(t, "oga-agdt-syntax/1", definition.InterpretationVersion)
	require.NotNil(t, definition.Translation)
	assert.Equal(t, int64(1101217), definition.Translation.Size)
	assert.Equal(t, "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7", definition.Translation.SHA256)
	assert.Equal(t, "tlg0032.tlg006.perseus-eng2.xml", definition.Translation.FileName)
	assert.Equal(t, "1.1.1", definition.Translation.CitationStart)
	assert.Equal(t, "7.8.24", definition.Translation.CitationEnd)

	for book := 1; book <= 7; book++ {
		perBook, ok := byKey[fmt.Sprintf("xenophon-anabasis-%d", book)]
		require.True(t, ok, "the per-book definition stays as a tombstone")
		assert.True(t, perBook.Withdrawn)
	}
}

func TestEmbeddedRepublicIsOneWholeWorkUnitOfTenBooks(t *testing.T) {
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	require.NoError(t, err)
	byKey := map[string]classical.Definition{}
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	definition := byKey["plato-republic"]
	assert.Nil(t, definition.ConstituentBook)
	assert.Equal(t, 10, definition.ConstituentBooks)
	assert.Len(t, definition.Contents, 10)
	assert.Equal(t, 89148, definition.WordCount)
	assert.Equal(t, "1.327", definition.CitationStart)
	assert.Equal(t, "10.621", definition.CitationEnd)
	assert.False(t, definition.Withdrawn)
	for book := 1; book <= 10; book++ {
		assert.True(t, byKey[fmt.Sprintf("plato-republic-%d", book)].Withdrawn)
	}
}

func TestEveryEmbeddedDefinitionCarriesItsOpening(t *testing.T) {
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	require.NoError(t, err)
	expected := map[string]string{
		"plato-apology":     "ὅτι μὲν ὑμεῖς, ὦ ἄνδρες Ἀθηναῖοι, πεπόνθατε ὑπὸ τῶν ἐμῶν κατηγόρων, οὐκ οἶδα·",
		"plato-crito":       "τί τηνικάδε ἀφῖξαι, ὦ Κρίτων;",
		"plato-symposium":   "δοκῶ μοι περὶ ὧν πυνθάνεσθε οὐκ ἀμελέτητος εἶναι.",
		"plato-republic":    "κατέβην χθὲς εἰς Πειραιᾶ μετὰ Γλαύκωνος τοῦ Ἀρίστωνος προσευξόμενός τε τῇ θεῷ καὶ ἅμα τὴν ἑορτὴν βουλόμενος θεάσασθαι τίνα τρόπον ποιήσουσιν ἅτε νῦν πρῶτον ἄγοντες.",
		"xenophon-anabasis": "Δαρείου καὶ Παρυσάτιδος γίγνονται παῖδες δύο, πρεσβύτερος μὲν Ἀρταξέρξης, νεώτερος δὲ Κῦρος·",
	}
	require.Len(t, definitions, 36, "7 Greek and 12 Latin Works, plus 17 per-book tombstones")
	for _, definition := range definitions {
		if definition.Withdrawn {
			continue
		}
		assert.NotEmpty(t, definition.Opening, definition.Key)
		if want, ok := expected[definition.Key]; ok {
			assert.Equal(t, want, definition.Opening, definition.Key)
		}
	}
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

func TestEveryEmbeddedTranslationIsCredited(t *testing.T) {
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	require.NoError(t, err)
	for _, definition := range definitions {
		if definition.Translation == nil {
			continue
		}
		attribution, ok, err := classical.EmbeddedPassageAttribution("sha256:" + definition.Translation.SHA256)
		require.NoError(t, err, definition.Key)
		require.True(t, ok, definition.Key)
		assert.NotEmpty(t, attribution.Label, definition.Key)
		assert.Contains(t, attribution.Notice, definition.Translation.SHA256, "%s: the notice names the pinned file", definition.Key)
	}
}

func TestBrownsonKeepsTheNoticeEarlierDecksCarried(t *testing.T) {
	attribution, ok, err := classical.EmbeddedPassageAttribution("sha256:f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "Brownson (1922), Xen. Anab.", attribution.Label)
	assert.Equal(t, dictionary.BrownsonSourceNotice, attribution.Notice)
}
