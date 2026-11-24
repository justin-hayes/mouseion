package webapp

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const constructionStudyText = "λέγει ὅτι ἄνδρας, καλούς."

// constructionStudy is a native_agdt sentence whose tokens are λέγει (0),
// ὅτι (1), ἄνδρας (2), "," (3), καλούς (4) and "." (5). The identified target
// is ὅτι. Occurrences are listed out of order on purpose.
func constructionStudy(occurrences ...domain.SentenceStudyConstruction) domain.SentenceStudy {
	token := func(ordinal, start, end int64, surface string) domain.SentenceStudyToken {
		return domain.SentenceStudyToken{Surface: surface, RawLemma: surface, EffectiveLemma: surface, Headword: surface,
			UPOS: "X", Dependency: "ATR", Ordinal: ordinal, StartOffset: start, EndOffset: end}
	}
	return domain.SentenceStudy{
		BookTitle: "Synthetic", ChapterTitle: "1.1", SentenceText: constructionStudyText, UnitText: constructionStudyText,
		SentenceStartOffset: 0, SentenceEndOffset: studyEnd(constructionStudyText), TargetOrdinal: 1, TargetSurface: "ὅτι",
		SyntaxState: "native_agdt",
		Tokens: []domain.SentenceStudyToken{
			token(0, 0, 5, "λέγει"), token(1, 6, 9, "ὅτι"), token(2, 10, 16, "ἄνδρας"),
			token(3, 16, 17, ","), token(4, 18, 24, "καλούς"), token(5, 24, 25, "."),
		},
		Constructions: occurrences,
	}
}

// outerNestedAndDiscontinuous is three occurrences: an indirect statement over
// the whole sentence (anchor λέγει), a discontinuous genitive absolute
// (anchor ἄνδρας, extent λέγει … ἄνδρας, καλούς), and a purpose clause nested
// inside the first whose anchor is the identified target (ὅτι).
func outerNestedAndDiscontinuous() []domain.SentenceStudyConstruction {
	return []domain.SentenceStudyConstruction{
		{ConstructionID: "purpose", Anchor: 1, Extent: []int{1, 2, 3, 4}},
		{ConstructionID: "genitive_absolute", Anchor: 2, Extent: []int{0, 2, 4}},
		{ConstructionID: "indirect_statement", SubtypeID: "infinitive", Anchor: 0, Extent: []int{0, 1, 2, 3, 4, 5}},
	}
}

func TestSentenceStudyConstructionEntriesOrderAndNest(t *testing.T) {
	entries := sentenceStudyConstructionEntries(constructionStudy(outerNestedAndDiscontinuous()...))
	require.Len(t, entries, 3)

	t.Run("entries start at the first extent ordinal, outer before inner", func(t *testing.T) {
		assert.Equal(t, []string{"Indirect statement (infinitive)", "Genitive absolute", "Purpose clause"},
			[]string{entries[0].Title, entries[1].Title, entries[2].Title})
		assert.Equal(t, []int{1, 2, 3}, []int{entries[0].Number, entries[1].Number, entries[2].Number})
	})
	t.Run("an entry inside the smallest earlier extent that strictly contains it says so", func(t *testing.T) {
		assert.Zero(t, entries[0].InsideOf)
		assert.Equal(t, 1, entries[1].InsideOf)
		assert.Equal(t, 1, entries[2].InsideOf)
	})
	t.Run("an inner entry is inside the smallest container, not the first", func(t *testing.T) {
		nested := sentenceStudyConstructionEntries(constructionStudy(
			domain.SentenceStudyConstruction{ConstructionID: "indirect_statement", Anchor: 0, Extent: []int{0, 1, 2, 3, 4, 5}},
			domain.SentenceStudyConstruction{ConstructionID: "purpose", Anchor: 1, Extent: []int{1, 2, 3, 4}},
			domain.SentenceStudyConstruction{ConstructionID: "result", Anchor: 4, Extent: []int{4}},
		))
		require.Len(t, nested, 3)
		assert.Equal(t, []int{0, 1, 2}, []int{nested[0].InsideOf, nested[1].InsideOf, nested[2].InsideOf})
	})
	t.Run("an equal extent is not inside", func(t *testing.T) {
		equal := sentenceStudyConstructionEntries(constructionStudy(
			domain.SentenceStudyConstruction{ConstructionID: "purpose", Anchor: 1, Extent: []int{1, 2}},
			domain.SentenceStudyConstruction{ConstructionID: "result", Anchor: 2, Extent: []int{1, 2}},
		))
		require.Len(t, equal, 2)
		assert.Zero(t, equal[0].InsideOf)
		assert.Zero(t, equal[1].InsideOf)
	})
}

