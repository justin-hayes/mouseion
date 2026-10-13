package webapp

import (
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// sentenceStudyLine is one sentence's source-word-aligned morphology line. The
// reader validates the sentence against its authoritative source slice and
// aligns an analyzed word only when the word's stored span is safe, following
// the accepted source-token evidence contract (§6). It never reconstructs the
// sentence from word surfaces.
type sentenceStudyLine struct {
	Segments []sentenceStudySegment
	// WholeLineFallback reports that the sentence bounds could not be trusted,
	// so no word is aligned: the line is withheld and its explanation says why.
	WholeLineFallback bool
	// Degraded reports that at least one analyzed word could not be aligned
	// locally; its source region is shown unannotated.
	Degraded bool
	// DegradedRegions lists the distinct source regions of words that degraded
	// locally, so the note can point the learner at them.
	DegradedRegions []string
	// HasAlternatives reports that at least one label joins analyzer-supplied
	// alternatives with a slash.
	HasAlternatives bool
	// HasUnannotated reports that at least one aligned word carried no core
	// grammatical features.
	HasUnannotated bool
}

// sentenceStudySegment is one run of the morphology line: either an aligned
// analyzed word, or an unaligned run of the exact source text.
type sentenceStudySegment struct {
	// Word is set for a run covered by one safely aligned analyzed word.
	Word *sentenceStudyWord
	// Text is the exact source slice for an unaligned run.
	Text string
}

// sentenceStudyWord is a single aligned source word with the labels stacked
// beneath it.
type sentenceStudyWord struct {
	Text   string
	Target bool
	Labels []sentenceStudyLabel
}

// sentenceStudyLabel is one position of a word's label sequence. Its
// alternatives are joined with a slash, each with its own abbreviation and
// plain-English expansion; the line never chooses between them.
type sentenceStudyLabel struct {
	Parts []sentenceStudyLabelPart
}

type sentenceStudyLabelPart struct {
	Abbreviation, Expansion string
}

// sentenceStudySource is the validated authoritative slice of the immutable
// unit text that one sentence occupies, in half-open Unicode code points.
type sentenceStudySource struct {
	runes      []rune
	start, end int64
}

// trusts reports whether the stored sentence bounds are a valid range into the
// immutable unit text whose slice exactly equals the stored sentence.
func (source sentenceStudySource) trusts(sentenceText string) bool {
	return source.start >= 0 && source.end >= source.start && source.end <= int64(len(source.runes)) &&
		source.slice(source.start, source.end) == sentenceText
}

// contains reports whether [start, end) is a valid non-empty range inside this
// source slice.
func (source sentenceStudySource) contains(start, end int64) bool {
	return start >= source.start && end <= source.end && end > start
}

func (source sentenceStudySource) slice(start, end int64) string {
	return string(source.runes[start:end])
}

// alignSentenceStudy validates the sentence against its authoritative source
// slice and aligns each safely attributable word. A sentence whose bounds do
// not match the immutable unit text degrades to whole-line fallback; a word
// whose span is invalid, overlaps a sibling, or disagrees with the source slice
// degrades locally while its neighbours stay annotated.
func alignSentenceStudy(study domain.SentenceStudy) sentenceStudyLine {
	source := sentenceStudySource{runes: []rune(study.UnitText), start: study.SentenceStartOffset, end: study.SentenceEndOffset}
	if !source.trusts(study.SentenceText) {
		return sentenceStudyLine{WholeLineFallback: true}
	}

	safe := sentenceStudySafeWords(study.Tokens, source)
	line := sentenceStudyLine{Segments: sentenceStudySegments(study, source, safe)}
	sentenceStudyFlags(&line, study.Tokens, safe, source)
	return line
}

// sentenceStudyFlags records the subtle states the view reacts to: unannotated
// words, slash alternatives, and which words degraded locally.
func sentenceStudyFlags(line *sentenceStudyLine, tokens []domain.SentenceStudyToken, safe []bool, source sentenceStudySource) {
	sentenceStudyWordFlags(line)
	sentenceStudyDegraded(line, tokens, safe, source)
}

func sentenceStudyWordFlags(line *sentenceStudyLine) {
	for _, segment := range line.Segments {
		if segment.Word == nil {
			continue
		}
		if len(segment.Word.Labels) == 0 {
			line.HasUnannotated = true
		}
		for _, label := range segment.Word.Labels {
			if len(label.Parts) > 1 {
				line.HasAlternatives = true
			}
		}
	}
}

func sentenceStudyDegraded(line *sentenceStudyLine, tokens []domain.SentenceStudyToken, safe []bool, source sentenceStudySource) {
	seen := make(map[string]struct{})
	for index, isSafe := range safe {
		if isSafe || !sentenceStudyTokenIsWord(tokens[index]) {
			continue
		}
		line.Degraded = true
		if region := sentenceStudyDegradedRegion(tokens[index], source); region != "" {
			if _, ok := seen[region]; !ok {
				seen[region] = struct{}{}
				line.DegradedRegions = append(line.DegradedRegions, region)
			}
		}
	}
}

// sentenceStudySafeWords marks each token that may be aligned: a valid in-bounds
// span whose source slice matches the stored surface and which overlaps no other
// word span. The overlap test considers every in-bounds sibling, safe or not,
// so a shared legacy span (a contraction and its words) never lets one word
// claim the region while the other silently disappears.
func sentenceStudySafeWords(tokens []domain.SentenceStudyToken, source sentenceStudySource) []bool {
	safe := make([]bool, len(tokens))
	for index, token := range tokens {
		safe[index] = sentenceStudyWordSpanIsSafe(token, source)
	}
	for index := range tokens {
		if !safe[index] {
			continue
		}
		for other := range tokens {
			if index == other || !source.contains(tokens[other].StartOffset, tokens[other].EndOffset) {
				continue
			}
			if sentenceStudySpansOverlap(tokens[index], tokens[other]) {
				safe[index] = false
				break
			}
		}
	}
	return safe
}

// sentenceStudySegments walks the trusted source range, emitting an aligned word
// for every safe span and the exact source text for everything else, so the
// sentence is never reconstructed from word surfaces.
func sentenceStudySegments(study domain.SentenceStudy, source sentenceStudySource, safe []bool) []sentenceStudySegment {
	safeOrder := make([]int, 0, len(study.Tokens))
	for index := range study.Tokens {
		if safe[index] {
			safeOrder = append(safeOrder, index)
		}
	}
	slices.SortStableFunc(safeOrder, func(a, b int) int {
		return int(study.Tokens[a].StartOffset - study.Tokens[b].StartOffset)
	})

	segments := make([]sentenceStudySegment, 0, 2*len(safeOrder)+1)
	cursor := source.start
	for _, index := range safeOrder {
		token := study.Tokens[index]
		if token.StartOffset < cursor {
			continue // Defensive: a safe span never reorders the source.
		}
		if token.StartOffset > cursor {
			segments = append(segments, sentenceStudySegment{Text: source.slice(cursor, token.StartOffset)})
		}
		segments = append(segments, sentenceStudyWordSegment(study, token, source))
		cursor = token.EndOffset
	}
	if cursor < source.end {
		segments = append(segments, sentenceStudySegment{Text: source.slice(cursor, source.end)})
	}
	return segments
}

// sentenceStudyWordSegment builds one aligned unit. Punctuation and symbols
// cover source text but carry no grammatical features, so they stay plain.
func sentenceStudyWordSegment(study domain.SentenceStudy, token domain.SentenceStudyToken, source sentenceStudySource) sentenceStudySegment {
	text := source.slice(token.StartOffset, token.EndOffset)
	if !sentenceStudyTokenIsWord(token) {
		return sentenceStudySegment{Text: text}
	}
	return sentenceStudySegment{Word: &sentenceStudyWord{
		Text:   text,
		Target: token.Ordinal == study.TargetOrdinal,
		Labels: sentenceStudyCoreLabels(token.Morphology),
	}}
}

// sentenceStudyDegradedRegion names the source region of a word that degraded
// locally, preferring its exact source text.
func sentenceStudyDegradedRegion(token domain.SentenceStudyToken, source sentenceStudySource) string {
	if source.contains(token.StartOffset, token.EndOffset) {
		return source.slice(token.StartOffset, token.EndOffset)
	}
	return token.Surface
}

// sentenceStudyDegradedNote points the learner at the words that could not be
// aligned and at the disclosure that lists them separately.
func sentenceStudyDegradedNote(line sentenceStudyLine) string {
	note := "Some words could not be aligned safely to their source text"
	if sample := sentenceStudyDegradedExample(line.DegradedRegions); sample != "" {
		note += " (" + sample + ")"
	}
	return note + " and are shown without labels here; their analyzer evidence is listed in the complete text alternative."
}

func sentenceStudyDegradedExample(regions []string) string {
	if len(regions) > 3 {
		regions = regions[:3]
	}
	return strings.Join(regions, ", ")
}

// sentenceStudyWordSpanIsSafe reports whether a word's stored span is a valid,
// non-empty, in-bounds code-point range whose source slice exactly matches the
// stored surface.
func sentenceStudyWordSpanIsSafe(token domain.SentenceStudyToken, source sentenceStudySource) bool {
	return source.contains(token.StartOffset, token.EndOffset) && source.slice(token.StartOffset, token.EndOffset) == token.Surface
}

func sentenceStudySpansOverlap(a, b domain.SentenceStudyToken) bool {
	return a.StartOffset < b.EndOffset && b.StartOffset < a.EndOffset
}

// sentenceStudyTokenIsWord reports whether an analyzed token should appear as a
// labelled interlinear unit. Punctuation and symbols stay as source text.
func sentenceStudyTokenIsWord(token domain.SentenceStudyToken) bool {
	return token.UPOS != "PUNCT" && token.UPOS != "SYM"
}

// sentenceStudyCoreLabels renders the curated core features the analyzer
// supplied, in one consistent order: fused person+number, gender, case, tense,
// aspect, mood, voice, verb form. A word with no core features has no labels.
func sentenceStudyCoreLabels(morphology map[string]string) []sentenceStudyLabel {
	var labels []sentenceStudyLabel
	person, number := morphology["Person"], morphology["Number"]
	if person != "" || number != "" {
		if fused, ok := sentenceStudyFusedPersonNumber(person, number); ok {
			labels = append(labels, fused)
		} else {
			if label, ok := sentenceStudyFeatureLabel("Person", person); ok {
				labels = append(labels, label)
			}
			if label, ok := sentenceStudyFeatureLabel("Number", number); ok {
				labels = append(labels, label)
			}
		}
	}
	for _, feature := range []string{"Gender", "Case", "Tense", "Aspect", "Mood", "Voice", "VerbForm"} {
		if label, ok := sentenceStudyFeatureLabel(feature, morphology[feature]); ok {
			labels = append(labels, label)
		}
	}
	return labels
}

// sentenceStudyFusedPersonNumber fuses a single-valued person and number into
// one label (for example 3SG). Supplied alternatives are never guessed into a
// combination, so they fall back to separate labels instead.
func sentenceStudyFusedPersonNumber(person, number string) (sentenceStudyLabel, bool) {
	persons := sentenceStudyAlternatives(person)
	numbers := sentenceStudyAlternatives(number)
	if len(persons) != 1 || len(numbers) != 1 {
		return sentenceStudyLabel{}, false
	}
	personPart := sentenceStudyMapFeatureValue("Person", persons[0])
	numberPart := sentenceStudyMapFeatureValue("Number", numbers[0])
	return sentenceStudyLabel{Parts: []sentenceStudyLabelPart{{
		Abbreviation: personPart.Abbreviation + numberPart.Abbreviation,
		Expansion:    strings.TrimSpace(personPart.Expansion + " " + numberPart.Expansion),
	}}}, true
}

// sentenceStudyFeatureLabel maps one feature's supplied alternatives, joining
// them with a slash so the line never chooses between them.
func sentenceStudyFeatureLabel(feature, value string) (sentenceStudyLabel, bool) {
	alternatives := sentenceStudyAlternatives(value)
	if len(alternatives) == 0 {
		return sentenceStudyLabel{}, false
	}
	parts := make([]sentenceStudyLabelPart, 0, len(alternatives))
	for _, alternative := range alternatives {
		parts = append(parts, sentenceStudyMapFeatureValue(feature, alternative))
	}
	return sentenceStudyLabel{Parts: parts}, true
}

func sentenceStudyAlternatives(value string) []string {
	var alternatives []string
	for part := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			alternatives = append(alternatives, trimmed)
		}
	}
	return alternatives
}

