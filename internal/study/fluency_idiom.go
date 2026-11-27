package study

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/riverqueue/river"
)

// workIdiom prepares a mature Idiom's Fluency contexts (ADR 0137 §6): the
// Idiom's other candidate sentences, those whose matched words differ in
// inflection or arrangement from the introducing sentence first, each checked
// under the idiom contract for agreement with the Learned meaning. A context
// that does not agree is stored with its reason as evidence and never shown.
// With the language's switch off, or with no cited sense in the Learned
// meaning to compare against, nothing is sent.
func (w *FluencyWorker) workIdiom(ctx context.Context, owner string, word domain.StudyProgress, assessed map[occurrenceKey]bool, agreeing int) error {
	reader := w.Idioms.reader(word.Language)
	if reader == nil || word.LearnedCitations.Empty() {
		return nil
	}
	headword, _ := word.Headword()
	entry, found, err := reader.entry(ctx, headword)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	material, err := w.Store.GetStudyMaterial(ctx, owner, word.MaterialID)
	if err != nil {
		return err
	}
	retired, err := w.Store.ListRetiredStudyOccurrences(ctx, owner, word.ID)
	if err != nil {
		return err
	}
	introducing := strings.TrimSpace(material.Sentence)
	choices, err := idiomChoicesOf(ctx, w.Store, owner, reader, entry, retired, func(choice idiomChoice) bool {
		return choice.sentence == introducing || assessed[occurrenceKey{choice.sentenceRef.AnalysisRunID, choice.sentenceRef.SentenceOrdinal, choice.sentenceRef.TokenOrdinal}]
	})
	if err != nil {
		return err
	}
	return w.askAboutIdiom(ctx, owner, word, headword, w.rankedIdiomChoices(ctx, owner, material, choices), agreeing)
}

// askAboutIdiom asks about each ranked sentence in turn until the Idiom has its
// agreeing contexts or maxFluencyAsks questions are spent, storing each answer.
func (w *FluencyWorker) askAboutIdiom(ctx context.Context, owner string, word domain.StudyProgress, headword string, ranked []idiomChoice, agreeing int) error {
	for asked, choice := range ranked {
		if agreeing >= domain.FluencyContextsPerWord || asked >= maxFluencyAsks {
			break
		}
		answer, err := w.assessIdiom(ctx, word, headword, choice)
		if errors.Is(err, enrichment.ErrExternalNotConfigured) {
			return river.JobSnooze(notConfiguredDelay)
		}
		if err != nil {
			return err
		}
		stored, err := w.recordContext(ctx, owner, word.ID, choice.sentenceRef, choice.sentence, strings.Join(choice.forms, " "), choice.form, answer)
		if err != nil {
			return err
		}
		if stored && answer.Agrees() {
			agreeing++
		}
	}
	return nil
}

// rankedIdiomChoices orders the Idiom's sentences to prefer matched words that
// take another form than in the introducing sentence.
func (w *FluencyWorker) rankedIdiomChoices(ctx context.Context, owner string, material domain.StudyMaterial, choices []idiomChoice) []idiomChoice {
	introduced := domain.FluencyForm{Surface: material.TargetWord}
	if sentences, err := w.Store.ListCorpusSentences(ctx, owner, material.CorpusID, []int64{material.SentenceOrdinal}); err == nil {
		if sentence, ok := sentences[material.SentenceOrdinal]; ok {
			if form := idiomFormOf(sentence.Tokens, material.MatchedTokenOrdinals); form.Surface != "" {
				introduced = form
			}
		}
	}
	byKey := make(map[string]idiomChoice, len(choices))
	candidates := make([]domain.FluencyCandidate, 0, len(choices))
	for _, choice := range choices {
		ref := choice.sentenceRef
		key := fmt.Sprintf("%s/%d/%d", ref.AnalysisRunID, ref.SentenceOrdinal, ref.TokenOrdinal)
		byKey[key] = choice
		candidates = append(candidates, domain.FluencyCandidate{Key: key, Form: choice.form, InCurrentReading: choice.inReading, Quality: choice.quality.GDEXScore + float64(choice.quality.Score)})
	}
	ordered := make([]idiomChoice, 0, len(choices))
	for _, candidate := range domain.RankFluencyCandidates(introduced, candidates) {
		ordered = append(ordered, byKey[candidate.Key])
	}
	return ordered
}

