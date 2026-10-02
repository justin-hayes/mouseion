package cardexport

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

var ErrAllMeaningsUnresolved = errors.New("cardexport: every selected contextual meaning is unresolved")

// CandidateProjection is the fact boundary assembled by selection and
// persistence adapters. Presentation owns all interpretation after this point.
type CandidateProjection struct {
	OwnerID, DeckName         string
	Candidate                 domain.SelectionCandidate
	Entry                     Entry
	Sentences                 map[int64]analyzer.Sentence
	Provider, ProviderVersion string
	TargetLanguage            string
	RequireContextualGloss    bool
}

// WorkItem is one exact external request derived from a frozen deck.
type WorkItem struct {
	Ordinal                   int
	Request                   enrichment.TranslationRequest
	CacheKey                  enrichment.CacheKey
	DictionaryProviderVersion string
}

// RequestCandidate adapts a work request to the result shape used by the
// enrichment overlay without exposing the frozen manifest.
func (w WorkItem) RequestCandidate() enrichment.Candidate {
	return enrichment.Candidate{
		Identity:                  enrichment.Identity{Language: w.Request.Language, CanonicalLemma: w.Request.CanonicalLemma, UPOS: w.Request.UPOS},
		TargetWord:                w.Request.TargetWord,
		ExampleSentence:           w.Request.ExampleSentence,
		DictionaryProviderVersion: w.DictionaryProviderVersion,
		CandidateSenses:           enrichment.CloneLexicalSenses(w.Request.CandidateSenses),
	}
}

// Summary is the stable count projection of a frozen deck.
type Summary struct {
	Completeness Completeness
	Selected     int
	Accepted     int
	Omitted      int
}

// Diagnostics contains structured presentation diagnostics. Meaning omission
// details are source-derived and remain in the owner-scoped preparation result;
// they are not emitted as metrics or aggregate telemetry.
type Diagnostics struct {
	QualityOmissions []Omission
	MeaningOmissions []MeaningOmission
	DegradationCodes []string
	GlossCoverage    []GlossCoverage
	EvidenceCoverage []EvidenceCoverage
}

type MeaningOmission struct {
	Target, Reason string
}

// EvidenceCoverage reports how much bounded local meaning evidence was frozen.
type EvidenceCoverage struct {
	Source     string
	Configured bool
	Selected   int
	Matched    int
	Candidates int
	Omitted    int
}

const (
	DegradationInvalidSenseSelection = "invalid_sense_selection"
	DegradationFallbackGlossApplied  = "fallback_gloss_applied"
	DegradationFallbackGlossRejected = "fallback_gloss_rejected"
)

type FreezeDiagnostics = Diagnostics
type FinalizeDiagnostics = Diagnostics

// RunFacts are immutable facts recorded by the durable run. They select no
// policy; they only tell finalization which already-frozen contract applies.
type RunFacts struct {
	Consent, Configured       bool
	ExecutionMode             string
	TargetLanguage            string
	Provider, ProviderVersion string
}

// StoredResult is one exact cache row loaded by a persistence adapter.
// Presentation interprets the row during finalization.
type StoredResult struct {
	CacheKey       enrichment.CacheKey
	Record         enrichment.CacheEntry
	OmissionReason string
}

// StorageProjection is the normalized durable representation of a deck.
type StorageProjection = ManifestSnapshot

// FinalArtifact is the complete rendered prepared-deck result.
type FinalArtifact = Artifact

// RecoveredRenderInput supplies a render input read from the immutable corpus
// for a legacy manifest that did not freeze its dependency parse.
type RecoveredRenderInput struct {
	ManifestOrdinal int
	Sentence        string
	SentenceTokens  []analyzer.Token
}

// Presentation owns the prepared-deck presentation lifecycle. The lexical
// provider is the only replaceable implementation seam.
type Presentation struct {
	lexical enrichment.LexicalProvider
}

func NewPresentation(provider enrichment.LexicalProvider) *Presentation {
	return &Presentation{lexical: provider}
}

// FrozenDeck is an opaque, immutable presentation plan.
type FrozenDeck struct {
	manifest manifest
}

// WithAnkiDeckIdentity returns a copy rendered with a stable Anki deck ID and
// the learner's current display name in the deck description. A zero ID retains
// the historical name-derived identity.
func (d FrozenDeck) WithAnkiDeckIdentity(id int64, displayName string) FrozenDeck {
	d.manifest = d.manifest.clone()
	d.manifest.ankiDeckID = id
	d.manifest.ankiDeckDescription = customDeckAnkiDescription(displayName)
	return d
}

