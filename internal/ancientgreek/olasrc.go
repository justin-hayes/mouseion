package ancientgreek

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
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
		copied := make(map[byte]string, len(values))
		for letter, value := range values {
			copied[letter] = value
		}
		merged[feature] = copied
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
		if _, ok := parseCitation(feat.Value); !ok {
			return nil, malformed(0, "Citation layer value is not a Citation")
		}
		if _, duplicate := citations[reference]; duplicate {
			return nil, malformed(0, "Citation layer names a token twice")
		}
		citations[reference] = feat.Value
	}
	if len(citations) == 0 {
		return nil, malformed(0, "Citation layer has no tokens")
	}
	return citations, nil
}

// parseOlaConllu parses the OLA CoNLL-U: "# absolute_id", "# clause_id" and
// "# file" comments, then ten-tab token lines, one sentence per blank-line
// block.
func parseOlaConllu(data []byte, citations map[string]string) (*sourceDocument, error) {
	doc := &sourceDocument{latin: true}
	var current *sourceSentence
	flush := func() {
		if current != nil && len(current.tokens) > 0 {
			doc.sentences = append(doc.sentences, *current)
		}
		current = nil
	}
	for index, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		lineNo := index + 1
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "#"):
			name, value, _ := strings.Cut(strings.TrimPrefix(line, "#"), " = ")
			if strings.TrimSpace(name) == "absolute_id" {
				flush()
				current = &sourceSentence{sentID: "ola-" + strings.TrimSpace(value)}
			}
		default:
			if current == nil {
				current = &sourceSentence{sentID: "ola-" + strconv.Itoa(len(doc.sentences))}
			}
			token, err := parseOlaTokenLine(line, lineNo, citations)
			if err != nil {
				return nil, err
			}
			current.tokens = append(current.tokens, token)
		}
	}
	flush()
	if len(doc.sentences) == 0 {
		return nil, malformed(0, "no sentences")
	}
	return doc, nil
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

// decodeLemmaFor splits a source lemma by the document's language. A Latin
// lemma's trailing digits are a source artefact (et1, ut1), not a homograph
// number: they are stripped and no discriminator is recorded (ADR 0132).
func decodeLemmaFor(lemma string, latin bool) (headword, discriminator string) {
	if !latin {
		return decodeLemma(lemma)
	}
	trimmed := strings.TrimRight(lemma, "0123456789")
	if trimmed == "" {
		return lemma, ""
	}
	return trimmed, ""
}

// mapFeaturesFor maps a postag with the document language's value mapping.
func mapFeaturesFor(postag string, latin bool) (map[string]string, []FeatureValueCount) {
	if latin {
		return mapFeaturesWith(postag, latinFeatureMappings)
	}
	return mapFeatures(postag)
}

// noSpaceBeforeFor reports whether a token must not be preceded by a space in
// the reconstructed sentence text, by the document language.
func noSpaceBeforeFor(surface string, latin bool) bool {
	if !latin {
		return noSpaceBefore(surface)
	}
	first, _ := utf8.DecodeRuneInString(surface)
	switch first {
	case ',', '.', ';', ':', '!', '?', ')', ']':
		return true
	default:
		return false
	}
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
