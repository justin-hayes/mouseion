package cardexport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// ManifestSchemaVersion identifies the canonical durable manifest codec. A
// new version is required for any change that alters digest inputs.
const (
	ManifestSchemaVersionV12 = 12
	ManifestSchemaVersionV13 = 13
	ManifestSchemaVersionV14 = 14
	ManifestSchemaVersionV15 = 15
	ManifestSchemaVersionV16 = 16
	ManifestSchemaVersion    = ManifestSchemaVersionV16

	// MinimumManifestSchemaVersion is the oldest manifest codec this build
	// decodes. Every caller that reads a stored manifest reads a live deck or a
	// deck whose run is in flight, so no reachable manifest predates it.
	MinimumManifestSchemaVersion = ManifestSchemaVersionV12
)

type ManifestDisposition string

const (
	ManifestAccepted       ManifestDisposition = "accepted"
	ManifestQualityOmitted ManifestDisposition = "quality_omitted"
)

var manifestQualityReasons = map[string]struct{}{
	"too short or fragmented":         {},
	"too long":                        {},
	"usable length":                   {},
	"useful context window":           {},
	"target not present as a word":    {},
	"target present":                  {},
	"invalid source location":         {},
	"valid source location":           {},
	"complete sentence boundaries":    {},
	"incomplete sentence boundaries":  {},
	"structural noise or boilerplate": {},
	"no obvious structural noise":     {},
	"no finite verb and subject":      {},
	"target in subordinate clause":    {},
	"deictic context":                 {},
	"named-entity density":            {},
	"optimal length":                  {},
	"outside optimal length":          {},
}

// ManifestItem is one frozen selection decision. Entry contains only
// provider-independent render inputs; external result fields must be empty.
type ManifestItem struct {
	Ordinal         int
	Disposition     ManifestDisposition
	CorpusID        string
	SentenceOrdinal int64
	// ItemKind is the frozen card kind. The zero value is a lemma item, which
	// is every item frozen before idiom cards existed.
	ItemKind enrichment.ItemKind
	Entry    Entry
	Quality  SentenceQuality
	CacheKey *enrichment.CacheKey
}

// ManifestSnapshot is the versioned, immutable representation persisted for a
// durable preparation run. Items retain selection order, including omissions.
type ManifestSnapshot struct {
	SchemaVersion int
	Owner         string
	DeckName      string
	Filename      string
	// IdiomOverflow is how many candidate idioms the per-deck bound left out
	// while freezing. It is part of manifest schema v16 and is not an omission
	// (doc/features/idiom-cards.md).
	IdiomOverflow int
	Items         []ManifestItem
}

func (m manifest) Snapshot() ManifestSnapshot {
	schemaVersion := m.schemaVersion
	if schemaVersion == 0 {
		schemaVersion = ManifestSchemaVersion
	}
	return ManifestSnapshot{
		SchemaVersion: schemaVersion,
		Owner:         m.owner,
		DeckName:      m.deckName,
		Filename:      DownloadFilename(m.deckName),
		IdiomOverflow: m.idiomOverflow,
		Items:         cloneManifestItems(m.decisions),
	}
}

func (s ManifestSnapshot) Counts() (selected, accepted, omitted int) {
	selected = len(s.Items)
	for _, item := range s.Items {
		switch item.Disposition {
		case ManifestAccepted:
			accepted++
		case ManifestQualityOmitted:
			omitted++
		}
	}
	return selected, accepted, omitted
}

// Digest returns the lowercase SHA-256 of the canonical, versioned snapshot.
func (s ManifestSnapshot) Digest() (string, error) {
	canonical, err := s.canonical()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("cardexport: encode manifest: %w", err)
	}
	return digestDomain("manifest", s.SchemaVersion, payload), nil
}

