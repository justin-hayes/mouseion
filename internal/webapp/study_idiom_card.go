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

// segmentMarked splits the sentence around the forms, taken in order, each the
// first whole-word occurrence after the previous one. Of the places the first
// form occurs, the one that makes the tightest run of forms wins, so a common
// word elsewhere in the sentence is not marked. A form that is not found is
// skipped, so the sentence is always shown whole.
func segmentMarked(sentence string, forms []string) []sentenceSegment {
	for len(forms) > 0 {
		if _, _, _, found := markTarget(sentence, forms[0]); found {
			break
		}
		forms = forms[1:]
	}
	var best []sentenceSegment
	bestSpan, bestMatched := -1, 0
	for from := 0; from <= len(sentence); {
		_, marked, after, found := markTarget(sentence[from:], firstForm(forms))
		if !found {
			break
		}
		start := len(sentence) - len(after) - len(marked)
		segments, end, matched := markFrom(sentence, start, forms)
		if span := end - start; bestSpan < 0 || matched > bestMatched || (matched == bestMatched && span < bestSpan) {
			best, bestSpan, bestMatched = segments, span, matched
		}
		from = start + 1
	}
	if best == nil {
		return []sentenceSegment{{Text: sentence}}
	}
	return best
}

func firstForm(forms []string) string {
	if len(forms) == 0 {
		return ""
	}
	return forms[0]
}

// markFrom marks the forms in order from the first form's place at start, and
// returns the segments, where the last marked word ends and how many forms
// were found.
func markFrom(sentence string, start int, forms []string) ([]sentenceSegment, int, int) {
	segments := []sentenceSegment{}
	if start > 0 {
		segments = append(segments, sentenceSegment{Text: sentence[:start]})
	}
	cursor, matched := start, 0
	for _, form := range forms {
		before, marked, after, found := markTarget(sentence[cursor:], form)
		if !found {
			continue
		}
		if before != "" {
			segments = append(segments, sentenceSegment{Text: before})
		}
		segments = append(segments, sentenceSegment{Text: marked, Marked: true})
		cursor = len(sentence) - len(after)
		matched++
	}
	if cursor < len(sentence) {
		segments = append(segments, sentenceSegment{Text: sentence[cursor:]})
	}
	return segments, cursor, matched
}

// idiomForms are an Idiom material's matched surface forms, stored joined by a
// space as its target word.
func idiomForms(targetWord string) []string {
	return strings.Fields(targetWord)
}
