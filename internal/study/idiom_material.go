package study

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// Idiom sentences are read in batches of idiomBatch, in order, until
// maxIdiomChoices usable ones are found or the candidate sentences run out, so
// no cap on the sentences read can make an Idiom with candidates unpreparable.
const (
	idiomBatch      = 100
	maxIdiomChoices = 30
)

// idiomChoicesOf returns the Idiom's usable sentences, best first, from any
// store of Idiom sentences. A choice
// that skip accepts is left out before it counts towards the bound, so Fluency
// can set aside the sentences it has no use for.
func idiomChoicesOf(ctx context.Context, store idiomSentenceStore, owner string, reader *idiomReader, entry idiom.Entry, retired []domain.StudyOccurrenceRef, skip func(idiomChoice) bool) ([]idiomChoice, error) {
	refs, err := store.ListStudyIdiomSentenceRefs(ctx, owner, reader.language, entry.Headword, entry.Words)
	if err != nil {
		return nil, err
	}
	var choices []idiomChoice
	for start := 0; start < len(refs) && len(choices) < maxIdiomChoices; start += idiomBatch {
		batch, err := store.LoadStudyIdiomSentences(ctx, owner, refs[start:min(start+idiomBatch, len(refs))])
		if err != nil {
			return nil, err
		}
		usable, err := reader.usableSentences(ctx, entry, batch, retired)
		if err != nil {
			return nil, err
		}
		for _, choice := range usable {
			if skip == nil || !skip(choice) {
				choices = append(choices, choice)
			}
		}
	}
	sort.SliceStable(choices, func(i, j int) bool {
		if choices[i].inReading != choices[j].inReading {
			return choices[i].inReading
		}
		return betterIdiomChoice(choices[i], choices[j])
	})
	return choices, nil
}

// workIdiom prepares one Idiom's Study material (ADR 0137 "Idiom Study
// material"): the best candidate sentence, the Current reading's first and
// never a retired one, goes to up to maxIdiomAttempts idiom-card requests, one per sentence,, and the confirmed Gloss,
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
		// The Idiom is a candidate only while the dictionary index lists it, so
		// this is the index changing under the job. It is recorded against the
		// index's version and tried again when that changes.
		return w.unpreparableNoDictionaryEntry(ctx, owner, material.ID, material.StudyItem, w.Idioms.IndexVersion)
	}
	retired, err := w.Store.ListRetiredStudyOccurrencesOfWord(ctx, owner, material.StudyItem)
	if err != nil {
		return err
	}
	choices, err := idiomChoicesOf(ctx, w.Store, owner, reader, entry, retired, nil)
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
		if errors.Is(err, enrichment.ErrInvalidEvidenceCitation) {
			reason = domain.StudyOmissionMeaning // an answer the evidence cannot defend
			continue
		}
		if err != nil {
			return idiomChoice{}, meaning, "", err
		}
		// A confirmed Gloss cites the dictionary sense it rests on (ADR 0137 §1).
		if meaning.Resolved() && strings.TrimSpace(meaning.SentenceTranslation) != "" && len(meaning.CitedSenseIDs) > 0 {
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
		Cited:                      domain.StudyCitations{SenseIDs: meaning.CitedSenseIDs, EntryIDs: slices.Repeat([]string{choice.headword}, len(meaning.CitedSenseIDs)), DictionaryPin: choice.pin},
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
	// pin is the dictionary entry's source and version, the pin its senses are cited against.
	pin string
	// headword names the dictionary entry the cited senses belong to.
	headword string
	// inReading is set for a sentence of the Current reading.
	inReading bool
	offset    int64
	// form is the form the Idiom's matched words take in the sentence, for Fluency.
	form domain.FluencyForm
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

// usableSentences returns the sentences where the matcher finds the headword on
// the effective lemmas, that are not retired and that pass the sentence-quality
// gate, in the order given.
func (r *idiomReader) usableSentences(ctx context.Context, entry idiom.Entry, sentences []persistence.StudyIdiomSentence, retired []domain.StudyOccurrenceRef) ([]idiomChoice, error) {
	var usable []idiomChoice
	for _, sentence := range sentences {
		ref := domain.StudyOccurrenceRef{BookID: sentence.BookID, AnalysisRunID: sentence.AnalysisRunID, CorpusID: sentence.CorpusID, SentenceOrdinal: sentence.SentenceOrdinal}
		if domain.RetiredStudyOccurrence(retired, ref) {
			continue
		}
		choice, ok, err := r.choose(ctx, entry, sentence, ref)
		if err != nil {
			return nil, err
		}
		if ok {
			usable = append(usable, choice)
		}
	}
	return usable, nil
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
		return idiomChoice{sentenceRef: ref, sentence: text, matched: matched, forms: forms, quality: quality, senses: candidate.Entry.Senses, pin: entryPin(candidate.Entry), headword: candidate.Entry.Headword, inReading: sentence.InCurrentReading, offset: sentence.BookStartOffset, form: idiomFormOf(sentence.Sentence.Tokens, matched)}, true, nil
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

// entryPin names the dictionary revision an entry's senses come from.
func entryPin(entry idiom.Entry) string {
	if pin := strings.Trim(entry.Source+"/"+entry.Version, "/"); pin != "" {
		return pin
	}
	return "idiom-entry"
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
