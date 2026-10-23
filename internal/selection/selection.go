// Package selection generates deterministic, owner-scoped vocabulary candidates.
package selection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
)

var (
	ErrInvalidConfig   = errors.New("selection: invalid configuration")
	ErrInvalidDecision = errors.New("selection: invalid occurrence decision")
)

// Store is the selection service's learner-state seam. Its identity arguments
// are the persisted tuple without the lexical discriminator: the discriminator
// column arrives with the slices that first write Greek rows, so the seam stays
// exact on language, canonical lemma and UPOS until then.
type Store interface {
	IsKnownVocabularyIdentity(context.Context, string, string, string, string) (bool, error)
	IsReservedVocabulary(context.Context, string, string, string, string) (bool, error)
	PutSelectionCandidate(context.Context, domain.SelectionCandidate) (bool, error)
}

// Identity is one vocabulary identity: the study language, the canonical
// lemma, its part of speech, and a scheme-qualified lexical discriminator.
// The discriminator is empty for modern-language vocabulary and distinguishes
// source homographs such as morpheus:2. Comparison is exact on all four fields.
type Identity struct{ Language, CanonicalLemma, UPOS, LexicalDiscriminator string }
type SentenceReference struct {
	SentenceIndex int                     `json:"sentence_index"`
	Text          string                  `json:"text"`
	Location      analyzer.SourceLocation `json:"location"`
}
type Provenance struct {
	MinOccurrences  int `json:"min_occurrences"`
	OccurrenceCount int `json:"occurrence_count"`
	// GateVersion is the sentence-quality gate version that admitted a frozen
	// Reading identity (ADR 0106). It is zero, and omitted, for the Browse-
	// selection store and for snapshots frozen before the gate was applied.
	GateVersion int `json:"gate_version,omitempty"`
}
type Candidate struct {
	Identity           Identity
	OccurrenceCount    int
	ObservedForms      []string
	SentenceReferences []SentenceReference
	Provenance         Provenance
}

// OccurrenceIdentity identifies one analyzed occurrence within a corpus. Its
// source span is stable for the lifetime of that analysis and avoids applying
// a learner decision to another occurrence with the same spelling.
type OccurrenceIdentity struct {
	SourceDocumentID string
	StartOffset      uint64
	EndOffset        uint64
}

// OccurrenceDecision changes only the effective vocabulary identity of one
// occurrence. The analyzer result remains immutable. Excluded occurrences do
// not contribute to candidate counts or sentence references.
type OccurrenceDecision struct {
	Occurrence OccurrenceIdentity
	Lemma      string
	Excluded   bool
}

type SelectionConfig struct {
	CorpusID       string
	MinOccurrences int
	AllowedPOS     map[string]bool
	// Structure is the exact snapshot's recorded source structure. Speaker-label
	// tokens are structural non-vocabulary: they contribute no identity,
	// occurrence count or coverage-denominator token, and a sentence overlapping
	// a label supplies no study or card context. A direction nested inside a
	// label is covered by the enclosing speaker span; a standalone direction
	// stays ordinary eligible text. The zero value means structure was not
	// supplied (a v1 snapshot), so no restriction can be derived.
	Structure domain.SnapshotStructure
}

