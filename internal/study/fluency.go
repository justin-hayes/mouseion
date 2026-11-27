package study

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
)

const (
	// maxFluencyPages bounds how much of a word's concordance one job reads.
	maxFluencyPages = 10
	// maxFluencyAsks bounds the provider calls one job makes for a word: the
	// ranked candidates it asks about before it stops with what agreed.
	maxFluencyAsks = 12
	// fluencyUniquePeriod keeps a word's preparation job from being enqueued
	// more than once an hour, so opening the page does not scan a word's
	// concordance again and again.
	fluencyUniquePeriod = time.Hour
)

// PrepareFluencyArgs identifies one mature word whose Fluency contexts are to
// be prepared. It carries no source text, sentence or Gloss.
type PrepareFluencyArgs struct {
	OwnerID    string `json:"owner_id"`
	ProgressID string `json:"progress_id" river:"unique"`
}

// Kind names the job for River.
func (PrepareFluencyArgs) Kind() string { return "study_prepare_fluency" }

// EnqueueFluencyPreparation asks for Fluency contexts for every mature word
// that has fewer than domain.FluencyContextsPerWord agreeing ones. It is safe to
// call whenever Today is opened: a word's job is enqueued once an hour at most,
// and an occurrence the job has assessed is never asked about again.
func (s *Service) EnqueueFluencyPreparation(ctx context.Context, owner, language string) error {
	progress, err := s.store.ListStudyProgress(ctx, owner, language)
	if err != nil {
		return err
	}
	contexts, err := s.store.ListStudyFluencyContexts(ctx, owner, language)
	if err != nil {
		return err
	}
	for _, word := range domain.FluencyContextsNeeded(progress, contexts) {
		if err := s.enqueueFluencyWord(ctx, owner, word.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) enqueueFluencyWord(ctx context.Context, owner, progressID string) (err error) {
	tx, err := s.store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err := s.client.InsertTx(ctx, tx, PrepareFluencyArgs{OwnerID: owner, ProgressID: progressID}, &river.InsertOpts{
		Queue: Queue, MaxAttempts: maxPreparationAttempts,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: fluencyUniquePeriod},
	}); err != nil {
		return fmt.Errorf("enqueue study fluency preparation: %w", err)
	}
	return tx.Commit(ctx)
}

// FluencyStore is what the Fluency worker needs from the store.
type FluencyStore interface {
	GetStudyProgress(ctx context.Context, owner, id string) (domain.StudyProgress, error)
	GetStudyMaterial(ctx context.Context, owner, id string) (domain.StudyMaterial, error)
	GetCurrentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error)
	ListVocabularyConcordance(ctx context.Context, owner, language string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error)
	ListCorpusSentences(ctx context.Context, owner, corpusID string, ordinals []int64) (map[int64]analyzer.Sentence, error)
	ListStudyEffectiveTokens(ctx context.Context, owner, book, run, corpus string, ordinals []int64) (map[int64][]analyzer.Token, error)
	ListStudyFluencyContexts(ctx context.Context, owner, language string) ([]domain.FluencyContext, error)
	RecordStudyFluencyContext(ctx context.Context, owner string, record domain.FluencyContextRecord) error
	// ListRetiredStudyOccurrences names the occurrences the learner reported as
	// an unsuitable context for the word; none is ever asked about (ADR 0135 §3).
	ListRetiredStudyOccurrences(ctx context.Context, owner, progressID string) ([]domain.StudyOccurrenceRef, error)
}

// FluencyEnricher asks the provider whether an occurrence uses the learned
// meaning.
type FluencyEnricher interface {
	EnrichFluencyAgreement(ctx context.Context, candidate enrichment.Candidate, learnedMeaning string) (enrichment.FluencyAgreement, error)
	// EnrichEvidenceAssistedPass is the per-item pass a source-evidence
	// occurrence gets first, exactly as Study material does (ADR 0138 §4).
	EnrichEvidenceAssistedPass(ctx context.Context, candidate enrichment.Candidate, passageContext string) (enrichment.EvidenceAssistedMeaning, error)
	// EnrichSourceEvidenceFluencyAgreement is the agreement check of a
	// source-evidence language: its payload adds both sides' cited senses.
	EnrichSourceEvidenceFluencyAgreement(ctx context.Context, candidate enrichment.Candidate, learnedMeaning string, learned, occurrence []string) (enrichment.FluencyAgreement, error)
}

