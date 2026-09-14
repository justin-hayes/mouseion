package cardexport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

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
	ManifestSchemaVersion         = ManifestSchemaVersionV5
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
	Ordinal     int
	Disposition ManifestDisposition
	Entry       Entry
	Quality     SentenceQuality
	CacheKey    *enrichment.CacheKey
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

func (m Manifest) Snapshot() ManifestSnapshot {
	return ManifestSnapshot{
		SchemaVersion: ManifestSchemaVersion,
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
	prefix := "mouseion-prepared-deck-manifest-v2\x00"
	if s.SchemaVersion == LegacyManifestSchemaVersion {
		prefix = "mouseion-prepared-deck-manifest-v1\x00"
	} else if s.SchemaVersion == PreviousManifestSchemaVersion {
		prefix = "mouseion-prepared-deck-manifest-v2\x00"
	} else if s.SchemaVersion == ManifestSchemaVersionV3 {
		prefix = "mouseion-prepared-deck-manifest-v3\x00"
	} else if s.SchemaVersion == ManifestSchemaVersionV4 {
		prefix = "mouseion-prepared-deck-manifest-v4\x00"
	} else {
		prefix = "mouseion-prepared-deck-manifest-v5\x00"
	}
	sum := sha256.Sum256(append([]byte(prefix), payload...))
	return hex.EncodeToString(sum[:]), nil
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
	prefix := "mouseion-prepared-deck-candidate-v2\x00"
	if schemaVersion == LegacyManifestSchemaVersion {
		prefix = "mouseion-prepared-deck-candidate-v1\x00"
	} else if schemaVersion == PreviousManifestSchemaVersion {
		prefix = "mouseion-prepared-deck-candidate-v2\x00"
	} else if schemaVersion == ManifestSchemaVersionV3 {
		prefix = "mouseion-prepared-deck-candidate-v3\x00"
	} else if schemaVersion == ManifestSchemaVersionV4 {
		prefix = "mouseion-prepared-deck-candidate-v4\x00"
	} else {
		prefix = "mouseion-prepared-deck-candidate-v5\x00"
	}
	sum := sha256.Sum256(append([]byte(prefix), payload...))
	return hex.EncodeToString(sum[:]), nil
}

// ManifestFromSnapshot validates a persisted snapshot and rebuilds the exact
// in-memory render plan without re-running selection or the quality gate.
func ManifestFromSnapshot(snapshot ManifestSnapshot) (Manifest, error) {
	if _, err := snapshot.canonical(); err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{
		owner: snapshot.Owner, deckName: snapshot.DeckName,
		accepted: make([]Entry, 0, len(snapshot.Items)), omitted: make([]Omission, 0, len(snapshot.Items)),
		enrichmentCandidates: make([]enrichment.Candidate, 0, len(snapshot.Items)),
		decisions:            cloneManifestItems(snapshot.Items),
	}
	allAcceptedHaveCacheKeys := true
	for _, item := range snapshot.Items {
		switch item.Disposition {
		case ManifestAccepted:
			manifest.accepted = append(manifest.accepted, item.Entry)
			manifest.enrichmentCandidates = append(manifest.enrichmentCandidates, enrichment.Candidate{
				Identity:        enrichment.Identity{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS},
				TargetWord:      item.Entry.TargetWord,
				ExampleSentence: strings.TrimSpace(item.Entry.Sentence),
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
	Language                  string `json:"language"`
	CanonicalLemma            string `json:"canonical_lemma"`
	UPOS                      string `json:"upos"`
	Sentence                  string `json:"sentence"`
	TargetWord                string `json:"target_word"`
	Morphology                string `json:"morphology"`
	Gloss                     string `json:"gloss,omitempty"`
	Plural                    string `json:"plural,omitempty"`
	DictionaryProviderVersion string `json:"dictionary_provider_version,omitempty"`
	SourceDocument            string `json:"source_document"`
	Notes                     string `json:"notes"`
	FirstEncounter            int64  `json:"first_encounter"`
}

type canonicalSentenceQuality struct {
	Accepted  bool     `json:"accepted"`
	Score     int      `json:"score"`
	GDEXScore float64  `json:"gdex_score,omitempty"`
	Reasons   []string `json:"reasons"`
}

type canonicalCacheKey struct {
	Language        string `json:"language"`
	TargetLanguage  string `json:"target_language,omitempty"`
	CanonicalLemma  string `json:"canonical_lemma"`
	UPOS            string `json:"upos"`
	Provider        string `json:"provider"`
	ProviderVersion string `json:"provider_version"`
	SentenceHash    string `json:"sentence_hash"`
}

func (s ManifestSnapshot) canonical() (canonicalSnapshot, error) {
	if (s.SchemaVersion != LegacyManifestSchemaVersion && s.SchemaVersion != PreviousManifestSchemaVersion && s.SchemaVersion != ManifestSchemaVersionV3 && s.SchemaVersion != ManifestSchemaVersionV4 && s.SchemaVersion != ManifestSchemaVersion) || strings.TrimSpace(s.Owner) == "" || strings.TrimSpace(s.DeckName) == "" || s.Filename != DownloadFilename(s.DeckName) {
		return canonicalSnapshot{}, fmt.Errorf("%w: invalid manifest header", ErrInvalidInput)
	}
	result := canonicalSnapshot{SchemaVersion: s.SchemaVersion, Owner: s.Owner, DeckName: s.DeckName, Filename: s.Filename, Items: make([]canonicalManifestItem, len(s.Items))}
	seen := make(map[string]struct{}, len(s.Items))
	acceptedWithKey, acceptedWithoutKey := 0, 0
	for i, item := range s.Items {
		if item.Ordinal != i {
			return canonicalSnapshot{}, fmt.Errorf("%w: manifest ordinal %d is not contiguous", ErrInvalidInput, item.Ordinal)
		}
		canonical, err := canonicalizeManifestItem(item, s.SchemaVersion)
		if err != nil {
			return canonicalSnapshot{}, fmt.Errorf("manifest item %d: %w", i, err)
		}
		if s.SchemaVersion == LegacyManifestSchemaVersion && canonical.CacheKey != nil {
			canonical.CacheKey.TargetLanguage = ""
		}
		digest, err := CandidateDigestVersion(item, s.SchemaVersion)
		if err != nil {
			return canonicalSnapshot{}, err
		}
		if _, exists := seen[digest]; exists {
			return canonicalSnapshot{}, fmt.Errorf("%w: duplicate manifest candidate", ErrInvalidInput)
		}
		seen[digest] = struct{}{}
		if item.Disposition == ManifestAccepted {
			if item.CacheKey == nil {
				acceptedWithoutKey++
			} else {
				acceptedWithKey++
			}
		}
		result.Items[i] = canonical
	}
	if acceptedWithKey > 0 && acceptedWithoutKey > 0 {
		return canonicalSnapshot{}, fmt.Errorf("%w: accepted manifest cache identity is partial", ErrInvalidInput)
	}
	return result, nil
}

func canonicalizeManifestItem(item ManifestItem, schemaVersion int) (canonicalManifestItem, error) {
	entry := item.Entry
	if strings.TrimSpace(entry.OwnerID) != "" || entry.Translation != "" || entry.SentenceTranslation != "" || entry.SentenceTranslationTarget != "" || strings.TrimSpace(entry.Language) == "" || strings.TrimSpace(entry.CanonicalLemma) == "" || strings.TrimSpace(entry.UPOS) == "" || entry.UPOS != strings.ToUpper(entry.UPOS) {
		return canonicalManifestItem{}, fmt.Errorf("%w: invalid or external manifest entry fields", ErrInvalidInput)
	}
	if item.Disposition != ManifestAccepted && item.Disposition != ManifestQualityOmitted {
		return canonicalManifestItem{}, fmt.Errorf("%w: invalid manifest disposition", ErrInvalidInput)
	}
	if item.Quality.Accepted != (item.Disposition == ManifestAccepted) {
		return canonicalManifestItem{}, fmt.Errorf("%w: disposition contradicts quality decision", ErrInvalidInput)
	}
	if item.Quality.Score < 0 || item.Quality.Score > 110 || len(item.Quality.Reasons) > 16 {
		return canonicalManifestItem{}, fmt.Errorf("%w: invalid manifest quality result", ErrInvalidInput)
	}
	if schemaVersion >= ManifestSchemaVersionV4 && (math.IsNaN(item.Quality.GDEXScore) || math.IsInf(item.Quality.GDEXScore, 0) || item.Quality.GDEXScore < 0 || item.Quality.GDEXScore > 1) {
		return canonicalManifestItem{}, fmt.Errorf("%w: invalid manifest GDEX score", ErrInvalidInput)
	}
	for _, reason := range item.Quality.Reasons {
		if _, ok := manifestQualityReasons[reason]; !ok {
			return canonicalManifestItem{}, fmt.Errorf("%w: unknown manifest quality reason", ErrInvalidInput)
		}
	}
	if entry.TargetWord != testedTarget(entry) {
		return canonicalManifestItem{}, fmt.Errorf("%w: manifest target is not canonical", ErrInvalidInput)
	}
	var key *canonicalCacheKey
	if item.CacheKey != nil {
		// Legacy v1 manifests persisted cache keys with an empty target
		// language (it was never part of the v1 digest). v2 requires an
		// explicit target language.
		targetOK := item.CacheKey.TargetLanguage != ""
		if schemaVersion == LegacyManifestSchemaVersion {
			targetOK = item.CacheKey.TargetLanguage == ""
		}
		if item.Disposition != ManifestAccepted || item.CacheKey.Language != entry.Language || !targetOK || item.CacheKey.CanonicalLemma != entry.CanonicalLemma || item.CacheKey.UPOS != entry.UPOS || strings.TrimSpace(item.CacheKey.Provider) == "" || strings.TrimSpace(item.CacheKey.ProviderVersion) == "" || (item.CacheKey.SentenceHash != "" && item.CacheKey.SentenceHash != enrichment.SentenceHash(strings.TrimSpace(entry.Sentence))) {
			return canonicalManifestItem{}, fmt.Errorf("%w: cache identity does not match manifest entry", ErrInvalidInput)
		}
		key = &canonicalCacheKey{Language: item.CacheKey.Language, TargetLanguage: item.CacheKey.TargetLanguage, CanonicalLemma: item.CacheKey.CanonicalLemma, UPOS: item.CacheKey.UPOS, Provider: item.CacheKey.Provider, ProviderVersion: item.CacheKey.ProviderVersion, SentenceHash: item.CacheKey.SentenceHash}
	}
	quality := canonicalSentenceQuality{Accepted: item.Quality.Accepted, Score: item.Quality.Score, Reasons: append([]string(nil), item.Quality.Reasons...)}
	if schemaVersion >= ManifestSchemaVersionV4 {
		quality.GDEXScore = item.Quality.GDEXScore
	}
	entryCanonical := canonicalEntry{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS, Sentence: entry.Sentence, TargetWord: entry.TargetWord, Morphology: entry.Morphology, SourceDocument: entry.SourceDocument, Notes: entry.Notes, FirstEncounter: entry.FirstEncounter}
	if schemaVersion >= ManifestSchemaVersionV4 {
		entryCanonical.Gloss = entry.Gloss
		entryCanonical.DictionaryProviderVersion = entry.DictionaryProviderVersion
	}
	if schemaVersion >= ManifestSchemaVersionV5 {
		entryCanonical.Plural = entry.Plural
	}
	return canonicalManifestItem{
		Ordinal: item.Ordinal, Disposition: item.Disposition,
		Entry:    entryCanonical,
		Quality:  quality,
		CacheKey: key,
	}, nil
}

func cloneManifestItems(items []ManifestItem) []ManifestItem {
	result := make([]ManifestItem, len(items))
	for i, item := range items {
		result[i] = item
		result[i].Quality.Reasons = append([]string(nil), item.Quality.Reasons...)
		if item.CacheKey != nil {
			key := *item.CacheKey
			result[i].CacheKey = &key
		}
	}
	return result
}
