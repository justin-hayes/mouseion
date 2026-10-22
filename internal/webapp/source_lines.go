package webapp

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// sourceTextSegment is one exact run of authoritative source text and whether
// it is the identified target. The run is never reconstructed from analyzer
// surfaces.
type sourceTextSegment struct {
	Text   string
	Target bool
}

// sourceTextLine is one authorial source line. A source line break is structure,
// not text: it starts a new line and is never rendered as a character.
type sourceTextLine struct {
	Segments []sourceTextSegment
}

// sourceTextLines splits segments at authorial source line breaks. A text
// segment may contain several breaks; target segments stay within their line.
// Empty leading, interior or trailing lines are retained so line structure is
// not silently repaired.
func sourceTextLines(segments ...sourceTextSegment) []sourceTextLine {
	lines := []sourceTextLine{{}}
	for _, segment := range segments {
		parts := strings.Split(segment.Text, "\n")
		for index, part := range parts {
			if index > 0 {
				lines = append(lines, sourceTextLine{})
			}
			if part == "" {
				continue
			}
			lines[len(lines)-1].Segments = append(lines[len(lines)-1].Segments, sourceTextSegment{Text: part, Target: segment.Target})
		}
	}
	return lines
}

// concordanceSummaryChunk is one piece of a one-line Concordance summary:
// either collapsed source text or one authorial break marker.
type concordanceSummaryChunk struct {
	Text  string
	Break bool
}

// concordanceSummaryBoundary is the no-break space that keeps a boundary space
// beside the target. The left context is laid out right-to-left so it can be
// cut at its start, and bidi rule L1 would otherwise move an ordinary space to
// the far edge of that line.
const concordanceSummaryBoundary = "\u00a0"

// concordanceSummaryChunks renders a sentence's key-word context on one line.
// Each authorial break becomes a break marker; whitespace within a source line
// collapses, and the boundary spaces beside the target keep their no-break
// treatment. A literal source slash stays ambiguous with the break marker.
func concordanceSummaryChunks(text string) []concordanceSummaryChunk {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	chunks := make([]concordanceSummaryChunk, 0, 2*len(lines))
	for index, line := range lines {
		if index > 0 {
			chunks = append(chunks, concordanceSummaryChunk{Break: true})
		}
		if collapsed := strings.Join(strings.Fields(line), " "); collapsed != "" {
			chunks = append(chunks, concordanceSummaryChunk{Text: collapsed})
		}
	}
	if len(chunks) == 0 {
		return []concordanceSummaryChunk{{Text: concordanceSummaryBoundary}}
	}
	if first, _ := utf8.DecodeRuneInString(text); unicode.IsSpace(first) {
		concordanceSummaryAddBoundary(chunks, true)
	}
	if last, _ := utf8.DecodeLastRuneInString(text); unicode.IsSpace(last) {
		concordanceSummaryAddBoundary(chunks, false)
	}
	return chunks
}

// concordanceSummaryAddBoundary prefixes or suffixes the nearest source text
// chunk with the no-break boundary space. A summary that is only breaks keeps
// its break markers; there is no text edge to anchor.
func concordanceSummaryAddBoundary(chunks []concordanceSummaryChunk, leading bool) {
	if leading {
		for index := range chunks {
			if !chunks[index].Break {
				chunks[index].Text = concordanceSummaryBoundary + chunks[index].Text
				return
			}
		}
		return
	}
	for index := len(chunks) - 1; index >= 0; index-- {
		if !chunks[index].Break {
			chunks[index].Text += concordanceSummaryBoundary
			return
		}
	}
}
