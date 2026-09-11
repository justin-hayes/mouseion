package analysis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkTextSingleChunkWhenSmall(t *testing.T) {
	text := "Ein Satz. Noch ein Satz."
	chunks := chunkText(text, DefaultMaxChunkChars)
	require.Len(t, chunks, 1, "expected single chunk")
	assert.Equal(t, text, chunks[0])
}

func TestChunkTextSplitsAtSentences(t *testing.T) {
	text := "Eins. Zwei! Drei? Vier."
	want := []string{"Eins. Zwei!", " Drei?", " Vier."}
	assert.Equal(t, want, chunkText(text, 11))
}

func TestChunkTextHardCutsOverlongSentence(t *testing.T) {
	// A single sentence with no spaces longer than maxChars must be hard-cut.
	long := strings.Repeat("ä", 250)
	text := long + "."
	chunks := chunkText(text, 100)
	require.GreaterOrEqual(t, len(chunks), 2, "expected hard cut into multiple chunks")
	for i, c := range chunks {
		assert.LessOrEqual(t, len([]rune(c)), 100, "chunk %d length %d exceeds max", i, len([]rune(c)))
	}
	assert.Equal(t, text, strings.Join(chunks, ""), "chunks do not reconstruct original")
}

func TestChunkTextUsesDefaultForInvalidLimit(t *testing.T) {
	text := strings.Repeat("x", DefaultMaxChunkChars+1)
	chunks := chunkText(text, 0)
	assert.Len(t, chunks, 2)
}

func TestChunkTextReconstructsWithPunctuation(t *testing.T) {
	sentences := []string{"Erstens. Zweitens!", "Drittens?", "Viertens."}
	text := strings.Join(sentences, " ")
	// force multiple chunks
	chunks := chunkText(text, 10)
	require.GreaterOrEqual(t, len(chunks), 2, "expected multiple chunks")
	assert.Equal(t, strings.Join(sentences, " "), strings.Join(chunks, ""), "reconstruction mismatch: %q", strings.Join(chunks, ""))
}
