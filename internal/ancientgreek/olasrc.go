package ancientgreek

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"maps"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// Latin interpretation slot versions recorded on an ola-conllu run.
const (
	// FeatsMappingVersionOLA names the OLA FEATS letter mapping.
	FeatsMappingVersionOLA = "ola-agdt-feats/1"
	// ProperNameRuleVersionOLA names the proper-name rule over OLA lemmas.
	ProperNameRuleVersionOLA = "ola-proper-name/1"
)

// olaFeatureSlots maps an OLA FEATS key onto the AGDT postag position the
// shared feature mapper reads.
var olaFeatureSlots = map[string]int{
	"Person": 1, "Number": 2, "Tense": 3, "Mood": 4, "Voice": 5, "Gender": 6, "Case": 7, "Degree": 8,
}

// latinFeatureMappings extends the Greek letter mapping with the values only
// Latin AGDT uses: the ablative, gerunds, gerundives, supines and the positive
// degree. Greek's own mapping is not touched.
var latinFeatureMappings = func() map[string]map[byte]string {
	merged := make(map[string]map[byte]string, len(featureMappings))
	for feature, values := range featureMappings {
		merged[feature] = maps.Clone(values)
	}
	merged["Case"]['b'] = "Abl"
	merged["Degree"]['p'] = "Pos"
	merged["Mood"]['g'], merged["Mood"]['d'], merged["Mood"]['u'] = "", "", ""
	merged["VerbForm"]['g'], merged["VerbForm"]['d'], merged["VerbForm"]['u'] = "Ger", "Gdv", "Sup"
	return merged
}()

// ParseOlaSource parses one Work's OLA CoNLL-U and its PAULA Citation layer
// into the same parsed-Work shape as ParseOgaSource. The CoNLL-U supplies
// tokens, lemmas, native AGDT parts of speech, morphology, heads and relations;
// the PAULA layer supplies each token's Citation, joined by the token's t_<n>
// reference. A token with no Citation, or one the Citation grammar refuses, is
// a malformed source.
func ParseOlaSource(conllu, paula []byte) (*ParsedWork, error) {
	citations, err := parsePaulaCitations(paula)
	if err != nil {
		return nil, err
	}
	doc, err := parseOlaConllu(conllu, citations)
	if err != nil {
		return nil, err
	}
	return &ParsedWork{doc: doc}, nil
}

// paulaFeatList is the PAULA feature layer: one feat per token.
type paulaFeatList struct {
	Feats []struct {
		Href  string `xml:"http://www.w3.org/1999/xlink href,attr"`
		Value string `xml:"value,attr"`
	} `xml:"featList>feat"`
}

// parsePaulaCitations reads the Citation of every token reference.
func parsePaulaCitations(data []byte) (map[string]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var layer paulaFeatList
	if err := decoder.Decode(&layer); err != nil && !errors.Is(err, io.EOF) {
		return nil, malformed(0, "Citation layer is not well-formed XML")
	}
	citations := make(map[string]string, len(layer.Feats))
	for _, feat := range layer.Feats {
		reference, ok := strings.CutPrefix(feat.Href, "#")
		if !ok || !numberedReference(reference) || feat.Value == "" {
			return nil, malformed(0, "Citation layer entry is not a token Citation")
		}
		citation, ok := paulaCitation(feat.Value)
		if !ok {
			return nil, malformed(0, "Citation layer value is not a Citation")
		}
		if _, duplicate := citations[reference]; duplicate {
			return nil, malformed(0, "Citation layer names a token twice")
		}
		citations[reference] = citation
	}
	if len(citations) == 0 {
		return nil, malformed(0, "Citation layer has no tokens")
	}
	return citations, nil
}

// paulaCitation reads one PAULA Citation value in its dotted form. OLA's PAULA
// layer joins a multi-level Citation's components with underscores (1_1_1);
// each underscore is read as a dot, and the result must satisfy the shared
// Citation grammar. A value mixing both separators is refused.
func paulaCitation(value string) (string, bool) {
	if strings.Contains(value, "_") {
		if strings.Contains(value, ".") {
			return "", false
		}
		value = strings.ReplaceAll(value, "_", ".")
	}
	if _, ok := parseCitation(value); !ok {
		return "", false
	}
	return value, true
}

// parseOlaConllu parses the OLA CoNLL-U: "# absolute_id", "# clause_id" and
// "# file" comments, then ten-tab token lines, one sentence per blank-line
// block.
func parseOlaConllu(data []byte, citations map[string]string) (*sourceDocument, error) {
	parser := olaParser{doc: &sourceDocument{latin: true}, citations: citations}
	for index, raw := range strings.Split(string(data), "\n") {
		if err := parser.acceptLine(strings.TrimSuffix(raw, "\r"), index+1); err != nil {
			return nil, err
		}
	}
	parser.endSentence()
	if len(parser.doc.sentences) == 0 {
		return nil, malformed(0, "no sentences")
	}
	return parser.doc, nil
}

// olaParser is the line-by-line state machine of parseOlaConllu.
type olaParser struct {
	doc       *sourceDocument
	current   *sourceSentence
	citations map[string]string
}

// endSentence closes the current sentence, if it has tokens.
func (p *olaParser) endSentence() {
	if p.current != nil && len(p.current.tokens) > 0 {
		p.doc.sentences = append(p.doc.sentences, *p.current)
	}
	p.current = nil
}

