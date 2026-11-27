package study

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/riverqueue/river"
)

const (
	// notConfiguredDelay is how long a job waits when no provider is
	// configured. A missing provider is never an omission.
	notConfiguredDelay = 10 * time.Minute
)

// MaterialStore is what the worker needs from the store.
type MaterialStore interface {
	GetStudyMaterial(ctx context.Context, owner, id string) (domain.StudyMaterial, error)
	GetCurrentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error)
	ListVocabularyConcordance(ctx context.Context, owner, language string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error)
	sentenceReader
	RecordStudyMaterialReady(ctx context.Context, owner, id string, content domain.StudyMaterialReadyContent) error
	RecordStudyMaterialUnpreparable(ctx context.Context, owner, id, reason string) error
	// RecordStudyMaterialNoDictionaryEntry records the no-dictionary-entry
	// omission with the dictionary pin it was made against, so the word is tried
	// again when the pin changes and not before.
	RecordStudyMaterialNoDictionaryEntry(ctx context.Context, owner, id, pin string) error
	seenEvidenceStore
	// ListRetiredStudyOccurrencesOfWord names the occurrences the learner
	// reported as an unsuitable context for the word (ADR 0135 §3). They are
	// never chosen again, whatever Analysis run they are now in.
	ListRetiredStudyOccurrencesOfWord(ctx context.Context, owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error)
	idiomSentenceStore
}

// seenEvidenceStore records what an attempt saw when a word is found
// unpreparable, so it is not tried again on the same evidence.
type seenEvidenceStore interface {
	ListStudyOccurrences(ctx context.Context, owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error)
	SetStudyMaterialEvidence(ctx context.Context, owner, id, evidence string) error
}

// idiomSentenceStore finds an Idiom's candidate sentences.
type idiomSentenceStore interface {
	// ListStudyIdiomSentenceRefs names, in order, the sentences that contain
	// every word of an Idiom headword; LoadStudyIdiomSentences reads a batch of
	// them with their effective tokens (ADR 0137).
	ListStudyIdiomSentenceRefs(ctx context.Context, owner, language, headword string, words []string) ([]persistence.StudyIdiomSentenceRef, error)
	LoadStudyIdiomSentences(ctx context.Context, owner string, refs []persistence.StudyIdiomSentenceRef) ([]persistence.StudyIdiomSentence, error)
}

// Enricher is the per-word enrichment entry point: a contextual Gloss for a
// modern language, the evidence-assisted pass for a source-evidence language.
type Enricher interface {
	EnrichExternalContextual(ctx context.Context, candidate enrichment.Candidate) (enrichment.ContextualMeaning, error)
	EnrichEvidenceAssistedPass(ctx context.Context, candidate enrichment.Candidate, passageContext string) (enrichment.EvidenceAssistedMeaning, error)
	// EnrichIdiomCard is the Idiom's request under the idiom-card contract.
	EnrichIdiomCard(ctx context.Context, card enrichment.IdiomCard) (enrichment.ContextualMeaning, error)
}

// MaterialWorker prepares one word's Study material per River invocation. Its
// dependencies are optional so worker registration tests can validate the job
// type without a database or provider.
type MaterialWorker struct {
	river.WorkerDefaults[PrepareMaterialArgs]
	Store    MaterialStore
	Enricher Enricher
	// Evidence is what a source-evidence language's material needs besides the
	// provider: its dictionary, its passage translations and the pass payload
	// gate. Without it such a word waits; a modern-language word never uses it.
	Evidence *SourceEvidence
	// Idioms makes a word's sentence prefer one where the word is not part of
	// an Idiom candidate; nil leaves the choice unchanged (ADR 0137 §3).
	Idioms *IdiomAvoidance
}

// AddMaterialWorker registers the worker with its dependencies. evidence is
// nil when no source-evidence language can be prepared, and idioms is nil when
// no language's idiom switch is on.
func AddMaterialWorker(workers *river.Workers, store MaterialStore, enricher Enricher, evidence *SourceEvidence, idioms *IdiomAvoidance) {
	river.AddWorker(workers, &MaterialWorker{Store: store, Enricher: enricher, Evidence: evidence, Idioms: idioms})
}