// digestDomain prefixes a digest so no two codec domains or schema versions
// share an identity.
func digestDomain(kind string, version int, payload []byte) string {
	prefix := fmt.Sprintf("mouseion-prepared-deck-%s-v%d\x00", kind, version)
	sum := sha256.Sum256(append([]byte(prefix), payload...))
	return hex.EncodeToString(sum[:])
}

// Digests validates a storage projection and returns the manifest and item
// identities that persistence stores alongside its normalized rows.
func (s ManifestSnapshot) Digests() (string, []string, error) {
	if _, err := s.canonical(); err != nil {
		return "", nil, err
	}
	manifestDigest, err := s.Digest()
	if err != nil {
		return "", nil, err
	}
	candidateDigests := make([]string, len(s.Items))
	for i, item := range s.Items {
		candidateDigests[i], err = CandidateDigestVersion(item, s.SchemaVersion)
		if err != nil {
			return "", nil, err
		}
	}
	return manifestDigest, candidateDigests, nil
}

// ValidateDigests checks identities recovered from normalized persistence
// without exposing the historical codec to the persistence adapter.
func (s ManifestSnapshot) ValidateDigests(manifestDigest string, candidateDigests []string) error {
	calculatedManifest, calculatedCandidates, err := s.Digests()
	if err != nil {
		return err
	}
	if calculatedManifest != manifestDigest || len(calculatedCandidates) != len(candidateDigests) {
		return fmt.Errorf("%w: stored manifest identity does not match projection", ErrInvalidInput)
	}
	for i := range calculatedCandidates {
		if calculatedCandidates[i] != candidateDigests[i] {
			return fmt.Errorf("%w: stored candidate identity does not match projection", ErrInvalidInput)
		}
	}
	return nil
}

// CandidateDigest returns the durable identity of one ordered decision.
func CandidateDigest(item ManifestItem) (string, error) {
	return CandidateDigestVersion(item, ManifestSchemaVersion)
}

// CandidateDigestVersion preserves historical digest codecs for durable
// manifests written before later render inputs were added.
func CandidateDigestVersion(item ManifestItem, schemaVersion int) (string, error) {
	if schemaVersion < MinimumManifestSchemaVersion || schemaVersion > ManifestSchemaVersion {
		return "", fmt.Errorf("%w: invalid manifest schema version", ErrInvalidInput)
	}
	canonical, err := canonicalizeManifestItem(item, schemaVersion)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("cardexport: encode manifest item: %w", err)
	}
	return digestDomain("candidate", schemaVersion, payload), nil
}

// manifestFromSnapshot validates a persisted snapshot and rebuilds the exact
// in-memory render plan without re-running selection or the quality gate.
func manifestFromSnapshot(snapshot ManifestSnapshot) (manifest, error) {
	if _, err := snapshot.canonical(); err != nil {
		return manifest{}, err
	}
	manifest := manifest{
		owner: snapshot.Owner, deckName: snapshot.DeckName,
		schemaVersion: snapshot.SchemaVersion,
		accepted:      make([]RenderInput, 0, len(snapshot.Items)), omitted: make([]Omission, 0, len(snapshot.Items)),
		enrichmentCandidates: make([]enrichment.Candidate, 0, len(snapshot.Items)),
		decisions:            cloneManifestItems(snapshot.Items),
	}
	if snapshot.SchemaVersion >= ManifestSchemaVersionV16 {
		manifest.idiomOverflow = snapshot.IdiomOverflow
	}
	allAcceptedHaveCacheKeys := true
	for _, item := range snapshot.Items {
		switch item.Disposition {
		case ManifestAccepted:
			entry := item.Entry
			entry.ItemKind = item.ItemKind.Normalized()
			input := renderInputFromEntry(entry)
			// Historical manifests keep their shipped Text rendering; only the
			// schema that froze attribution also froze source-line presentation.
			input.sourceLinePresentation = snapshot.SchemaVersion >= ManifestSchemaVersionV13
			manifest.accepted = append(manifest.accepted, input)
			manifest.enrichmentCandidates = append(manifest.enrichmentCandidates, enrichment.Candidate{
				Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS,
				TargetWord:                item.Entry.TargetWord,
				ExampleSentence:           strings.TrimSpace(item.Entry.Sentence),
				DictionaryProviderVersion: item.Entry.DictionaryProviderVersion,
				CandidateSenses:           enrichment.CloneLexicalSenses(item.Entry.CandidateSenses),
			})
			if item.CacheKey == nil {
				allAcceptedHaveCacheKeys = false
			} else {
				manifest.cacheKeys = append(manifest.cacheKeys, *item.CacheKey)
			}
		case ManifestQualityOmitted:
			manifest.omitted = append(manifest.omitted, Omission{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, Score: item.Quality.Score, Reasons: append([]string(nil), item.Quality.Reasons...)})
		}
	}
	if !allAcceptedHaveCacheKeys {
		manifest.cacheKeys = nil
	}
	return manifest, nil
}

