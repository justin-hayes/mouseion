package ancientgreek

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/classical"
)

// ManifestSource is one parsed whole-Work source the manifest generator
// reads: an oga_src file, or an OLA CoNLL-U with its PAULA Citation layer. It
// parses once and answers the generator's three questions, so the generator
// need not know which dialect it holds.
type ManifestSource struct {
	doc *sourceDocument
}

// ParseManifestSource parses and validates a whole oga_src file.
func ParseManifestSource(data []byte) (*ManifestSource, error) {
	doc, err := parseManifestDocument(data)
	if err != nil {
		return nil, err
	}
	return &ManifestSource{doc: doc}, nil
}

// ParseLatinManifestSource parses and validates one OLA Work from its CoNLL-U
// and PAULA Citation layer. A token with no valid Citation refuses it.
func ParseLatinManifestSource(conllu, paula []byte) (*ManifestSource, error) {
	parsed, err := ParseOlaSource(conllu, paula)
	if err != nil {
		return nil, err
	}
	if err := validateNativePOS(parsed.doc); err != nil {
		return nil, err
	}
	return &ManifestSource{doc: parsed.doc}, nil
}

// ProposeUnits proposes one range per distinct top-level Citation number.
func (s *ManifestSource) ProposeUnits() ([]UnitRange, error) { return proposeDocumentUnits(s.doc) }

// CheckUnitCoverage checks that every sentence belongs to exactly one unit.
func (s *ManifestSource) CheckUnitCoverage(units []UnitRange) (CoverageReport, error) {
	return checkDocumentCoverage(s.doc, units)
}

// SummarizeUnit derives one unit's word count and Opening.
func (s *ManifestSource) SummarizeUnit(start, end string) (UnitSummary, error) {
	return summarizeDocumentUnit(s.doc, start, end)
}

// LatinScan is the evidence the generator's text gates read from one OLA Work.
// It never carries source text.
type LatinScan struct {
	// Tokens counts printed tokens, punctuation included.
	Tokens int
	// Words counts printed tokens that are not punctuation.
	Words int
	// Uncited counts printed tokens with no Citation, or one the Citation
	// grammar refuses.
	Uncited int
	// Backwards counts the places where a token's Citation precedes the
	// previous cited token's.
	Backwards int
	// FirstBackwards is the Citation of the first token that runs backwards.
	FirstBackwards string
	// SplitWords counts words that are a single letter other than a real
	// one-letter Latin word or a praenomen abbreviation.
	SplitWords int
}

// SplitWordRate is the share of words that are split-word fragments.
func (s LatinScan) SplitWordRate() float64 {
	if s.Words == 0 {
		return 0
	}
	return float64(s.SplitWords) / float64(s.Words)
}

// ScanLatinWork reads one OLA Work's tokens and Citations for the generator's
// gates. It is lenient where ParseLatinManifestSource is strict, so a Work the
// parser would refuse is reported rather than lost: only an unreadable
// Citation layer or a token line that is not ten columns is an error.
func ScanLatinWork(conllu, paula []byte) (LatinScan, error) {
	citations, err := parsePaulaCitations(paula)
	if err != nil {
		return LatinScan{}, err
	}
	scanner := latinScanner{citations: citations}
	for index, raw := range strings.Split(string(conllu), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		columns := strings.Split(line, "\t")
		if len(columns) != 10 {
			return LatinScan{}, malformed(index+1, "token line does not have ten columns")
		}
		scanner.accept(columns[1], columns[3], columns[9])
	}
	return scanner.scan, nil
}

// latinScanner accumulates a LatinScan one token at a time.
type latinScanner struct {
	citations map[string]string
	previous  classical.Citation
	scan      LatinScan
}

// accept counts one token line's form, native part of speech and reference.
func (s *latinScanner) accept(surface, pos, reference string) {
	if isEllipsis(pos) {
		return
	}
	s.scan.Tokens++
	s.acceptCitation(s.citations[reference])
	if pos == "u" {
		return
	}
	s.scan.Words++
	if isSplitWord(surface, pos) {
		s.scan.SplitWords++
	}
}

// acceptCitation checks one token's Citation against the previous cited token.
func (s *latinScanner) acceptCitation(value string) {
	citation, cited := parseCitation(value)
	if !cited || len(citation) == 0 {
		s.scan.Uncited++
		return
	}
	if s.previous != nil && citation.Compare(s.previous) < 0 {
		if s.scan.Backwards == 0 {
			s.scan.FirstBackwards = citation.String()
		}
		s.scan.Backwards++
	}
	s.previous = citation
}

// praenomenInitials are the single letters Roman first names abbreviate to.
const praenomenInitials = "ACDKLMNPQST"

// isSplitWord reports whether a printed word is a one-letter fragment that is
// not a real one-letter Latin word (a, e, o, i), a praenomen initial or a
// Roman numeral letter (a numeral by native part of speech).
func isSplitWord(surface, pos string) bool {
	letter, size := utf8.DecodeRuneInString(surface)
	if size != len(surface) || !unicode.IsLetter(letter) || pos == "m" {
		return false
	}
	if strings.ContainsRune("aeoi", unicode.ToLower(letter)) {
		return false
	}
	return !strings.ContainsRune(praenomenInitials, letter)
}
