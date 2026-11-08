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

// idiomCaseKind names the review class of one idiom case. Positive cases must
// be confirmed, literal-use controls must be rejected as literal use,
// ambiguous controls (the sentence uses a sense no candidate lists) must be
// rejected as ambiguous, and lemma-card controls prove the anchor's own card
// moved away from the idiom sentence.
type idiomCaseKind string

const (
	idiomCasePositive   idiomCaseKind = "positive"
	idiomCaseLiteralUse idiomCaseKind = "literal_use_control"
	idiomCaseAmbiguous  idiomCaseKind = "ambiguous_control"
	idiomCaseLemmaCard  idiomCaseKind = "lemma_card_control"
)

var idiomCaseKinds = []idiomCaseKind{
	idiomCasePositive,
	idiomCaseLiteralUse,
	idiomCaseAmbiguous,
	idiomCaseLemmaCard,
}

// idiomReviewCaseSet is the versioned repository test data one idiom review
// runs. It carries only what the idiom codec sends: the headword, the matched
// forms, one representative sentence and public dictionary senses.
type idiomReviewCaseSet struct {
	Version string            `json:"version"`
	Cases   []idiomReviewCase `json:"cases"`
}

type idiomReviewCase struct {
	ID            string                    `json:"id"`
	Language      string                    `json:"language"`
	Kind          idiomCaseKind             `json:"kind"`
	Headword      string                    `json:"headword"`
	MatchedForms  []string                  `json:"matched_forms,omitempty"`
	Sentence      string                    `json:"sentence,omitempty"`
	Evidence      []enrichment.LexicalSense `json:"evidence,omitempty"`
	ExpectedGloss string                    `json:"expected_gloss,omitempty"`
	// GlossExcludes lists sentence words a positive case's confirmed Gloss
	// must not contain, such as the verb that governs the idiom (#2389).
	GlossExcludes     []string `json:"gloss_excludes,omitempty"`
	AnchorLemma       string   `json:"anchor_lemma,omitempty"`
	AnchorUPOS        string   `json:"anchor_upos,omitempty"`
	AnchorForm        string   `json:"anchor_form,omitempty"`
	IdiomSentence     string   `json:"idiom_sentence,omitempty"`
	LemmaCardSentence string   `json:"lemma_card_sentence,omitempty"`
	Notes             string   `json:"notes,omitempty"`
}

// loadIdiomCaseSet decodes and validates one frozen idiom review case set.
// Unknown fields are rejected so a typo cannot silently drop a case's evidence
// or control sentence.
func loadIdiomCaseSet(reader io.Reader) (idiomReviewCaseSet, error) {
	var cases idiomReviewCaseSet
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cases); err != nil {
		return idiomReviewCaseSet{}, fmt.Errorf("load idiom review cases: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return idiomReviewCaseSet{}, errors.New("load idiom review cases: trailing JSON content")
	}
	if err := cases.validate(); err != nil {
		return idiomReviewCaseSet{}, err
	}
	return cases, nil
}

// validate rejects a case set that cannot cover the frozen idiom review
// classes or that carries an incomplete request or lemma-card control.
func (s idiomReviewCaseSet) validate() error {
	identities := make([]reviewClassIdentity, 0, len(s.Cases))
	for i, testCase := range s.Cases {
		if err := testCase.validate(); err != nil {
			return fmt.Errorf("load idiom review cases: case %d: %w", i, err)
		}
		identities = append(identities, reviewClassIdentity{ID: testCase.ID, Language: testCase.Language, Class: string(testCase.Kind)})
	}
	classes := make([]string, 0, len(idiomCaseKinds))
	for _, kind := range idiomCaseKinds {
		classes = append(classes, string(kind))
	}
	return validateReviewCoverage("idiom", s.Version, identities, reviewLanguages, classes)
}

// validate rejects a case missing any field the idiom request or its report
// needs.
func (c idiomReviewCase) validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("id is required")
	}
	if !slices.Contains(reviewLanguages, c.Language) {
		return fmt.Errorf("case %q has unsupported language %q", c.ID, c.Language)
	}
	if !slices.Contains(idiomCaseKinds, c.Kind) {
		return fmt.Errorf("case %q has unknown kind %q", c.ID, c.Kind)
	}
	if strings.TrimSpace(c.Headword) == "" {
		return fmt.Errorf("case %q: headword is required", c.ID)
	}
	switch c.Kind {
	case idiomCasePositive, idiomCaseLiteralUse, idiomCaseAmbiguous:
		return c.validateProviderCase()
	case idiomCaseLemmaCard:
		return c.validateLemmaCardControl()
	default:
		return fmt.Errorf("case %q has unknown kind %q", c.ID, c.Kind)
	}
}

