package textmatch

import (
	"testing"
	"unicode/utf8"
)

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
			if !ok || test.text[start:end] != test.want || !utf8.ValidString(test.text[start:end]) {
				t.Fatalf("span=(%d,%d,%v) value=%q", start, end, ok, test.text[start:end])
			}
		})
	}
}

func TestFoldedWordSpanUsesUnicodeWordBoundaries(t *testing.T) {
	if _, _, ok := FoldedWordSpan("Das Hausboot liegt dort.", "Haus"); ok {
		t.Fatal("matched inside a larger word")
	}
	if _, _, ok := FoldedWordSpan("Das Cafe\u0301 liegt dort.", "Cafe"); ok {
		t.Fatal("matched before a combining mark")
	}
	if _, _, ok := FoldedWordSpan("Die U-Bahn fährt heute.", "Bahn"); ok {
		t.Fatal("matched inside a hyphenated word")
	}
	if _, _, ok := FoldedWordSpan("O'Neill wartet heute.", "Neill"); ok {
		t.Fatal("matched inside an apostrophized word")
	}
	start, end, ok := FoldedWordSpan("Das ‹Haus› liegt dort.", "haus")
	if !ok || "Das ‹Haus› liegt dort."[start:end] != "Haus" {
		t.Fatalf("punctuation-bounded span=(%d,%d,%v)", start, end, ok)
	}
}
