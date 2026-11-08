package cardexport

import (
	"context"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
)

// IdiomIndex is the optional dictionary capability idiom detection reads. The
// local dictionary index implements it; a lexical provider without it simply
// produces no idiom items.
type IdiomIndex interface {
	IdiomEntries(ctx context.Context, language, word string) ([]idiom.Entry, error)
}

// IdiomCardBound is the reviewed maximum number of idiom items one
// frozen deck carries. A phrase-dense Book must not flood the deck. The bound
// is applied per deck, after ranking, and overflowed idioms are reported but
// never treated as idiom omissions (doc/features/idiom-cards.md). Changing the
// value is a reviewed change alongside the idiom case set.
const IdiomCardBound = 30

// freezeOptions are the per-call idiom-card settings. They are passed to Freeze
// instead of stored on Presentation, so one Presentation stays safe for
// concurrent preparations with different language switches.
type freezeOptions struct {
	idiomLanguages       map[string]bool
	idiomProviderVersion string
	idiomParameters      idiom.Parameters
	idiomCardBound       int
}

// FreezeOption configures one freeze operation.
type FreezeOption func(*freezeOptions)

// WithIdiomCards enables idiom detection for the given languages. The provider
// version is the new prompt contract's provider identity; an empty version
// leaves the deck exactly as it was without idiom detection.
func WithIdiomCards(languages []string, providerVersion string) FreezeOption {
	return func(options *freezeOptions) {
		for _, language := range languages {
			language = canonicalization.NormalizeLanguage(language)
			if language == "" {
				continue
			}
			options.idiomLanguages[language] = true
		}
		options.idiomProviderVersion = strings.TrimSpace(providerVersion)
	}
}

// withIdiomCardBound overrides the reviewed per-deck idiom bound. Production
// callers use IdiomCardBound; tests use a small bound to exercise
// overflow without freezing a deck of thirty idioms.
func withIdiomCardBound(bound int) FreezeOption {
	return func(options *freezeOptions) {
		options.idiomCardBound = bound
	}
}

// WithoutIdiomCardBound lifts the per-deck idiom bound. Only the operator idiom
// review uses it, so its sample can draw from every idiom a deck matches; a
// prepared deck always keeps IdiomCardBound.
func WithoutIdiomCardBound() FreezeOption {
	return withIdiomCardBound(math.MaxInt)
}

// IdiomWorkItem is one accepted idiom item of a frozen deck: the exact provider
// request a prepared deck sends for it and the matcher evidence it was frozen
// with.
type IdiomWorkItem struct {
	Work     WorkItem
	Headword string
	Evidence IdiomEvidence
}

// IdiomWork returns the deck's accepted idiom items for one language in
// manifest order. Omitted items and other languages are left out.
func (d FrozenDeck) IdiomWork(language string) []IdiomWorkItem {
	language = canonicalization.NormalizeLanguage(language)
	items := make([]IdiomWorkItem, 0)
	for _, decision := range d.manifest.decisions {
		if decision.Disposition != ManifestAccepted || !decision.ItemKind.IsIdiom() {
			continue
		}
		if canonicalization.NormalizeLanguage(decision.Entry.Language) != language {
			continue
		}
		work, ok := d.WorkByOrdinal(decision.Ordinal)
		if !ok {
			continue
		}
		evidence := decision.Entry.IdiomEvidence
		evidence.MatchedTokenOrdinals = append([]int(nil), evidence.MatchedTokenOrdinals...)
		items = append(items, IdiomWorkItem{Work: work, Headword: decision.Entry.Headword, Evidence: evidence})
	}
	return items
}

func newFreezeOptions(options []FreezeOption) freezeOptions {
	config := freezeOptions{idiomLanguages: make(map[string]bool), idiomParameters: idiom.DefaultParameters(), idiomCardBound: IdiomCardBound}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return config
}

// idiomMatch is one matcher candidate resolved against the sentence and source
// projection it came from.
type idiomMatch struct {
	candidate       idiom.Candidate
	sentence        analyzer.Sentence
	sentenceOrdinal int64
	corpusID        string
	sourceDocument  string
	firstEncounter  int64
	provider        string
	providerVersion string
	targetLanguage  string
	ownerID         string
	deckName        string
}

type scoredIdiomMatch struct {
	match   idiomMatch
	quality sentencequality.SentenceQuality
	target  string
}