// Freeze selects representative sentences, resolves local lexical facts,
// quality-gates each candidate, and constructs exact external identities. The
// empty-deck variant is intentionally folded into this operation: the
// explicit identity supplies the metadata when there are no projections.
func (p *Presentation) Freeze(ctx context.Context, owner, deckName string, projections []CandidateProjection) (FrozenDeck, FreezeDiagnostics, error) {
	if p == nil {
		return FrozenDeck{}, FreezeDiagnostics{}, ErrInvalidInput
	}
	ordered := append([]CandidateProjection(nil), projections...)
	if len(ordered) > 0 {
		projectionOwner, projectionDeckName, err := projectionHeader(ordered)
		if err != nil {
			return FrozenDeck{}, FreezeDiagnostics{}, err
		}
		if strings.TrimSpace(owner) == "" || owner != projectionOwner {
			return FrozenDeck{}, FreezeDiagnostics{}, fmt.Errorf("%w: explicit owner contradicts candidate projections", ErrInvalidInput)
		}
		if strings.TrimSpace(deckName) == "" || deckName != projectionDeckName {
			return FrozenDeck{}, FreezeDiagnostics{}, fmt.Errorf("%w: explicit deck name contradicts candidate projections", ErrInvalidInput)
		}
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(deckName) == "" {
		return FrozenDeck{}, FreezeDiagnostics{}, fmt.Errorf("%w: freeze requires owner and deck name", ErrInvalidInput)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Candidate.FirstEncounter != ordered[j].Candidate.FirstEncounter {
			return ordered[i].Candidate.FirstEncounter < ordered[j].Candidate.FirstEncounter
		}
		return candidateKey(ordered[i].Candidate) < candidateKey(ordered[j].Candidate)
	})

	resolver := &lexicalResolver{lexical: p.lexical}
	entries := make([]Entry, 0, len(ordered))
	for _, projection := range ordered {
		entry := cloneEntry(projection.Entry)
		clearExternalFields(&entry)
		applySentenceDecision(&entry, projection.Candidate, projection.Sentences)
		if err := resolver.resolveLexicalEntry(ctx, &entry); err != nil {
			return FrozenDeck{}, FreezeDiagnostics{}, fmt.Errorf("%w: resolve lexical entry %s: %w", ErrInvalidInput, candidateKey(projection.Candidate), err)
		}
		entries = append(entries, entry)
	}
	coverage := glossCoverage(entries)
	manifest := newManifest(owner, deckName, entries)
	if provider, version, target, err := projectionProvider(ordered); err != nil {
		return FrozenDeck{}, FreezeDiagnostics{}, err
	} else if provider != "" {
		keys := make([]enrichment.CacheKey, len(manifest.enrichmentCandidatesProjection()))
		contextualGlossByCandidate := make(map[string]bool, len(ordered))
		for _, projection := range ordered {
			contextualGlossByCandidate[candidateKey(projection.Candidate)] = projection.RequireContextualGloss
		}
		for i, candidate := range manifest.enrichmentCandidatesProjection() {
			keys[i] = enrichment.CacheKey{
				Language: candidate.Language, TargetLanguage: target,
				CanonicalLemma: candidate.CanonicalLemma, UPOS: strings.ToUpper(candidate.UPOS),
				Provider: provider, ProviderVersion: version,
				DictionaryProviderVersion: candidate.DictionaryProviderVersion,
				SentenceHash:              enrichment.SentenceHash(candidate.ExampleSentence),
			}
			if contextualGlossByCandidate[candidate.Language+"\x00"+candidate.CanonicalLemma+"\x00"+candidate.UPOS] {
				keys[i].MeaningEvidenceHash = enrichment.MeaningEvidenceHash(candidate.CandidateSenses)
			}
		}
		manifest, err = manifest.bindCacheKeys(keys)
		if err != nil {
			return FrozenDeck{}, FreezeDiagnostics{}, err
		}
	}
	diagnostics := manifestDiagnostics(manifest)
	diagnostics.GlossCoverage = cloneGlossCoverage(coverage)
	return FrozenDeck{manifest: manifest.clone()}, diagnostics, nil
}

