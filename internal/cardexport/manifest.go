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
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// ManifestSchemaVersion identifies the canonical durable manifest codec. A
// new version is required for any change that alters digest inputs.
const (
	LegacyManifestSchemaVersion   = 1
	PreviousManifestSchemaVersion = 2
	ManifestSchemaVersionV3       = 3
	ManifestSchemaVersionV4       = 4
	ManifestSchemaVersionV5       = 5
	ManifestSchemaVersionV6       = 6
	ManifestSchemaVersionV7       = 7
	ManifestSchemaVersionV8       = 8
	ManifestSchemaVersionV9       = 9
	ManifestSchemaVersionV10      = 10
	ManifestSchemaVersionV11      = 11
	ManifestSchemaVersionV12      = 12
	ManifestSchemaVersion         = ManifestSchemaVersionV12
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
	Entry           Entry
	Quality         SentenceQuality
	CacheKey        *enrichment.CacheKey
}

// ManifestSnapshot is the versioned, immutable representation persisted for a
// durable preparation run. Items retain selection order, including omissions.
type ManifestSnapshot struct {
	SchemaVersion int
	Owner         string
	DeckName      string
	Filename      string
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
	var prefix string
	switch s.SchemaVersion {
	case LegacyManifestSchemaVersion:
		prefix = "mouseion-prepared-deck-manifest-v1\x00"
	case PreviousManifestSchemaVersion:
		prefix = "mouseion-prepared-deck-manifest-v2\x00"
	case ManifestSchemaVersionV3:
		prefix = "mouseion-prepared-deck-manifest-v3\x00"
	case ManifestSchemaVersionV4:
		prefix = "mouseion-prepared-deck-manifest-v4\x00"
	case ManifestSchemaVersionV5:
		prefix = "mouseion-prepared-deck-manifest-v5\x00"
	case ManifestSchemaVersionV6:
		prefix = "mouseion-prepared-deck-manifest-v6\x00"
	case ManifestSchemaVersionV7:
		prefix = "mouseion-prepared-deck-manifest-v7\x00"
	case ManifestSchemaVersionV8:
		prefix = "mouseion-prepared-deck-manifest-v8\x00"
	case ManifestSchemaVersionV9:
		prefix = "mouseion-prepared-deck-manifest-v9\x00"
	case ManifestSchemaVersionV10:
		prefix = "mouseion-prepared-deck-manifest-v10\x00"
	case ManifestSchemaVersionV11:
		prefix = "mouseion-prepared-deck-manifest-v11\x00"
	default:
		prefix = "mouseion-prepared-deck-manifest-v12\x00"
	}
	sum := sha256.Sum256(append([]byte(prefix), payload...))
	return hex.EncodeToString(sum[:]), nil
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
	canonical, err := canonicalizeManifestItem(item, schemaVersion)
	if err != nil {
		return "", err
	}
	if schemaVersion == LegacyManifestSchemaVersion && canonical.CacheKey != nil {
		canonical.CacheKey.TargetLanguage = ""
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("cardexport: encode manifest item: %w", err)
	}
	var prefix string
	switch schemaVersion {
	case LegacyManifestSchemaVersion:
		prefix = "mouseion-prepared-deck-candidate-v1\x00"
	case PreviousManifestSchemaVersion:
		prefix = "mouseion-prepared-deck-candidate-v2\x00"
	case ManifestSchemaVersionV3:
		prefix = "mouseion-prepared-deck-candidate-v3\x00"
	case ManifestSchemaVersionV4:
		prefix = "mouseion-prepared-deck-candidate-v4\x00"
	case ManifestSchemaVersionV5:
		prefix = "mouseion-prepared-deck-candidate-v5\x00"
	case ManifestSchemaVersionV6:
		prefix = "mouseion-prepared-deck-candidate-v6\x00"
	case ManifestSchemaVersionV7:
		prefix = "mouseion-prepared-deck-candidate-v7\x00"
	case ManifestSchemaVersionV8:
		prefix = "mouseion-prepared-deck-candidate-v8\x00"
	case ManifestSchemaVersionV9:
		prefix = "mouseion-prepared-deck-candidate-v9\x00"
	case ManifestSchemaVersionV10:
		prefix = "mouseion-prepared-deck-candidate-v10\x00"
	case ManifestSchemaVersionV11:
		prefix = "mouseion-prepared-deck-candidate-v11\x00"
	default:
		prefix = "mouseion-prepared-deck-candidate-v12\x00"
	}
	sum := sha256.Sum256(append([]byte(prefix), payload...))
	return hex.EncodeToString(sum[:]), nil
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
	allAcceptedHaveCacheKeys := true
	for _, item := range snapshot.Items {
		switch item.Disposition {
		case ManifestAccepted:
			manifest.accepted = append(manifest.accepted, renderInputFromEntry(item.Entry))
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
	Items         []canonicalManifestItem `json:"items"`
}

type canonicalManifestItem struct {
	Ordinal     int                      `json:"ordinal"`
	Disposition ManifestDisposition      `json:"disposition"`
	Entry       canonicalEntry           `json:"entry"`
	Quality     canonicalSentenceQuality `json:"quality"`
	CacheKey    *canonicalCacheKey       `json:"cache_key"`
}

type canonicalEntry struct {
	Language                  string                    `json:"language"`
	CanonicalLemma            string                    `json:"canonical_lemma"`
	UPOS                      string                    `json:"upos"`
	LexicalDiscriminator      string                    `json:"lexical_discriminator,omitempty"`
	Headword                  string                    `json:"headword,omitempty"`
	Sentence                  string                    `json:"sentence"`
	TargetWord                string                    `json:"target_word"`
	Morphology                string                    `json:"morphology"`
	Gloss                     string                    `json:"gloss,omitempty"`
	Plural                    string                    `json:"plural,omitempty"`
	IPA                       string                    `json:"ipa,omitempty"`
	PrincipalParts            string                    `json:"principal_parts,omitempty"`
	DictionaryProviderVersion string                    `json:"dictionary_provider_version,omitempty"`
	PassageContext            string                    `json:"passage_context,omitempty"`
	PassageCitation           string                    `json:"passage_citation,omitempty"`
	BrownsonPin               string                    `json:"brownson_pin,omitempty"`
	DictionaryEntry           string                    `json:"dictionary_entry,omitempty"`
	DictionaryEntryID         string                    `json:"dictionary_entry_id,omitempty"`
	DictionaryPin             string                    `json:"dictionary_pin,omitempty"`
	LSJSummaryProviderVersion string                    `json:"lsj_summary_provider_version,omitempty"`
	OmittedEvidenceCount      int                       `json:"omitted_evidence_count,omitempty"`
	CandidateSenses           []enrichment.LexicalSense `json:"candidate_senses,omitempty"`
	SourceDocument            string                    `json:"source_document"`
	Notes                     string                    `json:"notes"`
	FirstEncounter            int64                     `json:"first_encounter"`
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
	Provider            string `json:"provider"`
	ProviderVersion     string `json:"provider_version"`
	SentenceHash        string `json:"sentence_hash"`
	MeaningEvidenceHash string `json:"meaning_evidence_hash,omitempty"`
}

func (s ManifestSnapshot) canonical() (canonicalSnapshot, error) {
	if err := s.validateHeader(); err != nil {
		return canonicalSnapshot{}, err
	}
	result := canonicalSnapshot{SchemaVersion: s.SchemaVersion, Owner: s.Owner, DeckName: s.DeckName, Filename: s.Filename, Items: make([]canonicalManifestItem, len(s.Items))}
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

// validateHeader rejects a snapshot whose schema version is unknown or whose
// owner and deck identity are missing or inconsistent with the download name.
func (s ManifestSnapshot) validateHeader() error {
	if (s.SchemaVersion != LegacyManifestSchemaVersion && s.SchemaVersion != PreviousManifestSchemaVersion && s.SchemaVersion != ManifestSchemaVersionV3 && s.SchemaVersion != ManifestSchemaVersionV4 && s.SchemaVersion != ManifestSchemaVersionV5 && s.SchemaVersion != ManifestSchemaVersionV6 && s.SchemaVersion != ManifestSchemaVersionV7 && s.SchemaVersion != ManifestSchemaVersionV8 && s.SchemaVersion != ManifestSchemaVersionV9 && s.SchemaVersion != ManifestSchemaVersionV10 && s.SchemaVersion != ManifestSchemaVersionV11 && s.SchemaVersion != ManifestSchemaVersionV12) || strings.TrimSpace(s.Owner) == "" || strings.TrimSpace(s.DeckName) == "" || s.Filename != DownloadFilename(s.DeckName) {
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
	if schemaVersion == LegacyManifestSchemaVersion && canonical.CacheKey != nil {
		canonical.CacheKey.TargetLanguage = ""
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
	if err := validateManifestQuality(item, schemaVersion); err != nil {
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
	return canonicalManifestItem{
		Ordinal: item.Ordinal, Disposition: item.Disposition,
		Entry:    entryCanonical,
		Quality:  canonicalizeManifestQuality(item.Quality, schemaVersion),
		CacheKey: key,
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
// disposition, bounds the score and reason count, applies the schema-versioned
// GDEX bound, and rejects reasons outside the fixed vocabulary.
func validateManifestQuality(item ManifestItem, schemaVersion int) error {
	if item.Quality.Accepted != (item.Disposition == ManifestAccepted) {
		return fmt.Errorf("%w: disposition contradicts quality decision", ErrInvalidInput)
	}
	if item.Quality.Score < 0 || item.Quality.Score > 110 || len(item.Quality.Reasons) > 16 {
		return fmt.Errorf("%w: invalid manifest quality result", ErrInvalidInput)
	}
	if schemaVersion >= ManifestSchemaVersionV4 && (math.IsNaN(item.Quality.GDEXScore) || math.IsInf(item.Quality.GDEXScore, 0) || item.Quality.GDEXScore < 0 || item.Quality.GDEXScore > 1) {
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
// manifest entry and returns its canonical form. The target language is ignored
// by the legacy v1 codec because that codec never persisted it. The dictionary
// identity, sentence hash, and meaning-evidence hash are enforced only once
// their schema version introduced them.
func canonicalizeManifestCacheKey(item ManifestItem, schemaVersion int) (canonicalCacheKey, error) {
	entry := item.Entry
	// Legacy v1 manifests persisted cache keys with an empty target language
	// (it was never part of the v1 digest). The normalized storage projection
	// may carry the database default, which is also ignored by the v1 codec.
	targetOK := item.CacheKey.TargetLanguage != ""
	if schemaVersion == LegacyManifestSchemaVersion {
		targetOK = true
	}
	if item.Disposition != ManifestAccepted || item.CacheKey.Language != entry.Language || !targetOK || item.CacheKey.CanonicalLemma != entry.CanonicalLemma || item.CacheKey.UPOS != entry.UPOS || strings.TrimSpace(item.CacheKey.Provider) == "" || strings.TrimSpace(item.CacheKey.ProviderVersion) == "" || (schemaVersion >= ManifestSchemaVersionV4 && item.CacheKey.DictionaryProviderVersion != entry.DictionaryProviderVersion) || (item.CacheKey.SentenceHash != "" && item.CacheKey.SentenceHash != enrichment.SentenceHash(strings.TrimSpace(entry.Sentence))) || (schemaVersion >= ManifestSchemaVersionV8 && item.CacheKey.MeaningEvidenceHash != "" && item.CacheKey.MeaningEvidenceHash != enrichment.MeaningEvidenceHash(entry.CandidateSenses)) {
		return canonicalCacheKey{}, fmt.Errorf("%w: cache identity does not match manifest entry", ErrInvalidInput)
	}
	key := canonicalCacheKey{Language: item.CacheKey.Language, TargetLanguage: item.CacheKey.TargetLanguage, CanonicalLemma: item.CacheKey.CanonicalLemma, UPOS: item.CacheKey.UPOS, Provider: item.CacheKey.Provider, ProviderVersion: item.CacheKey.ProviderVersion, SentenceHash: item.CacheKey.SentenceHash}
	if schemaVersion >= ManifestSchemaVersionV8 {
		key.MeaningEvidenceHash = item.CacheKey.MeaningEvidenceHash
	}
	return key, nil
}

// canonicalizeManifestQuality copies the quality result, adding the GDEX score
// only for schemas that persist it.
func canonicalizeManifestQuality(quality SentenceQuality, schemaVersion int) canonicalSentenceQuality {
	canonical := canonicalSentenceQuality{Accepted: quality.Accepted, Score: quality.Score, Reasons: append([]string(nil), quality.Reasons...)}
	if schemaVersion >= ManifestSchemaVersionV4 {
		canonical.GDEXScore = quality.GDEXScore
	}
	return canonical
}

// canonicalizeManifestEntry copies the render inputs that each schema version
// persists, enforcing the bounded candidate-sense and non-negative omitted
// evidence counts introduced by v6 and v7.
func canonicalizeManifestEntry(entry Entry, schemaVersion int) (canonicalEntry, error) {
	entryCanonical := canonicalEntry{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS, Sentence: entry.Sentence, TargetWord: entry.TargetWord, Morphology: entry.Morphology, SourceDocument: entry.SourceDocument, Notes: entry.Notes, FirstEncounter: entry.FirstEncounter}
	if schemaVersion >= ManifestSchemaVersionV9 {
		entryCanonical.LexicalDiscriminator = entry.LexicalDiscriminator
		entryCanonical.Headword = entry.Headword
	}
	if schemaVersion >= ManifestSchemaVersionV4 {
		entryCanonical.Gloss = entry.Gloss
		entryCanonical.DictionaryProviderVersion = entry.DictionaryProviderVersion
	}
	if schemaVersion >= ManifestSchemaVersionV5 {
		entryCanonical.Plural = entry.Plural
		entryCanonical.IPA = entry.IPA
		entryCanonical.PrincipalParts = entry.PrincipalParts
	}
	if schemaVersion >= ManifestSchemaVersionV6 {
		if len(entry.CandidateSenses) > enrichment.DefaultMaxCandidateSenses {
			return canonicalEntry{}, fmt.Errorf("%w: too many manifest candidate senses", ErrInvalidInput)
		}
		entryCanonical.CandidateSenses = enrichment.CloneLexicalSenses(entry.CandidateSenses)
	}
	if schemaVersion >= ManifestSchemaVersionV11 {
		entryCanonical.DictionaryEntry = entry.DictionaryEntry
		entryCanonical.DictionaryEntryID = entry.DictionaryEntryID
		entryCanonical.DictionaryPin = entry.DictionaryPin
	}
	if schemaVersion >= ManifestSchemaVersionV12 {
		entryCanonical.LSJSummaryProviderVersion = entry.LSJSummaryProviderVersion
	}
	if schemaVersion >= ManifestSchemaVersionV7 {
		if entry.OmittedEvidenceCount < 0 {
			return canonicalEntry{}, fmt.Errorf("%w: invalid omitted evidence count", ErrInvalidInput)
		}
		entryCanonical.OmittedEvidenceCount = entry.OmittedEvidenceCount
	}
	if schemaVersion >= ManifestSchemaVersionV10 {
		entryCanonical.PassageContext = entry.PassageContext
		entryCanonical.PassageCitation = entry.PassageCitation
		entryCanonical.BrownsonPin = entry.BrownsonPin
	}
	return entryCanonical, nil
}

func cloneManifestItems(items []ManifestItem) []ManifestItem {
	result := make([]ManifestItem, len(items))
	for i, item := range items {
		result[i] = item
		result[i].Quality.Reasons = append([]string(nil), item.Quality.Reasons...)
		result[i].Entry.CandidateSenses = enrichment.CloneLexicalSenses(item.Entry.CandidateSenses)
		result[i].Entry.SentenceTokens = cloneTokens(item.Entry.SentenceTokens)
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
	return entry
}
