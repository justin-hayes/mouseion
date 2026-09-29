package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

type LemmaReviewIdentity struct {
	Language, CanonicalLemma, UPOS string
}

// LemmaReviewStateFingerprint binds a proposal to the exact occurrences and
// learner-facing identity, Known, and Reserved state used to preview it.
func LemmaReviewStateFingerprint(occurrences []LemmaReviewOccurrence, vocabulary AnalysisCorpusVocabulary, known []KnownVocabulary, reserved map[LemmaReviewIdentity]bool, extras []LemmaReviewIdentity) string {
	h := sha256.New()
	for _, occurrence := range occurrences {
		_, _ = h.Write([]byte(strings.Join([]string{
			occurrence.AnalysisRunID, occurrence.SourceDocumentID,
			strconv.FormatInt(occurrence.StartOffset, 10), strconv.FormatInt(occurrence.EndOffset, 10),
			occurrence.Surface, occurrence.RawLemma, occurrence.CanonicalLemma, occurrence.UPOS,
			occurrence.CorrectedLemma, strconv.FormatBool(occurrence.Excluded),
		}, "\x00") + "\n"))
	}
	identities := make(map[LemmaReviewIdentity]int64, len(vocabulary.Lemmas)+len(extras))
	for _, lemma := range vocabulary.Lemmas {
		identity := LemmaReviewIdentity{Language: lemma.Language, CanonicalLemma: lemma.CanonicalLemma, UPOS: lemma.UPOS}
		identities[identity] = lemma.OccurrenceCount
	}
	for _, identity := range extras {
		if _, exists := identities[identity]; !exists {
			identities[identity] = 0
		}
	}
	identityKeys := make([]string, 0, len(identities))
	for identity := range identities {
		identityKeys = append(identityKeys, strings.Join([]string{identity.Language, identity.CanonicalLemma, identity.UPOS}, "\x00"))
	}
	sort.Strings(identityKeys)
	for _, key := range identityKeys {
		parts := strings.SplitN(key, "\x00", 3)
		identity := LemmaReviewIdentity{Language: parts[0], CanonicalLemma: parts[1], UPOS: parts[2]}
		_, _ = h.Write([]byte(key + "\x00" + strconv.FormatInt(identities[identity], 10) + "\x00" + strconv.FormatBool(reserved[identity]) + "\n"))
	}
	knownKeys := make([]string, 0, len(known))
	for _, item := range known {
		knownKeys = append(knownKeys, item.Language+"\x00"+item.CanonicalLemma+"\x00"+item.UPOS)
	}
	sort.Strings(knownKeys)
	for _, key := range knownKeys {
		_, _ = h.Write([]byte(key + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