// Restore validates a durable projection with its historical schema codec and
// rebuilds the frozen plan without selection, lexical lookup, or quality work.
func (p *Presentation) Restore(projection StorageProjection) (FrozenDeck, error) {
	return p.restore(projection, nil)
}

// RestoreWithRecoveredRenderInputs restores a legacy projection and applies
// render inputs recovered from the immutable corpus. The durable projection is
// never changed; the recovered inputs belong to this in-memory presentation.
func (p *Presentation) RestoreWithRecoveredRenderInputs(projection StorageProjection, recovered []RecoveredRenderInput) (FrozenDeck, error) {
	return p.restore(projection, recovered)
}

func (p *Presentation) restore(projection StorageProjection, recovered []RecoveredRenderInput) (FrozenDeck, error) {
	if p == nil {
		return FrozenDeck{}, ErrInvalidInput
	}
	manifest, err := manifestFromSnapshot(projection)
	if err != nil {
		return FrozenDeck{}, err
	}
	if err := applyRecoveredRenderInputs(&manifest, recovered); err != nil {
		return FrozenDeck{}, err
	}
	return FrozenDeck{manifest: manifest.clone()}, nil
}

func applyRecoveredRenderInputs(manifest *manifest, recovered []RecoveredRenderInput) error {
	if manifest == nil {
		return ErrInvalidInput
	}
	acceptedIndexes := make(map[int]int)
	for index := range manifest.accepted {
		acceptedIndexes[manifest.decisionsOrdinalForAccepted(index)] = index
	}
	seen := make(map[int]struct{}, len(recovered))
	for _, input := range recovered {
		if _, duplicate := seen[input.ManifestOrdinal]; duplicate {
			return fmt.Errorf("%w: duplicate recovered render input %d", ErrInvalidInput, input.ManifestOrdinal)
		}
		seen[input.ManifestOrdinal] = struct{}{}
		decisionIndex := -1
		for index, decision := range manifest.decisions {
			if decision.Ordinal == input.ManifestOrdinal {
				decisionIndex = index
				break
			}
		}
		acceptedIndex, accepted := acceptedIndexes[input.ManifestOrdinal]
		if decisionIndex < 0 || !accepted || manifest.decisions[decisionIndex].Disposition != ManifestAccepted {
			return fmt.Errorf("%w: recovered render input %d is not accepted", ErrInvalidInput, input.ManifestOrdinal)
		}
		decision := manifest.decisions[decisionIndex]
		if input.Sentence != decision.Entry.Sentence || strings.TrimSpace(input.Sentence) == "" || len(input.SentenceTokens) == 0 {
			return fmt.Errorf("%w: recovered render input %d does not match the manifest", ErrInvalidInput, input.ManifestOrdinal)
		}
		if len(decision.Entry.SentenceTokens) > 0 {
			return fmt.Errorf("%w: render input %d is already present", ErrInvalidInput, input.ManifestOrdinal)
		}
		manifest.decisions[decisionIndex].Entry.SentenceTokens = cloneTokens(input.SentenceTokens)
		manifest.accepted[acceptedIndex].SentenceTokens = cloneTokens(input.SentenceTokens)
	}
	return nil
}

func (d FrozenDeck) StorageProjection() StorageProjection {
	return d.manifest.Snapshot()
}

func (d FrozenDeck) WorkProjection() []WorkItem {
	if len(d.manifest.cacheKeys) != len(d.manifest.accepted) {
		return nil
	}
	items := make([]WorkItem, 0, len(d.manifest.accepted))
	for i := range d.manifest.accepted {
		items = append(items, d.workItem(i))
	}
	return items
}

// WorkByOrdinal selects one frozen external request without exposing the
// manifest or requiring an adapter to reconstruct its identity.
func (d FrozenDeck) WorkByOrdinal(ordinal int) (WorkItem, bool) {
	if len(d.manifest.cacheKeys) != len(d.manifest.accepted) {
		return WorkItem{}, false
	}
	for i := range d.manifest.accepted {
		if d.manifest.decisionsOrdinalForAccepted(i) == ordinal {
			return d.workItem(i), true
		}
	}
	return WorkItem{}, false
}

