package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// verdictClass names what the review case is testing. They are the frozen
// vocabulary of the v15 failure and win record in #2325.
type verdictClass string

const (
	verdictPlainEquivalent      verdictClass = "plain_equivalent"
	verdictDropSentenceMaterial verdictClass = "drop_sentence_material"
	verdictProtectedWin         verdictClass = "protected_win"
	verdictLemmaMismatch        verdictClass = "lemma_mismatch"
	verdictSpotCheck            verdictClass = "spot_check"
)

var reviewVerdictClasses = []verdictClass{
	verdictPlainEquivalent,
	verdictDropSentenceMaterial,
	verdictProtectedWin,
	verdictLemmaMismatch,
	verdictSpotCheck,
}

// reviewLanguages are the languages the frozen case set must cover.
var reviewLanguages = []string{"de", "it", "el"}

// reviewCaseSet is the versioned repository test data one review runs. Cases
// carry only what normal preparation sends: the lemma, target form, one
// representative sentence and public lexical evidence.
type reviewCaseSet struct {
	Version string       `json:"version"`
	Cases   []reviewCase `json:"cases"`
}

type reviewCase struct {
	ID                   string                    `json:"id"`
	Language             string                    `json:"language"`
	Lemma                string                    `json:"lemma"`
	UPOS                 string                    `json:"upos"`
	TargetForm           string                    `json:"target_form"`
	Sentence             string                    `json:"sentence"`
	Evidence             []enrichment.LexicalSense `json:"evidence"`
	ExpectedVerdictClass verdictClass              `json:"expected_verdict_class"`
	ExpectedGloss        string                    `json:"expected_gloss,omitempty"`
	Notes                string                    `json:"notes,omitempty"`
}

// loadCaseSet decodes and validates one frozen review case set. Unknown fields
// are rejected so a typo cannot silently drop a case's evidence or expectation.
func loadCaseSet(reader io.Reader) (reviewCaseSet, error) {
	var cases reviewCaseSet
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cases); err != nil {
		return reviewCaseSet{}, fmt.Errorf("load gloss review cases: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return reviewCaseSet{}, errors.New("load gloss review cases: trailing JSON content")
	}
	if err := cases.validate(); err != nil {
		return reviewCaseSet{}, err
	}
	return cases, nil
}

// reviewClassIdentity is the coverage evidence one case contributes to its
// set: a unique id, a language and the class the set must cover.
type reviewClassIdentity struct {
	ID, Language, Class string
}

// validateReviewCoverage applies the shared case-set discipline: a version,
// at least one case, unique ids, and one case for every required language and
// class.
func validateReviewCoverage(label, version string, identities []reviewClassIdentity, languages, classes []string) error {
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("load %s review cases: version is required", label)
	}
	if len(identities) == 0 {
		return fmt.Errorf("load %s review cases: no cases", label)
	}
	seenIDs := make(map[string]struct{}, len(identities))
	seenLanguages := make(map[string]struct{}, len(languages))
	seenClasses := make(map[string]struct{}, len(classes))
	for _, identity := range identities {
		if _, duplicate := seenIDs[identity.ID]; duplicate {
			return fmt.Errorf("load %s review cases: duplicate case id %q", label, identity.ID)
		}
		seenIDs[identity.ID] = struct{}{}
		seenLanguages[identity.Language] = struct{}{}
		seenClasses[identity.Class] = struct{}{}
	}
	for _, language := range languages {
		if _, ok := seenLanguages[language]; !ok {
			return fmt.Errorf("load %s review cases: no %s case", label, language)
		}
	}
	for _, class := range classes {
		if _, ok := seenClasses[class]; !ok {
			return fmt.Errorf("load %s review cases: no %s case", label, class)
		}
	}
	return nil
}

// validate rejects a case set that cannot cover the frozen review classes or
// that carries an incomplete request.
func (s reviewCaseSet) validate() error {
	identities := make([]reviewClassIdentity, 0, len(s.Cases))
	for i, testCase := range s.Cases {
		if err := testCase.validate(); err != nil {
			return fmt.Errorf("load gloss review cases: case %d: %w", i, err)
		}
		identities = append(identities, reviewClassIdentity{ID: testCase.ID, Language: testCase.Language, Class: string(testCase.ExpectedVerdictClass)})
	}
	classes := make([]string, 0, len(reviewVerdictClasses))
	for _, class := range reviewVerdictClasses {
		classes = append(classes, string(class))
	}
	return validateReviewCoverage("gloss", s.Version, identities, reviewLanguages, classes)
}

// validateLexicalEvidence rejects evidence without a unique, non-empty id and
// a gloss.
func validateLexicalEvidence(caseID string, evidence []enrichment.LexicalSense) error {
	seenEvidence := make(map[string]struct{}, len(evidence))
	for _, sense := range evidence {
		if strings.TrimSpace(sense.EvidenceID) == "" {
			return fmt.Errorf("case %q: evidence id is required", caseID)
		}
		if _, duplicate := seenEvidence[sense.EvidenceID]; duplicate {
			return fmt.Errorf("case %q: duplicate evidence id %q", caseID, sense.EvidenceID)
		}
		seenEvidence[sense.EvidenceID] = struct{}{}
		if strings.TrimSpace(sense.Gloss) == "" {
			return fmt.Errorf("case %q: evidence %q has no gloss", caseID, sense.EvidenceID)
		}
	}
	return nil
}

// validate rejects a case missing any field the real provider request needs.
func (c reviewCase) validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("id is required")
	}
	if !slices.Contains(reviewLanguages, c.Language) {
		return fmt.Errorf("case %q has unsupported language %q", c.ID, c.Language)
	}
	if strings.TrimSpace(c.Lemma) == "" {
		return fmt.Errorf("case %q: lemma is required", c.ID)
	}
	if strings.TrimSpace(c.UPOS) == "" {
		return fmt.Errorf("case %q: upos is required", c.ID)
	}
	if strings.TrimSpace(c.TargetForm) == "" {
		return fmt.Errorf("case %q: target form is required", c.ID)
	}
	if strings.TrimSpace(c.Sentence) == "" {
		return fmt.Errorf("case %q: sentence is required", c.ID)
	}
	if len(c.Evidence) == 0 {
		return fmt.Errorf("case %q: frozen meaning evidence is required", c.ID)
	}
	if !slices.Contains(reviewVerdictClasses, c.ExpectedVerdictClass) {
		return fmt.Errorf("case %q has unknown expected verdict class %q", c.ID, c.ExpectedVerdictClass)
	}
	return validateLexicalEvidence(c.ID, c.Evidence)
}

// request builds the exact normal-preparation request for one case.
func (c reviewCase) request() enrichment.TranslationRequest {
	return enrichment.TranslationRequest{
		Language:               c.Language,
		TargetLanguage:         "en",
		CanonicalLemma:         c.Lemma,
		UPOS:                   c.UPOS,
		TargetWord:             c.TargetForm,
		ExampleSentence:        c.Sentence,
		CandidateSenses:        c.Evidence,
		RequireContextualGloss: true,
	}
}

func (class verdictClass) label() string {
	switch class {
	case verdictPlainEquivalent:
		return "plain equivalent"
	case verdictDropSentenceMaterial:
		return "drop sentence material"
	case verdictProtectedWin:
		return "protected contextual win"
	case verdictLemmaMismatch:
		return "lemma mismatch"
	case verdictSpotCheck:
		return "spot check"
	default:
		return string(class)
	}
}
