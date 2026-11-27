package study

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/justin-hayes/mouseion/internal/textmatch"
	"github.com/riverqueue/river"
)

// workIdiom prepares one Idiom's Study material (ADR 0137 "Idiom Study
// material"): the best candidate sentence, the Current reading's first and
// never a retired one, goes to one idiom-card request, and the confirmed Gloss,
// the sentence translation and the matched words are stored. An Idiom with no
// usable sentence, or whose sentence uses its words literally or has no
// defensible sense, is unpreparable with its reason. A missing provider or a
// provider failure is never recorded: the job waits or fails and River retries
// it. With the language's switch off nothing is prepared and the record stays
// pending.
func (w *MaterialWorker) workIdiom(ctx context.Context, owner string, material domain.StudyMaterial) error {
	headword, _ := material.Headword()
	reader := w.Idioms.reader(material.Language)
	if reader == nil {
		return nil
	}
	entry, found, err := reader.entry(ctx, headword)
	if err != nil {
		return err
	}
	if !found {
		return w.unpreparable(ctx, owner, material.ID, material.StudyItem, domain.StudyOmissionSentenceQuality)
	}
	retired, err := w.Store.ListRetiredStudyOccurrencesOfWord(ctx, owner, material.StudyItem)
	if err != nil {
		return err
	}
	sentences, err := w.Store.ListStudyIdiomSentences(ctx, owner, material.Language, entry.Words)
	if err != nil {
		return err
	}
	choices, err := reader.rankedSentences(ctx, entry, sentences, retired)
	if err != nil {
		return err
	}
	if len(choices) == 0 {
		return w.unpreparable(ctx, owner, material.ID, material.StudyItem, domain.StudyOmissionSentenceQuality)
	}
	choice, meaning, reason, err := w.confirmIdiom(ctx, material, choices)
	if err != nil {
		return err
	}
	if reason != "" {
		return w.unpreparable(ctx, owner, material.ID, material.StudyItem, reason)
	}
	return w.recordIdiomReady(ctx, owner, material.ID, choice, meaning)
}

// confirmIdiom asks the provider about each candidate sentence, best first,
// until one confirms the Idiom. When none does it returns the omission reason
// of the last answer: literal use or no defensible meaning. The Idiom is given
// up on only when every sentence tried came back literal or ambiguous
// (bounded by maxIdiomAttempts).
func (w *MaterialWorker) confirmIdiom(ctx context.Context, material domain.StudyMaterial, choices []idiomChoice) (idiomChoice, enrichment.ContextualMeaning, string, error) {
	headword, _ := material.Headword()
	reason := domain.StudyOmissionMeaning
	for _, choice := range choices[:min(len(choices), maxIdiomAttempts)] {
		meaning, err := w.Enricher.EnrichIdiomCard(ctx, enrichment.IdiomCard{
			Language: material.Language, Headword: headword, MatchedForms: choice.forms,
			Sentence: choice.sentence, Senses: boundedIdiomSenses(choice.senses),
		})
		if errors.Is(err, enrichment.ErrExternalNotConfigured) {
			return idiomChoice{}, meaning, "", river.JobSnooze(notConfiguredDelay)
		}
		if err != nil {
			return idiomChoice{}, meaning, "", err
		}
		if meaning.Resolved() && strings.TrimSpace(meaning.SentenceTranslation) != "" {
			return choice, meaning, "", nil
		}
		reason = domain.StudyOmissionMeaning
		if meaning.UnresolvedKind == enrichment.ContextualGlossUnresolvedLiteralUse {
			reason = domain.StudyOmissionLiteralUse
		}
	}
	return idiomChoice{}, enrichment.ContextualMeaning{}, reason, nil
}

// maxIdiomAttempts bounds how many candidate sentences one Idiom's preparation
// sends before giving up on it.
const maxIdiomAttempts = 3

func (w *MaterialWorker) recordIdiomReady(ctx context.Context, owner, id string, choice idiomChoice, meaning enrichment.ContextualMeaning) error {
	return w.Store.RecordStudyMaterialReady(ctx, owner, id, domain.StudyMaterialReadyContent{
		BookID: choice.sentenceRef.BookID, AnalysisRunID: choice.sentenceRef.AnalysisRunID, CorpusID: choice.sentenceRef.CorpusID,
		SentenceOrdinal: choice.sentenceRef.SentenceOrdinal, TokenOrdinal: int64(choice.matched[0]),
		Sentence: choice.sentence, TargetWord: strings.Join(choice.forms, " "),
		Gloss: meaning.Gloss, SentenceTranslation: meaning.SentenceTranslation,
		SentenceTranslationTargets: meaning.SentenceTranslationTargets,
		MatchedTokenOrdinals:       choice.matched,
	})
}

// boundedIdiomSenses is the entry's senses the request may carry, as a deck's
// idiom request carries them.
func boundedIdiomSenses(senses []enrichment.LexicalSense) []enrichment.LexicalSense {
	if len(senses) > enrichment.DefaultMaxCandidateSenses {
		senses = senses[:enrichment.DefaultMaxCandidateSenses]
	}
	return enrichment.CloneLexicalSenses(senses)
}