type canonicalSnapshot struct {
	SchemaVersion int                     `json:"schema_version"`
	Owner         string                  `json:"owner"`
	DeckName      string                  `json:"deck_name"`
	Filename      string                  `json:"filename"`
	IdiomOverflow int                     `json:"idiom_overflow,omitempty"`
	Items         []canonicalManifestItem `json:"items"`
}

type canonicalManifestItem struct {
	Ordinal     int                      `json:"ordinal"`
	Disposition ManifestDisposition      `json:"disposition"`
	ItemKind    enrichment.ItemKind      `json:"item_kind,omitempty"`
	Entry       canonicalEntry           `json:"entry"`
	Quality     canonicalSentenceQuality `json:"quality"`
	CacheKey    *canonicalCacheKey       `json:"cache_key"`
}

// canonicalAttribution is the versioned codec for the optional typed source
// attribution introduced by manifest schema v13 (ADR 0103).
type canonicalAttribution struct {
	Kind    string `json:"kind"`
	Speaker string `json:"speaker,omitempty"`
}

// canonicalIdiomEvidence is the versioned codec for the frozen idiom evidence
// introduced by manifest schema v15 (ADR 0114).
type canonicalIdiomEvidence struct {
	MatchedTokenOrdinals       []int  `json:"matched_token_ordinals"`
	AnchorLanguage             string `json:"anchor_language"`
	AnchorCanonicalLemma       string `json:"anchor_canonical_lemma"`
	AnchorUPOS                 string `json:"anchor_upos"`
	AnchorLexicalDiscriminator string `json:"anchor_lexical_discriminator,omitempty"`
	Source                     string `json:"source"`
	SourceVersion              string `json:"source_version,omitempty"`
	MatcherVersion             string `json:"matcher_version"`
	GapBound                   int    `json:"gap_bound"`
}

type canonicalEntry struct {
	Language                  string                  `json:"language"`
	CanonicalLemma            string                  `json:"canonical_lemma"`
	UPOS                      string                  `json:"upos"`
	LexicalDiscriminator      string                  `json:"lexical_discriminator,omitempty"`
	Headword                  string                  `json:"headword,omitempty"`
	Idiom                     *canonicalIdiomEvidence `json:"idiom,omitempty"`
	Sentence                  string                  `json:"sentence"`
	TargetWord                string                  `json:"target_word"`
	Morphology                string                  `json:"morphology"`
	Gloss                     string                  `json:"gloss,omitempty"`
	Plural                    string                  `json:"plural,omitempty"`
	IPA                       string                  `json:"ipa,omitempty"`
	PrincipalParts            string                  `json:"principal_parts,omitempty"`
	DictionaryProviderVersion string                  `json:"dictionary_provider_version,omitempty"`
	PassageContext            string                  `json:"passage_context,omitempty"`
	PassageCitation           string                  `json:"passage_citation,omitempty"`
	// PassagePin keeps the brownson_pin key so manifests frozen before
	// translations were credited per file keep their canonical bytes.
	PassagePin                string                    `json:"brownson_pin,omitempty"`
	DictionaryEntry           string                    `json:"dictionary_entry,omitempty"`
	DictionaryEntryID         string                    `json:"dictionary_entry_id,omitempty"`
	DictionaryPin             string                    `json:"dictionary_pin,omitempty"`
	LSJSummaryProviderVersion string                    `json:"lsj_summary_provider_version,omitempty"`
	OmittedEvidenceCount      int                       `json:"omitted_evidence_count,omitempty"`
	CandidateSenses           []enrichment.LexicalSense `json:"candidate_senses,omitempty"`
	SourceDocument            string                    `json:"source_document"`
	Notes                     string                    `json:"notes"`
	FirstEncounter            int64                     `json:"first_encounter"`
	Attribution               *canonicalAttribution     `json:"attribution,omitempty"`
}