// FluencyWorker prepares one mature word's Fluency contexts per River
// invocation. Its dependencies are optional so worker registration tests can
// validate the job type without a database or provider.
type FluencyWorker struct {
	river.WorkerDefaults[PrepareFluencyArgs]
	Store    FluencyStore
	Enricher FluencyEnricher
	// MaySend reports whether the Fluency agreement payload may be sent for a
	// language: its provider payload assessment is recorded and matches
	// (ADR 0133). While it fails, nothing is transmitted.
	MaySend func(language string) error
	// Evidence is what a source-evidence language's contexts need besides the
	// provider: its dictionary, its passage translations and the pass payload
	// gate. Without it, or without MaySendAgreement, such a word sends nothing.
	Evidence *SourceEvidence
	// MaySendAgreement reports whether the source-evidence variant of the
	// agreement payload may be sent for a language (ADR 0133, ADR 0138 §4). The
	// pass's own gate is Evidence.MaySend; both must pass before any send.
	MaySendAgreement func(language string) error
	// Idioms makes the ranking prefer an occurrence where the word is not part
	// of an Idiom candidate; nil leaves the ranking unchanged (ADR 0137 §3).
	Idioms *IdiomAvoidance
}

// AddFluencyWorker registers the worker with its dependencies. evidence and
// maySendAgreement are nil when no source-evidence language can be prepared,
// and idioms is nil when no language's idiom switch is on.
func AddFluencyWorker(workers *river.Workers, store FluencyStore, enricher FluencyEnricher, maySend func(language string) error, evidence *SourceEvidence, maySendAgreement func(language string) error, idioms *IdiomAvoidance) {
	river.AddWorker(workers, &FluencyWorker{Store: store, Enricher: enricher, MaySend: maySend, Evidence: evidence, MaySendAgreement: maySendAgreement, Idioms: idioms})
}

