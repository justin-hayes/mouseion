package ancientgreek

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// sourceDocument is the parsed oga_src CoNLL-U file, before the citation cut.
type sourceDocument struct {
	sentences []sourceSentence
}

// sourceSentence is one sentence with its source header and tokens.
type sourceSentence struct {
	source string
	tokens []sourceToken
}

// sourceToken is one source word or ellipsis token.
type sourceToken struct {
	surface  string
	lemma    string
	pos      string
	postag   string
	relation string
	head     int
	citation string
}

// parseOgaSrc parses the single-file CoNLL-U packaging of oga_src: the three
// sentence headers (source, text, sent_id) and ten-tab token lines. It is
// strict: an unexpected comment, a token line outside a sentence, a malformed
// column count, a missing citation reference or a non-nine-character postag
// stops the import with source_malformed.
func parseOgaSrc(data []byte) (*sourceDocument, error) {
	parser := ogaSrcParser{doc: &sourceDocument{}}
	for index, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if err := parser.acceptLine(line, index+1); err != nil {
			return nil, err
		}
	}
	return parser.finish()
}

// ogaSrcParser is the line-by-line state machine of parseOgaSrc.
type ogaSrcParser struct {
	doc     *sourceDocument
	current *sourceSentence
	header  int // 0 source, 1 text, 2 sent_id, 3 tokens
}

// acceptLine feeds one line to the state machine.
func (p *ogaSrcParser) acceptLine(line string, lineNo int) error {
	if line == "" {
		return p.endSentence(lineNo)
	}
	if body, ok := strings.CutPrefix(line, "#"); ok {
		return p.acceptHeader(body, lineNo)
	}
	return p.acceptToken(line, lineNo)
}

// endSentence closes the current sentence, if any.
func (p *ogaSrcParser) endSentence(lineNo int) error {
	if p.current != nil {
		if p.header < 3 {
			return malformed(lineNo, "incomplete sentence header")
		}
		p.doc.sentences = append(p.doc.sentences, *p.current)
		p.current = nil
	}
	p.header = 0
	return nil
}

// acceptHeader advances the sentence header state.
func (p *ogaSrcParser) acceptHeader(body string, lineNo int) error {
	name, value, _ := strings.Cut(body, " = ")
	name = strings.TrimSpace(name)
	switch {
	case p.header == 0 && name == "source":
		p.current = &sourceSentence{source: value}
		p.header = 1
	case p.header == 1 && name == "text":
		p.header = 2
	case p.header == 2 && name == "sent_id":
		p.header = 3
	default:
		return malformed(lineNo, "unexpected comment line")
	}
	return nil
}

// acceptToken appends one token to the current sentence.
func (p *ogaSrcParser) acceptToken(line string, lineNo int) error {
	if p.header != 3 || p.current == nil {
		return malformed(lineNo, "token line outside a sentence")
	}
	token, err := parseTokenLine(line, lineNo)
	if err != nil {
		return err
	}
	p.current.tokens = append(p.current.tokens, token)
	return nil
}

// finish closes the final sentence and returns the document.
func (p *ogaSrcParser) finish() (*sourceDocument, error) {
	if p.current != nil {
		if p.header < 3 {
			return nil, malformed(0, "incomplete sentence header")
		}
		p.doc.sentences = append(p.doc.sentences, *p.current)
	}
	if len(p.doc.sentences) == 0 {
		return nil, malformed(0, "no sentences")
	}
	return p.doc, nil
}

// malformed builds a source_malformed refusal.
func malformed(lineNo int, detail string) error {
	return &ImportError{Code: CodeMalformed, Line: lineNo, Detail: detail}
}