type canonicalSentenceQuality struct {
	Accepted  bool     `json:"accepted"`
	Score     int      `json:"score"`
	GDEXScore float64  `json:"gdex_score,omitempty"`
	Reasons   []string `json:"reasons"`
}

type canonicalCacheKey struct {
	Language            string `json:"language"`
	TargetLanguage      string `json:"target_language,omitempty"`
	CanonicalLemma      string `json:"canonical_lemma"`
	UPOS                string `json:"upos"`
	ItemKind            string `json:"item_kind,omitempty"`
	Provider            string `json:"provider"`
	ProviderVersion     string `json:"provider_version"`
	SentenceHash        string `json:"sentence_hash"`
	MeaningEvidenceHash string `json:"meaning_evidence_hash,omitempty"`
}

func (s ManifestSnapshot) canonical() (canonicalSnapshot, error) {
	if err := s.validateHeader(); err != nil {
		return canonicalSnapshot{}, err
	}
	if s.SchemaVersion >= ManifestSchemaVersionV16 && s.IdiomOverflow < 0 {
		return canonicalSnapshot{}, fmt.Errorf("%w: invalid manifest idiom overflow", ErrInvalidInput)
	}
	result := canonicalSnapshot{SchemaVersion: s.SchemaVersion, Owner: s.Owner, DeckName: s.DeckName, Filename: s.Filename, Items: make([]canonicalManifestItem, len(s.Items))}
	if s.SchemaVersion >= ManifestSchemaVersionV16 {
		result.IdiomOverflow = s.IdiomOverflow
	}
	seen := make(map[string]struct{}, len(s.Items))
	acceptedWithKey, acceptedWithoutKey := 0, 0
	for i, item := range s.Items {
		canonical, cacheKeyPresent, err := canonicalizeSnapshotItem(item, i, s.SchemaVersion, seen)
		if err != nil {
			return canonicalSnapshot{}, err
		}
		if item.Disposition == ManifestAccepted {
			if cacheKeyPresent {
				acceptedWithKey++
			} else {
				acceptedWithoutKey++
			}
		}
		result.Items[i] = canonical
	}
	if acceptedWithKey > 0 && acceptedWithoutKey > 0 {
		return canonicalSnapshot{}, fmt.Errorf("%w: accepted manifest cache identity is partial", ErrInvalidInput)
	}
	return result, nil
}

// validateHeader rejects a snapshot whose schema version is outside the
// supported range or whose owner and deck identity are missing or inconsistent
// with the download name.
func (s ManifestSnapshot) validateHeader() error {
	if s.SchemaVersion < MinimumManifestSchemaVersion || s.SchemaVersion > ManifestSchemaVersion || strings.TrimSpace(s.Owner) == "" || strings.TrimSpace(s.DeckName) == "" || s.Filename != DownloadFilename(s.DeckName) {
		return fmt.Errorf("%w: invalid manifest header", ErrInvalidInput)
	}
	return nil
}