func (d FrozenDeck) workItem(acceptedIndex int) WorkItem {
	entry := d.manifest.accepted[acceptedIndex]
	key := d.manifest.cacheKeys[acceptedIndex]
	targetLanguage := key.TargetLanguage
	if targetLanguage == "" {
		targetLanguage = "en"
	}
	return WorkItem{
		Ordinal: d.manifest.decisionsOrdinalForAccepted(acceptedIndex),
		Request: enrichment.TranslationRequest{
			Language: entry.Language, TargetLanguage: targetLanguage,
			CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS,
			TargetWord: testedRenderTarget(entry), ExampleSentence: entry.Sentence,
			CandidateSenses:        enrichment.CloneLexicalSenses(entry.CandidateSenses),
			RequireContextualGloss: key.MeaningEvidenceHash != "",
		},
		CacheKey: key, DictionaryProviderVersion: entry.DictionaryProviderVersion,
	}
}

func (d FrozenDeck) Summary() Summary {
	selected, accepted, omitted := d.manifest.Snapshot().Counts()
	return Summary{Completeness: d.manifest.completeness(), Selected: selected, Accepted: accepted, Omitted: omitted}
}

func (d FrozenDeck) Diagnostics() Diagnostics {
	return manifestDiagnostics(d.manifest)
}

// Finalize overlays only exact stored results and renders the complete artifact.
func (p *Presentation) Finalize(ctx context.Context, deck FrozenDeck, results []StoredResult, facts RunFacts) (FinalArtifact, FinalizeDiagnostics, error) {
	if p == nil {
		return Artifact{}, FinalizeDiagnostics{}, ErrInvalidInput
	}
	manifest := deck.manifest.clone()
	keys := manifest.cacheKeys
	if len(keys) == 0 {
		if len(results) != 0 {
			return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: external results supplied for a local deck", ErrInvalidInput)
		}
		artifact, _, err := renderManifest(ctx, manifest, nil)
		if err != nil {
			return Artifact{}, FinalizeDiagnostics{}, err
		}
		diagnostics := manifestDiagnostics(manifest)
		artifact.Diagnostics = cloneDiagnostics(diagnostics)
		return artifact, diagnostics, nil
	}
	if facts.Consent && facts.Configured {
		mode := strings.ToLower(strings.TrimSpace(facts.ExecutionMode))
		if mode != "standard" && mode != "batch" {
			return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: invalid prepared deck execution mode", ErrInvalidInput)
		}
		if facts.TargetLanguage == "" || facts.Provider == "" || facts.ProviderVersion == "" {
			return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: incomplete external run facts", ErrInvalidInput)
		}
		for _, key := range keys {
			historicalV1Target := manifest.schemaVersion == LegacyManifestSchemaVersion && key.TargetLanguage == ""
			if (!historicalV1Target && key.TargetLanguage != facts.TargetLanguage) || key.Provider != facts.Provider || key.ProviderVersion != facts.ProviderVersion {
				return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: run facts contradict manifest cache identity", ErrInvalidInput)
			}
		}
	}

	byKey := make(map[enrichment.CacheKey]StoredResult, len(results))
	for _, result := range results {
		if _, duplicate := byKey[result.CacheKey]; duplicate {
			return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: duplicate stored enrichment result", ErrInvalidInput)
		}
		if result.Record.CacheKey != (enrichment.CacheKey{}) && result.Record.CacheKey != result.CacheKey {
			return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: stored enrichment record identity does not match key", ErrInvalidInput)
		}
		if !containsCacheKey(keys, result.CacheKey) {
			return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: stored enrichment identity is not frozen", ErrInvalidInput)
		}
		byKey[result.CacheKey] = result
	}
	mode := strings.TrimSpace(facts.ExecutionMode)
	contextualIdentity := slices.ContainsFunc(keys, func(key enrichment.CacheKey) bool { return key.MeaningEvidenceHash != "" })
	contextualBatch := strings.EqualFold(mode, "batch") && contextualIdentity
	required := facts.Consent && facts.Configured && (strings.EqualFold(mode, "standard") || contextualBatch)
	aligned := make([]ExactEnrichment, 0, len(keys))
	accepted := make([]RenderInput, 0, len(manifest.accepted))
	cacheKeys := make([]enrichment.CacheKey, 0, len(keys))
	meaningOmissions := make([]MeaningOmission, 0)
	degraded := make([]string, 0)
	for i, key := range keys {
		result, found := byKey[key]
		if !found {
			if required {
				return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: required enrichment result is missing", ErrInvalidInput)
			}
			aligned = append(aligned, exactEnrichmentFromStoredResult(StoredResult{CacheKey: key, Record: enrichment.CacheEntry{CacheKey: key}}, manifest, i))
			accepted = append(accepted, manifest.accepted[i])
			cacheKeys = append(cacheKeys, key)
			continue
		}
		if result.OmissionReason != "" {
			if result.Record.Translation != "" || result.Record.SentenceTranslation != "" || result.Record.FallbackGloss != "" {
				return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: unresolved result includes translation fields", ErrInvalidInput)
			}
			meaningOmissions = append(meaningOmissions, MeaningOmission{Target: manifest.accepted[i].TargetWord, Reason: result.OmissionReason})
			continue
		}
		if required && (!storedRecordHasRequiredFields(result.Record, manifest.accepted[i]) || (contextualIdentity && strings.TrimSpace(result.Record.FallbackGloss) == "")) {
			return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: required enrichment result is incomplete", ErrInvalidInput)
		}
		aligned = append(aligned, exactEnrichmentFromStoredResult(result, manifest, i))
		accepted = append(accepted, manifest.accepted[i])
		cacheKeys = append(cacheKeys, key)
	}
	if len(keys) > 0 && len(meaningOmissions) == len(keys) {
		return Artifact{}, FinalizeDiagnostics{}, ErrAllMeaningsUnresolved
	}
	manifest.accepted = accepted
	manifest.cacheKeys = cacheKeys
	artifact, renderDiagnostics, err := renderManifest(ctx, manifest, aligned)
	if err != nil {
		return Artifact{}, FinalizeDiagnostics{}, err
	}
	degraded = appendUniqueCodes(degraded, renderDiagnostics...)
	diagnostics := manifestDiagnostics(manifest)
	diagnostics.MeaningOmissions = append([]MeaningOmission(nil), meaningOmissions...)
	diagnostics.DegradationCodes = append(diagnostics.DegradationCodes, degraded...)
	artifact.Diagnostics = cloneDiagnostics(diagnostics)
	return artifact, diagnostics, nil
}