func DefaultConfig(corpusID string) SelectionConfig {
	return SelectionConfig{CorpusID: corpusID, MinOccurrences: 1,
		AllowedPOS: map[string]bool{"NOUN": true, "VERB": true, "ADJ": true, "ADV": true}}
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

type aggregate struct {
	count int
	forms map[string]bool
	refs  []SentenceReference
}

// Project derives deterministic candidate facts from immutable analyzer
// evidence and optional occurrence-scoped learner decisions. It deliberately
// does not apply owner state (Known or Reserved) or persist results, so all
// consumers can share the same vocabulary projection before applying their
// own eligibility and freeze rules.
func Project(corpus analyzer.Result, cfg SelectionConfig, decisions []OccurrenceDecision) ([]Candidate, error) {
	if err := validateProjectInput(corpus, cfg); err != nil {
		return nil, err
	}
	byOccurrence, err := indexOccurrenceDecisions(corpus, decisions)
	if err != nil {
		return nil, err
	}
	aggs := aggregateTokens(corpus, cfg, byOccurrence)
	ids := sortedIdentities(aggs)
	out := make([]Candidate, 0, len(ids))
	for _, id := range ids {
		out = append(out, projectCandidate(id, aggs[id], cfg))
	}
	return out, nil
}

// validateProjectInput rejects inputs with no meaningful projection: a
// positive occurrence floor, an allowed-POS set, and a corpus language.
func validateProjectInput(corpus analyzer.Result, cfg SelectionConfig) error {
	if cfg.MinOccurrences < 1 || cfg.AllowedPOS == nil || corpus.Language == "" {
		return ErrInvalidConfig
	}
	return nil
}

// indexOccurrenceDecisions validates each learner decision and indexes it by
// occurrence. Each occurrence carries at most one decision, and that decision
// must match exactly one analyzed occurrence.
func indexOccurrenceDecisions(corpus analyzer.Result, decisions []OccurrenceDecision) (map[OccurrenceIdentity]OccurrenceDecision, error) {
	byOccurrence := make(map[OccurrenceIdentity]OccurrenceDecision, len(decisions))
	for _, decision := range decisions {
		if !validOccurrenceDecision(decision) {
			return nil, ErrInvalidDecision
		}
		if _, exists := byOccurrence[decision.Occurrence]; exists {
			return nil, ErrInvalidDecision
		}
		byOccurrence[decision.Occurrence] = decision
	}
	if len(byOccurrence) == 0 {
		return byOccurrence, nil
	}
	if err := requireUniqueOccurrences(corpus, byOccurrence); err != nil {
		return nil, err
	}
	return byOccurrence, nil
}

// validOccurrenceDecision reports whether a decision addresses an occurrence
// with a non-inverted span and carries exactly one action: an exclusion or a
// replacement lemma, never both and never neither.
func validOccurrenceDecision(decision OccurrenceDecision) bool {
	if decision.Occurrence.SourceDocumentID == "" || decision.Occurrence.EndOffset < decision.Occurrence.StartOffset {
		return false
	}
	return decision.Excluded != (strings.TrimSpace(decision.Lemma) != "")
}

// requireUniqueOccurrences verifies that every indexed occurrence matches
// exactly one analyzed token, so no decision applies to an ambiguous span.
func requireUniqueOccurrences(corpus analyzer.Result, byOccurrence map[OccurrenceIdentity]OccurrenceDecision) error {
	occurrenceCounts := make(map[OccurrenceIdentity]int, len(byOccurrence))
	for _, sentence := range corpus.Sentences {
		for _, token := range sentence.Tokens {
			identity := occurrenceIdentity(token)
			if _, requested := byOccurrence[identity]; requested {
				occurrenceCounts[identity]++
			}
		}
	}
	for identity := range byOccurrence {
		if occurrenceCounts[identity] != 1 {
			return ErrInvalidDecision
		}
	}
	return nil
}

// sortedIdentities orders aggregate identities by lemma, then part of speech,
// then discriminator, the deterministic candidate order Project returns.
func sortedIdentities(aggs map[Identity]*aggregate) []Identity {
	ids := make([]Identity, 0, len(aggs))
	for id := range aggs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return identityLess(ids[i], ids[j]) })
	return ids
}

// identityLess orders identities by canonical lemma, then part of speech, then
// lexical discriminator. It is the deterministic order every identity list
// keeps.
func identityLess(a, b Identity) bool {
	if a.CanonicalLemma != b.CanonicalLemma {
		return a.CanonicalLemma < b.CanonicalLemma
	}
	if a.UPOS != b.UPOS {
		return a.UPOS < b.UPOS
	}
	return a.LexicalDiscriminator < b.LexicalDiscriminator
}

// projectCandidate materializes one aggregate's deterministic candidate facts,
// sorting the observed surface forms.
func projectCandidate(id Identity, a *aggregate, cfg SelectionConfig) Candidate {
	forms := make([]string, 0, len(a.forms))
	for form := range a.forms {
		forms = append(forms, form)
	}
	sort.Strings(forms)
	// A candidate whose every occurrence overlaps a speaker label has no
	// acceptable study reference. Persist an explicit empty array rather than
	// JSON null, so fact loaders can expand the column unconditionally.
	refs := a.refs
	if refs == nil {
		refs = []SentenceReference{}
	}
	return Candidate{Identity: id, OccurrenceCount: a.count, ObservedForms: forms, SentenceReferences: refs, Provenance: Provenance{MinOccurrences: cfg.MinOccurrences, OccurrenceCount: a.count}}
}

