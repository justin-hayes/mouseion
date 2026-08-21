package analysis

import (
	"strings"
	"testing"
)

func TestChunkTextSingleChunkWhenSmall(t *testing.T) {
	text := "Ein Satz. Noch ein Satz."
	chunks := chunkText(text, DefaultMaxChunkChars)
	if len(chunks) != 1 || chunks[0] != text {
		t.Fatalf("expected single chunk, got %d: %v", len(chunks), chunks)
	}
}

func TestChunkTextSplitsAtSentences(t *testing.T) {
	text := "Eins. Zwei! Drei? Vier."
	want := []string{"Eins. Zwei!", " Drei?", " Vier."}
	if got := chunkText(text, 11); !equalStrings(got, want) {
		t.Fatalf("chunks = %#v, want %#v", got, want)
	}
}

func TestChunkTextHardCutsOverlongSentence(t *testing.T) {
	// A single sentence with no spaces longer than maxChars must be hard-cut.
	long := strings.Repeat("ä", 250)
	text := long + "."
	chunks := chunkText(text, 100)
	if len(chunks) < 2 {
		t.Fatalf("expected hard cut into multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len([]rune(c)) > 100 {
			t.Fatalf("chunk %d length %d exceeds max", i, len([]rune(c)))
		}
	}
	if got := strings.Join(chunks, ""); got != text {
		t.Fatalf("chunks do not reconstruct original")
	}
}

func TestChunkTextUsesDefaultForInvalidLimit(t *testing.T) {
	text := strings.Repeat("x", DefaultMaxChunkChars+1)
	chunks := chunkText(text, 0)
	if len(chunks) != 2 {
		t.Fatalf("expected two chunks, got %d", len(chunks))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestChunkTextReconstructsWithPunctuation(t *testing.T) {
	sentences := []string{"Erstens. Zweitens!", "Drittens?", "Viertens."}
	text := strings.Join(sentences, " ")
	// force multiple chunks
	chunks := chunkText(text, 10)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	got := strings.Join(chunks, "")
	if got != strings.Join(sentences, " ") {
		t.Fatalf("reconstruction mismatch: %q", got)
	}
}