// canonicalizeSnapshotItem validates one ordered decision, records its
// position-independent identity in seen, and returns its canonical form. The
// second result reports whether an accepted item carries a cache key.
func canonicalizeSnapshotItem(item ManifestItem, ordinal, schemaVersion int, seen map[string]struct{}) (canonicalManifestItem, bool, error) {
	if item.Ordinal != ordinal {
		return canonicalManifestItem{}, false, fmt.Errorf("%w: manifest ordinal %d is not contiguous", ErrInvalidInput, item.Ordinal)
	}
	canonical, err := canonicalizeManifestItem(item, schemaVersion)
	if err != nil {
		return canonicalManifestItem{}, false, fmt.Errorf("manifest item %d: %w", ordinal, err)
	}
	// Candidate digests include the ordinal because the persisted identity is
	// order-sensitive. Duplicate validation must ignore that position so a
	// candidate cannot be repeated at a different ordinal.
	identity := item
	identity.Ordinal = 0
	digest, err := CandidateDigestVersion(identity, schemaVersion)
	if err != nil {
		return canonicalManifestItem{}, false, err
	}
	if _, exists := seen[digest]; exists {
		return canonicalManifestItem{}, false, fmt.Errorf("%w: duplicate manifest candidate", ErrInvalidInput)
	}
	seen[digest] = struct{}{}
	return canonical, item.CacheKey != nil, nil
}

func canonicalizeManifestItem(item ManifestItem, schemaVersion int) (canonicalManifestItem, error) {
	if err := validateManifestEntry(item.Entry); err != nil {
		return canonicalManifestItem{}, err
	}
	if item.Disposition != ManifestAccepted && item.Disposition != ManifestQualityOmitted {
		return canonicalManifestItem{}, fmt.Errorf("%w: invalid manifest disposition", ErrInvalidInput)
	}
	if err := validateManifestItemKind(item); err != nil {
		return canonicalManifestItem{}, err
	}
	if err := validateManifestQuality(item); err != nil {
		return canonicalManifestItem{}, err
	}
	if item.Entry.TargetWord != testedEntryTarget(item.Entry) {
		return canonicalManifestItem{}, fmt.Errorf("%w: manifest target is not canonical", ErrInvalidInput)
	}
	var key *canonicalCacheKey
	if item.CacheKey != nil {
		canonicalKey, err := canonicalizeManifestCacheKey(item, schemaVersion)
		if err != nil {
			return canonicalManifestItem{}, err
		}
		key = &canonicalKey
	}
	entryCanonical, err := canonicalizeManifestEntry(item.Entry, schemaVersion)
	if err != nil {
		return canonicalManifestItem{}, err
	}
	if schemaVersion >= ManifestSchemaVersionV15 {
		idiom, err := canonicalizeIdiomEvidence(item)
		if err != nil {
			return canonicalManifestItem{}, err
		}
		entryCanonical.Idiom = idiom
	}
	canonical := canonicalManifestItem{
		Ordinal: item.Ordinal, Disposition: item.Disposition,
		Entry:    entryCanonical,
		Quality:  canonicalizeManifestQuality(item.Quality),
		CacheKey: key,
	}
	if schemaVersion >= ManifestSchemaVersionV14 {
		canonical.ItemKind = item.ItemKind.Normalized()
	}
	return canonical, nil
}

// validateManifestItemKind rejects a value this contract does not define.
// Historical schema versions do not encode the field and ignore it in their
// digest, but an invalid value is still an invalid frozen decision.
func validateManifestItemKind(item ManifestItem) error {
	if !item.ItemKind.Valid() {
		return fmt.Errorf("%w: invalid manifest item kind", ErrInvalidInput)
	}
	return nil
}