// acceptLine feeds one line to the state machine. Only absolute_id starts a
// sentence header; the other comments are ignored.
func (p *olaParser) acceptLine(line string, lineNo int) error {
	switch {
	case line == "":
		p.endSentence()
	case strings.HasPrefix(line, "#"):
		if name, value, _ := strings.Cut(strings.TrimPrefix(line, "#"), " = "); strings.TrimSpace(name) == "absolute_id" {
			p.endSentence()
			p.current = &sourceSentence{sentID: "ola-" + strings.TrimSpace(value)}
		}
	default:
		if p.current == nil {
			p.current = &sourceSentence{sentID: "ola-" + strconv.Itoa(len(p.doc.sentences))}
		}
		token, err := parseOlaTokenLine(line, lineNo, p.citations)
		if err != nil {
			return err
		}
		p.current.tokens = append(p.current.tokens, token)
	}
	return nil
}

// parseOlaTokenLine reads one ten-column OLA token line.
func parseOlaTokenLine(line string, lineNo int, citations map[string]string) (sourceToken, error) {
	columns := strings.Split(line, "\t")
	if len(columns) != 10 {
		return sourceToken{}, malformed(lineNo, "token line does not have ten columns")
	}
	id, err := strconv.Atoi(columns[0])
	if err != nil {
		return sourceToken{}, malformed(lineNo, "token id is not a number")
	}
	if columns[1] == "" || columns[2] == "" {
		return sourceToken{}, malformed(lineNo, "empty surface or lemma")
	}
	head, err := strconv.Atoi(columns[6])
	if err != nil || head < 0 {
		return sourceToken{}, malformed(lineNo, "head is not a non-negative number")
	}
	reference := columns[9]
	if !numberedReference(reference) {
		return sourceToken{}, malformed(lineNo, "token has no reference")
	}
	citation, ok := citations[reference]
	if !ok {
		return sourceToken{}, malformed(lineNo, "token has no Citation")
	}
	postag, err := olaPostag(columns[3], columns[5], lineNo)
	if err != nil {
		return sourceToken{}, err
	}
	return sourceToken{
		id:       id,
		surface:  columns[1],
		lemma:    columns[2],
		pos:      columns[3],
		postag:   postag,
		relation: columns[7],
		head:     head,
		citation: citation,
		ref:      reference,
	}, nil
}

// olaPostag rebuilds the nine-character AGDT postag from a native part of
// speech and a FEATS field, so the shared feature mapper reads Latin and Greek
// alike. A value that is not one letter is a malformed source.
func olaPostag(pos, feats string, lineNo int) (string, error) {
	if len(pos) != 1 {
		return "", malformed(lineNo, "part of speech is not one letter")
	}
	postag := []byte(pos + "--------")
	if feats == "_" || feats == "" {
		return string(postag), nil
	}
	for pair := range strings.SplitSeq(feats, "|") {
		key, value, ok := strings.Cut(pair, "=")
		slot, known := olaFeatureSlots[key]
		if !ok || !known || len(value) != 1 {
			return "", malformed(lineNo, "feature is not a known one-letter AGDT value")
		}
		if value != "_" {
			postag[slot] = value[0]
		}
	}
	return string(postag), nil
}

// sourceDialect is the set of language rules a source document is read with:
// how a lemma splits, how a postag maps to features, and which tokens take no
// space before them. A document selects one dialect once; the Greek oga_src
// dialect is the zero document's.
type sourceDialect struct {
	decodeLemma   func(lemma string) (headword, discriminator string)
	mapFeatures   func(postag string) (map[string]string, []FeatureValueCount)
	noSpaceBefore func(surface string) bool
}

var (
	greekDialect = sourceDialect{decodeLemma: decodeLemma, mapFeatures: mapFeatures, noSpaceBefore: noSpaceBefore}
	// latinDialect reads an OLA lemma's one trailing digit as a source artefact
	// (et1, ut1), not a homograph number: it is stripped and no discriminator is
	// recorded (ADR 0132). A lemma that still has a digit (rex12 -> rex1) keeps
	// its token in the text; Latin vocabulary admission refuses it.
	latinDialect = sourceDialect{
		decodeLemma: func(lemma string) (string, string) {
			trimmed := lemma
			if last := len(lemma) - 1; last > 0 && lemma[last] >= '0' && lemma[last] <= '9' {
				trimmed = lemma[:last]
			}
			return trimmed, ""
		},
		mapFeatures: func(postag string) (map[string]string, []FeatureValueCount) {
			return mapFeaturesWith(postag, latinFeatureMappings)
		},
		noSpaceBefore: func(surface string) bool {
			first, _ := utf8.DecodeRuneInString(surface)
			switch first {
			case ',', '.', ';', ':', '!', '?', ')', ']':
				return true
			default:
				return false
			}
		},
	}
)

// dialectFor selects the dialect of a document's language.
func dialectFor(latin bool) sourceDialect {
	if latin {
		return latinDialect
	}
	return greekDialect
}

// latinInterpretationVersion is the interpretation identity of an ola-conllu
// run: every mapping slot that shaped its tokens.
func latinInterpretationVersion(definition domain.ReadingUnitDefinition) string {
	normalization := "unknown"
	if profile, err := canonicalization.For(classical.LanguageLatin); err == nil {
		normalization = profile.Name() + "/" + profile.Version()
	}
	return strings.Join([]string{
		definition.InterpretationVersion,
		FeatsMappingVersionOLA,
		normalization,
		ProperNameRuleVersionOLA,
		SearchKeyRuleVersion,
		SentenceCitationRuleVersion,
	}, "+")
}