// sentenceStudyMapFeatureValue applies the Leipzig abbreviation table. Values
// with no abbreviation are spelled out; a core value absent from the table is
// shown as its raw analyzer value.
func sentenceStudyMapFeatureValue(feature, value string) sentenceStudyLabelPart {
	if table, ok := sentenceStudyAbbreviations[feature]; ok {
		if mapped, ok := table[value]; ok {
			return mapped
		}
	}
	return sentenceStudyLabelPart{Abbreviation: value, Expansion: "no standard abbreviation; shown as the analyzer's raw " + strings.ToLower(feature) + " value"}
}

// sentenceStudyAbbreviationTable records the curated core abbreviation data, one
// entry per line as "<Feature> <UD value> <abbreviation> <plain-English
// expansion>". A value with no Leipzig abbreviation keeps its spelled-out word
// as the abbreviation. Italian Tense=Imp (imperfect) and Greek Aspect=Imp
// (imperfective) are deliberately distinct.
const sentenceStudyAbbreviationTable = `
Person 1 1 first person
Person 2 2 second person
Person 3 3 third person
Number Sing SG singular
Number Plur PL plural
Gender Masc M masculine
Gender Fem F feminine
Gender Neut N neuter
Case Nom NOM nominative case
Case Acc ACC accusative case
Case Dat DAT dative case
Case Gen GEN genitive case
Case Voc VOC vocative case
Tense Pres PRS present tense
Tense Past PST past tense
Tense Fut FUT future tense
Tense Imp imperfect imperfect tense
Tense Pqp pluperfect pluperfect tense
Aspect Imp IPFV imperfective aspect
Aspect Perf PFV perfective aspect
Mood Ind IND indicative mood
Mood Sub SBJV subjunctive mood
Mood Imp IMP imperative mood
Mood Cnd COND conditional mood
Voice Act active active voice
Voice Pass PASS passive voice
VerbForm Fin finite finite verb form
VerbForm Inf INF infinitive
VerbForm Part PTCP participle
VerbForm Ger gerund gerund
VerbForm Conv CVB converb
`