// parseTokenLine reads one ten-column token line.
func parseTokenLine(line string, lineNo int) (sourceToken, error) {
	columns := strings.Split(line, "\t")
	if len(columns) != 10 {
		return sourceToken{}, malformed(lineNo, "token line does not have ten columns")
	}
	if _, err := strconv.Atoi(columns[0]); err != nil {
		return sourceToken{}, malformed(lineNo, "token id is not a number")
	}
	if columns[1] == "" {
		return sourceToken{}, malformed(lineNo, "empty surface")
	}
	if len(columns[4]) != 9 {
		return sourceToken{}, malformed(lineNo, "postag is not nine characters")
	}
	head, err := strconv.Atoi(columns[6])
	if err != nil || head < 0 {
		return sourceToken{}, malformed(lineNo, "head is not a non-negative number")
	}
	citation, ok := citationFromMisc(columns[9])
	if !ok {
		return sourceToken{}, malformed(lineNo, "missing citation reference")
	}
	return sourceToken{
		surface:  columns[1],
		lemma:    columns[2],
		pos:      columns[3],
		postag:   columns[4],
		relation: columns[7],
		head:     head,
		citation: citation,
	}, nil
}

// citationFromMisc extracts the citation from an oga_src MISC field of the form
// "<parent>|<grandparent>|ref=<abbreviation>_<citation>".
func citationFromMisc(misc string) (string, bool) {
	_, ref, ok := strings.CutLast(misc, "|ref=")
	if !ok {
		return "", false
	}
	_, citation, ok := strings.CutLast(ref, "_")
	if !ok || citation == "" {
		return "", false
	}
	return citation, true
}

// interpretation is one versioned reading of a source document: the
// reconstructed full text, the source-neutral analyzer result and the report
// counts.
type interpretation struct {
	FullText string
	Result   analyzer.Result
	Report   ImportReport
}

// uposProperName is the one mapped part of speech the report counts.
const uposProperName = "PROPN"

// interpret cuts the source to the definition's citation range and turns it into
// one source-neutral candidate. The proper-name rule is computed per lemma over
// the whole parsed document; every other rule applies to the included tokens.
func interpret(doc *sourceDocument, definition domain.ReadingUnitDefinition, unitID string) (interpretation, error) {
	profile, err := canonicalization.For(definition.Language)
	if err != nil {
		return interpretation{}, &ImportError{Code: CodeUnknownInterpret, Detail: "no canonicalization profile"}
	}
	start, ok := parseCitation(definition.CitationStart)
	if !ok {
		return interpretation{}, &ImportError{Code: CodeUnknownInterpret, Detail: "citation start is not a citation"}
	}
	end, ok := parseCitation(definition.CitationEnd)
	if !ok {
		return interpretation{}, &ImportError{Code: CodeUnknownInterpret, Detail: "citation end is not a citation"}
	}
	if err := validateNativePOS(doc); err != nil {
		return interpretation{}, err
	}
	names := properNameStats(doc, profile)
	state := newInterpretState(definition, unitID)
	for _, sentence := range doc.sentences {
		if err := state.addSentence(sentence, profile, names, start, end); err != nil {
			return interpretation{}, err
		}
	}
	return state.interpretation(), nil
}

// interpretState accumulates the cut, rebuilt sentences and report counts.
type interpretState struct {
	unitID      string
	full        []rune
	result      analyzer.Result
	report      ImportReport
	unmapped    map[FeatureValueCount]int
	numbered    map[string]struct{}
	hasBare     map[string]struct{}
	hasNumbered map[string]struct{}
}

func newInterpretState(definition domain.ReadingUnitDefinition, unitID string) *interpretState {
	return &interpretState{
		unitID: unitID,
		result: analyzer.Result{
			SchemaVersion: "1",
			Language:      definition.Language,
			Analysis: analyzer.AnalysisProvenance{
				AnalyzerName:    ImportAnalyzerName,
				AnalyzerVersion: InterpretationVersion(definition),
			},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "ancient-greek", Version: "1"},
		},
		unmapped:    map[FeatureValueCount]int{},
		numbered:    map[string]struct{}{},
		hasBare:     map[string]struct{}{},
		hasNumbered: map[string]struct{}{},
	}
}

