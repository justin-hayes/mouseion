package cardexport

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// CandidateProjection is the fact boundary assembled by selection and
// persistence adapters. Presentation owns all interpretation after this point.
type CandidateProjection struct {
	OwnerID, DeckName         string
	Candidate                 domain.SelectionCandidate
	Entry                     Entry
	Sentences                 map[int64]analyzer.Sentence
	Provider, ProviderVersion string
	TargetLanguage            string
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

// Diagnostics contains structured, content-free presentation diagnostics.
type Diagnostics struct {
	QualityOmissions []Omission
	DegradationCodes []string
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
	CacheKey enrichment.CacheKey
	Record   enrichment.CacheEntry
}

// StorageProjection is the normalized durable representation of a deck.
type StorageProjection = ManifestSnapshot

// FinalArtifact is the complete rendered prepared-deck result.
type FinalArtifact = Artifact

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
	manifest Manifest
}

// Freeze selects representative sentences, resolves local lexical facts,
// quality-gates each candidate, and constructs exact external identities.
func (p *Presentation) Freeze(ctx context.Context, projections []CandidateProjection) (FrozenDeck, FreezeDiagnostics, error) {
	return p.freeze(ctx, "", "", projections)
}

// FreezeForDeck freezes selected projections with the owner and deck identity
// supplied by the fact adapter. It also represents an empty local deck without
// inventing a candidate projection.
func (p *Presentation) FreezeForDeck(ctx context.Context, owner, deckName string, projections []CandidateProjection) (FrozenDeck, FreezeDiagnostics, error) {
	return p.freeze(ctx, owner, deckName, projections)
}

func (p *Presentation) freeze(ctx context.Context, explicitOwner, explicitDeckName string, projections []CandidateProjection) (FrozenDeck, FreezeDiagnostics, error) {
	if p == nil {
		return FrozenDeck{}, FreezeDiagnostics{}, ErrInvalidInput
	}
	if len(projections) == 0 && (strings.TrimSpace(explicitOwner) == "" || strings.TrimSpace(explicitDeckName) == "") {
		return FrozenDeck{}, FreezeDiagnostics{}, fmt.Errorf("%w: no candidate projections", ErrInvalidInput)
	}
	ordered := append([]CandidateProjection(nil), projections...)
	owner, deckName := explicitOwner, explicitDeckName
	if len(ordered) > 0 {
		projectionOwner, projectionDeckName, err := projectionHeader(ordered)
		if err != nil {
			return FrozenDeck{}, FreezeDiagnostics{}, err
		}
		if owner == "" {
			owner = projectionOwner
		} else if projectionOwner != "" && owner != projectionOwner {
			return FrozenDeck{}, FreezeDiagnostics{}, fmt.Errorf("%w: explicit owner contradicts candidate projections", ErrInvalidInput)
		}
		if deckName == "" {
			deckName = projectionDeckName
		} else if projectionDeckName != "" && deckName != projectionDeckName {
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

	service := &Service{lexical: p.lexical}
	entries := make([]Entry, 0, len(ordered))
	for _, projection := range ordered {
		entry := cloneEntry(projection.Entry)
		clearExternalFields(&entry)
		applySentenceDecision(&entry, projection.Candidate, projection.Sentences)
		if err := service.resolveLexicalEntry(ctx, &entry); err != nil {
			return FrozenDeck{}, FreezeDiagnostics{}, fmt.Errorf("%w: resolve lexical entry %s: %v", ErrInvalidInput, candidateKey(projection.Candidate), err)
		}
		entries = append(entries, entry)
	}
	manifest := NewManifest(owner, deckName, entries)
	if provider, version, target, err := projectionProvider(ordered); err != nil {
		return FrozenDeck{}, FreezeDiagnostics{}, err
	} else if provider != "" {
		keys := make([]enrichment.CacheKey, len(manifest.EnrichmentCandidates()))
		for i, candidate := range manifest.EnrichmentCandidates() {
			keys[i] = enrichment.CacheKey{
				Language: candidate.Language, TargetLanguage: target,
				CanonicalLemma: candidate.CanonicalLemma, UPOS: strings.ToUpper(candidate.UPOS),
				Provider: provider, ProviderVersion: version,
				DictionaryProviderVersion: candidate.DictionaryProviderVersion,
				SentenceHash:              enrichment.SentenceHash(candidate.ExampleSentence),
			}
		}
		manifest, err = manifest.BindCacheKeys(keys)
		if err != nil {
			return FrozenDeck{}, FreezeDiagnostics{}, err
		}
	}
	return FrozenDeck{manifest: manifest.clone()}, manifestDiagnostics(manifest), nil
}

// Restore validates a durable projection with its historical schema codec and
// rebuilds the frozen plan without selection, lexical lookup, or quality work.
func (p *Presentation) Restore(projection StorageProjection) (FrozenDeck, error) {
	if p == nil {
		return FrozenDeck{}, ErrInvalidInput
	}
	manifest, err := ManifestFromSnapshot(projection)
	if err != nil {
		return FrozenDeck{}, err
	}
	return FrozenDeck{manifest: manifest.clone()}, nil
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
			CandidateSenses: enrichment.CloneLexicalSenses(entry.CandidateSenses),
		},
		CacheKey: key, DictionaryProviderVersion: entry.DictionaryProviderVersion,
	}
}

