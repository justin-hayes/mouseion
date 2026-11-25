package ancientgreek

import (
	"sort"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/classical"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// UnitRange is one Reading unit's inclusive citation range in a whole-work
// check. Key is reported back and may be empty.
type UnitRange struct {
	Key   string
	Start string
	End   string
}

// SentenceCrossing reports a source sentence whose tokens touch more than one
// Reading unit. It lists the citations the sentence touches in citation order;
// it never carries sentence text.
type SentenceCrossing struct {
	UnitKey   string
	Citations []string
}

// CoverageReport is the outcome of one whole-work sentence-ownership check.
type CoverageReport struct {
	Sentences int
	Crossings []SentenceCrossing
}

// CheckUnitCoverage parses a whole oga_src file and checks that every
// sentence's first printed token belongs to exactly one of the supplied
// Reading units. A sentence in no unit or in more than one stops the check with
// a stable code. A sentence that crosses a unit boundary is assigned to the
// unit holding its first token and reported as a crossing.
func CheckUnitCoverage(data []byte, units []UnitRange) (CoverageReport, error) {
	source, err := ParseManifestSource(data)
	if err != nil {
		return CoverageReport{}, err
	}
	return source.CheckUnitCoverage(units)
}

// checkDocumentCoverage is CheckUnitCoverage over a parsed document.
func checkDocumentCoverage(doc *sourceDocument, units []UnitRange) (CoverageReport, error) {
	ranges, err := parseUnitRanges(units)
	if err != nil {
		return CoverageReport{}, err
	}
	if err := validateNativePOS(doc); err != nil {
		return CoverageReport{}, err
	}
	report := CoverageReport{}
	for _, sentence := range doc.sentences {
		owner, crossing, err := sentenceCoverage(sentence, ranges)
		if err != nil {
			return CoverageReport{}, err
		}
		if owner < 0 {
			continue
		}
		report.Sentences++
		if crossing {
			report.Crossings = append(report.Crossings, SentenceCrossing{UnitKey: ranges[owner].key, Citations: sentenceCitations(sentence)})
		}
	}
	return report, nil
}

// parseUnitRanges validates and parses the supplied Reading-unit ranges.
func parseUnitRanges(units []UnitRange) ([]parsedUnitRange, error) {
	if len(units) == 0 {
		return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "no reading units"}
	}
	ranges := make([]parsedUnitRange, len(units))
	for index, unit := range units {
		start, ok := parseCitation(unit.Start)
		if !ok {
			return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "unit citation start is not a citation"}
		}
		end, ok := parseCitation(unit.End)
		if !ok || compareCitation(start, end) > 0 {
			return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "unit citation end is not a citation"}
		}
		ranges[index] = parsedUnitRange{key: unit.Key, start: start, end: end}
	}
	return ranges, nil
}

// sentenceCoverage returns the unit that owns a sentence, -1 when the sentence
// has no printed token, and whether a later token leaves the owning unit.
func sentenceCoverage(sentence sourceSentence, ranges []parsedUnitRange) (int, bool, error) {
	first, ok := firstPrintedToken(sentence)
	if !ok {
		return -1, false, nil
	}
	citation, ok := parseCitation(first.citation)
	if !ok {
		return -1, false, malformed(0, "token citation is not a citation")
	}
	owner := -1
	for index, unit := range ranges {
		if !citationWithin(citation, unit) {
			continue
		}
		if owner >= 0 {
			return -1, false, &ImportError{Code: CodeSentenceAmbiguous, Detail: asciiValue(first.citation)}
		}
		owner = index
	}
	if owner < 0 {
		return -1, false, &ImportError{Code: CodeSentenceUnassigned, Detail: asciiValue(first.citation)}
	}
	crossing, err := sentenceCrosses(sentence, ranges[owner])
	if err != nil {
		return -1, false, err
	}
	return owner, crossing, nil
}

// sentenceCrosses reports whether any printed token leaves the owning unit. It
// also validates every token's citation.
func sentenceCrosses(sentence sourceSentence, unit parsedUnitRange) (bool, error) {
	crossing := false
	for _, token := range sentence.tokens {
		if isEllipsis(token.pos) {
			continue
		}
		citation, ok := parseCitation(token.citation)
		if !ok {
			return false, malformed(0, "token citation is not a citation")
		}
		if !citationWithin(citation, unit) {
			crossing = true
		}
	}
	return crossing, nil
}