// idiomChoice is the sentence chosen for an Idiom.
type idiomChoice struct {
	sentenceRef domain.StudyOccurrenceRef
	sentence    string
	// matched are the matched token ordinals in sentence order, forms their
	// surface forms.
	matched []int
	forms   []string
	quality sentencequality.SentenceQuality
	// senses are the matched entry's senses (every part of speech pooled).
	senses []enrichment.LexicalSense
	// inReading is set for a sentence of the Current reading.
	inReading bool
	offset    int64
}

// entry finds the dictionary entry of the headword by looking up its words.
func (r *idiomReader) entry(ctx context.Context, headword string) (idiom.Entry, bool, error) {
	for word := range strings.FieldsSeq(headword) {
		entries, err := r.index.IdiomEntries(ctx, r.language, word)
		if err != nil {
			return idiom.Entry{}, false, fmt.Errorf("study: idiom entries: %w", err)
		}
		for _, entry := range entries {
			if entry.Headword == headword {
				return entry, true, nil
			}
		}
	}
	return idiom.Entry{}, false, nil
}

// rankedSentences returns the usable sentences of the Idiom, best first: those
// where the matcher finds the headword on the effective lemmas, that are not
// retired and that pass the sentence-quality gate. The Current reading's come
// first; within each group the best GDEX score, quality score and source
// position win, so the order is deterministic.
func (r *idiomReader) rankedSentences(ctx context.Context, entry idiom.Entry, sentences []persistence.StudyIdiomSentence, retired []domain.StudyOccurrenceRef) ([]idiomChoice, error) {
	var inReading, elsewhere []idiomChoice
	for _, sentence := range sentences {
		ref := domain.StudyOccurrenceRef{BookID: sentence.BookID, AnalysisRunID: sentence.AnalysisRunID, CorpusID: sentence.CorpusID, SentenceOrdinal: sentence.SentenceOrdinal}
		if domain.RetiredStudyOccurrence(retired, ref) {
			continue
		}
		choice, ok, err := r.choose(ctx, entry, sentence, ref)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if sentence.InCurrentReading {
			inReading = append(inReading, choice)
		} else {
			elsewhere = append(elsewhere, choice)
		}
	}
	for _, group := range [][]idiomChoice{inReading, elsewhere} {
		sort.SliceStable(group, func(i, j int) bool { return betterIdiomChoice(group[i], group[j]) })
	}
	return append(inReading, elsewhere...), nil
}

// choose reads the sentence's Idiom candidates on its effective tokens and
// scores the one of the entry's headword.
func (r *idiomReader) choose(ctx context.Context, entry idiom.Entry, sentence persistence.StudyIdiomSentence, ref domain.StudyOccurrenceRef) (idiomChoice, bool, error) {
	candidates, err := idiom.CandidatesInSentence(ctx, r.index, r.avoidance.Parameters, r.language, sentence.Effective)
	if err != nil {
		return idiomChoice{}, false, fmt.Errorf("study: idiom candidates: %w", err)
	}
	for _, candidate := range candidates {
		if candidate.Entry.Headword != entry.Headword {
			continue
		}
		forms, matched := matchedForms(sentence.Sentence.Tokens, candidate.MatchedTokenOrdinals)
		if len(forms) == 0 {
			return idiomChoice{}, false, nil
		}
		text := strings.TrimSpace(sentence.Sentence.Text)
		quality := sentencequality.ScoreSentenceQuality(r.language, text, sentence.Sentence.Tokens[matched[0]].Surface, sentence.BookStartOffset, sentence.Sentence.Tokens, sentence.Sentence.Location.StartOffset)
		if !quality.Accepted {
			return idiomChoice{}, false, nil
		}
		ref.TokenOrdinal = int64(matched[0])
		return idiomChoice{sentenceRef: ref, sentence: text, matched: matched, forms: forms, quality: quality, senses: candidate.Entry.Senses, inReading: sentence.InCurrentReading, offset: sentence.BookStartOffset}, true, nil
	}
	return idiomChoice{}, false, nil
}

// matchedForms are the surface forms of the matched tokens, in sentence order,
// with the ordinals they stand at. A token the sentence does not have is
// skipped.
func matchedForms(tokens []analyzer.Token, ordinals []int) ([]string, []int) {
	forms := make([]string, 0, len(ordinals))
	kept := make([]int, 0, len(ordinals))
	for _, ordinal := range ordinals {
		if ordinal < 0 || ordinal >= len(tokens) {
			continue
		}
		if form := textmatch.CleanLexicalSurface(tokens[ordinal].Surface); form != "" {
			forms, kept = append(forms, form), append(kept, ordinal)
		}
	}
	return forms, kept
}

// betterIdiomChoice orders by descending GDEX score, descending quality
// score, then source position.
func betterIdiomChoice(a, b idiomChoice) bool {
	switch {
	case a.quality.GDEXScore != b.quality.GDEXScore:
		return a.quality.GDEXScore > b.quality.GDEXScore
	case a.quality.Score != b.quality.Score:
		return a.quality.Score > b.quality.Score
	case a.offset != b.offset:
		return a.offset < b.offset
	default:
		return a.sentenceRef.SentenceOrdinal < b.sentenceRef.SentenceOrdinal
	}
}
