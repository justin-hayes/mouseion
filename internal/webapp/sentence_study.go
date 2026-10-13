package webapp

import (
	"slices"
	"strings"
	"unicode"

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

	alignments := sentenceStudyAlignments(study.Tokens, source)
	line := sentenceStudyLine{Segments: sentenceStudySegments(study, source, alignments)}
	sentenceStudyFlags(&line, study.Tokens, alignments, source)
	return line
}

// sentenceStudyAlignment is one token's decided alignment: the source sub-span
// it may claim as an aligned word unit, and whether it may be aligned at all.
type sentenceStudyAlignment struct {
	start, end int64
	safe       bool
}

// sentenceStudyFlags records the subtle states the view reacts to: unannotated
// words, slash alternatives, and which words degraded locally.
func sentenceStudyFlags(line *sentenceStudyLine, tokens []domain.SentenceStudyToken, alignments []sentenceStudyAlignment, source sentenceStudySource) {
	sentenceStudyWordFlags(line)
	sentenceStudyDegraded(line, tokens, alignments, source)
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

func sentenceStudyDegraded(line *sentenceStudyLine, tokens []domain.SentenceStudyToken, alignments []sentenceStudyAlignment, source sentenceStudySource) {
	seen := make(map[string]struct{})
	for index, alignment := range alignments {
		if alignment.safe || !sentenceStudyTokenIsWord(tokens[index]) {
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

// sentenceStudyAlignments decides every token's alignment (see
// sentenceStudyTokenAlignment), then clears any safe token whose stored span
// overlaps an in-bounds sibling. The overlap test considers every in-bounds
// sibling, safe or not, on the stored spans, so a shared legacy span (a
// contraction and its words) never lets one word claim the region while the
// other silently disappears.
func sentenceStudyAlignments(tokens []domain.SentenceStudyToken, source sentenceStudySource) []sentenceStudyAlignment {
	alignments := make([]sentenceStudyAlignment, len(tokens))
	for index, token := range tokens {
		alignments[index] = sentenceStudyTokenAlignment(token, source)
	}
	for index := range alignments {
		if !alignments[index].safe {
			continue
		}
		for other := range tokens {
			if index == other || !source.contains(tokens[other].StartOffset, tokens[other].EndOffset) {
				continue
			}
			if sentenceStudySpansOverlap(tokens[index], tokens[other]) {
				alignments[index].safe = false
				break
			}
		}
	}
	return alignments
}

// sentenceStudySegments walks the trusted source range, emitting an aligned word
// for every safe span and the exact source text for everything else, so the
// sentence is never reconstructed from word surfaces. A word whose span covers
// stripped edge decoration emits the trimmed word unit and the stripped edges as
// plain source text.
func sentenceStudySegments(study domain.SentenceStudy, source sentenceStudySource, alignments []sentenceStudyAlignment) []sentenceStudySegment {
	order := make([]int, 0, len(study.Tokens))
	for index := range alignments {
		if alignments[index].safe {
			order = append(order, index)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return int(alignments[a].start - alignments[b].start)
	})

	segments := make([]sentenceStudySegment, 0, 2*len(order)+1)
	cursor := source.start
	for _, index := range order {
		alignment := alignments[index]
		if alignment.start < cursor {
			continue // Defensive: a safe span never reorders the source.
		}
		if alignment.start > cursor {
			segments = append(segments, sentenceStudySegment{Text: source.slice(cursor, alignment.start)})
		}
		segments = append(segments, sentenceStudyWordSegment(study, study.Tokens[index], alignment, source))
		cursor = alignment.end
	}
	if cursor < source.end {
		segments = append(segments, sentenceStudySegment{Text: source.slice(cursor, source.end)})
	}
	return segments
}

// sentenceStudyWordSegment builds one aligned unit over its decided sub-span.
// Punctuation and symbols cover source text but carry no grammatical features,
// so they stay plain.
func sentenceStudyWordSegment(study domain.SentenceStudy, token domain.SentenceStudyToken, alignment sentenceStudyAlignment, source sentenceStudySource) sentenceStudySegment {
	text := source.slice(alignment.start, alignment.end)
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

// sentenceStudyTokenAlignment decides the source sub-span a token may claim as
// an aligned unit, and whether it may be aligned at all. A token is safe when
// its stored span is a valid, non-empty, in-bounds range whose source slice
// matches the stored surface. A word is also safe when the slice differs from
// the surface only by leading and/or trailing edge decorations that the
// producer strips from the persisted surface (see _clean_surface in
// nlp/src/mouseion_nlp/producer.py); its sub-span is then narrowed to the
// trimmed slice that equals the surface. Punctuation and symbol tokens keep
// exact-match only, so their plain source runs are unchanged. Any other
// disagreement is unsafe.
func sentenceStudyTokenAlignment(token domain.SentenceStudyToken, source sentenceStudySource) sentenceStudyAlignment {
	start, end := token.StartOffset, token.EndOffset
	if !source.contains(start, end) {
		return sentenceStudyAlignment{}
	}
	if source.slice(start, end) != token.Surface {
		if !sentenceStudyTokenIsWord(token) {
			return sentenceStudyAlignment{}
		}
		start, end = sentenceStudyTrimEdgeDecorations(start, end, source)
	}
	if start < end && source.slice(start, end) == token.Surface {
		return sentenceStudyAlignment{start: start, end: end, safe: true}
	}
	return sentenceStudyAlignment{}
}

// sentenceStudyTrimEdgeDecorations narrows a span past the leading and trailing
// edge decorations the producer strips from a persisted surface. It mirrors
// _clean_surface in nlp/src/mouseion_nlp/producer.py: Unicode punctuation (P*)
// and symbols (S*) are decorations, except the apostrophes ' (U+0027) and
// ’ (U+2019), which are lexical in elided forms such as Italian dell'.
func sentenceStudyTrimEdgeDecorations(start, end int64, source sentenceStudySource) (int64, int64) {
	for start < end && sentenceStudyEdgeDecoration(source.runes[start]) {
		start++
	}
	for end > start && sentenceStudyEdgeDecoration(source.runes[end-1]) {
		end--
	}
	return start, end
}

// sentenceStudyEdgeDecoration reports whether a rune is stripped from a surface
// edge by the producer's edge-cleaning rule. Go's unicode.IsPunct and
// unicode.IsSymbol correspond to the Unicode P and S general categories the
// producer tests; the apostrophes are the deliberate lexical exception.
func sentenceStudyEdgeDecoration(r rune) bool {
	return r != '\'' && r != '\u2019' && (unicode.IsPunct(r) || unicode.IsSymbol(r))
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
// explanation>". The explanation says what the term means, not just its
// expanded name, because the notation key is where an unfamiliar term stops
// being the entry price. A value with no Leipzig abbreviation keeps its
// spelled-out word as the abbreviation. Italian Tense=Imp (imperfect) and Greek
// Aspect=Imp (imperfective) are deliberately distinct.
const sentenceStudyAbbreviationTable = `
Person 1 1 first person
Person 2 2 second person
Person 3 3 third person
Number Sing SG singular
Number Plur PL plural
Gender Masc M masculine
Gender Fem F feminine
Gender Neut N neuter
Case Nom NOM nominative case, the case of the subject
Case Acc ACC accusative case, the case of the direct object
Case Dat DAT dative case, the case of the indirect object or recipient
Case Gen GEN genitive case, the case of possession or belonging
Case Voc VOC vocative case, the case used when addressing someone
Tense Pres PRS present tense, for what happens now or is generally true
Tense Past PST past tense, for what happened before now
Tense Fut FUT future tense, for what has not happened yet
Tense Imp imperfect imperfect tense, an ongoing or repeated past situation
Tense Pqp pluperfect pluperfect tense, a past situation before another past situation
Aspect Imp IPFV imperfective aspect, an ongoing or repeated situation without a stated endpoint
Aspect Perf PFV perfective aspect, a situation viewed as a complete whole
Mood Ind IND indicative mood, stating what is the case
Mood Sub SBJV subjunctive mood, for possibility, wish or doubt
Mood Imp IMP imperative mood, giving a command or request
Mood Cnd COND conditional mood, for what depends on a condition
Voice Act active active voice, where the subject performs the action
Voice Pass PASS passive voice, where the subject is acted on
VerbForm Fin finite finite verb form, carrying tense and agreeing with the subject
VerbForm Inf INF infinitive, the verb's dictionary form
VerbForm Part PTCP participle, a verb form used like an adjective or in compound tenses
VerbForm Ger gerund gerund, a verb form used like a noun
VerbForm Conv CVB converb, a verb form that links clauses
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

// sentenceStudyKey lists only the abbreviations and spelled-out labels used in
// this line, sorted deterministically, each with a plain-English explanation of
// the term rather than just its expanded name.
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