// Work prepares the word's Fluency contexts: it ranks other occurrences of the
// word across the library to prefer a different form, asks the provider about
// each in turn whether it uses the learned meaning, and stores the answers
// until the word has its agreeing contexts. A disagreeing occurrence is stored
// too, so it is not asked about again. A missing provider or assessment sends
// nothing and waits; a provider failure is returned and River retries it.
func (w *FluencyWorker) Work(ctx context.Context, job *river.Job[PrepareFluencyArgs]) error {
	if job == nil || w.Store == nil || w.Enricher == nil || w.MaySend == nil {
		return errors.New("study: fluency worker is not configured")
	}
	owner := job.Args.OwnerID
	word, err := w.Store.GetStudyProgress(ctx, owner, job.Args.ProgressID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !domain.IsStudyMature(word) {
		// The word lapsed since the job was enqueued; it is prepared when it is
		// mature again.
		return nil
	}
	existing, err := w.Store.ListStudyFluencyContexts(ctx, owner, word.Language)
	if err != nil {
		return err
	}
	assessed, agreeing := assessedOccurrences(existing, word.ID)
	if agreeing >= domain.FluencyContextsPerWord {
		return nil
	}
	policy := cardpolicy.For(word.Language)
	var evidence *fluencyEvidence
	if policy.SourceEvidence {
		var ready, wait bool
		if evidence, ready, wait = w.sourceEvidence(ctx, word, policy); !ready {
			if wait {
				// A missing dictionary, or one without the source, waits like a
				// missing provider; it is never an answer.
				return river.JobSnooze(notConfiguredDelay)
			}
			// Nothing is sent: a gate is not recorded, or the Learned meaning
			// cites nothing to compare.
			return nil
		}
	} else if err := w.MaySend(word.Language); err != nil {
		// Nothing is sent. The job ends instead of waiting: opening the Study
		// page enqueues it again once an hour, and a waiting job per hour would
		// all run, and ask about the same occurrences, the moment the assessment
		// is recorded.
		return nil //nolint:nilerr // an unrecorded assessment is not a failure: nothing is sent
	}
	ranked, err := w.rankedCandidates(ctx, owner, word, assessed)
	if err != nil {
		return err
	}
	return w.askAbout(ctx, owner, word, ranked, agreeing, evidence)
}

// askAbout asks the provider about each ranked occurrence in turn until the word
// has its agreeing contexts or maxFluencyAsks questions are spent, storing each
// answer. A missing provider waits; a provider failure is returned for River to
// retry.
func (w *FluencyWorker) askAbout(ctx context.Context, owner string, word domain.StudyProgress, ranked []sentenceChoice, agreeing int, evidence *fluencyEvidence) error {
	for asked, choice := range ranked {
		if agreeing >= domain.FluencyContextsPerWord || asked >= maxFluencyAsks {
			break
		}
		answer, err := w.assess(ctx, word, choice, evidence)
		if errors.Is(err, enrichment.ErrExternalNotConfigured) {
			return river.JobSnooze(notConfiguredDelay)
		}
		if err != nil {
			return err
		}
		stored, err := w.record(ctx, owner, word.ID, choice, answer)
		if err != nil {
			return err
		}
		if stored && answer.Agrees() {
			agreeing++
		}
	}
	return nil
}

// assess asks the provider about one occurrence: a modern language's agreement
// check, or a source-evidence language's pass and then, when the pass cites a
// sense, its agreement variant. Both calls are one ask.
func (w *FluencyWorker) assess(ctx context.Context, word domain.StudyProgress, choice sentenceChoice, evidence *fluencyEvidence) (fluencyAnswer, error) {
	candidate := enrichment.Candidate{
		Language: word.Language, CanonicalLemma: word.CanonicalLemma, UPOS: word.UPOS,
		TargetWord: choice.target, ExampleSentence: choice.sentence,
	}
	if evidence == nil {
		agreement, err := w.Enricher.EnrichFluencyAgreement(ctx, candidate, word.LearnedMeaning)
		return fluencyAnswer{FluencyAgreement: agreement}, err
	}
	return w.assessSourceEvidence(ctx, word, choice, candidate, evidence)
}

func (w *FluencyWorker) record(ctx context.Context, owner, progressID string, choice sentenceChoice, agreement fluencyAnswer) (bool, error) {
	record := domain.FluencyContextRecord{
		ProgressID: progressID,
		BookID:     choice.occurrence.BookID, AnalysisRunID: choice.occurrence.AnalysisRunID, CorpusID: choice.occurrence.CorpusID,
		SentenceOrdinal: choice.occurrence.SentenceOrdinal, TokenOrdinal: choice.occurrence.TokenOrdinal,
		Sentence: choice.sentence, TargetWord: choice.target, Form: choice.form,
	}
	reason, ok := domain.ParseFluencyReason(agreement.Reason)
	if !ok {
		return false, fmt.Errorf("record fluency context: unknown reason %q", agreement.Reason)
	}
	record.Reason, record.Agreement = reason, reason.Agreement()
	record.Gloss, record.SentenceTranslation = agreement.Gloss, agreement.SentenceTranslation
	record.SentenceTranslationTargets = agreement.SentenceTranslationTargets
	record.Cited = agreement.Cited
	err := w.Store.RecordStudyFluencyContext(ctx, owner, record)
	if errors.Is(err, domain.ErrFluencyContextNotStored) {
		return false, nil
	}
	return err == nil, err
}

type occurrenceKey struct {
	analysisRunID           string
	sentenceOrd, tokenOrdin int64
}

// assessedOccurrences returns the occurrences already asked about for a word
// and how many agreed.
func assessedOccurrences(contexts []domain.FluencyContext, progressID string) (map[occurrenceKey]bool, int) {
	assessed := map[occurrenceKey]bool{}
	agreeing := 0
	for _, item := range contexts {
		if item.ProgressID != progressID {
			continue
		}
		assessed[occurrenceKey{item.AnalysisRunID, item.SentenceOrdinal, item.TokenOrdinal}] = true
		if item.Agreement == domain.FluencyAgrees {
			agreeing++
		}
	}
	return assessed, agreeing
}

// rankedCandidates reads the word's other occurrences across the library, the
// Current reading's first, keeps those the sentence-quality gate accepts and
// that were not assessed, and orders them to prefer a different form than the
// one the word was introduced in.
func (w *FluencyWorker) rankedCandidates(ctx context.Context, owner string, word domain.StudyProgress, assessed map[occurrenceKey]bool) ([]sentenceChoice, error) {
	material, err := w.Store.GetStudyMaterial(ctx, owner, word.MaterialID)
	if err != nil {
		return nil, err
	}
	introduced := w.introducedForm(ctx, owner, material)
	reading, err := w.Store.GetCurrentReading(ctx, owner, word.Language)
	if err != nil {
		return nil, err
	}
	priority, lookupPriority := "", domain.ConcordancePriorityNone
	if reading.IsActive() {
		priority, lookupPriority = reading.BookID, reading.BookID
	}
	retired, err := w.Store.ListRetiredStudyOccurrences(ctx, owner, word.ID)
	if err != nil {
		return nil, err
	}
	choices, err := w.otherOccurrences(ctx, owner, word, material, assessed, retired, lookupPriority, w.Idioms.reader(word.Language))
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]sentenceChoice, len(choices))
	candidates := make([]domain.FluencyCandidate, 0, len(choices))
	for _, choice := range choices {
		occurrence := choice.occurrence
		key := fmt.Sprintf("%s/%d/%d", occurrence.AnalysisRunID, occurrence.SentenceOrdinal, occurrence.TokenOrdinal)
		byKey[key] = choice
		candidates = append(candidates, domain.FluencyCandidate{
			Key: key, Form: choice.form, InIdiom: choice.inIdiom, InCurrentReading: priority != "" && occurrence.BookID == priority, Quality: choice.quality.GDEXScore + float64(choice.quality.Score),
		})
	}
	ranked := domain.RankFluencyCandidates(introduced, candidates)
	ordered := make([]sentenceChoice, 0, len(ranked))
	for _, candidate := range ranked {
		ordered = append(ordered, byKey[candidate.Key])
	}
	return ordered, nil
}

