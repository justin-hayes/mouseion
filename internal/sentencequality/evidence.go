package sentencequality

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/textmatch"
)

// SentenceEvidence is one candidate sentence resolved for scoring and
// rendering. It carries the persisted sentence's text, its matching target, the
// source location and the dependency tokens so a caller can gate, rank or anchor
// the target without reloading the corpus.
type SentenceEvidence struct {
	Sentence, Target    string
	CorpusID            string
	FirstEncounter      int64
	SentenceOrdinal     int64
	Quality             SentenceQuality
	Tokens              []analyzer.Token
	SentenceStartOffset uint64
}

type sentenceReference struct {
	SentenceIndex int             `json:"sentence_index"`
	Text          string          `json:"text"`
	Location      json.RawMessage `json:"location"`
}

// BestSentenceEvidence ranks every text-only source reference without changing
// the candidate's first-encounter ordering. Persisted exports use
// BestSentenceEvidenceFromCorpus below.
func BestSentenceEvidence(candidate domain.SelectionCandidate) (SentenceEvidence, bool) {
	return bestSentenceEvidence(candidate, nil)
}

// BestSentenceEvidenceFromCorpus ranks source references using the persisted
// sentence text and dependency tokens for each sentence ordinal.
func BestSentenceEvidenceFromCorpus(candidate domain.SelectionCandidate, sentences map[int64]analyzer.Sentence) (SentenceEvidence, bool) {
	return bestSentenceEvidence(candidate, sentences)
}

// AnyReferencePasses reports whether at least one of the candidate's sentence
// references resolves and passes the gate for the candidate's language. It
// stops at the first passing reference. References the persisted corpus cannot
// resolve are ignored, exactly as BestSentenceEvidenceFromCorpus drops them.
func AnyReferencePasses(candidate domain.SelectionCandidate, sentences map[int64]analyzer.Sentence) bool {
	refs, forms, ok := decodeCandidate(candidate)
	if !ok {
		return false
	}
	for _, ref := range refs {
		entry, ok := sentenceEvidenceForReference(candidate.Language, ref, forms, sentences)
		if ok && entry.evidence.Quality.Accepted {
			return true
		}
	}
	return false
}

// rankedSentenceEvidence is one source reference paired with the evidence that
// ranks it.
type rankedSentenceEvidence struct {
	evidence SentenceEvidence
	sentence int
}

func bestSentenceEvidence(candidate domain.SelectionCandidate, persisted map[int64]analyzer.Sentence) (SentenceEvidence, bool) {
	refs, forms, ok := decodeCandidate(candidate)
	if !ok {
		return SentenceEvidence{}, false
	}
	ranked := rankSentenceEvidence(candidate.Language, refs, forms, persisted)
	if len(ranked) == 0 {
		return SentenceEvidence{}, false
	}
	sort.SliceStable(ranked, func(i, j int) bool { return betterSentenceEvidence(ranked[i], ranked[j]) })
	evidence := ranked[0].evidence
	evidence.CorpusID = candidate.CorpusID
	evidence.SentenceOrdinal = int64(ranked[0].sentence)
	return evidence, true
}

// decodeCandidate decodes the candidate's sentence references and observed
// forms, appending its canonical lemma as a final target form. A malformed
// candidate reports false and is unrankable.
func decodeCandidate(candidate domain.SelectionCandidate) ([]sentenceReference, []string, bool) {
	var refs []sentenceReference
	if json.Unmarshal(candidate.SentenceReferences, &refs) != nil {
		return nil, nil, false
	}
	var forms []string
	if err := json.Unmarshal(candidate.ObservedForms, &forms); err != nil {
		return nil, nil, false
	}
	forms = append(forms, candidate.CanonicalLemma)
	return refs, forms, true
}

// rankSentenceEvidence scores every source reference the persisted corpus can
// resolve. Unresolvable references are dropped so they cannot displace the rest.
func rankSentenceEvidence(language string, refs []sentenceReference, forms []string, persisted map[int64]analyzer.Sentence) []rankedSentenceEvidence {
	ranked := make([]rankedSentenceEvidence, 0, len(refs))
	for _, ref := range refs {
		entry, ok := sentenceEvidenceForReference(language, ref, forms, persisted)
		if ok {
			ranked = append(ranked, entry)
		}
	}
	return ranked
}

// sentenceEvidenceForReference resolves one source reference to its text,
// target, quality and tokens. It reports false when a persisted run cannot
// resolve the reference's sentence.
func sentenceEvidenceForReference(language string, ref sentenceReference, forms []string, persisted map[int64]analyzer.Sentence) (rankedSentenceEvidence, bool) {
	text, tokens, sentenceStartOffset, ok := persistedSentence(persisted, ref)
	if !ok {
		return rankedSentenceEvidence{}, false
	}
	location, valid := referenceStartOffset(ref.Location)
	if !valid {
		location = -1
	}
	target := MatchingSentenceTarget(text, forms, tokens, sentenceStartOffset)
	quality := ScoreSentenceQuality(language, text, target, location, tokens, sentenceStartOffset)
	var evidenceTokens []analyzer.Token
	if persisted != nil {
		evidenceTokens = append([]analyzer.Token{}, tokens...)
	}
	return rankedSentenceEvidence{
		evidence: SentenceEvidence{Sentence: text, Target: target, FirstEncounter: location, Quality: quality, Tokens: evidenceTokens, SentenceStartOffset: sentenceStartOffset},
		sentence: ref.SentenceIndex,
	}, true
}