// canonicalizeIdiomEvidence validates and encodes the frozen evidence of an
// idiom item. A lemma item carries none; an idiom item must carry its headword,
// every matched token position, its anchor identity and its matching
// provenance, so a re-render never falls back to a lemma interpretation
// (ADR 0114).
func canonicalizeIdiomEvidence(item ManifestItem) (*canonicalIdiomEvidence, error) {
	evidence := item.Entry
	kind := item.ItemKind.Normalized()
	if !kind.IsIdiom() {
		if len(evidence.MatchedTokenOrdinals) > 0 || evidence.Source != "" || evidence.AnchorCanonicalLemma != "" {
			return nil, fmt.Errorf("%w: lemma item carries idiom evidence", ErrInvalidInput)
		}
		return nil, nil //nolint:nilnil // absent optional evidence is the canonical lemma encoding
	}
	if strings.TrimSpace(evidence.Headword) == "" || strings.TrimSpace(evidence.Source) == "" || strings.TrimSpace(evidence.MatcherVersion) == "" || strings.TrimSpace(evidence.AnchorLanguage) == "" || strings.TrimSpace(evidence.AnchorCanonicalLemma) == "" || strings.TrimSpace(evidence.AnchorUPOS) == "" {
		return nil, fmt.Errorf("%w: idiom item evidence is incomplete", ErrInvalidInput)
	}
	if len(evidence.MatchedTokenOrdinals) == 0 || evidence.GapBound < 0 {
		return nil, fmt.Errorf("%w: idiom item evidence is invalid", ErrInvalidInput)
	}
	ordinals := make([]int, len(evidence.MatchedTokenOrdinals))
	for i, ordinal := range evidence.MatchedTokenOrdinals {
		if ordinal < 0 || (i > 0 && ordinal <= evidence.MatchedTokenOrdinals[i-1]) {
			return nil, fmt.Errorf("%w: idiom matched ordinals are not in sentence order", ErrInvalidInput)
		}
		ordinals[i] = ordinal
	}
	return &canonicalIdiomEvidence{
		MatchedTokenOrdinals:       ordinals,
		AnchorLanguage:             evidence.AnchorLanguage,
		AnchorCanonicalLemma:       evidence.AnchorCanonicalLemma,
		AnchorUPOS:                 evidence.AnchorUPOS,
		AnchorLexicalDiscriminator: evidence.AnchorLexicalDiscriminator,
		Source:                     evidence.Source,
		SourceVersion:              evidence.SourceVersion,
		MatcherVersion:             evidence.MatcherVersion,
		GapBound:                   evidence.GapBound,
	}, nil
}

// validateManifestEntry rejects owner-scoped or external fields and requires the
// provider-independent identity fields to be present and canonical.
func validateManifestEntry(entry Entry) error {
	if strings.TrimSpace(entry.OwnerID) != "" || entry.Translation != "" || entry.SentenceTranslation != "" || entry.SentenceTranslationTarget != "" || strings.TrimSpace(entry.Language) == "" || strings.TrimSpace(entry.CanonicalLemma) == "" || strings.TrimSpace(entry.UPOS) == "" || entry.UPOS != strings.ToUpper(entry.UPOS) {
		return fmt.Errorf("%w: invalid or external manifest entry fields", ErrInvalidInput)
	}
	return nil
}

// validateManifestQuality requires the accepted flag to agree with the
// disposition, bounds the score, reason count and GDEX score, and rejects
// reasons outside the fixed vocabulary.
func validateManifestQuality(item ManifestItem) error {
	if item.Quality.Accepted != (item.Disposition == ManifestAccepted) {
		return fmt.Errorf("%w: disposition contradicts quality decision", ErrInvalidInput)
	}
	if item.Quality.Score < 0 || item.Quality.Score > 110 || len(item.Quality.Reasons) > 16 {
		return fmt.Errorf("%w: invalid manifest quality result", ErrInvalidInput)
	}
	if math.IsNaN(item.Quality.GDEXScore) || math.IsInf(item.Quality.GDEXScore, 0) || item.Quality.GDEXScore < 0 || item.Quality.GDEXScore > 1 {
		return fmt.Errorf("%w: invalid manifest GDEX score", ErrInvalidInput)
	}
	for _, reason := range item.Quality.Reasons {
		if _, ok := manifestQualityReasons[reason]; !ok {
			return fmt.Errorf("%w: unknown manifest quality reason", ErrInvalidInput)
		}
	}
	return nil
}