// addSentence includes and rebuilds one sentence, or skips it.
func (s *interpretState) addSentence(sentence sourceSentence, profile canonicalization.Profile, names map[string]nameStat, start, end []int) error {
	if len(sentence.tokens) == 0 {
		return nil
	}
	located, err := sentenceLocation(sentence, start, end)
	if err != nil {
		return err
	}
	if located == locationOutside {
		return nil
	}
	relative := reconstruct(sentence)
	if len(s.full) > 0 {
		s.full = append(s.full, '\n')
	}
	sentenceStart := len(s.full)
	s.full = append(s.full, []rune(relative.text)...)
	tokens := make([]analyzer.Token, 0, len(relative.kept))
	properNames := 0
	for index, token := range relative.kept {
		built, mappedUPOS, err := s.buildToken(token, profile, names)
		if err != nil {
			return err
		}
		if mappedUPOS == uposProperName {
			properNames++
		}
		//nolint:gosec // sentenceStart and the relative spans are non-negative by construction.
		built.Location = analyzer.SourceLocation{SourceDocumentID: s.unitID, StartOffset: uint64(sentenceStart + relative.spans[index][0]), EndOffset: uint64(sentenceStart + relative.spans[index][1])}
		tokens = append(tokens, built)
	}
	sentenceEnd := uint64(sentenceStart + len([]rune(relative.text)))
	s.result.Sentences = append(s.result.Sentences, analyzer.Sentence{
		Text:     relative.text,
		Tokens:   tokens,
		Location: analyzer.SourceLocation{SourceDocumentID: s.unitID, StartOffset: uint64(sentenceStart), EndOffset: sentenceEnd},
		// The passage context of ADR 0101 needs every section the sentence
		// touches, so a sentence crossing 1.2.17-18 carries both.
		Citations: sentenceCitations(sentence),
	})
	s.report.Sentences++
	s.report.Tokens += len(tokens)
	s.report.EllipsisTokensSkipped += relative.ellipsis
	s.report.ProperNamesDerived += properNames
	return nil
}

// buildToken maps one source token to a source-neutral token and records its
// lemma-identity statistics.
func (s *interpretState) buildToken(token sourceToken, profile canonicalization.Profile, names map[string]nameStat) (analyzer.Token, string, error) {
	headword, discriminator := decodeLemma(token.lemma)
	canonical := profile.Canonical(headword)
	key := canonical + "\x00" + discriminator
	upos, err := mapUPOS(token.pos, token.relation, names[key].isProperName())
	if err != nil {
		return analyzer.Token{}, "", err
	}
	morphology, skipped := mapFeatures(token.postag)
	for _, count := range skipped {
		s.unmapped[count]++
	}
	if discriminator == "" {
		s.hasBare[canonical] = struct{}{}
	} else {
		s.numbered[key] = struct{}{}
		s.hasNumbered[canonical] = struct{}{}
	}
	return analyzer.Token{
		Surface:              token.surface,
		RawLemma:             token.lemma,
		CanonicalLemma:       canonical,
		UPOS:                 upos,
		Dependency:           absentSyntaxDependency,
		Head:                 0,
		Morphology:           morphology,
		Headword:             headword,
		LexicalDiscriminator: discriminator,
		// The form search key is the profile key of the surface form: NFC,
		// simple lowercase, final sigma to sigma, NFC. Concordance form search
		// matches this stored key for grc, never SQL lower(), so NFD, case and
		// final-sigma variants unify while accents stay significant.
		SearchKey: profile.Canonical(token.surface),
	}, upos, nil
}

// interpretation finishes the accumulated report counts.
func (s *interpretState) interpretation() interpretation {
	s.report.NumberedLemmaTypes = len(s.numbered)
	for canonical := range s.hasBare {
		if _, ok := s.hasNumbered[canonical]; ok {
			s.report.BareLemmasWithNumberedSibling++
		}
	}
	s.report.UnmappedFeatureValues = sortedFeatureCounts(s.unmapped)
	return interpretation{FullText: string(s.full), Result: s.result, Report: s.report}
}

// locationDecision is the citation-cut decision for one sentence.
type locationDecision int

const (
	locationInside locationDecision = iota
	locationOutside
)

// sentenceLocation decides whether every token of a sentence falls within the
// citation range, or only outside it. A sentence that both enters and leaves the
// range stops the import.
func sentenceLocation(sentence sourceSentence, start, end []int) (locationDecision, error) {
	inside, outside := 0, 0
	for _, token := range sentence.tokens {
		citation, ok := parseCitation(token.citation)
		if !ok {
			return locationOutside, malformed(0, "token citation is not a citation")
		}
		if compareCitation(citation, start) >= 0 && compareCitation(citation, end) <= 0 {
			inside++
		} else {
			outside++
		}
	}
	switch {
	case outside == 0:
		return locationInside, nil
	case inside == 0:
		return locationOutside, nil
	default:
		return locationOutside, &ImportError{Code: CodeCitationStraddle}
	}
}

