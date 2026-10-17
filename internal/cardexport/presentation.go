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
	// SkipMeaningEvidence marks a projection whose deck is built from source
	// evidence alone. It resolves no Wiktionary local dictionary entry and
	// freezes no candidate senses, so a source-evidence deck carries no
	// Wiktionary meaning evidence. An Ancient Greek projection still freezes its
	// matched LSJ entry, which is the Greek meaning evidence (ADR 0101).
	SkipMeaningEvidence bool
	// ReferenceTranslations maps a cited section to its imported English
	// translation, and ReferencePin is the pin it was imported from. They are
	// empty for a deck that has no imported reference translation; a Greek
	// projection resolves its representative sentence's passage context from
	// them (ADR 0101).
	ReferenceTranslations map[string]string
	ReferencePin          string
	// EvidenceAssistedPass marks a source-evidence projection that also opts
	// into the Ancient Greek provider pass. The projection still freezes its
	// LSJ entry and passage context, and the pass adds provider-proposed
	// fields on top only when the provider resolves a contextual Gloss
	// (ADR 0101).
	EvidenceAssistedPass bool
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
		Language: w.Request.Language, CanonicalLemma: w.Request.CanonicalLemma, UPOS: w.Request.UPOS,
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
	ManifestOrdinal     int
	Sentence            string
	SentenceTokens      []analyzer.Token
	SentenceStartOffset uint64
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

// Freeze selects representative sentences, resolves local lexical facts,
// quality-gates each candidate, and constructs exact external identities. The
// empty-deck variant is intentionally folded into this operation: the
// explicit identity supplies the metadata when there are no projections.
func (p *Presentation) Freeze(ctx context.Context, owner, deckName string, projections []CandidateProjection) (FrozenDeck, FreezeDiagnostics, error) {
	if p == nil {
		return FrozenDeck{}, FreezeDiagnostics{}, ErrInvalidInput
	}
	ordered, err := orderedProjections(owner, deckName, projections)
	if err != nil {
		return FrozenDeck{}, FreezeDiagnostics{}, err
	}
	entries, err := freezeEntries(ctx, p.lexical, ordered)
	if err != nil {
		return FrozenDeck{}, FreezeDiagnostics{}, err
	}
	coverage := glossCoverage(entries)
	frozen, err := bindFrozenCacheKeys(newManifest(owner, deckName, entries), ordered)
	if err != nil {
		return FrozenDeck{}, FreezeDiagnostics{}, err
	}
	diagnostics := manifestDiagnostics(frozen)
	diagnostics.GlossCoverage = cloneGlossCoverage(coverage)
	return FrozenDeck{manifest: frozen.clone()}, diagnostics, nil
}

// orderedProjections validates the explicit owner and deck name against the
// candidate projections, then returns them in stable first-encounter order.
func orderedProjections(owner, deckName string, projections []CandidateProjection) ([]CandidateProjection, error) {
	ordered := append([]CandidateProjection(nil), projections...)
	if len(ordered) > 0 {
		projectionOwner, projectionDeckName, err := projectionHeader(ordered)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(owner) == "" || owner != projectionOwner {
			return nil, fmt.Errorf("%w: explicit owner contradicts candidate projections", ErrInvalidInput)
		}
		if strings.TrimSpace(deckName) == "" || deckName != projectionDeckName {
			return nil, fmt.Errorf("%w: explicit deck name contradicts candidate projections", ErrInvalidInput)
		}
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(deckName) == "" {
		return nil, fmt.Errorf("%w: freeze requires owner and deck name", ErrInvalidInput)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Candidate.FirstEncounter != ordered[j].Candidate.FirstEncounter {
			return ordered[i].Candidate.FirstEncounter < ordered[j].Candidate.FirstEncounter
		}
		if left, right := candidateKey(ordered[i].Candidate), candidateKey(ordered[j].Candidate); left != right {
			return left < right
		}
		return ordered[i].Candidate.LexicalDiscriminator < ordered[j].Candidate.LexicalDiscriminator
	})
	return ordered, nil
}