// AnalyzableStatistics calculates the immutable coverage denominator using the
// same token filters as candidate selection. Distinct lemmas are the
// lemma-and-UPOS identities used by vocabulary selection.
func AnalyzableStatistics(corpus analyzer.Result, cfg SelectionConfig) domain.AnalysisStatistics {
	aggs := aggregateTokens(corpus, cfg, nil)
	statistics := domain.AnalysisStatistics{DistinctLemmaCount: int64(len(aggs)), TextProfile: &domain.TextProfile{SentenceCount: int64(len(corpus.Sentences))}}
	profile := statistics.TextProfile
	for _, aggregate := range aggs {
		statistics.AnalyzableTokenCount += int64(aggregate.count)
	}
	lengths := make([]int, 0, len(corpus.Sentences))
	for _, sentence := range corpus.Sentences {
		length := len(sentence.Tokens)
		lengths = append(lengths, length)
		profile.NormalizedTokenCount += int64(length)
		if length == 0 {
			profile.EmptySentenceCount++
		}
		if length > 35 {
			profile.LongSentenceCount++
		}
	}
	if len(lengths) > 0 {
		sort.Ints(lengths)
		middle := len(lengths) / 2
		profile.MedianSentenceTokenCount = float64(lengths[middle])
		if len(lengths)%2 == 0 {
			profile.MedianSentenceTokenCount = float64(lengths[middle-1]+lengths[middle]) / 2
		}
		profile.P90SentenceTokenCount = int64(lengths[(9*len(lengths)+9)/10-1])
	}
	return statistics
}

func aggregateTokens(corpus analyzer.Result, cfg SelectionConfig, decisions map[OccurrenceIdentity]OccurrenceDecision) map[Identity]*aggregate {
	aggs := map[Identity]*aggregate{}
	for si, sentence := range corpus.Sentences {
		aggregateSentence(aggs, si, sentence, corpus.Language, cfg, decisions)
	}
	return aggs
}

// aggregateSentence folds one sentence's eligible tokens into the identity
// aggregates. A label-overlapping token is structural non-vocabulary even when
// a correction names the occurrence. A sentence overlapping a label is never
// study or card context, so its other eligible tokens still count but supply no
// sentence reference. Analyzer boundaries are never altered.
func aggregateSentence(aggs map[Identity]*aggregate, si int, sentence analyzer.Sentence, language string, cfg SelectionConfig, decisions map[OccurrenceIdentity]OccurrenceDecision) {
	studyContext := !cfg.Structure.OverlapsSpeakerLabel(sentence.Location.SourceDocumentID, sentence.Location.StartOffset, sentence.Location.EndOffset)
	for _, token := range sentence.Tokens {
		if cfg.Structure.OverlapsSpeakerLabel(token.Location.SourceDocumentID, token.Location.StartOffset, token.Location.EndOffset) {
			continue
		}
		var decision *OccurrenceDecision
		if learner, ok := decisions[occurrenceIdentity(token)]; ok {
			decision = &learner
		}
		id, counts := Effective(cfg, language, OccurrenceFact{Lemma: token.CanonicalLemma, UPOS: token.UPOS, Dependency: token.Dependency, LexicalDiscriminator: token.LexicalDiscriminator}, decision)
		if !counts {
			continue
		}
		a := aggs[id]
		if a == nil {
			a = &aggregate{forms: map[string]bool{}}
			aggs[id] = a
		}
		a.count++
		a.forms[token.Surface] = true
		if studyContext {
			a.refs = append(a.refs, SentenceReference{si, sentence.Text, token.Location})
		}
	}
}

// OccurrenceFact is the analyzer evidence for one analyzed token, the inputs
// Effective reads alongside a learner decision. LexicalDiscriminator is empty
// for modern-language tokens.
type OccurrenceFact struct{ Lemma, UPOS, Dependency, LexicalDiscriminator string }