func TestSentenceStudyConstructionEntriesNameSignalAndTarget(t *testing.T) {
	entries := sentenceStudyConstructionEntries(constructionStudy(outerNestedAndDiscontinuous()...))
	require.Len(t, entries, 3)

	t.Run("each entry names its anchor", func(t *testing.T) {
		assert.Equal(t, []string{"λέγει", "ἄνδρας", "ὅτι"}, []string{entries[0].AnchorSurface, entries[1].AnchorSurface, entries[2].AnchorSurface})
	})
	t.Run("the target phrase follows the identified target", func(t *testing.T) {
		assert.Equal(t, "Contains the identified target.", entries[0].TargetPhrase, "target in the extent")
		assert.Empty(t, entries[1].TargetPhrase, "target outside the extent says nothing")
		assert.Equal(t, "The identified target signals this construction.", entries[2].TargetPhrase, "target is the anchor")
	})
	t.Run("a labelled subtype shows in parentheses and an unlabelled one does not", func(t *testing.T) {
		labelled := sentenceStudyConstructionEntries(constructionStudy(
			domain.SentenceStudyConstruction{ConstructionID: "articular_infinitive", SubtypeID: "genitive", Anchor: 4, Extent: []int{4}},
			domain.SentenceStudyConstruction{ConstructionID: "articular_infinitive", SubtypeID: "accusative", Anchor: 2, Extent: []int{2}},
		))
		require.Len(t, labelled, 2)
		assert.Equal(t, "Articular infinitive", labelled[0].Title, "an unlabelled subtype shows no label")
		assert.Equal(t, "Articular infinitive (genitive)", labelled[1].Title)
	})
	t.Run("the note supplies the definition and the Smyth citation", func(t *testing.T) {
		note := constructions.Notes[constructions.Purpose]
		assert.Equal(t, note.Definition, entries[2].Definition)
		assert.Equal(t, constructions.CitationText(note.Citation), entries[2].CitationText)
		assert.Equal(t, constructions.CitationURL(note.Citation), entries[2].CitationURL)
	})
}

func TestSentenceStudyConstructionEntriesFailClosed(t *testing.T) {
	t.Run("an unknown construction id is skipped", func(t *testing.T) {
		entries := sentenceStudyConstructionEntries(constructionStudy(
			domain.SentenceStudyConstruction{ConstructionID: "nonexistent", Anchor: 1, Extent: []int{1}},
			domain.SentenceStudyConstruction{ConstructionID: "purpose", Anchor: 1, Extent: []int{1}},
		))
		require.Len(t, entries, 1)
		assert.Equal(t, "Purpose clause", entries[0].Title)
	})
	t.Run("an anchor with no stored word is skipped", func(t *testing.T) {
		assert.Empty(t, sentenceStudyConstructionEntries(constructionStudy(
			domain.SentenceStudyConstruction{ConstructionID: "purpose", Anchor: 9, Extent: []int{9}})))
	})
	t.Run("only a native_agdt run shows constructions", func(t *testing.T) {
		for _, state := range []string{"", "ud", "absent"} {
			study := constructionStudy(outerNestedAndDiscontinuous()...)
			study.SyntaxState = state
			assert.Empty(t, sentenceStudyConstructionEntries(study), "state %q", state)
		}
	})
	t.Run("a sentence with no occurrences has no entries", func(t *testing.T) {
		assert.Empty(t, sentenceStudyConstructionEntries(constructionStudy()))
	})
}