// canonicalizeManifestCacheKey validates the frozen cache identity against the
// manifest entry and returns its canonical form. The dictionary identity,
// sentence hash and meaning-evidence hash are part of every supported codec.
func canonicalizeManifestCacheKey(item ManifestItem, schemaVersion int) (canonicalCacheKey, error) {
	entry := item.Entry
	if item.Disposition != ManifestAccepted || item.CacheKey.Language != entry.Language || item.CacheKey.TargetLanguage == "" || item.CacheKey.CanonicalLemma != entry.CanonicalLemma || item.CacheKey.UPOS != entry.UPOS || strings.TrimSpace(item.CacheKey.Provider) == "" || strings.TrimSpace(item.CacheKey.ProviderVersion) == "" || item.CacheKey.DictionaryProviderVersion != entry.DictionaryProviderVersion || (item.CacheKey.SentenceHash != "" && item.CacheKey.SentenceHash != enrichment.SentenceHash(strings.TrimSpace(entry.Sentence))) || (item.CacheKey.MeaningEvidenceHash != "" && item.CacheKey.MeaningEvidenceHash != enrichment.MeaningEvidenceHash(entry.CandidateSenses)) {
		return canonicalCacheKey{}, fmt.Errorf("%w: cache identity does not match manifest entry", ErrInvalidInput)
	}
	key := canonicalCacheKey{Language: item.CacheKey.Language, TargetLanguage: item.CacheKey.TargetLanguage, CanonicalLemma: item.CacheKey.CanonicalLemma, UPOS: item.CacheKey.UPOS, Provider: item.CacheKey.Provider, ProviderVersion: item.CacheKey.ProviderVersion, SentenceHash: item.CacheKey.SentenceHash, MeaningEvidenceHash: item.CacheKey.MeaningEvidenceHash}
	if schemaVersion >= ManifestSchemaVersionV14 {
		keyKind := item.CacheKey.ItemKind.Normalized()
		if !keyKind.Valid() || keyKind != item.ItemKind.Normalized() {
			return canonicalCacheKey{}, fmt.Errorf("%w: cache identity does not match manifest entry", ErrInvalidInput)
		}
		key.ItemKind = string(keyKind)
	}
	return key, nil
}

// canonicalizeManifestQuality copies the quality result, including the GDEX
// score that every supported schema persists.
func canonicalizeManifestQuality(quality SentenceQuality) canonicalSentenceQuality {
	return canonicalSentenceQuality{Accepted: quality.Accepted, Score: quality.Score, GDEXScore: quality.GDEXScore, Reasons: append([]string(nil), quality.Reasons...)}
}

// canonicalizeManifestEntry copies the render inputs every supported schema
// version persists, enforcing the bounded candidate-sense and non-negative
// omitted evidence counts.
func canonicalizeManifestEntry(entry Entry, schemaVersion int) (canonicalEntry, error) {
	entryCanonical := canonicalEntry{
		Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS,
		LexicalDiscriminator: entry.LexicalDiscriminator, Headword: entry.Headword,
		Sentence: entry.Sentence, TargetWord: entry.TargetWord, Morphology: entry.Morphology,
		Gloss: entry.Gloss, Plural: entry.Plural, IPA: entry.IPA, PrincipalParts: entry.PrincipalParts,
		DictionaryProviderVersion: entry.DictionaryProviderVersion,
		PassageContext:            entry.PassageContext, PassageCitation: entry.PassageCitation, PassagePin: entry.PassagePin,
		DictionaryEntry: entry.DictionaryEntry, DictionaryEntryID: entry.DictionaryEntryID, DictionaryPin: entry.DictionaryPin,
		SourceDocument: entry.SourceDocument, Notes: entry.Notes, FirstEncounter: entry.FirstEncounter,
	}
	if len(entry.CandidateSenses) > enrichment.DefaultMaxCandidateSenses {
		return canonicalEntry{}, fmt.Errorf("%w: too many manifest candidate senses", ErrInvalidInput)
	}
	entryCanonical.CandidateSenses = enrichment.CloneLexicalSenses(entry.CandidateSenses)
	if entry.OmittedEvidenceCount < 0 {
		return canonicalEntry{}, fmt.Errorf("%w: invalid omitted evidence count", ErrInvalidInput)
	}
	entryCanonical.OmittedEvidenceCount = entry.OmittedEvidenceCount
	entryCanonical.LSJSummaryProviderVersion = entry.LSJSummaryProviderVersion
	if schemaVersion >= ManifestSchemaVersionV13 {
		attribution, err := canonicalizeAttribution(entry.Attribution)
		if err != nil {
			return canonicalEntry{}, err
		}
		entryCanonical.Attribution = attribution
	}
	return entryCanonical, nil
}