// Effective is the one per-occurrence step that decides an analyzed token's
// effective vocabulary identity (ADR 0081, ADR 0087). The learner decision, when
// present, replaces the analyzer lemma; an excluded occurrence has no identity.
// A decision that really changes the lemma is a learner correction, whose
// identity carries no discriminator; a keep preserves the analyzer identity. It
// returns the identity and whether the occurrence counts at all. An occurrence
// that OccurrenceEligible rejects keeps its identity but does not count.
func Effective(cfg SelectionConfig, language string, fact OccurrenceFact, decision *OccurrenceDecision) (Identity, bool) {
	lemma := fact.Lemma
	discriminator := fact.LexicalDiscriminator
	if decision != nil {
		if decision.Excluded {
			return Identity{}, false
		}
		if strings.TrimSpace(decision.Lemma) != strings.TrimSpace(fact.Lemma) {
			discriminator = ""
		}
		lemma = decision.Lemma
	}
	id := Identity{language, strings.TrimSpace(lemma), strings.ToUpper(strings.TrimSpace(fact.UPOS)), strings.TrimSpace(discriminator)}
	return id, OccurrenceEligible(cfg, language, lemma, fact.UPOS, fact.Dependency)
}

// OccurrenceEligible is the per-occurrence test that decides whether an
// analyzed token can count as vocabulary at all: it needs a valid lemma, an
// allowed part of speech (trimmed and upper-cased), must not be a separable
// verb particle, and must not be a language stopword whose part of speech is
// otherwise eligible (French ADV ne/n' and pas, ADR 0102). Project and lemma
// review share it so neither duplicates the rule in SQL.
func OccurrenceEligible(cfg SelectionConfig, language, lemma, upos, dependency string) bool {
	return dependency != "compound:prt" &&
		lexical.IsLemma(strings.TrimSpace(lemma)) &&
		cfg.AllowedPOS[strings.ToUpper(strings.TrimSpace(upos))] &&
		!lexical.IsVocabularyStopword(language, lemma, upos)
}

func occurrenceIdentity(token analyzer.Token) OccurrenceIdentity {
	return OccurrenceIdentity{SourceDocumentID: token.Location.SourceDocumentID, StartOffset: token.Location.StartOffset, EndOffset: token.Location.EndOffset}
}

func (s *Service) Select(ctx context.Context, owner string, corpus analyzer.Result, cfg SelectionConfig) ([]Candidate, error) {
	if owner == "" || cfg.CorpusID == "" {
		return nil, ErrInvalidConfig
	}
	projected, err := Project(corpus, cfg, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0)
	for _, candidate := range projected {
		kept, err := s.selectCandidate(ctx, owner, cfg, candidate)
		if err != nil {
			return nil, err
		}
		if kept {
			out = append(out, candidate)
		}
	}
	return out, nil
}

// selectCandidate applies owner state to one projected candidate and persists
// it, reporting whether the store kept the row.
func (s *Service) selectCandidate(ctx context.Context, owner string, cfg SelectionConfig, candidate Candidate) (bool, error) {
	id := candidate.Identity
	if candidate.OccurrenceCount < cfg.MinOccurrences {
		return false, nil
	}
	known, err := s.store.IsKnownVocabularyIdentity(ctx, owner, id.Language, id.CanonicalLemma, id.UPOS)
	if err != nil {
		return false, fmt.Errorf("get known vocabulary: %w", err)
	}
	reserved, err := s.store.IsReservedVocabulary(ctx, owner, id.Language, id.CanonicalLemma, id.UPOS)
	if err != nil {
		return false, fmt.Errorf("get reserved vocabulary: %w", err)
	}
	if !allowsIdentity(candidate.OccurrenceCount, cfg.MinOccurrences, known, reserved) {
		return false, nil
	}
	formsJSON, refsJSON, provenanceJSON, err := marshalCandidate(candidate)
	if err != nil {
		return false, err
	}
	kept, err := s.store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner, CorpusID: cfg.CorpusID, Language: id.Language, CanonicalLemma: id.CanonicalLemma, UPOS: id.UPOS, LexicalDiscriminator: id.LexicalDiscriminator, OccurrenceCount: candidate.OccurrenceCount, ObservedForms: formsJSON, SentenceReferences: refsJSON, Provenance: provenanceJSON})
	if err != nil {
		return false, fmt.Errorf("persist candidate: %w", err)
	}
	return kept, nil
}

// marshalCandidate encodes the JSON columns of one candidate row.
func marshalCandidate(candidate Candidate) ([]byte, []byte, []byte, error) {
	formsJSON, err := json.Marshal(candidate.ObservedForms)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode observed forms: %w", err)
	}
	refsJSON, err := json.Marshal(candidate.SentenceReferences)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode sentence references: %w", err)
	}
	provenanceJSON, err := json.Marshal(candidate.Provenance)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode candidate provenance: %w", err)
	}
	return formsJSON, refsJSON, provenanceJSON, nil
}
