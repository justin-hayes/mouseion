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

// validate rejects a case set that cannot cover the frozen review classes or
// that carries an incomplete request.
func (s reviewCaseSet) validate() error {
	if strings.TrimSpace(s.Version) == "" {
		return errors.New("load gloss review cases: version is required")
	}
	if len(s.Cases) == 0 {
		return errors.New("load gloss review cases: no cases")
	}
	seenIDs := make(map[string]struct{}, len(s.Cases))
	seenLanguages := make(map[string]struct{}, len(reviewLanguages))
	seenVerdicts := make(map[verdictClass]struct{}, len(reviewVerdictClasses))
	for i, testCase := range s.Cases {
		if err := testCase.validate(); err != nil {
			return fmt.Errorf("load gloss review cases: case %d: %w", i, err)
		}
		if _, duplicate := seenIDs[testCase.ID]; duplicate {
			return fmt.Errorf("load gloss review cases: duplicate case id %q", testCase.ID)
		}
		seenIDs[testCase.ID] = struct{}{}
		seenLanguages[testCase.Language] = struct{}{}
		seenVerdicts[testCase.ExpectedVerdictClass] = struct{}{}
	}
	for _, language := range reviewLanguages {
		if _, ok := seenLanguages[language]; !ok {
			return fmt.Errorf("load gloss review cases: no %s case", language)
		}
	}
	for _, verdict := range reviewVerdictClasses {
		if _, ok := seenVerdicts[verdict]; !ok {
			return fmt.Errorf("load gloss review cases: no %s case", verdict)
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
	seenEvidence := make(map[string]struct{}, len(c.Evidence))
	for _, evidence := range c.Evidence {
		if strings.TrimSpace(evidence.EvidenceID) == "" {
			return fmt.Errorf("case %q: evidence id is required", c.ID)
		}
		if _, duplicate := seenEvidence[evidence.EvidenceID]; duplicate {
			return fmt.Errorf("case %q: duplicate evidence id %q", c.ID, evidence.EvidenceID)
		}
		seenEvidence[evidence.EvidenceID] = struct{}{}
		if strings.TrimSpace(evidence.Gloss) == "" {
			return fmt.Errorf("case %q: evidence %q has no gloss", c.ID, evidence.EvidenceID)
		}
	}
	return nil
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