// reconstructed is one sentence's reconstructed text and the spans of its
// printed tokens, relative to that text.
type reconstructed struct {
	text     string
	kept     []sourceToken
	spans    [][2]int
	ellipsis int
}

// reconstruct joins a sentence's printed tokens with single spaces, omitting the
// space before a punctuation mark and dropping ellipsis tokens.
func reconstruct(sentence sourceSentence) reconstructed {
	kept := make([]sourceToken, 0, len(sentence.tokens))
	ellipsis := 0
	for _, token := range sentence.tokens {
		if isEllipsis(token.pos) {
			ellipsis++
			continue
		}
		kept = append(kept, token)
	}
	var text []rune
	spans := make([][2]int, len(kept))
	for index, token := range kept {
		if index > 0 && !noSpaceBefore(token.surface) {
			text = append(text, ' ')
		}
		spans[index][0] = len(text)
		text = append(text, []rune(token.surface)...)
		spans[index][1] = len(text)
	}
	return reconstructed{text: string(text), kept: kept, spans: spans, ellipsis: ellipsis}
}

// absentSyntaxDependency is the non-committal relation stored for a run that
// records syntax as absent. The run's syntax state carries the decision; the
// value only satisfies the corpus-token checks and is never read as UD.
const absentSyntaxDependency = "dep"

// nativePOSValues are the native AGDT parts of speech oga-agdt-upos/4 maps.
// Every token in the whole parsed document is checked, including tokens outside
// the imported citation range, so an unknown value can never pass unnoticed.
var nativePOSValues = map[string]struct{}{
	"n": {}, "v": {}, "a": {}, "d": {}, "l": {}, "g": {},
	"c": {}, "r": {}, "p": {}, "m": {}, "i": {}, "u": {},
}

// validateNativePOS refuses the whole document when any token carries a native
// part of speech the mapping does not list.
func validateNativePOS(doc *sourceDocument) error {
	for _, sentence := range doc.sentences {
		for _, token := range sentence.tokens {
			if isEllipsis(token.pos) {
				continue
			}
			if _, ok := nativePOSValues[token.pos]; !ok {
				return &ImportError{Code: CodeUnknownPOS, Detail: asciiValue(token.pos)}
			}
		}
	}
	return nil
}

// mapUPOS applies oga-agdt-upos/4: the native AGDT part of speech becomes a UD
// part of speech. An unlisted native value stops the import.
func mapUPOS(pos, relation string, properName bool) (string, error) {
	switch pos {
	case "n":
		if properName {
			return uposProperName, nil
		}
		return "NOUN", nil
	case "v":
		return "VERB", nil
	case "a":
		return "ADJ", nil
	case "d":
		return "ADV", nil
	case "l":
		return "DET", nil
	case "g":
		return "PART", nil
	case "c":
		if relation == "AuxC" {
			return "SCONJ", nil
		}
		return "CCONJ", nil
	case "r":
		return "ADP", nil
	case "p":
		return "PRON", nil
	case "m":
		return "NUM", nil
	case "i":
		return "INTJ", nil
	case "u":
		return "PUNCT", nil
	default:
		return "", &ImportError{Code: CodeUnknownPOS, Detail: asciiValue(pos)}
	}
}

// featureSlots maps the AGDT postag positions to UD feature names.
var featureSlots = []struct {
	feature string
	index   int
}{
	{"Person", 1},
	{"Number", 2},
	{"Tense", 3},
	{"Mood", 4},
	{"Voice", 5},
	{"Gender", 6},
	{"Case", 7},
	{"Degree", 8},
}