// Work prepares the record's sentence, Gloss and translation. A word with no
// usable sentence, or whose meaning cannot be defended, is recorded as
// unpreparable with its reason. A missing provider or a provider failure is
// never recorded: the job waits or fails and River retries it.
func (w *MaterialWorker) Work(ctx context.Context, job *river.Job[PrepareMaterialArgs]) error {
	if job == nil || w.Store == nil || w.Enricher == nil {
		return errors.New("study: material worker is not configured")
	}
	owner, id := job.Args.OwnerID, job.Args.MaterialID
	material, err := w.Store.GetStudyMaterial(ctx, owner, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if material.State != domain.StudyMaterialPending {
		return nil
	}
	if material.IsIdiom() {
		return w.workIdiom(ctx, owner, material)
	}
	if policy := cardpolicy.For(material.Language); policy.SourceEvidence {
		return w.workSourceEvidence(ctx, owner, material, policy)
	}
	evidence, found, err := w.representativeSentence(ctx, owner, material.StudyItem)
	if err != nil {
		return err
	}
	if !found {
		return w.unpreparable(ctx, owner, id, material.StudyItem, domain.StudyOmissionSentenceQuality)
	}
	meaning, err := w.Enricher.EnrichExternalContextual(ctx, enrichment.Candidate{
		Language: material.Language, CanonicalLemma: material.CanonicalLemma, UPOS: material.UPOS,
		TargetWord:      evidence.target,
		ExampleSentence: evidence.sentence,
	})
	if errors.Is(err, enrichment.ErrExternalNotConfigured) {
		return river.JobSnooze(notConfiguredDelay)
	}
	if err != nil {
		return err
	}
	if !meaning.Resolved() || strings.TrimSpace(meaning.SentenceTranslation) == "" {
		return w.unpreparable(ctx, owner, id, material.StudyItem, domain.StudyOmissionMeaning)
	}
	return w.Store.RecordStudyMaterialReady(ctx, owner, id, domain.StudyMaterialReadyContent{
		BookID: evidence.occurrence.BookID, AnalysisRunID: evidence.occurrence.AnalysisRunID, CorpusID: evidence.occurrence.CorpusID,
		SentenceOrdinal: evidence.occurrence.SentenceOrdinal, TokenOrdinal: evidence.occurrence.TokenOrdinal,
		Sentence: evidence.sentence, TargetWord: evidence.target,
		Gloss: meaning.Gloss, SentenceTranslation: meaning.SentenceTranslation,
		SentenceTranslationTargets: meaning.SentenceTranslationTargets,
	})
}

// unpreparable records the occurrences the attempt saw and then the omission.
func (w *MaterialWorker) unpreparable(ctx context.Context, owner, id string, identity domain.StudyItem, reason string) error {
	if err := w.recordSeenEvidence(ctx, owner, id, identity); err != nil {
		return err
	}
	return w.Store.RecordStudyMaterialUnpreparable(ctx, owner, id, reason)
}

// unpreparableNoDictionaryEntry records the occurrences the attempt saw and then
// the no-dictionary-entry omission with the pin of the dictionary that was asked.
func (w *MaterialWorker) unpreparableNoDictionaryEntry(ctx context.Context, owner, id string, identity domain.StudyItem, pin string) error {
	if err := w.recordSeenEvidence(ctx, owner, id, identity); err != nil {
		return err
	}
	return w.Store.RecordStudyMaterialNoDictionaryEntry(ctx, owner, id, pin)
}

func (w *MaterialWorker) recordSeenEvidence(ctx context.Context, owner, id string, identity domain.StudyItem) error {
	seen, err := w.Store.ListStudyOccurrences(ctx, owner, identity)
	if err != nil {
		return err
	}
	return w.Store.SetStudyMaterialEvidence(ctx, owner, id, domain.StudyEvidenceFingerprint(seen))
}

// sentenceChoice is the Representative sentence chosen for a word.
type sentenceChoice struct {
	occurrence domain.ConcordanceResultOccurrence
	sentence   string
	target     string
	quality    sentencequality.SentenceQuality
	// form is the form the word takes in the occurrence, for Fluency.
	form domain.FluencyForm
	// citations are the cited source sections the sentence touches; they are
	// empty for a modern-language analysis.
	citations []string
	// inIdiom is set when the word's token lies inside an Idiom candidate and
	// the language's idiom switch is on; the sentence then ranks behind any
	// sentence where it does not.
	inIdiom bool
}

// representativeSentence chooses the word's Representative sentence from
// current evidence with the existing sentence-quality gate and ranking: the
// Current reading's occurrences when any of them passes, otherwise the
// library's. The concordance lists the Current reading's occurrences first and
// is read page by page until the choice is settled or the evidence is
// exhausted, so found is false only when every occurrence fails the gate.
func (w *MaterialWorker) representativeSentence(ctx context.Context, owner string, identity domain.StudyItem) (sentenceChoice, bool, error) {
	reading, err := w.Store.GetCurrentReading(ctx, owner, identity.Language)
	if err != nil {
		return sentenceChoice{}, false, err
	}
	priority := ""
	if reading.IsActive() {
		priority = reading.BookID
	}
	lookupPriority := priority
	if lookupPriority == "" {
		lookupPriority = domain.ConcordancePriorityNone
	}
	retired, err := w.Store.ListRetiredStudyOccurrencesOfWord(ctx, owner, identity)
	if err != nil {
		return sentenceChoice{}, false, err
	}
	reader := w.Idioms.reader(identity.Language)
	search := newSearch(priority, reader != nil)
	for page := 1; ; page++ {
		result, err := w.Store.ListVocabularyConcordance(ctx, owner, identity.Language, domain.ConcordanceLookup{
			Language: identity.Language, Term: identity.CanonicalLemma, Match: domain.ConcordanceMatchLemma,
			UPOS: identity.UPOS, Priority: lookupPriority, Page: page,
		})
		if err != nil {
			return sentenceChoice{}, false, err
		}
		choices, err := w.scoreOccurrences(ctx, owner, identity, result.Occurrences, retired, reader)
		if err != nil {
			return sentenceChoice{}, false, err
		}
		search.add(result.Occurrences, choices)
		if search.settled() || !result.HasNext {
			break
		}
	}
	return search.best()
}

// search is the sentence search of one word. With the idiom switch on it runs
// the Idiom-avoiding search and, beside it, the search as it ran before Idioms,
// which stops reading where it always did and is the fallback when no sentence
// lies outside every Idiom candidate.
type search struct {
	preferred *sentenceSearch
	existing  sentenceSearch
}

func newSearch(priority string, avoidIdioms bool) *search {
	result := &search{existing: sentenceSearch{priority: priority}}
	if avoidIdioms {
		result.preferred = &sentenceSearch{priority: priority, preferOutsideIdiom: true}
	}
	return result
}

func (s *search) add(occurrences []domain.ConcordanceResultOccurrence, choices []sentenceChoice) {
	if !s.existing.settled() {
		s.existing.add(occurrences, choices)
	}
	if s.preferred != nil {
		s.preferred.add(occurrences, choices)
	}
}

func (s *search) settled() bool {
	return s.existing.settled() && (s.preferred == nil || s.preferred.settled())
}

func (s *search) best() (sentenceChoice, bool, error) {
	if s.preferred != nil {
		if choice, found, err := s.preferred.best(); found || err != nil {
			return choice, found, err
		}
	}
	return s.existing.best()
}

// sentenceSearch collects accepted sentences page by page, the Current
// reading's apart from the library's.
type sentenceSearch struct {
	priority             string
	inReading, elsewhere []sentenceChoice
	pastReading          bool
	// preferOutsideIdiom limits the search to sentences where the word is
	// outside every Idiom candidate, ahead of the Current reading's order.
	preferOutsideIdiom bool
}

// outsideIdiom counts the choices outside every Idiom candidate.
func outsideIdiom(choices []sentenceChoice) []sentenceChoice {
	kept := make([]sentenceChoice, 0, len(choices))
	for _, choice := range choices {
		if !choice.inIdiom {
			kept = append(kept, choice)
		}
	}
	return kept
}

func (s *sentenceSearch) inPriorityBook(occurrence domain.ConcordanceResultOccurrence) bool {
	return s.priority != "" && occurrence.BookID == s.priority
}

func (s *sentenceSearch) add(occurrences []domain.ConcordanceResultOccurrence, choices []sentenceChoice) {
	for _, choice := range choices {
		if s.inPriorityBook(choice.occurrence) {
			s.inReading = append(s.inReading, choice)
		} else {
			s.elsewhere = append(s.elsewhere, choice)
		}
	}
	for _, occurrence := range occurrences {
		s.pastReading = s.pastReading || !s.inPriorityBook(occurrence)
	}
}

// settled reports that one more page cannot change the choice. The concordance
// lists the Current reading's occurrences first, so once it has moved past them,
// or there is no Current reading, an accepted sentence settles it.
func (s *sentenceSearch) settled() bool {
	inReading, elsewhere := s.inReading, s.elsewhere
	if s.preferOutsideIdiom {
		inReading, elsewhere = outsideIdiom(inReading), outsideIdiom(elsewhere)
	}
	return (len(inReading) > 0 && s.pastReading) || (s.priority == "" && len(elsewhere) > 0)
}

// best returns the Current reading's best sentence, else the library's.
func (s *sentenceSearch) best() (sentenceChoice, bool, error) {
	groups := [][]sentenceChoice{s.inReading, s.elsewhere}
	if s.preferOutsideIdiom {
		// Only a sentence outside every Idiom candidate, the Current reading's
		// before the library's; the caller falls back to the existing search.
		groups = [][]sentenceChoice{outsideIdiom(s.inReading), outsideIdiom(s.elsewhere)}
	}
	for _, group := range groups {
		if best, ok := bestChoice(group); ok {
			return best, true, nil
		}
	}
	return sentenceChoice{}, false, nil
}

// scoreOccurrences keeps the occurrences of the word's full identity whose
// sentence the gate accepts. An occurrence belongs to the identity only when
// its effective lexical discriminator matches: a corrected occurrence carries
// none, and any other carries its token's. A retired occurrence is skipped.
func (w *MaterialWorker) scoreOccurrences(ctx context.Context, owner string, identity domain.StudyItem, occurrences []domain.ConcordanceResultOccurrence, retired []domain.StudyOccurrenceRef, reader *idiomReader) ([]sentenceChoice, error) {
	return scoreOccurrences(ctx, w.Store, owner, identity, occurrences, retired, reader)
}

// sentenceReader is what scoring occurrences needs of the store.
type sentenceReader interface {
	ListCorpusSentences(ctx context.Context, owner, corpusID string, ordinals []int64) (map[int64]analyzer.Sentence, error)
	// ListStudyEffectiveTokens are the sentences' tokens with Lemma corrections
	// and Occurrence exclusions applied, which Idiom matching reads.
	ListStudyEffectiveTokens(ctx context.Context, owner, book, run, corpus string, ordinals []int64) (map[int64][]analyzer.Token, error)
}

func scoreOccurrences(ctx context.Context, store sentenceReader, owner string, identity domain.StudyItem, occurrences []domain.ConcordanceResultOccurrence, retired []domain.StudyOccurrenceRef, reader *idiomReader) ([]sentenceChoice, error) {
	usable := make([]domain.ConcordanceResultOccurrence, 0, len(occurrences))
	ordinals := map[string][]int64{}
	scopes := map[string]domain.ConcordanceResultOccurrence{}
	for _, occurrence := range occurrences {
		if occurrence.Excluded || occurrence.StudyUnavailable || occurrence.SpeakerLabel || domain.RetiredStudyOccurrence(retired, occurrence.StudyRef()) {
			continue
		}
		usable = append(usable, occurrence)
		ordinals[occurrence.CorpusID] = append(ordinals[occurrence.CorpusID], occurrence.SentenceOrdinal)
		scopes[occurrence.CorpusID] = occurrence
	}
	sentences, effective, err := loadScoredSentences(ctx, store, owner, ordinals, scopes, reader != nil)
	if err != nil {
		return nil, err
	}
	choices := make([]sentenceChoice, 0, len(usable))
	for _, occurrence := range usable {
		sentence, ok := sentences[occurrence.CorpusID][occurrence.SentenceOrdinal]
		if !ok || effectiveDiscriminator(occurrence, sentence) != identity.LexicalDiscriminator {
			continue
		}
		quality := sentencequality.ScoreSentenceQuality(identity.Language, occurrence.SentenceText, occurrence.Surface, occurrence.BookStartOffset, sentence.Tokens, sentence.Location.StartOffset)
		if !quality.Accepted {
			continue
		}
		inIdiom, err := reader.inCandidate(ctx, effectiveOrRaw(effective[occurrence.CorpusID][occurrence.SentenceOrdinal], sentence.Tokens), occurrence.TokenOrdinal)
		if err != nil {
			return nil, err
		}
		choices = append(choices, sentenceChoice{occurrence: occurrence, sentence: strings.TrimSpace(occurrence.SentenceText), target: occurrence.Surface, quality: quality, form: fluencyFormOf(sentence.Tokens, occurrence.TokenOrdinal), citations: sentence.Citations, inIdiom: inIdiom})
	}
	return choices, nil
}

// loadScoredSentences reads the sentences of the occurrences being scored by
// corpus. With effective set it also reads their effective tokens, which Idiom
// candidates are matched on: a Lemma correction or Occurrence exclusion the
// learner made changes what matches.
func loadScoredSentences(ctx context.Context, store sentenceReader, owner string, ordinals map[string][]int64, scopes map[string]domain.ConcordanceResultOccurrence, withEffective bool) (map[string]map[int64]analyzer.Sentence, map[string]map[int64][]analyzer.Token, error) {
	sentences := make(map[string]map[int64]analyzer.Sentence, len(ordinals))
	effective := make(map[string]map[int64][]analyzer.Token, len(ordinals))
	for corpusID, list := range ordinals {
		loaded, err := store.ListCorpusSentences(ctx, owner, corpusID, list)
		if err != nil {
			return nil, nil, err
		}
		sentences[corpusID] = loaded
		if !withEffective {
			continue
		}
		scope := scopes[corpusID]
		if effective[corpusID], err = store.ListStudyEffectiveTokens(ctx, owner, scope.BookID, scope.AnalysisRunID, corpusID, list); err != nil {
			return nil, nil, err
		}
	}
	return sentences, effective, nil
}

// effectiveOrRaw is the sentence's effective tokens, or its persisted tokens
// when none were read.
func effectiveOrRaw(effective, persisted []analyzer.Token) []analyzer.Token {
	if len(effective) == len(persisted) && effective != nil {
		return effective
	}
	return persisted
}

// effectiveDiscriminator is the lexical discriminator an occurrence counts
// under: none after a Lemma correction, otherwise its token's.
func effectiveDiscriminator(occurrence domain.ConcordanceResultOccurrence, sentence analyzer.Sentence) string {
	if occurrence.Corrected || occurrence.TokenOrdinal < 0 || occurrence.TokenOrdinal >= int64(len(sentence.Tokens)) {
		return ""
	}
	return sentence.Tokens[occurrence.TokenOrdinal].LexicalDiscriminator
}

// bestChoice orders accepted sentences by descending GDEX score, descending
// quality score, then source position, so the choice is deterministic.
func bestChoice(choices []sentenceChoice) (sentenceChoice, bool) {
	if len(choices) == 0 {
		return sentenceChoice{}, false
	}
	sort.SliceStable(choices, func(i, j int) bool {
		a, b := choices[i], choices[j]
		switch {
		case a.quality.GDEXScore != b.quality.GDEXScore:
			return a.quality.GDEXScore > b.quality.GDEXScore
		case a.quality.Score != b.quality.Score:
			return a.quality.Score > b.quality.Score
		case a.occurrence.BookStartOffset != b.occurrence.BookStartOffset:
			return a.occurrence.BookStartOffset < b.occurrence.BookStartOffset
		case a.occurrence.SentenceOrdinal != b.occurrence.SentenceOrdinal:
			return a.occurrence.SentenceOrdinal < b.occurrence.SentenceOrdinal
		default:
			return a.sentence < b.sentence
		}
	})
	return choices[0], true
}
