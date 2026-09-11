package textmatch

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanLexicalSurface(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "Souveränität›", want: "Souveränität"},
		{input: "die Besten›", want: "die Besten"},
		{input: "‹die", want: "die"},
		{input: "„O'Neill-like“", want: "O'Neill-like"},
		{input: " L' ", want: "L'"},
		{input: "dell’", want: "dell’"},
		{input: ".", want: "."},
		{input: "—", want: "—"},
	}
	for _, test := range tests {
		assert.Equal(t, test.want, CleanLexicalSurface(test.input))
	}
}

func TestFoldedWordSpanPreservesOriginalUTF8Offsets(t *testing.T) {
	for _, test := range []struct {
		name, text, target, want string
	}{
		{name: "capital sharp s before target", text: "ẞ Haus", target: "Haus", want: "Haus"},
		{name: "capital sharp s target", text: "Das ẞ steht", target: "ß", want: "ẞ"},
		{name: "kelvin sign before target", text: "K Haus", target: "Haus", want: "Haus"},
	} {
		t.Run(test.name, func(t *testing.T) {
			start, end, ok := FoldedWordSpan(test.text, test.target)
			require.True(t, ok)
			span := test.text[start:end]
			assert.Equal(t, test.want, span)
			assert.True(t, utf8.ValidString(span))
		})
	}
}

func TestFoldedWordSpanUsesUnicodeWordBoundaries(t *testing.T) {
	_, _, ok := FoldedWordSpan("Das Hausboot liegt dort.", "Haus")
	assert.False(t, ok, "matched inside a larger word")
	_, _, ok = FoldedWordSpan("Das Cafe\u0301 liegt dort.", "Cafe")
	assert.False(t, ok, "matched before a combining mark")
	_, _, ok = FoldedWordSpan("Die U-Bahn fährt heute.", "Bahn")
	assert.False(t, ok, "matched inside a hyphenated word")
	_, _, ok = FoldedWordSpan("O'Neill wartet heute.", "Neill")
	assert.False(t, ok, "matched inside an apostrophized word")
	start, end, ok := FoldedWordSpan("Das ‹Haus› liegt dort.", "haus")
	require.True(t, ok)
	assert.Equal(t, "Haus", "Das ‹Haus› liegt dort."[start:end])
}