// featureMappings is oga-agdt-feats/1: the AGDT letter-coded value of each
// feature becomes its UD value. A value with no UD counterpart is omitted and
// counted; a "-" means the category does not apply and is neither.
var featureMappings = map[string]map[byte]string{
	"Person": {'1': "1", '2': "2", '3': "3"},
	"Number": {'s': "Sing", 'p': "Plur", 'd': "Dual"},
	"Tense":  {'p': "Pres", 'i': "Imp", 'f': "Fut", 'l': "Pqp", 'a': "Past"},
	"Mood":   {'i': "Ind", 's': "Sub", 'o': "Opt", 'm': "Imp"},
	"Voice":  {'a': "Act", 'p': "Pass", 'm': "Mid", 'e': "Mid"},
	"Gender": {'m': "Masc", 'f': "Fem", 'n': "Neut"},
	"Case":   {'n': "Nom", 'g': "Gen", 'd': "Dat", 'a': "Acc", 'v': "Voc", 'l': "Loc"},
	"Degree": {'c': "Cmp", 's': "Sup"},
}

// mapFeatures maps one token's postag to UD FEATS and reports every value the
// mapping leaves out.
func mapFeatures(postag string) (map[string]string, []FeatureValueCount) {
	morphology := map[string]string{}
	var unmapped []FeatureValueCount
	for _, slot := range featureSlots {
		value := postag[slot.index]
		if value == '-' {
			continue
		}
		if mapped, ok := featureMappings[slot.feature][value]; ok {
			morphology[slot.feature] = mapped
			continue
		}
		unmapped = append(unmapped, FeatureValueCount{Feature: slot.feature, Value: asciiValue(string(value))})
	}
	return morphology, unmapped
}

// decodeLemma splits a trailing homograph number off a source lemma. The
// headword keeps the diacritics; the number becomes the morpheus discriminator.
func decodeLemma(lemma string) (headword, discriminator string) {
	index := len(lemma)
	for index > 0 && lemma[index-1] >= '0' && lemma[index-1] <= '9' {
		index--
	}
	if index == len(lemma) || index == 0 {
		return lemma, ""
	}
	return lemma[:index], "morpheus:" + lemma[index:]
}

// isEllipsis reports whether a native part of speech marks an elliptical node.
func isEllipsis(pos string) bool {
	return pos == "-" || pos == "x"
}

// noSpaceBefore reports whether a token whose surface starts with rune must not
// be preceded by a space in the reconstructed sentence text.
func noSpaceBefore(surface string) bool {
	first, _ := utf8.DecodeRuneInString(surface)
	switch first {
	case ',', '.', ';', '\u00b7', '\u037e', '\u0387':
		return true
	default:
		return false
	}
}

// capitalized reports whether a surface begins with an uppercase letter.
func capitalized(surface string) bool {
	first, _ := utf8.DecodeRuneInString(surface)
	return unicode.IsUpper(first)
}

// nameStat records one lemma identity's capitalization over the whole document.
type nameStat struct {
	total, initial, initialCap, nonInitial, nonInitialCap int
}

// isProperName applies the proper-name rule, which errs toward PROPN.
func (s nameStat) isProperName() bool {
	if s.nonInitial > 0 && s.nonInitialCap*2 >= s.nonInitial {
		return true
	}
	return s.total > 0 && s.initial == s.total && s.initialCap == s.total
}

// properNameStats computes, per canonical lemma identity, the capitalization a
// noun shows over the whole document, whether or not the token is in the
// imported range.
func properNameStats(doc *sourceDocument, profile canonicalization.Profile) map[string]nameStat {
	stats := map[string]nameStat{}
	for _, sentence := range doc.sentences {
		first := firstKeptIndex(sentence)
		for index, token := range sentence.tokens {
			if token.pos != "n" {
				continue
			}
			headword, discriminator := decodeLemma(token.lemma)
			key := profile.Canonical(headword) + "\x00" + discriminator
			recordName(stats, key, index == first, capitalized(token.surface))
		}
	}
	return stats
}

// firstKeptIndex is the index of a sentence's first printed token, or -1.
func firstKeptIndex(sentence sourceSentence) int {
	for index, token := range sentence.tokens {
		if !isEllipsis(token.pos) {
			return index
		}
	}
	return -1
}