// freezeIdiomProjections runs the dictionary-anchored matcher over each enabled
// language's accepted sentences and returns one projection per distinct idiom
// (language plus headword), bounded per deck, plus the number of idioms the
// bound left out. The returned projections are frozen after the vocabulary
// projections, so existing lemma ordinals never move.
func freezeIdiomProjections(ctx context.Context, lexical enrichment.LexicalProvider, ordered []CandidateProjection, config freezeOptions) ([]CandidateProjection, int, error) {
	index, ok := lexical.(IdiomIndex)
	if !ok || index == nil || len(config.idiomLanguages) == 0 || config.idiomProviderVersion == "" {
		return nil, 0, nil
	}
	matches := make([]idiomMatch, 0)
	occurrences := make(map[string]int)
	for _, projection := range ordered {
		found, err := idiomMatchesForProjection(ctx, index, projection, config, occurrences)
		if err != nil {
			return nil, 0, err
		}
		matches = append(matches, found...)
	}
	projections := idiomProjectionsFromMatches(matches, occurrences)
	bounded, overflow := boundIdiomProjections(projections, config.idiomCardBound)
	return bounded, overflow, nil
}

// boundIdiomProjections keeps the highest-priority idiom projections and
// returns how many the bound left out. The input is already ranked by anchor
// occurrence count and deterministic tie-breaks.
func boundIdiomProjections(projections []CandidateProjection, bound int) ([]CandidateProjection, int) {
	if bound < 0 {
		bound = 0
	}
	if len(projections) <= bound {
		return projections, 0
	}
	return projections[:bound], len(projections) - bound
}

// idiomMatchesForProjection matches one frozen vocabulary identity's accepted
// sentences and records the identity's occurrence count for anchor choice.
func idiomMatchesForProjection(ctx context.Context, index IdiomIndex, projection CandidateProjection, config freezeOptions, occurrences map[string]int) ([]idiomMatch, error) {
	language := canonicalization.NormalizeLanguage(projection.Entry.Language)
	if !config.idiomLanguages[language] {
		return nil, nil
	}
	identity := anchorIdentityFor(projection.Entry)
	identityKey := anchorIdentityKey(identity)
	if projection.Candidate.OccurrenceCount > occurrences[identityKey] {
		occurrences[identityKey] = projection.Candidate.OccurrenceCount
	}
	matches := make([]idiomMatch, 0)
	for _, sentenceEntry := range sortedProjectionSentences(projection.Sentences) {
		sentence := sentenceEntry.sentence
		tokens := idiomTokens(sentence.Tokens)
		if len(tokens) == 0 {
			continue
		}
		anchors := anchorTokens(tokens, sentence.Tokens, identity)
		if len(anchors) == 0 {
			continue
		}
		entries, err := idiomEntriesForSentence(ctx, index, language, sentence.Tokens)
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			continue
		}
		firstEncounter, ok := sentenceFirstEncounter(sentence)
		if !ok {
			continue
		}
		for _, candidate := range idiom.Match(config.idiomParameters, tokens, anchors, entries) {
			matches = append(matches, idiomMatch{
				candidate: candidate, sentence: sentence, sentenceOrdinal: sentenceEntry.ordinal,
				corpusID: projection.Entry.CorpusID, sourceDocument: projection.Entry.SourceDocument,
				firstEncounter: firstEncounter,
				provider:       projection.Provider, providerVersion: config.idiomProviderVersion,
				targetLanguage: projection.TargetLanguage, ownerID: projection.OwnerID,
				deckName: projection.DeckName,
			})
		}
	}
	return matches, nil
}

func sentenceFirstEncounter(sentence analyzer.Sentence) (int64, bool) {
	value, err := checked.Int64FromUint64(sentence.Location.StartOffset)
	return value, err == nil
}

type orderedSentence struct {
	ordinal  int64
	sentence analyzer.Sentence
}

func sortedProjectionSentences(sentences map[int64]analyzer.Sentence) []orderedSentence {
	ordinals := make([]int64, 0, len(sentences))
	for ordinal := range sentences {
		ordinals = append(ordinals, ordinal)
	}
	result := make([]orderedSentence, 0, len(ordinals))
	for _, ordinal := range slices.Sorted(slices.Values(ordinals)) {
		result = append(result, orderedSentence{ordinal: ordinal, sentence: sentences[ordinal]})
	}
	return result
}

func idiomTokens(tokens []analyzer.Token) []idiom.Token {
	result := make([]idiom.Token, len(tokens))
	for index, token := range tokens {
		result[index] = idiom.Token{Ordinal: index, Surface: token.Surface, CanonicalLemma: token.CanonicalLemma}
	}
	return result
}