// extractMarks renders an entry's extract as a compact string: extent text
// plain, muted text in square brackets and the anchor in asterisks.
func extractMarks(entry sentenceStudyConstructionEntry) string {
	var b strings.Builder
	for _, part := range entry.Extract {
		switch {
		case part.Anchor:
			b.WriteString("*" + part.Text + "*")
		case part.InExtent:
			b.WriteString(part.Text)
		default:
			b.WriteString("[" + part.Text + "]")
		}
	}
	return b.String()
}

func TestSentenceStudyConstructionExtract(t *testing.T) {
	entries := sentenceStudyConstructionEntries(constructionStudy(outerNestedAndDiscontinuous()...))
	require.Len(t, entries, 3)

	t.Run("the whole sentence is reprinted, so every extract carries the sentence's text", func(t *testing.T) {
		for _, entry := range entries {
			assert.Equal(t, constructionStudyText, strings.NewReplacer("[", "", "]", "", "*", "").Replace(extractMarks(entry)))
		}
	})
	t.Run("a discontinuous extent mutes the words between its parts", func(t *testing.T) {
		assert.Equal(t, "λέγει[ ὅτι ]*ἄνδρας*, καλούς[.]", extractMarks(entries[1]))
	})
	t.Run("a gap made only of punctuation between extent words is part of the extent", func(t *testing.T) {
		assert.Equal(t, "[λέγει ]*ὅτι* ἄνδρας, καλούς[.]", extractMarks(entries[2]))
	})
	t.Run("the outer extent covers the whole sentence", func(t *testing.T) {
		assert.Equal(t, "*λέγει* ὅτι ἄνδρας, καλούς.", extractMarks(entries[0]))
	})
}

func TestSentenceStudyConstructionWordsLine(t *testing.T) {
	entries := sentenceStudyConstructionEntries(constructionStudy(outerNestedAndDiscontinuous()...))
	require.Len(t, entries, 3)
	assert.Equal(t, "Words: λέγει ὅτι ἄνδρας καλούς", entries[0].Words, "punctuation is not a word")
	assert.Equal(t, "Words: λέγει … ἄνδρας καλούς", entries[1].Words, "… between discontinuous parts, none across punctuation")
	assert.Equal(t, "Words: ὅτι ἄνδρας καλούς", entries[2].Words)
}

func TestSentenceStudyConstructionExtractFallsBackToWordsWhenTheSourceCannotBeAligned(t *testing.T) {
	study := constructionStudy(outerNestedAndDiscontinuous()...)
	study.UnitText = "Ganz anderer gespeicherter Text."
	entries := sentenceStudyConstructionEntries(study)
	require.Len(t, entries, 3)
	assert.Equal(t, "λέγει[ ὅτι ]*ἄνδρας*, καλούς[.]", extractMarks(entries[1]), "words are reprinted from the stored surfaces")
}

func TestSentenceStudyConstructionWordAnnotations(t *testing.T) {
	annotations := sentenceStudyConstructionWords(constructionStudy(outerNestedAndDiscontinuous()...))
	assert.Equal(t, "Constructions: 1 (Indirect statement), 2 (Genitive absolute). Signals: 1 (Indirect statement).", annotations[0])
	assert.Equal(t, "Constructions: 1 (Indirect statement), 3 (Purpose clause). Signals: 3 (Purpose clause).", annotations[1])
	assert.Equal(t, "Constructions: 1 (Indirect statement), 2 (Genitive absolute), 3 (Purpose clause). Signals: 2 (Genitive absolute).", annotations[2])
	assert.Equal(t, "Constructions: 1 (Indirect statement), 3 (Purpose clause).", annotations[3], "the comma is in the outer and purpose extents")
	assert.NotContains(t, annotations, int64(9))
}