// persistedSentence resolves a reference against the persisted corpus. Without
// a persisted run it returns the reference's own text, no tokens and no start
// offset; with one, an unresolvable reference reports false.
func persistedSentence(persisted map[int64]analyzer.Sentence, ref sentenceReference) (string, []analyzer.Token, uint64, bool) {
	if persisted == nil {
		return ref.Text, nil, 0, true
	}
	sentence, ok := persisted[int64(ref.SentenceIndex)]
	if !ok {
		return "", nil, 0, false
	}
	if sentence.Tokens == nil {
		return sentence.Text, []analyzer.Token{}, sentence.Location.StartOffset, true
	}
	return sentence.Text, sentence.Tokens, sentence.Location.StartOffset, true
}

// MatchingSentenceTarget returns the first observed form that appears as a
// whole word or a persisted token in text, or the empty string when none does.
// Callers that also need the matched span use TargetSpan.
func MatchingSentenceTarget(text string, forms []string, tokens []analyzer.Token, sentenceStartOffset uint64) string {
	for _, form := range forms {
		cleanForm := textmatch.CleanLexicalSurface(form)
		if targetIndex(text, cleanForm, tokens, sentenceStartOffset) >= 0 {
			return cleanForm
		}
	}
	return ""
}

// betterSentenceEvidence orders accepted candidates first, then by descending
// GDEX score, ascending first encounter, sentence ordinal and sentence text.
func betterSentenceEvidence(a, b rankedSentenceEvidence) bool {
	switch {
	case a.evidence.Quality.Accepted != b.evidence.Quality.Accepted:
		return a.evidence.Quality.Accepted
	case a.evidence.Quality.GDEXScore != b.evidence.Quality.GDEXScore:
		return a.evidence.Quality.GDEXScore > b.evidence.Quality.GDEXScore
	case a.evidence.FirstEncounter != b.evidence.FirstEncounter:
		return a.evidence.FirstEncounter < b.evidence.FirstEncounter
	case a.sentence != b.sentence:
		return a.sentence < b.sentence
	default:
		return a.evidence.Sentence < b.evidence.Sentence
	}
}

func referenceStartOffset(raw json.RawMessage) (int64, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return 0, false
	}
	for _, key := range []string{"start_offset", "StartOffset"} {
		if value, ok := fields[key]; ok {
			var offset uint64
			if json.Unmarshal(value, &offset) == nil && offset <= math.MaxInt64 {
				return int64(offset), true
			}
		}
	}
	return 0, false
}

func targetIndex(sentence, target string, tokens []analyzer.Token, sentenceStartOffset uint64) int {
	start, _, ok := targetSpan(sentence, textmatch.CleanLexicalSurface(target), tokens, sentenceStartOffset)
	if ok {
		return start
	}
	return -1
}

// TargetSpan returns the byte span of a cleaned target within sentence. The
// unchanged word-bounded matcher runs first, so a target it already anchors
// keeps its span; only when it finds none does the persisted token anchor run.
func TargetSpan(sentence, target string, tokens []analyzer.Token, sentenceStartOffset uint64) (int, int, bool) {
	target = textmatch.CleanLexicalSurface(target)
	return targetSpan(sentence, target, tokens, sentenceStartOffset)
}

func targetSpan(sentence, target string, tokens []analyzer.Token, sentenceStartOffset uint64) (int, int, bool) {
	if start, end, ok := textmatch.FoldedWordSpan(sentence, target); ok {
		return start, end, true
	}
	return tokenAnchorSpan(sentence, target, tokens, sentenceStartOffset)
}

// tokenAnchorSpan anchors target on the first persisted token, in sentence
// order, whose cleaned surface case-folds equal to it. Token offsets are
// document-absolute Unicode code points; the span is taken relative to the
// sentence's persisted start offset and converted to byte offsets in the
// sentence text. An elided or inversion-hyphen target (l'homme, d'accord,
// va-t-il) has no word boundary for the word-bounded matcher, but its own token
// does carry the exact span (ADR 0102).
//
// The span is accepted only when the sentence text at that span, cleaned the
// same way, case-folds equal to the target. That check rejects stale or
// inconsistent offsets and multiword-token parts, which carry the whole
// contraction's span.
func tokenAnchorSpan(sentence, target string, tokens []analyzer.Token, sentenceStartOffset uint64) (int, int, bool) {
	if target == "" || len(tokens) == 0 {
		return 0, 0, false
	}
	for _, token := range tokens {
		if !strings.EqualFold(textmatch.CleanLexicalSurface(token.Surface), target) {
			continue
		}
		start, end, ok := tokenSpanBytes(sentence, token.Location.StartOffset, token.Location.EndOffset, sentenceStartOffset)
		if !ok || !strings.EqualFold(textmatch.CleanLexicalSurface(sentence[start:end]), target) {
			return 0, 0, false
		}
		return start, end, true
	}
	return 0, 0, false
}

// tokenSpanBytes converts a token's document-absolute code-point span to byte
// offsets within sentence, relative to sentenceStartOffset. It reports false
// when the offsets do not describe a span inside the sentence text.
func tokenSpanBytes(sentence string, start, end, sentenceStartOffset uint64) (int, int, bool) {
	if start < sentenceStartOffset || end < start {
		return 0, 0, false
	}
	byteOffsets := make([]int, 0, utf8.RuneCountInString(sentence)+1)
	for offset := range sentence {
		byteOffsets = append(byteOffsets, offset)
	}
	byteOffsets = append(byteOffsets, len(sentence))
	relativeStart, relativeEnd := start-sentenceStartOffset, end-sentenceStartOffset
	if relativeStart >= uint64(len(byteOffsets)) || relativeEnd >= uint64(len(byteOffsets)) {
		return 0, 0, false
	}
	return byteOffsets[relativeStart], byteOffsets[relativeEnd], true
}