// assessIdiom asks the idiom-card request about one sentence and decides its
// agreement: the context agrees when the Gloss is confirmed and cites a sense
// of the Learned meaning's dictionary entry that the Learned meaning cites too.
// Literal use, a different sense and an answer that defends none are kept as
// disagreement with their reason.
func (w *FluencyWorker) assessIdiom(ctx context.Context, word domain.StudyProgress, headword string, choice idiomChoice) (fluencyAnswer, error) {
	meaning, err := w.Enricher.EnrichIdiomCard(ctx, enrichment.IdiomCard{
		Language: word.Language, Headword: headword, MatchedForms: choice.forms,
		Sentence: choice.sentence, Senses: boundedIdiomSenses(choice.senses),
	})
	if errors.Is(err, enrichment.ErrInvalidEvidenceCitation) {
		return idiomDisagreement(enrichment.FluencyReasonUndefendable, enrichment.ContextualMeaning{}, domain.StudyCitations{}), nil
	}
	if err != nil {
		return fluencyAnswer{}, err
	}
	cited := domain.StudyCitations{SenseIDs: meaning.CitedSenseIDs, EntryIDs: slices.Repeat([]string{choice.headword}, len(meaning.CitedSenseIDs)), DictionaryPin: choice.pin}
	switch {
	case meaning.Resolved() && strings.TrimSpace(meaning.SentenceTranslation) != "" && len(meaning.CitedSenseIDs) > 0:
		if !sharesCitedSense(word.LearnedCitations, cited) {
			return idiomDisagreement(enrichment.FluencyReasonDifferentMeaning, meaning, cited), nil
		}
		return idiomAnswer(enrichment.FluencyReasonSameMeaning, meaning, cited), nil
	case meaning.UnresolvedKind == enrichment.ContextualGlossUnresolvedLiteralUse:
		return idiomDisagreement(enrichment.FluencyReasonNotThisLemma, meaning, domain.StudyCitations{}), nil
	}
	return idiomDisagreement(enrichment.FluencyReasonUndefendable, meaning, domain.StudyCitations{}), nil
}

func idiomDisagreement(reason string, meaning enrichment.ContextualMeaning, cited domain.StudyCitations) fluencyAnswer {
	return idiomAnswer(reason, meaning, cited)
}

// idiomAnswer is the answer of one Idiom context: its reason with the Gloss and
// translation the provider gave, and the senses cited.
func idiomAnswer(reason string, meaning enrichment.ContextualMeaning, cited domain.StudyCitations) fluencyAnswer {
	answer := fluencyAnswer{Cited: cited}
	answer.Reason, answer.Gloss = reason, meaning.Gloss
	answer.SentenceTranslation, answer.SentenceTranslationTargets = meaning.SentenceTranslation, meaning.SentenceTranslationTargets
	return answer
}

// sharesCitedSense reports whether both citations are of the same dictionary
// revision and name at least one sense of the same entry in common.
func sharesCitedSense(learned, occurrence domain.StudyCitations) bool {
	if learned.DictionaryPin != occurrence.DictionaryPin || len(learned.EntryIDs) != len(learned.SenseIDs) || len(occurrence.EntryIDs) != len(occurrence.SenseIDs) {
		return false
	}
	for i, sense := range occurrence.SenseIDs {
		for j, other := range learned.SenseIDs {
			if sense == other && occurrence.EntryIDs[i] == learned.EntryIDs[j] {
				return true
			}
		}
	}
	return false
}

// idiomFormOf describes the form an Idiom's matched words take in a sentence:
// their surfaces in sentence order, and for each its morphological features in
// a fixed order and the gap before it, so a different inflection and a
// different arrangement both make a different form.
func idiomFormOf(tokens []analyzer.Token, matched []int) domain.FluencyForm {
	var surfaces, features []string
	previous := -1
	for _, ordinal := range matched {
		if ordinal < 0 || ordinal >= len(tokens) {
			continue
		}
		token := tokens[ordinal]
		surfaces = append(surfaces, token.Surface)
		gap := 0
		if previous >= 0 {
			gap = ordinal - previous - 1
		}
		features = append(features, fmt.Sprintf("gap=%d;%s", gap, fluencyFormOf(tokens, int64(ordinal)).Features))
		previous = ordinal
	}
	return domain.FluencyForm{Surface: strings.Join(surfaces, " "), Features: strings.Join(features, "|")}
}
