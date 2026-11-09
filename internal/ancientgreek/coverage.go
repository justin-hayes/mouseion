package ancientgreek

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
	ranges, err := parseUnitRanges(units)
	if err != nil {
		return CoverageReport{}, err
	}
	doc, err := parseOgaSrc(data)
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
	start, end []int
}

// citationWithin reports whether a citation lies inside a unit's inclusive
// range.
func citationWithin(citation []int, unit parsedUnitRange) bool {
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