func anchorIdentityFor(entry Entry) idiom.AnchorIdentity {
	return idiom.AnchorIdentity{
		Language:             canonicalization.NormalizeLanguage(entry.Language),
		CanonicalLemma:       entry.CanonicalLemma,
		UPOS:                 strings.ToUpper(strings.TrimSpace(entry.UPOS)),
		LexicalDiscriminator: entry.LexicalDiscriminator,
	}
}

func anchorIdentityKey(identity idiom.AnchorIdentity) string {
	return identity.Language + "\x00" + identity.CanonicalLemma + "\x00" + identity.UPOS + "\x00" + identity.LexicalDiscriminator
}

// anchorTokens marks every sentence token that belongs to the frozen vocabulary
// identity. The matcher drops a candidate that has no anchor, so only the
// learner's selected words can introduce an idiom card.
func anchorTokens(tokens []idiom.Token, analyzed []analyzer.Token, identity idiom.AnchorIdentity) []idiom.Anchor {
	anchors := make([]idiom.Anchor, 0, 1)
	for index, token := range tokens {
		if !strings.EqualFold(normSurface(token.CanonicalLemma), normSurface(identity.CanonicalLemma)) {
			continue
		}
		if identity.UPOS != "" && strings.ToUpper(strings.TrimSpace(analyzed[index].UPOS)) != identity.UPOS {
			continue
		}
		anchors = append(anchors, idiom.Anchor{TokenOrdinal: token.Ordinal, Identity: identity})
	}
	return anchors
}

func normSurface(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func idiomEntriesForSentence(ctx context.Context, index IdiomIndex, language string, tokens []analyzer.Token) ([]idiom.Entry, error) {
	seen := make(map[string]struct{}, len(tokens))
	entries := make([]idiom.Entry, 0)
	for _, token := range tokens {
		for _, word := range []string{token.CanonicalLemma, token.Surface} {
			word = strings.TrimSpace(word)
			if word == "" {
				continue
			}
			cacheKey := language + "\x00" + word
			if _, ok := seen[cacheKey]; ok {
				continue
			}
			seen[cacheKey] = struct{}{}
			found, err := index.IdiomEntries(ctx, language, word)
			if err != nil {
				return nil, err
			}
			entries = append(entries, found...)
		}
	}
	return entries, nil
}

func idiomProjectionsFromMatches(matches []idiomMatch, occurrences map[string]int) []CandidateProjection {
	grouped := make(map[string][]idiomMatch)
	order := make([]string, 0)
	for _, match := range matches {
		key := canonicalization.NormalizeLanguage(match.candidate.Entry.Language) + "\x00" + match.candidate.Entry.Headword
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], match)
	}
	result := make([]CandidateProjection, 0, len(grouped))
	for _, key := range order {
		if projection, ok := idiomProjectionForMatches(grouped[key], occurrences); ok {
			result = append(result, projection)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return idiomProjectionLess(result[i], result[j])
	})
	return result
}

// idiomProjectionLess ranks idiom projections for the per-deck bound. More
// anchor occurrences go first; ties break by earliest sentence, headword and
// language, so the bound is deterministic for a given deck.
func idiomProjectionLess(left, right CandidateProjection) bool {
	if left.Candidate.OccurrenceCount != right.Candidate.OccurrenceCount {
		return left.Candidate.OccurrenceCount > right.Candidate.OccurrenceCount
	}
	if left.Candidate.FirstEncounter != right.Candidate.FirstEncounter {
		return left.Candidate.FirstEncounter < right.Candidate.FirstEncounter
	}
	if left.Entry.Headword != right.Entry.Headword {
		return left.Entry.Headword < right.Entry.Headword
	}
	return left.Entry.Language < right.Entry.Language
}