// otherOccurrences reads the word's concordance page by page and keeps the
// occurrences the sentence-quality gate accepts, leaving out the occurrences
// already assessed and the word's own introducing sentence, wherever else the
// library repeats that text.
func (w *FluencyWorker) otherOccurrences(ctx context.Context, owner string, word domain.StudyProgress, material domain.StudyMaterial, assessed map[occurrenceKey]bool, retired []domain.StudyOccurrenceRef, lookupPriority string, reader *idiomReader) ([]sentenceChoice, error) {
	introducing := strings.TrimSpace(material.Sentence)
	var kept []sentenceChoice
	for page := 1; page <= maxFluencyPages; page++ {
		result, err := w.Store.ListVocabularyConcordance(ctx, owner, word.Language, domain.ConcordanceLookup{
			Language: word.Language, Term: word.CanonicalLemma, Match: domain.ConcordanceMatchLemma,
			UPOS: word.UPOS, Priority: lookupPriority, Page: page,
		})
		if err != nil {
			return nil, err
		}
		choices, err := scoreOccurrences(ctx, w.Store, owner, word.StudyItem, result.Occurrences, retired, reader)
		if err != nil {
			return nil, err
		}
		for _, choice := range choices {
			occurrence := choice.occurrence
			if !assessed[occurrenceKey{occurrence.AnalysisRunID, occurrence.SentenceOrdinal, occurrence.TokenOrdinal}] && choice.sentence != introducing {
				kept = append(kept, choice)
			}
		}
		if !result.HasNext {
			break
		}
	}
	return kept, nil
}

// introducedForm is the form the word took in its introducing sentence. It is
// empty when the sentence can no longer be read, which only weakens the form
// preference.
func (w *FluencyWorker) introducedForm(ctx context.Context, owner string, material domain.StudyMaterial) domain.FluencyForm {
	form := domain.FluencyForm{Surface: material.TargetWord}
	sentences, err := w.Store.ListCorpusSentences(ctx, owner, material.CorpusID, []int64{material.SentenceOrdinal})
	if err != nil {
		return form
	}
	if sentence, ok := sentences[material.SentenceOrdinal]; ok {
		form = fluencyFormOf(sentence.Tokens, material.TokenOrdinal)
		if form.Surface == "" {
			form.Surface = material.TargetWord
		}
	}
	return form
}

// fluencyFormOf describes the form the token at ordinal takes: its surface, its
// morphological features in a fixed order, and whether a separable particle of
// the verb is split off as its own token.
func fluencyFormOf(tokens []analyzer.Token, ordinal int64) domain.FluencyForm {
	if ordinal < 0 || ordinal >= int64(len(tokens)) {
		return domain.FluencyForm{}
	}
	token := tokens[ordinal]
	keys := make([]string, 0, len(token.Morphology))
	for key := range token.Morphology {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	features := make([]string, 0, len(keys))
	for _, key := range keys {
		features = append(features, key+"="+token.Morphology[key])
	}
	form := domain.FluencyForm{Surface: token.Surface, Features: strings.Join(features, "|")}
	for _, other := range tokens {
		if other.Dependency == "compound:prt" && int64(other.Head) == ordinal {
			form.Split = true
			break
		}
	}
	return form
}