// canonicalizeAttribution validates and encodes the optional typed source
// attribution. An absent attribution is nil; a speaker requires a non-empty
// displayed name, and a stage direction carries no speaker (ADR 0103).
func canonicalizeAttribution(attribution domain.SourceAttribution) (*canonicalAttribution, error) {
	switch attribution.Kind {
	case domain.AttributionNone:
		if strings.TrimSpace(attribution.Speaker) != "" {
			return nil, fmt.Errorf("%w: attribution speaker has no kind", ErrInvalidInput)
		}
		// A nil pointer is the canonical encoding of "no attribution", not an
		// error; the caller stores it as an absent optional codec field.
		return nil, nil //nolint:nilnil // absent optional value

	case domain.AttributionSpeaker:
		speaker := strings.TrimSpace(attribution.Speaker)
		if speaker == "" {
			return nil, fmt.Errorf("%w: speaker attribution has no name", ErrInvalidInput)
		}
		return &canonicalAttribution{Kind: string(attribution.Kind), Speaker: speaker}, nil
	case domain.AttributionStageDirection:
		if strings.TrimSpace(attribution.Speaker) != "" {
			return nil, fmt.Errorf("%w: stage direction attribution carries a speaker", ErrInvalidInput)
		}
		return &canonicalAttribution{Kind: string(attribution.Kind)}, nil
	default:
		return nil, fmt.Errorf("%w: unknown attribution kind", ErrInvalidInput)
	}
}

func cloneManifestItems(items []ManifestItem) []ManifestItem {
	result := make([]ManifestItem, len(items))
	for i, item := range items {
		result[i] = item
		result[i].Quality.Reasons = append([]string(nil), item.Quality.Reasons...)
		result[i].Entry.CandidateSenses = enrichment.CloneLexicalSenses(item.Entry.CandidateSenses)
		result[i].Entry.SentenceTokens = cloneTokens(item.Entry.SentenceTokens)
		result[i].Entry.MatchedTokenOrdinals = append([]int(nil), item.Entry.MatchedTokenOrdinals...)
		if item.CacheKey != nil {
			key := *item.CacheKey
			result[i].CacheKey = &key
		}
	}
	return result
}

func cloneTokens(tokens []analyzer.Token) []analyzer.Token {
	if tokens == nil {
		return nil
	}
	cloned := make([]analyzer.Token, len(tokens))
	for i, token := range tokens {
		cloned[i] = token
		if token.Morphology != nil {
			morphology := make(map[string]string, len(token.Morphology))
			maps.Copy(morphology, token.Morphology)
			cloned[i].Morphology = morphology
		}
	}
	return cloned
}

func cloneEntry(entry Entry) Entry {
	entry.CandidateSenses = enrichment.CloneLexicalSenses(entry.CandidateSenses)
	entry.SentenceTokens = cloneTokens(entry.SentenceTokens)
	entry.MatchedTokenOrdinals = append([]int(nil), entry.MatchedTokenOrdinals...)
	return entry
}