// freezeEntries turns each projection into a render entry: it clones the entry,
// clears external fields, applies the frozen sentence decision and resolves
// local lexical facts. A source-evidence projection resolves no local lexical
// facts, so its entry keeps the representative sentence alone.
func freezeEntries(ctx context.Context, lexical enrichment.LexicalProvider, ordered []CandidateProjection) ([]Entry, error) {
	resolver := &lexicalResolver{lexical: lexical}
	entries := make([]Entry, 0, len(ordered))
	for _, projection := range ordered {
		entry := cloneEntry(projection.Entry)
		clearExternalFields(&entry)
		applySentenceDecision(&entry, projection.Candidate, projection.Sentences)
		applyPassageContext(&entry, projection)
		entry.ProviderProposed = projection.EvidenceAssistedPass
		if projection.SkipMeaningEvidence {
			clearDictionaryFacts(&entry)
			matched, found := resolveAncientGreekLSJ(ctx, lexical, &entry)
			if projection.EvidenceAssistedPass && found {
				// The pass transmits the pinned LSJ senses with their native IDs
				// as its meaning evidence; the displayed entry is the frozen
				// entry above (ADR 0101).
				entry.CandidateSenses = ancientGreekLSJPassSenses(matched)
			}
			entries = append(entries, entry)
			continue
		}
		if err := resolver.resolveLexicalEntry(ctx, &entry); err != nil {
			return nil, fmt.Errorf("%w: resolve lexical entry %s: %w", ErrInvalidInput, candidateKey(projection.Candidate), err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// applyPassageContext freezes the imported Brownson passage of the sections a
// representative sentence touches onto an Ancient Greek entry. A deck without
// imported translations, and every modern-language entry, is left unchanged; a
// sentence whose touched sections are not all translated keeps no English.
func applyPassageContext(entry *Entry, projection CandidateProjection) {
	if !IsAncientGreek(entry.Language) || len(projection.ReferenceTranslations) == 0 {
		return
	}
	sentence, ok := projection.Sentences[entry.SentenceOrdinal]
	if !ok {
		return
	}
	entry.PassageContext, entry.PassageCitation, entry.BrownsonPin = resolvePassage(sentence.Citations, projection.ReferenceTranslations, projection.ReferencePin)
}

// bindFrozenCacheKeys gives an externally enriched manifest its exact cache
// identities. A manifest without a provider keeps no cache keys.
func bindFrozenCacheKeys(built manifest, ordered []CandidateProjection) (manifest, error) {
	provider, version, target, err := projectionProvider(ordered)
	if err != nil {
		return manifest{}, err
	}
	if provider == "" {
		return built, nil
	}
	candidates := built.enrichmentCandidatesProjection()
	keys := make([]enrichment.CacheKey, len(candidates))
	contextualGlossByCandidate := make(map[string]bool, len(ordered))
	for _, projection := range ordered {
		contextualGlossByCandidate[candidateKey(projection.Candidate)] = projection.RequireContextualGloss || projection.EvidenceAssistedPass
	}
	for i, candidate := range candidates {
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
	return built.bindCacheKeys(keys)
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
		decisionIndex, acceptedIndex, err := matchRecoveredRenderInput(manifest, input, acceptedIndexes)
		if err != nil {
			return err
		}
		manifest.decisions[decisionIndex].Entry.SentenceTokens = cloneTokens(input.SentenceTokens)
		manifest.decisions[decisionIndex].Entry.SentenceStartOffset = input.SentenceStartOffset
		manifest.accepted[acceptedIndex].SentenceTokens = cloneTokens(input.SentenceTokens)
		manifest.accepted[acceptedIndex].SentenceStartOffset = input.SentenceStartOffset
	}
	return nil
}

// matchRecoveredRenderInput locates the accepted decision a recovered render
// input belongs to. It rejects an input whose ordinal is unknown, omitted or
// duplicated, whose sentence disagrees with the manifest, or that is already
// present.
func matchRecoveredRenderInput(manifest *manifest, input RecoveredRenderInput, acceptedIndexes map[int]int) (decisionIndex, acceptedIndex int, err error) {
	decisionIndex = -1
	for index, decision := range manifest.decisions {
		if decision.Ordinal == input.ManifestOrdinal {
			decisionIndex = index
			break
		}
	}
	acceptedIndex, accepted := acceptedIndexes[input.ManifestOrdinal]
	if decisionIndex < 0 || !accepted || manifest.decisions[decisionIndex].Disposition != ManifestAccepted {
		return -1, -1, fmt.Errorf("%w: recovered render input %d is not accepted", ErrInvalidInput, input.ManifestOrdinal)
	}
	decision := manifest.decisions[decisionIndex]
	if input.Sentence != decision.Entry.Sentence || strings.TrimSpace(input.Sentence) == "" || len(input.SentenceTokens) == 0 {
		return -1, -1, fmt.Errorf("%w: recovered render input %d does not match the manifest", ErrInvalidInput, input.ManifestOrdinal)
	}
	if len(decision.Entry.SentenceTokens) > 0 {
		return -1, -1, fmt.Errorf("%w: render input %d is already present", ErrInvalidInput, input.ManifestOrdinal)
	}
	return decisionIndex, acceptedIndex, nil
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
			PassageContext:  entry.PassageContext,
			CandidateSenses: enrichment.CloneLexicalSenses(entry.CandidateSenses),
			// A pass entry resolves its contextual Gloss through the
			// evidence-assisted contract, not the modern-language
			// contextual-gloss contract.
			RequireContextualGloss:      key.MeaningEvidenceHash != "" && !entry.ProviderProposed,
			RequireEvidenceAssistedPass: entry.ProviderProposed,
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
	if len(manifest.cacheKeys) == 0 {
		return finalizeLocalDeck(ctx, manifest, results)
	}
	if err := validateExternalRunFacts(manifest, facts); err != nil {
		return Artifact{}, FinalizeDiagnostics{}, err
	}
	byKey, err := indexStoredResults(manifest.cacheKeys, results)
	if err != nil {
		return Artifact{}, FinalizeDiagnostics{}, err
	}
	alignment, err := alignStoredResults(manifest, byKey, facts)
	if err != nil {
		return Artifact{}, FinalizeDiagnostics{}, err
	}
	manifest.accepted = alignment.accepted
	manifest.cacheKeys = alignment.cacheKeys
	artifact, renderDiagnostics, err := renderManifest(ctx, manifest, alignment.enrichments)
	if err != nil {
		return Artifact{}, FinalizeDiagnostics{}, err
	}
	diagnostics := manifestDiagnostics(manifest)
	diagnostics.MeaningOmissions = append([]MeaningOmission(nil), alignment.meaningOmissions...)
	diagnostics.DegradationCodes = appendUniqueCodes(diagnostics.DegradationCodes, renderDiagnostics...)
	artifact.Diagnostics = cloneDiagnostics(diagnostics)
	return artifact, diagnostics, nil
}

// finalizeLocalDeck renders a deck that never froze an external identity. It
// rejects supplied external results because no work was requested for it.
func finalizeLocalDeck(ctx context.Context, manifest manifest, results []StoredResult) (FinalArtifact, FinalizeDiagnostics, error) {
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

// validateExternalRunFacts enforces that a consented, configured external run
// describes exactly the provider, version, target and execution mode frozen into
// the manifest. Unconsented or unconfigured runs select no external policy.
func validateExternalRunFacts(manifest manifest, facts RunFacts) error {
	if !facts.Consent || !facts.Configured {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(facts.ExecutionMode))
	if mode != "standard" && mode != "batch" {
		return fmt.Errorf("%w: invalid prepared deck execution mode", ErrInvalidInput)
	}
	if facts.TargetLanguage == "" || facts.Provider == "" || facts.ProviderVersion == "" {
		return fmt.Errorf("%w: incomplete external run facts", ErrInvalidInput)
	}
	for _, key := range manifest.cacheKeys {
		historicalV1Target := manifest.schemaVersion == LegacyManifestSchemaVersion && key.TargetLanguage == ""
		if (!historicalV1Target && key.TargetLanguage != facts.TargetLanguage) || key.Provider != facts.Provider || key.ProviderVersion != facts.ProviderVersion {
			return fmt.Errorf("%w: run facts contradict manifest cache identity", ErrInvalidInput)
		}
	}
	return nil
}

// indexStoredResults validates and indexes the exact stored enrichment rows by
// cache identity. A duplicate row, a row whose record identity disagrees with
// its key, and a row for a key the manifest never froze are each rejected.
func indexStoredResults(keys []enrichment.CacheKey, results []StoredResult) (map[enrichment.CacheKey]StoredResult, error) {
	byKey := make(map[enrichment.CacheKey]StoredResult, len(results))
	for _, result := range results {
		if _, duplicate := byKey[result.CacheKey]; duplicate {
			return nil, fmt.Errorf("%w: duplicate stored enrichment result", ErrInvalidInput)
		}
		if result.Record.CacheKey != (enrichment.CacheKey{}) && result.Record.CacheKey != result.CacheKey {
			return nil, fmt.Errorf("%w: stored enrichment record identity does not match key", ErrInvalidInput)
		}
		if !containsCacheKey(keys, result.CacheKey) {
			return nil, fmt.Errorf("%w: stored enrichment identity is not frozen", ErrInvalidInput)
		}
		byKey[result.CacheKey] = result
	}
	return byKey, nil
}

// finalizeAlignment is the render plan produced by overlaying stored results:
// the exact enrichments to apply, the surviving accepted inputs and cache keys,
// and the meanings the external provider explicitly could not resolve.
type finalizeAlignment struct {
	enrichments      []ExactEnrichment
	accepted         []RenderInput
	cacheKeys        []enrichment.CacheKey
	meaningOmissions []MeaningOmission
}

// alignStoredResults overlays stored results onto the frozen manifest in
// manifest order. Each accepted candidate survives with an exact enrichment, an
// explicitly unresolved meaning is dropped and recorded, and a missing result
// is tolerated only when the run does not require one.
func alignStoredResults(manifest manifest, byKey map[enrichment.CacheKey]StoredResult, facts RunFacts) (finalizeAlignment, error) {
	keys := manifest.cacheKeys
	providerProposed := manifestHasProviderProposed(manifest)
	required, contextualIdentity := externalResultRequirements(keys, facts, providerProposed)
	alignment := finalizeAlignment{
		enrichments:      make([]ExactEnrichment, 0, len(keys)),
		accepted:         make([]RenderInput, 0, len(manifest.accepted)),
		cacheKeys:        make([]enrichment.CacheKey, 0, len(keys)),
		meaningOmissions: make([]MeaningOmission, 0),
	}
	for i, key := range keys {
		stored, err := storedResultFor(key, byKey, required)
		if err != nil {
			return finalizeAlignment{}, err
		}
		if stored.OmissionReason != "" {
			if providerProposed {
				// The evidence-assisted pass only adds: a recorded abstention
				// or tolerated provider failure leaves the source-evidence card
				// exactly as it was rather than dropping the item (ADR 0101).
				alignment.enrichments = append(alignment.enrichments, exactEnrichmentFromStoredResult(stored, manifest, i))
				alignment.accepted = append(alignment.accepted, manifest.accepted[i])
				alignment.cacheKeys = append(alignment.cacheKeys, key)
				continue
			}
			omission, err := unresolvedOmission(stored, manifest.accepted[i].TargetWord)
			if err != nil {
				return finalizeAlignment{}, err
			}
			alignment.meaningOmissions = append(alignment.meaningOmissions, omission)
			continue
		}
		if requiredResultIncomplete(stored, manifest.accepted[i], required, contextualIdentity) {
			return finalizeAlignment{}, fmt.Errorf("%w: required enrichment result is incomplete", ErrInvalidInput)
		}
		alignment.enrichments = append(alignment.enrichments, exactEnrichmentFromStoredResult(stored, manifest, i))
		alignment.accepted = append(alignment.accepted, manifest.accepted[i])
		alignment.cacheKeys = append(alignment.cacheKeys, key)
	}
	if !providerProposed && len(keys) > 0 && len(alignment.meaningOmissions) == len(keys) {
		return finalizeAlignment{}, ErrAllMeaningsUnresolved
	}
	return alignment, nil
}

// manifestHasProviderProposed reports whether the frozen deck is an Ancient
// Greek evidence-assisted pass deck. Its accepted entries alone carry the
// provider-proposed flag, so a missing result never drops the source-evidence
// card.
func manifestHasProviderProposed(manifest manifest) bool {
	return slices.ContainsFunc(manifest.accepted, func(input RenderInput) bool { return input.ProviderProposed })
}

// storedResultFor resolves the exact stored result for one frozen key. A missing
// result is an error when the run requires one and an empty placeholder
// otherwise.
func storedResultFor(key enrichment.CacheKey, byKey map[enrichment.CacheKey]StoredResult, required bool) (StoredResult, error) {
	stored, found := byKey[key]
	if found {
		return stored, nil
	}
	if required {
		return StoredResult{}, fmt.Errorf("%w: required enrichment result is missing", ErrInvalidInput)
	}
	return StoredResult{CacheKey: key, Record: enrichment.CacheEntry{CacheKey: key}}, nil
}

// unresolvedOmission converts an explicitly unresolved stored result into a
// meaning omission, rejecting one that still carries translation fields.
func unresolvedOmission(stored StoredResult, target string) (MeaningOmission, error) {
	if stored.Record.Translation != "" || stored.Record.SentenceTranslation != "" || stored.Record.FallbackGloss != "" {
		return MeaningOmission{}, fmt.Errorf("%w: unresolved result includes translation fields", ErrInvalidInput)
	}
	return MeaningOmission{Target: target, Reason: stored.OmissionReason}, nil
}

// requiredResultIncomplete reports whether a stored result lacks fields the run
// requires, including the contextual Gloss a contextual identity demands.
func requiredResultIncomplete(stored StoredResult, entry RenderInput, required, contextualIdentity bool) bool {
	if !required {
		return false
	}
	if !storedRecordHasRequiredFields(stored.Record, entry) {
		return true
	}
	return contextualIdentity && strings.TrimSpace(stored.Record.FallbackGloss) == ""
}

// externalResultRequirements reports whether a missing stored result is an error
// for this run, and whether any frozen identity carries contextual evidence that
// makes a contextual Gloss mandatory. An evidence-assisted pass never requires a
// stored result: the pass only adds, so a missing one leaves the source-evidence
// card (ADR 0101).
func externalResultRequirements(keys []enrichment.CacheKey, facts RunFacts, providerProposed bool) (required, contextualIdentity bool) {
	mode := strings.TrimSpace(facts.ExecutionMode)
	contextualIdentity = slices.ContainsFunc(keys, func(key enrichment.CacheKey) bool { return key.MeaningEvidenceHash != "" })
	contextualBatch := strings.EqualFold(mode, "batch") && contextualIdentity
	required = facts.Consent && facts.Configured && !providerProposed && (strings.EqualFold(mode, "standard") || contextualBatch)
	return required, contextualIdentity
}

func projectionHeader(projections []CandidateProjection) (string, string, error) {
	owner, deckName := "", ""
	for _, projection := range projections {
		if !accumulateProjectionField(&owner, projectionOwner(projection)) {
			return "", "", fmt.Errorf("%w: candidate projections have different owners", ErrInvalidInput)
		}
		if !accumulateProjectionField(&deckName, projectionDeckName(projection)) {
			return "", "", fmt.Errorf("%w: candidate projections have different deck names", ErrInvalidInput)
		}
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(deckName) == "" {
		return "", "", fmt.Errorf("%w: candidate projections require owner and deck name", ErrInvalidInput)
	}
	return owner, deckName, nil
}

// accumulateProjectionField keeps the first non-empty value and reports false
// when a later non-empty value disagrees with it.
func accumulateProjectionField(current *string, candidate string) bool {
	if candidate == "" || *current == candidate {
		return true
	}
	if *current == "" {
		*current = candidate
		return true
	}
	return false
}

// projectionOwner resolves the owner a projection declares, preferring its
// explicit owner and falling back to the candidate then entry identity.
func projectionOwner(projection CandidateProjection) string {
	for _, candidate := range []string{projection.OwnerID, projection.Candidate.OwnerID, projection.Entry.OwnerID} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// projectionDeckName resolves the deck name a projection declares, preferring
// its explicit name and falling back to its source document.
func projectionDeckName(projection CandidateProjection) string {
	if name := strings.TrimSpace(projection.DeckName); name != "" {
		return name
	}
	return strings.TrimSpace(projection.Entry.SourceDocument)
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