// recordName counts one noun occurrence toward its identity's name statistics.
func recordName(stats map[string]nameStat, key string, sentenceInitial, capitalized bool) {
	stat := stats[key]
	stat.total++
	if sentenceInitial {
		stat.initial++
		if capitalized {
			stat.initialCap++
		}
	} else {
		stat.nonInitial++
		if capitalized {
			stat.nonInitialCap++
		}
	}
	stats[key] = stat
}

// sentenceCitations returns every distinct cited section a source sentence
// touches, in citation order. A sentence crossing 1.2.17-18 carries both.
func sentenceCitations(sentence sourceSentence) []string {
	seen := make(map[string]struct{}, len(sentence.tokens))
	citations := make([]string, 0, len(sentence.tokens))
	for _, token := range sentence.tokens {
		if token.citation == "" {
			continue
		}
		if _, duplicate := seen[token.citation]; duplicate {
			continue
		}
		seen[token.citation] = struct{}{}
		citations = append(citations, token.citation)
	}
	sort.SliceStable(citations, func(i, j int) bool {
		left, leftOK := parseCitation(citations[i])
		right, rightOK := parseCitation(citations[j])
		if !leftOK || !rightOK {
			return citations[i] < citations[j]
		}
		return compareCitation(left, right) < 0
	})
	return citations
}

// parseCitation splits a dotted citation such as "1.10.19" into numbers.
func parseCitation(value string) ([]int, bool) {
	parts := strings.Split(value, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return nil, false
		}
		out = append(out, number)
	}
	return out, true
}

// compareCitation compares two citations component-wise, treating a missing
// component as zero.
func compareCitation(a, b []int) int {
	length := max(len(a), len(b))
	for index := range length {
		av, bv := 0, 0
		if index < len(a) {
			av = a[index]
		}
		if index < len(b) {
			bv = b[index]
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

// sortedFeatureCounts turns the unmapped-value tally into a sorted slice.
func sortedFeatureCounts(counts map[FeatureValueCount]int) []FeatureValueCount {
	out := make([]FeatureValueCount, 0, len(counts))
	for item, count := range counts {
		item.Count = count
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Feature != out[j].Feature {
			return out[i].Feature < out[j].Feature
		}
		return out[i].Value < out[j].Value
	})
	return out
}

// asciiValue keeps only an ASCII value, so no source text can reach a report or
// a log through an error detail.
func asciiValue(value string) string {
	if value == "" {
		return "empty"
	}
	for _, r := range value {
		if r > unicode.MaxASCII || !unicode.IsGraphic(r) {
			return "non-ascii"
		}
	}
	return value
}

// validate checks that the interpretation is self-consistent: sentence bounds
// slice the full text exactly, token spans sit inside their sentence, and no two
// tokens share a span.
func (i interpretation) validate() error {
	runes := []rune(i.FullText)
	for index, sentence := range i.Result.Sentences {
		if err := validateSentence(index, sentence, runes); err != nil {
			return err
		}
	}
	return nil
}

// validateSentence checks one sentence's span and each of its tokens.
func validateSentence(index int, sentence analyzer.Sentence, runes []rune) error {
	//nolint:gosec // offsets are small and non-negative; the bounds check below rejects the rest.
	start, end := int(sentence.Location.StartOffset), int(sentence.Location.EndOffset)
	if start < 0 || end < start || end > len(runes) {
		return fmt.Errorf("ancientgreek: import produced an invalid sentence span %d", index)
	}
	if string(runes[start:end]) != sentence.Text {
		return fmt.Errorf("ancientgreek: import sentence %d does not match its span", index)
	}
	last := start
	for _, token := range sentence.Tokens {
		//nolint:gosec // offsets are small and non-negative; the bounds check below rejects the rest.
		tokenStart, tokenEnd := int(token.Location.StartOffset), int(token.Location.EndOffset)
		if tokenStart < start || tokenEnd <= tokenStart || tokenEnd > end {
			return fmt.Errorf("ancientgreek: import token span leaves its sentence %d", index)
		}
		if string(runes[tokenStart:tokenEnd]) != token.Surface {
			return fmt.Errorf("ancientgreek: import token surface disagrees with its span %d", index)
		}
		if tokenStart < last {
			return fmt.Errorf("ancientgreek: import tokens share a span %d", index)
		}
		last = tokenEnd
	}
	return nil
}
