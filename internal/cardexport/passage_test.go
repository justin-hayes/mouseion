package cardexport

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePassage(t *testing.T) {
	translations := map[string]string{
		"1.2.16": "A section the reading unit still keeps.",
		"1.2.17": "He halted his chariot in front of the phalanx and gave orders.",
		"1.2.18": "As for the barbarians, they were frightened.",
	}
	pin := "sha256:f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7"

	t.Run("a single section is shown with its citation", func(t *testing.T) {
		text, citation, gotPin := resolvePassage([]string{"1.2.17"}, translations, pin)
		assert.Equal(t, translations["1.2.17"], text)
		assert.Equal(t, "1.2.17", citation)
		assert.Equal(t, pin, gotPin)
	})

	t.Run("a crossing sentence shows both sections as one range", func(t *testing.T) {
		text, citation, gotPin := resolvePassage([]string{"1.2.17", "1.2.18"}, translations, pin)
		assert.Equal(t, translations["1.2.17"]+" "+translations["1.2.18"], text)
		assert.Equal(t, "1.2.17\u201318", citation)
		assert.Equal(t, pin, gotPin)
	})

	t.Run("any missing key withholds the whole passage", func(t *testing.T) {
		text, citation, gotPin := resolvePassage([]string{"1.2.17", "1.2.99"}, translations, pin)
		assert.Empty(t, text)
		assert.Empty(t, citation)
		assert.Empty(t, gotPin)
	})

	t.Run("no citations or no pin withholds the passage", func(t *testing.T) {
		text, citation, gotPin := resolvePassage(nil, translations, pin)
		assert.Empty(t, text+citation+gotPin)
		text, citation, gotPin = resolvePassage([]string{"1.2.17"}, translations, "")
		assert.Empty(t, text+citation+gotPin)
		text, citation, gotPin = resolvePassage([]string{"1.2.17"}, nil, pin)
		assert.Empty(t, text+citation+gotPin)
	})

	t.Run("non-consecutive sections are listed", func(t *testing.T) {
		_, citation, _ := resolvePassage([]string{"1.2.16", "1.2.18"}, translations, pin)
		assert.Equal(t, "1.2.16, 1.2.18", citation)
	})
}

func TestFormatCitationRange(t *testing.T) {
	for _, test := range []struct {
		name      string
		citations []string
		want      string
	}{
		{name: "single", citations: []string{"1.2.17"}, want: "1.2.17"},
		{name: "consecutive pair", citations: []string{"1.2.17", "1.2.18"}, want: "1.2.17\u201318"},
		{name: "consecutive triple", citations: []string{"1.2.17", "1.2.18", "1.2.19"}, want: "1.2.17\u201319"},
		{name: "two runs", citations: []string{"1.2.1", "1.2.2", "1.2.5"}, want: "1.2.1\u20132, 1.2.5"},
		{name: "different chapter", citations: []string{"1.2.17", "1.3.17"}, want: "1.2.17, 1.3.17"},
		{name: "reversed input is ordered", citations: []string{"1.3.17", "1.2.17"}, want: "1.2.17, 1.3.17"},
		{name: "mixed component counts", citations: []string{"1.2.17", "1.2"}, want: "1.2, 1.2.17"},
		{name: "unknown citation is listed verbatim", citations: []string{"1.2.17", "appendix"}, want: "1.2.17, appendix"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, formatCitationRange(test.citations))
		})
	}
}

func TestApplyPassageContextLeavesModernAndUnknownSentencesAlone(t *testing.T) {
	translations := map[string]string{"1.2.17": "The passage."}
	sentences := map[int64]analyzer.Sentence{0: {Citations: []string{"1.2.17"}}}

	modern := Entry{Language: "de", SentenceOrdinal: 0}
	applyPassageContext(&modern, CandidateProjection{ReferenceTranslations: translations, ReferencePin: "sha256:x", Sentences: sentences})
	assert.Empty(t, modern.PassageContext, "a modern-language card has no passage")

	unknown := Entry{Language: "grc", SentenceOrdinal: 9}
	applyPassageContext(&unknown, CandidateProjection{ReferenceTranslations: translations, ReferencePin: "sha256:x", Sentences: sentences})
	assert.Empty(t, unknown.PassageContext, "a sentence the corpus does not carry has no passage")

	withoutTranslations := Entry{Language: "grc", SentenceOrdinal: 0}
	applyPassageContext(&withoutTranslations, CandidateProjection{Sentences: sentences})
	assert.Empty(t, withoutTranslations.PassageContext, "a deck without translations has no passage")
}

func TestMakeNoteRendersUnhighlightedPassageContext(t *testing.T) {
	note, err := makeNote("owner-1", renderInputFromEntry(Entry{
		Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", Headword: "λόγος",
		Sentence: "ἄνθρωπος ὁρᾷ τὸν λόγον;", TargetWord: "λόγον",
		PassageContext:  "The man sees the word <b>and</b> the passage continues.",
		PassageCitation: "1.2.17\u201318",
		BrownsonPin:     "sha256:f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7",
	}))
	require.NoError(t, err)
	assert.Equal(t, "Passage context: Brownson (1922), Xen. Anab. 1.2.17\u201318. Not a translation of this sentence", note.PassageLabel)
	assert.Equal(t, "The man sees the word &lt;b&gt;and&lt;/b&gt; the passage continues.", note.PassageText, "translation markup is escaped, never treated as bold")
	assert.NotContains(t, note.PassageText, "<b>", "nothing in the passage is highlighted")
	assert.Contains(t, note.BackExtra, note.PassageLabel)
	assert.Contains(t, note.BackExtra, note.PassageText)
}

func TestMakeNoteWithoutPassageHasNoPassageFields(t *testing.T) {
	note, err := makeNote("owner-1", renderInputFromEntry(Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht dort.", TargetWord: "Haus"}))
	require.NoError(t, err)
	assert.Empty(t, note.PassageLabel)
	assert.Empty(t, note.PassageText)
	assert.NotContains(t, note.BackExtra, "Passage context")
}

func TestAttributionNoticesNameBrownsonWhenAPassageWasFrozen(t *testing.T) {
	entries := []RenderInput{{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", BrownsonPin: "sha256:abc"}}
	notices := attributionNotices(entries)
	assert.Contains(t, notices, "Brownson (1921-22)")
	assert.Contains(t, notices, "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7")
	assert.Contains(t, notices, "CC BY-SA 4.0")
	assert.True(t, deckHasBrownsonPassage(entries))

	assert.NotContains(t, attributionNotices([]RenderInput{{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN"}}), "Brownson", "a deck without a passage claims no Brownson notice")
	assert.False(t, deckHasBrownsonPassage(nil))
	assert.Contains(t, attributionNotices(entries), "OGA", "the OGA notice is still present")
}