func (d FrozenDeck) Summary() Summary {
	selected, accepted, omitted := d.manifest.Snapshot().Counts()
	return Summary{Completeness: d.manifest.Completeness(), Selected: selected, Accepted: accepted, Omitted: omitted}
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
		artifact, err := (&Service{}).RenderManifest(ctx, manifest, nil)
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
	required := facts.Consent && facts.Configured && strings.EqualFold(strings.TrimSpace(facts.ExecutionMode), "standard")
	aligned := make([]ExactEnrichment, len(keys))
	degraded := make([]string, 0)
	for i, key := range keys {
		result, found := byKey[key]
		if !found {
			if required {
				return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: required enrichment result is missing", ErrInvalidInput)
			}
			aligned[i] = exactEnrichmentFromStoredResult(StoredResult{CacheKey: key, Record: enrichment.CacheEntry{CacheKey: key}}, manifest, i)
			continue
		}
		if required && !storedRecordHasRequiredFields(result.Record, manifest.accepted[i]) {
			return Artifact{}, FinalizeDiagnostics{}, fmt.Errorf("%w: required enrichment result is incomplete", ErrInvalidInput)
		}
		aligned[i] = exactEnrichmentFromStoredResult(result, manifest, i)
	}
	artifact, renderDiagnostics, err := (&Service{}).renderManifest(ctx, manifest, aligned)
	if err != nil {
		return Artifact{}, FinalizeDiagnostics{}, err
	}
	degraded = appendUniqueCodes(degraded, renderDiagnostics...)
	diagnostics := manifestDiagnostics(manifest)
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

func manifestDiagnostics(manifest Manifest) Diagnostics {
	return Diagnostics{QualityOmissions: cloneOmissions(manifest.omitted)}
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
	return Diagnostics{QualityOmissions: cloneOmissions(diagnostics.QualityOmissions), DegradationCodes: append([]string(nil), diagnostics.DegradationCodes...)}
}

func (m Manifest) decisionsOrdinalForAccepted(acceptedIndex int) int {
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
	for _, key := range keys {
		if key == wanted {
			return true
		}
	}
	return false
}

func storedRecordHasRequiredFields(record enrichment.CacheEntry, entry RenderInput) bool {
	return enrichment.HasRequiredTranslationFields(record, entry.Sentence)
}

func exactEnrichmentFromStoredResult(stored StoredResult, manifest Manifest, acceptedIndex int) ExactEnrichment {
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
	for _, existing := range codes {
		if existing == code {
			return codes
		}
	}
	return append(codes, code)
}

func appendUniqueCodes(codes []string, additions ...string) []string {
	for _, code := range additions {
		codes = appendUniqueCode(codes, code)
	}
	return codes
}