func TestSentenceStudyPageRendersConstructions(t *testing.T) {
	body := renderSentenceStudy(t, constructionStudy(outerNestedAndDiscontinuous()...))

	t.Run("the region has an h3 and an ordered list of entries", func(t *testing.T) {
		assert.Contains(t, body, `<h3 id="sentence-study-constructions-heading">Constructions</h3>`)
		assert.Equal(t, 3, strings.Count(body, `class="sentence-study-construction"`))
		assert.Contains(t, body, `<ol class="sentence-study-construction-list">`)
	})
	t.Run("an entry names the construction and subtype, then the note", func(t *testing.T) {
		assert.Contains(t, body, `<p class="sentence-study-construction-name"><strong>Indirect statement (infinitive)</strong></p>`)
		assert.Contains(t, body, `<p class="sentence-study-construction-name"><strong>Genitive absolute</strong></p>`)
		assert.Contains(t, body, constructions.Notes[constructions.GenitiveAbsolute].Definition)
	})
	t.Run("the extract is hidden from assistive technology and a Words line replaces it", func(t *testing.T) {
		assert.Contains(t, body, `class="sentence-study-construction-extract" aria-hidden="true"`)
		assert.Contains(t, body, `<p class="sr-only">Words: λέγει … ἄνδρας καλούς</p>`)
	})
	t.Run("extent, muted text and the anchor use weight, tone and underline classes", func(t *testing.T) {
		assert.Contains(t, body, `<span class="construction-extent construction-anchor">ἄνδρας</span>`)
		assert.Contains(t, body, `<span class="construction-muted"> ὅτι </span>`)
	})
	t.Run("nesting, signal and target are stated in words", func(t *testing.T) {
		assert.Contains(t, body, `<p class="sentence-study-construction-nesting">Inside construction 1.</p>`)
		assert.Contains(t, body, `Signalled by <em>ὅτι</em>.`)
		assert.Contains(t, body, `The identified target signals this construction.`)
		assert.Contains(t, body, `Contains the identified target.`)
	})
	t.Run("the citation links to Smyth in a new tab without opener", func(t *testing.T) {
		note := constructions.Notes[constructions.Purpose]
		assert.Contains(t, body, `<a href="`+constructions.CitationURL(note.Citation)+`" target="_blank" rel="noopener noreferrer">`+constructions.CitationText(note.Citation)+`</a>`)
	})
	t.Run("the text alternative names each word's constructions and the one it signals", func(t *testing.T) {
		assert.Contains(t, body, `Constructions: 1 (Indirect statement), 3 (Purpose clause). Signals: 3 (Purpose clause).`)
	})
	t.Run("the region sits after the sentence and before the morphology line", func(t *testing.T) {
		sentence := strings.Index(body, `class="reading-text sentence-study-text"`)
		region := strings.Index(body, `id="sentence-study-constructions"`)
		line := strings.Index(body, `class="sentence-study-line"`)
		key := strings.Index(body, `id="sentence-study-notation-key"`)
		relations := strings.Index(body, `id="sentence-study-word-relations"`)
		alternative := strings.Index(body, `id="sentence-study-text-alternative"`)
		require.True(t, sentence >= 0 && region > sentence && line > region && key > line && relations > key && alternative > relations,
			"sentence %d region %d line %d key %d relations %d alternative %d", sentence, region, line, key, relations, alternative)
	})
	t.Run("the sentence itself stays unmarked", func(t *testing.T) {
		start := strings.Index(body, `class="reading-text sentence-study-text"`)
		end := strings.Index(body, `id="sentence-study-constructions"`)
		assert.NotContains(t, body[start:end], "construction-")
	})
}

func TestSentenceStudyPageIsSilentWithoutConstructions(t *testing.T) {
	for name, study := range map[string]domain.SentenceStudy{
		"no occurrences": constructionStudy(),
		"absent run": func() domain.SentenceStudy {
			s := constructionStudy(outerNestedAndDiscontinuous()...)
			s.SyntaxState = "absent"
			return s
		}(),
		"ud run": func() domain.SentenceStudy {
			s := constructionStudy(outerNestedAndDiscontinuous()...)
			s.SyntaxState = "ud"
			return s
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			body := renderSentenceStudy(t, study)
			assert.NotContains(t, body, "Constructions")
			assert.NotContains(t, body, "sentence-study-constructions")
			assert.NotContains(t, body, "construction-extent")
		})
	}
}
