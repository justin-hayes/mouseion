package webapp

import (
	"strings"

	"github.com/justin-hayes/mouseion/internal/cardexport"
)

// sentenceSegment is a run of an Idiom card's sentence: Marked runs are the
// Idiom's matched words, shown in bold.
type sentenceSegment struct {
	Text   string
	Marked bool
}

// markIdiom turns the face into the idiom layout of the deck's card
// (ADR 0067): the sentence with every matched word marked, an "idiom" label
// beside the headword on the answer side, and the translation with its
// aligned excerpts highlighted. forms are the matched surface forms in
// sentence order; targets are the translation's validated excerpts.
func (f *cardFace) markIdiom(sentence string, forms []string, translation string, targets []string) {
	f.Idiom = true
	f.Segments = segmentMarked(sentence, forms)
	f.TranslationHTML = cardexport.HighlightEnglishTargets(translation, targets, "")
}

// segmentMarked splits the sentence around each form, taken in order as the
// first whole-word occurrence after the previous one. A form that is not found
// is skipped, so the sentence is always shown whole.
func segmentMarked(sentence string, forms []string) []sentenceSegment {
	var segments []sentenceSegment
	rest := sentence
	for _, form := range forms {
		before, marked, after, found := markTarget(rest, form)
		if !found {
			continue
		}
		if before != "" {
			segments = append(segments, sentenceSegment{Text: before})
		}
		segments = append(segments, sentenceSegment{Text: marked, Marked: true})
		rest = after
	}
	if rest != "" {
		segments = append(segments, sentenceSegment{Text: rest})
	}
	return segments
}

// idiomForms are an Idiom material's matched surface forms, stored joined by a
// space as its target word.
func idiomForms(targetWord string) []string {
	return strings.Fields(targetWord)
}