func (c idiomReviewCase) validateProviderCase() error {
	if err := validateIdiomMatchedForms(c.ID, c.MatchedForms); err != nil {
		return err
	}
	if strings.TrimSpace(c.Sentence) == "" {
		return fmt.Errorf("case %q: sentence is required", c.ID)
	}
	if len(c.Evidence) == 0 {
		return fmt.Errorf("case %q: frozen meaning evidence is required", c.ID)
	}
	if c.Kind == idiomCasePositive && strings.TrimSpace(c.ExpectedGloss) == "" {
		return fmt.Errorf("case %q: a positive case needs an expected Gloss", c.ID)
	}
	if (c.Kind == idiomCaseLiteralUse || c.Kind == idiomCaseAmbiguous) && strings.TrimSpace(c.ExpectedGloss) != "" {
		return fmt.Errorf("case %q: a %s has no expected Gloss", c.ID, c.Kind.label())
	}
	if err := c.validateGlossExcludes(); err != nil {
		return err
	}
	return validateLexicalEvidence(c.ID, c.Evidence)
}

// validateGlossExcludes accepts Gloss exclusions only on a positive case,
// where a confirmed Gloss exists to check.
func (c idiomReviewCase) validateGlossExcludes() error {
	if len(c.GlossExcludes) == 0 {
		return nil
	}
	if c.Kind != idiomCasePositive {
		return fmt.Errorf("case %q: only a positive case lists Gloss exclusions", c.ID)
	}
	for _, word := range c.GlossExcludes {
		if strings.TrimSpace(word) == "" {
			return fmt.Errorf("case %q: Gloss exclusions cannot be blank", c.ID)
		}
	}
	return nil
}

func validateIdiomMatchedForms(caseID string, forms []string) error {
	if len(forms) == 0 {
		return fmt.Errorf("case %q: matched forms are required", caseID)
	}
	for _, form := range forms {
		if strings.TrimSpace(form) == "" {
			return fmt.Errorf("case %q: matched forms cannot be blank", caseID)
		}
	}
	return nil
}

func (c idiomReviewCase) validateLemmaCardControl() error {
	if strings.TrimSpace(c.AnchorLemma) == "" {
		return fmt.Errorf("case %q: anchor lemma is required", c.ID)
	}
	if strings.TrimSpace(c.AnchorUPOS) == "" {
		return fmt.Errorf("case %q: anchor upos is required", c.ID)
	}
	if strings.TrimSpace(c.AnchorForm) == "" {
		return fmt.Errorf("case %q: anchor form is required", c.ID)
	}
	if problem := lemmaCardControlProblem(c); problem != "" {
		return fmt.Errorf("case %q: %s", c.ID, problem)
	}
	return nil
}

// lemmaCardControlProblem is the shared rule for a lemma-card control: the
// recorded card sentence must move off the idiom sentence and still carry the
// anchor form.
func lemmaCardControlProblem(c idiomReviewCase) string {
	switch {
	case strings.TrimSpace(c.IdiomSentence) == "" || strings.TrimSpace(c.LemmaCardSentence) == "":
		return "control is missing a sentence"
	case strings.EqualFold(strings.TrimSpace(c.IdiomSentence), strings.TrimSpace(c.LemmaCardSentence)):
		return "lemma-card control must move to another sentence"
	case !containsFold(c.LemmaCardSentence, c.AnchorForm):
		return fmt.Sprintf("lemma-card sentence does not carry the anchor form %q", c.AnchorForm)
	default:
		return ""
	}
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(strings.TrimSpace(needle)))
}

// request builds the exact normal-preparation idiom request for one case.
func (c idiomReviewCase) request() enrichment.TranslationRequest {
	return enrichment.TranslationRequest{
		Language:          c.Language,
		TargetLanguage:    "en",
		CanonicalLemma:    c.Headword,
		ExampleSentence:   c.Sentence,
		CandidateSenses:   c.Evidence,
		RequireIdiomCard:  true,
		IdiomHeadword:     c.Headword,
		IdiomMatchedForms: append([]string(nil), c.MatchedForms...),
	}
}

func (kind idiomCaseKind) label() string {
	switch kind {
	case idiomCasePositive:
		return "positive"
	case idiomCaseLiteralUse:
		return "literal-use control"
	case idiomCaseAmbiguous:
		return "ambiguous control"
	case idiomCaseLemmaCard:
		return "lemma-card control"
	default:
		return string(kind)
	}
}