var sentenceStudyAbbreviations = sentenceStudyParseAbbreviations(sentenceStudyAbbreviationTable)

func sentenceStudyParseAbbreviations(data string) map[string]map[string]sentenceStudyLabelPart {
	table := make(map[string]map[string]sentenceStudyLabelPart)
	for line := range strings.SplitSeq(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if table[fields[0]] == nil {
			table[fields[0]] = make(map[string]sentenceStudyLabelPart)
		}
		table[fields[0]][fields[1]] = sentenceStudyLabelPart{
			Abbreviation: fields[2],
			Expansion:    strings.Join(fields[3:], " "),
		}
	}
	return table
}

// sentenceStudyWordLabels renders one word's stacked labels, one consistent
// order joined with a period and analyzer-supplied alternatives with a slash.
func sentenceStudyWordLabels(word sentenceStudyWord) string {
	rendered := make([]string, 0, len(word.Labels))
	for _, label := range word.Labels {
		alternatives := make([]string, 0, len(label.Parts))
		for _, part := range label.Parts {
			alternatives = append(alternatives, part.Abbreviation)
		}
		rendered = append(rendered, strings.Join(alternatives, "/"))
	}
	return strings.Join(rendered, ".")
}

// sentenceStudyKey lists only the abbreviations used in this line, sorted
// deterministically, each with a plain-English expansion.
func sentenceStudyKey(line sentenceStudyLine) []sentenceStudyLabelPart {
	seen := map[sentenceStudyLabelPart]struct{}{}
	entries := make([]sentenceStudyLabelPart, 0)
	for _, segment := range line.Segments {
		if segment.Word == nil {
			continue
		}
		for _, label := range segment.Word.Labels {
			for _, part := range label.Parts {
				if _, ok := seen[part]; ok {
					continue
				}
				seen[part] = struct{}{}
				entries = append(entries, part)
			}
		}
	}
	slices.SortFunc(entries, func(a, b sentenceStudyLabelPart) int {
		if order := strings.Compare(a.Abbreviation, b.Abbreviation); order != 0 {
			return order
		}
		return strings.Compare(a.Expansion, b.Expansion)
	})
	return entries
}