func idiomProjectionForMatches(matches []idiomMatch, occurrences map[string]int) (CandidateProjection, bool) { //nolint:gocognit // scoring and assembly are one deterministic decision
	if len(matches) == 0 {
		return CandidateProjection{}, false
	}
	anchorKeys := make([]string, 0, len(matches))
	seenAnchors := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		key := anchorIdentityKey(match.candidate.Anchor)
		if _, ok := seenAnchors[key]; ok {
			continue
		}
		seenAnchors[key] = struct{}{}
		anchorKeys = append(anchorKeys, key)
	}
	sort.Slice(anchorKeys, func(i, j int) bool {
		left, right := occurrences[anchorKeys[i]], occurrences[anchorKeys[j]]
		if left != right {
			return left > right
		}
		return anchorKeys[i] < anchorKeys[j]
	})
	bestAnchor := anchorKeys[0]
	candidates := make([]scoredIdiomMatch, 0, len(matches))
	for _, match := range matches {
		if anchorIdentityKey(match.candidate.Anchor) != bestAnchor {
			continue
		}
		target, ok := matchedAnchorSurface(match.candidate, match.sentence.Tokens)
		if !ok {
			continue
		}
		start, ok := sentenceFirstEncounter(match.sentence)
		if !ok {
			continue
		}
		quality := sentencequality.ScoreSentenceQuality(match.candidate.Entry.Language, match.sentence.Text, target, start, match.sentence.Tokens, match.sentence.Location.StartOffset)
		if !quality.Accepted {
			continue
		}
		candidates = append(candidates, scoredIdiomMatch{match: match, quality: quality, target: target})
	}
	if len(candidates) == 0 {
		return CandidateProjection{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.quality.GDEXScore != right.quality.GDEXScore {
			return left.quality.GDEXScore > right.quality.GDEXScore
		}
		if left.match.firstEncounter != right.match.firstEncounter {
			return left.match.firstEncounter < right.match.firstEncounter
		}
		if left.match.sentenceOrdinal != right.match.sentenceOrdinal {
			return left.match.sentenceOrdinal < right.match.sentenceOrdinal
		}
		return left.match.sentence.Text < right.match.sentence.Text
	})
	best := candidates[0]
	firstEncounter, ok := sentenceFirstEncounter(best.match.sentence)
	if !ok {
		return CandidateProjection{}, false
	}
	entry := Entry{
		Language: best.match.candidate.Entry.Language, CanonicalLemma: best.match.candidate.Entry.Headword,
		UPOS: "IDIOM", Headword: best.match.candidate.Entry.Headword, ItemKind: enrichment.ItemKindIdiom,
		Sentence: best.match.sentence.Text, TargetWord: best.target,
		SentenceTokens: cloneTokens(best.match.sentence.Tokens), SentenceStartOffset: best.match.sentence.Location.StartOffset,
		FirstEncounter: firstEncounter, CorpusID: best.match.corpusID,
		SentenceOrdinal: best.match.sentenceOrdinal, SourceDocument: best.match.sourceDocument,
		CandidateSenses:           boundedIdiomSenses(best.match.candidate.Entry.Senses),
		DictionaryProviderVersion: best.match.candidate.Entry.Version,
		IdiomEvidence: IdiomEvidence{ //nolint:modernize // explicit embedded literal keeps the evidence readable
			MatchedTokenOrdinals:       append([]int(nil), best.match.candidate.MatchedTokenOrdinals...),
			AnchorLanguage:             best.match.candidate.Anchor.Language,
			AnchorCanonicalLemma:       best.match.candidate.Anchor.CanonicalLemma,
			AnchorUPOS:                 best.match.candidate.Anchor.UPOS,
			AnchorLexicalDiscriminator: best.match.candidate.Anchor.LexicalDiscriminator,
			Source:                     best.match.candidate.Provenance.Source,
			SourceVersion:              best.match.candidate.Provenance.Version,
			MatcherVersion:             best.match.candidate.Provenance.MatcherVersion,
			GapBound:                   best.match.candidate.Provenance.GapBound,
		},
	}
	return CandidateProjection{
		OwnerID: best.match.ownerID, DeckName: best.match.deckName,
		Candidate: domain.SelectionCandidate{
			OwnerID: best.match.ownerID, CorpusID: best.match.corpusID,
			Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS,
			OccurrenceCount: occurrences[bestAnchor], FirstEncounter: entry.FirstEncounter,
		},
		Entry: entry, Provider: best.match.provider, ProviderVersion: best.match.providerVersion,
		TargetLanguage: best.match.targetLanguage, RequireIdiomCard: true,
	}, true
}

func matchedAnchorSurface(candidate idiom.Candidate, tokens []analyzer.Token) (string, bool) {
	if candidate.AnchorTokenOrdinal < 0 || candidate.AnchorTokenOrdinal >= len(tokens) {
		return "", false
	}
	surface := strings.TrimSpace(tokens[candidate.AnchorTokenOrdinal].Surface)
	return surface, surface != ""
}

func boundedIdiomSenses(senses []enrichment.LexicalSense) []enrichment.LexicalSense {
	if len(senses) > enrichment.DefaultMaxCandidateSenses {
		senses = senses[:enrichment.DefaultMaxCandidateSenses]
	}
	return enrichment.CloneLexicalSenses(senses)
}