func projectionHeader(projections []CandidateProjection) (string, string, error) {
	owner, deckName := "", ""
	for _, projection := range projections {
		candidateOwner := projection.OwnerID
		if candidateOwner == "" {
			candidateOwner = projection.Candidate.OwnerID
		}
		if candidateOwner == "" {
			candidateOwner = projection.Entry.OwnerID
		}
		if owner == "" {
			owner = candidateOwner
		} else if candidateOwner != "" && owner != candidateOwner {
			return "", "", fmt.Errorf("%w: candidate projections have different owners", ErrInvalidInput)
		}
		candidateDeck := strings.TrimSpace(projection.DeckName)
		if candidateDeck == "" {
			candidateDeck = strings.TrimSpace(projection.Entry.SourceDocument)
		}
		if deckName == "" {
			deckName = candidateDeck
		} else if candidateDeck != "" && deckName != candidateDeck {
			return "", "", fmt.Errorf("%w: candidate projections have different deck names", ErrInvalidInput)
		}
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(deckName) == "" {
		return "", "", fmt.Errorf("%w: candidate projections require owner and deck name", ErrInvalidInput)
	}
	return owner, deckName, nil
}

func projectionProvider(projections []CandidateProjection) (string, string, string, error) {
	provider, version, target := "", "", ""
	set := false
	for _, projection := range projections {
		if !set {
			provider, version, target = projection.Provider, projection.ProviderVersion, projection.TargetLanguage
			set = true
		} else if provider != projection.Provider || version != projection.ProviderVersion || target != projection.TargetLanguage {
			return "", "", "", fmt.Errorf("%w: candidate projections have different enrichment providers", ErrInvalidInput)
		}
	}
	if provider == "" && version != "" {
		return "", "", "", fmt.Errorf("%w: enrichment provider version has no provider", ErrInvalidInput)
	}
	if provider != "" && strings.TrimSpace(version) == "" {
		return "", "", "", fmt.Errorf("%w: enrichment provider version is required", ErrInvalidInput)
	}
	if provider != "" && target == "" {
		target = "en"
	}
	return provider, version, target, nil
}

func manifestDiagnostics(manifest manifest) Diagnostics {
	entries := make([]RenderInput, 0, len(manifest.decisions))
	for _, decision := range manifest.decisions {
		entries = append(entries, renderInputFromEntry(decision.Entry))
	}
	return Diagnostics{QualityOmissions: cloneOmissions(manifest.omitted), EvidenceCoverage: evidenceCoverage(entries)}
}

func cloneOmissions(omissions []Omission) []Omission {
	result := make([]Omission, len(omissions))
	for i, omission := range omissions {
		result[i] = omission
		result[i].Reasons = append([]string(nil), omission.Reasons...)
	}
	return result
}

func cloneDiagnostics(diagnostics Diagnostics) Diagnostics {
	return Diagnostics{QualityOmissions: cloneOmissions(diagnostics.QualityOmissions), MeaningOmissions: append([]MeaningOmission(nil), diagnostics.MeaningOmissions...), DegradationCodes: append([]string(nil), diagnostics.DegradationCodes...), GlossCoverage: cloneGlossCoverage(diagnostics.GlossCoverage), EvidenceCoverage: append([]EvidenceCoverage(nil), diagnostics.EvidenceCoverage...)}
}

func evidenceCoverage(entries []RenderInput) []EvidenceCoverage {
	coverage := EvidenceCoverage{Source: "wiktionary", Configured: false, Selected: len(entries)}
	for _, entry := range entries {
		if entry.DictionaryProviderVersion != "" {
			coverage.Configured = true
		}
		if len(entry.CandidateSenses) > 0 {
			coverage.Matched++
		}
		coverage.Candidates += len(entry.CandidateSenses)
		coverage.Omitted += entry.OmittedEvidenceCount
	}
	return []EvidenceCoverage{coverage}
}

func (m manifest) decisionsOrdinalForAccepted(acceptedIndex int) int {
	seen := 0
	for _, decision := range m.decisions {
		if decision.Disposition != ManifestAccepted {
			continue
		}
		if seen == acceptedIndex {
			return decision.Ordinal
		}
		seen++
	}
	return -1
}

func containsCacheKey(keys []enrichment.CacheKey, wanted enrichment.CacheKey) bool {
	return slices.Contains(keys, wanted)
}

func storedRecordHasRequiredFields(record enrichment.CacheEntry, entry RenderInput) bool {
	return enrichment.HasRequiredTranslationFields(record, entry.Sentence)
}

func exactEnrichmentFromStoredResult(stored StoredResult, manifest manifest, acceptedIndex int) ExactEnrichment {
	record := stored.Record
	provenance := enrichment.Provenance{Provider: stored.CacheKey.Provider, ProviderVersion: stored.CacheKey.ProviderVersion, CachedAt: record.CachedAt, External: true}
	result := enrichment.Result{Candidate: manifest.enrichmentCandidates[acceptedIndex]}
	if record.Translation != "" {
		result.Translation = enrichment.Field[string]{Value: record.Translation, Available: true, Provenance: provenance}
	}
	if record.FallbackGloss != "" {
		result.FallbackGloss = enrichment.Field[string]{Value: record.FallbackGloss, Available: true, Provenance: provenance}
	}
	if record.SentenceTranslation != "" {
		result.SentenceTranslation = enrichment.Field[string]{Value: record.SentenceTranslation, Available: true, Provenance: provenance}
	}
	if record.SentenceTranslationTarget != "" {
		result.SentenceTranslationTarget = enrichment.Field[string]{Value: record.SentenceTranslationTarget, Available: true, Provenance: provenance}
	}
	if record.SentenceTranslationTargets != nil {
		result.SentenceTranslationTargets = enrichment.Field[[]string]{Value: append([]string(nil), record.SentenceTranslationTargets...), Available: true, Provenance: provenance}
	}
	if record.SenseSelection != nil {
		result.SenseSelection = enrichment.Field[[]int]{Value: append([]int(nil), record.SenseSelection...), Available: true, Provenance: provenance}
	}
	return ExactEnrichment{CacheKey: stored.CacheKey, Result: result}
}

func fallbackGlossEligible(gloss string) bool {
	value := strings.TrimSpace(gloss)
	return value != "" && !strings.ContainsAny(value, "<>") && len([]rune(value)) <= enrichment.MaxFallbackGlossRunes
}

func appendUniqueCode(codes []string, code string) []string {
	if slices.Contains(codes, code) {
		return codes
	}
	return append(codes, code)
}

func appendUniqueCodes(codes []string, additions ...string) []string {
	for _, code := range additions {
		codes = appendUniqueCode(codes, code)
	}
	return codes
}
