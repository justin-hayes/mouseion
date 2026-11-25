package study

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/riverqueue/river"
)

const (
	// concordancePageLimit bounds how many concordance pages one word's
	// sentence choice reads (25 occurrences each).
	concordancePageLimit = 8
	// notConfiguredDelay is how long a job waits when no provider is
	// configured. A missing provider is never an omission.
	notConfiguredDelay = 10 * time.Minute
)

// MaterialStore is what the worker needs from the store.
type MaterialStore interface {
	GetStudyMaterial(ctx context.Context, owner, id string) (domain.StudyMaterial, error)
	GetCurrentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error)
	ListVocabularyConcordance(ctx context.Context, owner, language string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error)
	ListCorpusSentences(ctx context.Context, owner, corpusID string, ordinals []int64) (map[int64]analyzer.Sentence, error)
	RecordStudyMaterialReady(ctx context.Context, owner, id string, content domain.StudyMaterialReadyContent) error
	RecordStudyMaterialUnpreparable(ctx context.Context, owner, id, reason string) error
}

// Enricher is the per-word enrichment entry point with a contextual Gloss
// required.
type Enricher interface {
	EnrichExternalContextual(ctx context.Context, candidate enrichment.Candidate) (enrichment.ContextualMeaning, error)
}

// MaterialWorker prepares one word's Study material per River invocation. Its
// dependencies are optional so worker registration tests can validate the job
// type without a database or provider.
type MaterialWorker struct {
	river.WorkerDefaults[PrepareMaterialArgs]
	Store    MaterialStore
	Enricher Enricher
}

// AddMaterialWorker registers the worker with its dependencies.
func AddMaterialWorker(workers *river.Workers, store MaterialStore, enricher Enricher) {
	river.AddWorker(workers, &MaterialWorker{Store: store, Enricher: enricher})
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
	evidence, found, err := w.representativeSentence(ctx, owner, material.StudyIdentity)
	if err != nil {
		return err
	}
	if !found {
		return w.Store.RecordStudyMaterialUnpreparable(ctx, owner, id, domain.StudyOmissionSentenceQuality)
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
		return w.Store.RecordStudyMaterialUnpreparable(ctx, owner, id, domain.StudyOmissionMeaning)
	}
	return w.Store.RecordStudyMaterialReady(ctx, owner, id, domain.StudyMaterialReadyContent{
		BookID: evidence.occurrence.BookID, AnalysisRunID: evidence.occurrence.AnalysisRunID, CorpusID: evidence.occurrence.CorpusID,
		SentenceOrdinal: evidence.occurrence.SentenceOrdinal, TokenOrdinal: evidence.occurrence.TokenOrdinal,
		Sentence: evidence.sentence, TargetWord: evidence.target,
		Gloss: meaning.Gloss, SentenceTranslation: meaning.SentenceTranslation,
		SentenceTranslationTargets: meaning.SentenceTranslationTargets,
	})
}

// sentenceChoice is the Representative sentence chosen for a word.
type sentenceChoice struct {
	occurrence domain.ConcordanceResultOccurrence
	sentence   string
	target     string
	quality    sentencequality.SentenceQuality
}

// representativeSentence chooses the word's Representative sentence from
// current evidence with the existing sentence-quality gate and ranking: the
// Current reading's occurrences when any of them passes, otherwise the
// library's. found is false when every sentence fails the gate.
func (w *MaterialWorker) representativeSentence(ctx context.Context, owner string, identity domain.StudyIdentity) (sentenceChoice, bool, error) {
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
	occurrences, err := w.occurrences(ctx, owner, identity, lookupPriority)
	if err != nil {
		return sentenceChoice{}, false, err
	}
	choices, err := w.scoreOccurrences(ctx, owner, identity.Language, occurrences)
	if err != nil {
		return sentenceChoice{}, false, err
	}
	var inReading, elsewhere []sentenceChoice
	for _, choice := range choices {
		if priority != "" && choice.occurrence.BookID == priority {
			inReading = append(inReading, choice)
		} else {
			elsewhere = append(elsewhere, choice)
		}
	}
	for _, group := range [][]sentenceChoice{inReading, elsewhere} {
		if best, ok := bestChoice(group); ok {
			return best, true, nil
		}
	}
	return sentenceChoice{}, false, nil
}

// occurrences reads the word's occurrences, the Current reading's first, over
// the concordance's pages.
func (w *MaterialWorker) occurrences(ctx context.Context, owner string, identity domain.StudyIdentity, priority string) ([]domain.ConcordanceResultOccurrence, error) {
	var occurrences []domain.ConcordanceResultOccurrence
	for page := 1; page <= concordancePageLimit; page++ {
		result, err := w.Store.ListVocabularyConcordance(ctx, owner, identity.Language, domain.ConcordanceLookup{
			Language: identity.Language, Term: identity.CanonicalLemma, Match: domain.ConcordanceMatchLemma,
			UPOS: identity.UPOS, Priority: priority, Page: page,
		})
		if err != nil {
			return nil, err
		}
		for _, occurrence := range result.Occurrences {
			if occurrence.Excluded || occurrence.StudyUnavailable || occurrence.SpeakerLabel {
				continue
			}
			occurrences = append(occurrences, occurrence)
		}
		if !result.HasNext {
			break
		}
	}
	return occurrences, nil
}

// scoreOccurrences scores each occurrence's sentence and keeps the ones the
// gate accepts.
func (w *MaterialWorker) scoreOccurrences(ctx context.Context, owner, language string, occurrences []domain.ConcordanceResultOccurrence) ([]sentenceChoice, error) {
	ordinals := map[string][]int64{}
	for _, occurrence := range occurrences {
		ordinals[occurrence.CorpusID] = append(ordinals[occurrence.CorpusID], occurrence.SentenceOrdinal)
	}
	sentences := make(map[string]map[int64]analyzer.Sentence, len(ordinals))
	for corpusID, list := range ordinals {
		loaded, err := w.Store.ListCorpusSentences(ctx, owner, corpusID, list)
		if err != nil {
			return nil, err
		}
		sentences[corpusID] = loaded
	}
	choices := make([]sentenceChoice, 0, len(occurrences))
	for _, occurrence := range occurrences {
		sentence, ok := sentences[occurrence.CorpusID][occurrence.SentenceOrdinal]
		if !ok {
			continue
		}
		quality := sentencequality.ScoreSentenceQuality(language, occurrence.SentenceText, occurrence.Surface, occurrence.BookStartOffset, sentence.Tokens, sentence.Location.StartOffset)
		if !quality.Accepted {
			continue
		}
		choices = append(choices, sentenceChoice{occurrence: occurrence, sentence: strings.TrimSpace(occurrence.SentenceText), target: occurrence.Surface, quality: quality})
	}
	return choices, nil
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
