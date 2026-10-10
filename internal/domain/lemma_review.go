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

// LemmaReviewIdentityState is everything a lemma review impact preview reads
// about one identity a proposal affects: its projected effective count in the
// reviewed Book and across the learner's other currently analyzed Books, and
// whether it is Known or Reserved.
type LemmaReviewIdentityState struct {
	Identity           LemmaReviewIdentity
	InBook, OtherBooks int64
	Known, Reserved    bool
}

// LemmaReviewStateFingerprint binds a proposal to the exact occurrences and to
// the state of only the identities it affects, so confirming is rejected as
// stale whenever anything the preview's impact depended on has changed.
func LemmaReviewStateFingerprint(occurrences []LemmaReviewOccurrence, states []LemmaReviewIdentityState) string {
	h := sha256.New()
	for _, occurrence := range occurrences {
		_, _ = h.Write([]byte(strings.Join([]string{
			occurrence.AnalysisRunID, occurrence.SourceDocumentID,
			strconv.FormatInt(occurrence.StartOffset, 10), strconv.FormatInt(occurrence.EndOffset, 10),
			occurrence.Surface, occurrence.RawLemma, occurrence.CanonicalLemma, occurrence.UPOS,
			occurrence.CorrectedLemma, strconv.FormatBool(occurrence.Excluded),
		}, "\x00") + "\n"))
	}
	lines := make([]string, 0, len(states))
	for _, state := range states {
		lines = append(lines, strings.Join([]string{
			state.Identity.Language, state.Identity.CanonicalLemma, state.Identity.UPOS,
			strconv.FormatInt(state.InBook, 10), strconv.FormatInt(state.OtherBooks, 10),
			strconv.FormatBool(state.Known), strconv.FormatBool(state.Reserved),
		}, "\x00"))
	}
	sort.Strings(lines)
	for _, line := range lines {
		_, _ = h.Write([]byte(line + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