// parsedUnitRange is one validated UnitRange.
type parsedUnitRange struct {
	key        string
	start, end classical.Citation
}

// citationWithin reports whether a citation lies inside a unit's inclusive
// range.
func citationWithin(citation classical.Citation, unit parsedUnitRange) bool {
	return citationWithinRange(citation, unit.start, unit.end)
}

// firstPrintedToken returns a sentence's first non-ellipsis token.
func firstPrintedToken(sentence sourceSentence) (sourceToken, bool) {
	for _, token := range sentence.tokens {
		if !isEllipsis(token.pos) {
			return token, true
		}
	}
	return sourceToken{}, false
}

// ProposeUnits parses a whole oga_src file and proposes one Reading unit per
// distinct top-level citation number, in ascending order. Each proposed range
// covers every printed token whose citation begins with that number, so a
// sentence that crosses a unit boundary belongs to the unit holding its first
// token while both ranges still cover the Edition exactly. The caller supplies
// each unit's permanent key; proposals carry none.
func ProposeUnits(data []byte) ([]UnitRange, error) {
	doc, err := parseManifestDocument(data)
	if err != nil {
		return nil, err
	}
	return proposeDocumentUnits(doc)
}

// proposeDocumentUnits is ProposeUnits over a parsed document.
func proposeDocumentUnits(doc *sourceDocument) ([]UnitRange, error) {
	books, err := citationExtents(doc)
	if err != nil {
		return nil, err
	}
	if len(books) == 0 {
		return nil, &ImportError{Code: CodeEmptyCitationRange, Detail: "no cited tokens"}
	}
	numbers := make([]int, 0, len(books))
	for number := range books {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	units := make([]UnitRange, 0, len(numbers))
	for _, number := range numbers {
		extent := books[number]
		units = append(units, UnitRange{Start: citationString(extent.start), End: citationString(extent.end)})
	}
	return units, nil
}

// citationExtent is the first and last citation seen for one top-level number.
type citationExtent struct {
	start, end classical.Citation
}

// citationExtents accumulates every printed token's citation under its
// top-level number.
func citationExtents(doc *sourceDocument) (map[int]citationExtent, error) {
	books := map[int]citationExtent{}
	for _, sentence := range doc.sentences {
		for _, token := range sentence.tokens {
			if isEllipsis(token.pos) {
				continue
			}
			citation, ok := parseCitation(token.citation)
			if !ok || len(citation) == 0 {
				return nil, malformed(0, "token citation is not a citation")
			}
			if citation[0].Name != "" {
				return nil, malformed(0, "token citation has a named top-level section")
			}
			addCitationExtent(books, citation[0].Number, citation)
		}
	}
	return books, nil
}

// addCitationExtent widens one top-level number's citation extent.
func addCitationExtent(books map[int]citationExtent, book int, citation classical.Citation) {
	current, seen := books[book]
	if !seen {
		books[book] = citationExtent{start: citation, end: citation}
		return
	}
	if compareCitation(citation, current.start) < 0 {
		current.start = citation
	}
	if compareCitation(citation, current.end) > 0 {
		current.end = citation
	}
	books[book] = current
}

// UnitSummary is the source-derived facts of one Reading unit.
type UnitSummary struct {
	WordCount int
	Opening   string
}

// SummarizeUnit derives one Reading unit's word count and Opening in one parse.
// It counts every non-ellipsis token except punctuation, so a sentence crossing
// the unit boundary counts whole toward the unit holding its first token. It
// also refuses a range whose boundary citations do not resolve to real tokens,
// and checks every token's native part of speech.
// The Opening is the first sentence the unit owns that is not a speaker label,
// reconstructed exactly as the source prints it. A unit with no other sentence
// refuses rather than guessing one.
func SummarizeUnit(data []byte, start, end string) (UnitSummary, error) {
	doc, err := parseManifestDocument(data)
	if err != nil {
		return UnitSummary{}, err
	}
	return summarizeDocumentUnit(doc, start, end)
}

// summarizeDocumentUnit is SummarizeUnit over a parsed document.
func summarizeDocumentUnit(doc *sourceDocument, start, end string) (UnitSummary, error) {
	language := Language
	if doc.latin {
		language = classical.LanguageLatin
	}
	startCitation, endCitation, err := parseUnitBounds(start, end)
	if err != nil {
		return UnitSummary{}, err
	}
	if err := requireBoundaryTokens(doc, startCitation, endCitation); err != nil {
		return UnitSummary{}, err
	}
	reading, err := interpret(doc, domain.ReadingUnitDefinition{
		Key:           "manifest-unit",
		Language:      language,
		CitationStart: start,
		CitationEnd:   end,
	}, "manifest-unit")
	if err != nil {
		return UnitSummary{}, err
	}
	words := countPrintedWords(reading.Result)
	if words == 0 {
		return UnitSummary{}, &ImportError{Code: CodeEmptyCitationRange, Detail: "unit citation range has no words"}
	}
	opening, err := unitOpening(doc, startCitation, endCitation)
	if err != nil {
		return UnitSummary{}, err
	}
	return UnitSummary{WordCount: words, Opening: opening}, nil
}

// unitOpening returns the reconstructed first sentence the unit owns that is not
// a speaker label.
func unitOpening(doc *sourceDocument, start, end classical.Citation) (string, error) {
	for _, sentence := range doc.sentences {
		if len(sentence.tokens) == 0 {
			continue
		}
		located, _, err := sentenceLocation(sentence, start, end)
		if err != nil {
			return "", err
		}
		if located != locationInside || isSpeakerLabel(sentence) {
			continue
		}
		opening := reconstructFor(sentence, doc.latin).text
		if strings.TrimSpace(opening) != "" {
			return opening, nil
		}
	}
	return "", &ImportError{Code: CodeOpeningMissing, Detail: "unit has no sentence after speaker labels"}
}

// isSpeakerLabel reports whether a sentence is only a dialogue speaker's
// abbreviation. The pinned treebank marks every such abbreviation as a single
// all-uppercase nominal token, optionally beside an elliptical node.
func isSpeakerLabel(sentence sourceSentence) bool {
	words := 0
	var only sourceToken
	for _, token := range sentence.tokens {
		if isEllipsis(token.pos) || token.pos == "u" {
			continue
		}
		words++
		only = token
		if words > 1 {
			return false
		}
	}
	return words == 1 && only.pos == "n" && isUppercaseAbbreviation(only.surface)
}

// isUppercaseAbbreviation reports whether a surface form has letters and every
// one is uppercase, so an ordinary capitalised name is not mistaken for one.
func isUppercaseAbbreviation(surface string) bool {
	hasLetter := false
	for _, r := range surface {
		if !unicode.IsLetter(r) {
			continue
		}
		hasLetter = true
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return hasLetter
}

// parseManifestDocument parses and validates a whole source file.
func parseManifestDocument(data []byte) (*sourceDocument, error) {
	doc, err := parseOgaSrc(data)
	if err != nil {
		return nil, err
	}
	if err := validateNativePOS(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// parseUnitBounds validates a unit's boundary citations.
func parseUnitBounds(start, end string) (classical.Citation, classical.Citation, error) {
	startCitation, startOK := parseCitation(start)
	endCitation, endOK := parseCitation(end)
	if !startOK || !endOK || compareCitation(startCitation, endCitation) > 0 {
		return nil, nil, &ImportError{Code: CodeEmptyCitationRange, Detail: "unit citation range is invalid"}
	}
	return startCitation, endCitation, nil
}

// requireBoundaryTokens refuses a range whose start or end citation never
// appears on a token.
func requireBoundaryTokens(doc *sourceDocument, start, end classical.Citation) error {
	startSeen, endSeen := false, false
	for _, sentence := range doc.sentences {
		for _, token := range sentence.tokens {
			citation, ok := parseCitation(token.citation)
			if !ok {
				return malformed(0, "token citation is not a citation")
			}
			if compareCitation(citation, start) == 0 {
				startSeen = true
			}
			if compareCitation(citation, end) == 0 {
				endSeen = true
			}
		}
	}
	if !startSeen || !endSeen {
		return &ImportError{Code: CodeEmptyCitationRange, Detail: "unit citation range has no tokens"}
	}
	return nil
}

// countPrintedWords counts non-punctuation tokens in an interpretation.
func countPrintedWords(result analyzer.Result) int {
	words := 0
	for _, sentence := range result.Sentences {
		for _, token := range sentence.Tokens {
			if token.UPOS != "PUNCT" {
				words++
			}
		}
	}
	return words
}

// citationString joins a parsed citation back into its dotted form.
func citationString(citation classical.Citation) string { return citation.String() }
