package study

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sourceevidence"
	"github.com/riverqueue/river"
)

// SourceEvidence is what a source-evidence language's Study material needs
// besides the provider (ADR 0138 §1).
type SourceEvidence struct {
	// Lexical is the imported dictionary. It is nil when none is configured, and
	// the word then waits: a missing dictionary is never a "no entry" answer.
	Lexical enrichment.LexicalProvider
	// Translations is the seam to the Book's imported passage translations.
	Translations sourceevidence.TranslationStore
	// MaySend reports whether the evidence-assisted pass payload may be sent for
	// a language: its provider payload assessment is recorded and matches the
	// codec (ADR 0133). While it fails, nothing is transmitted.
	MaySend func(language string) error
	// Available reports whether the configured dictionary holds the source the
	// policy names. A loaded index may lack it; the word then waits, because
	// "no entry" is only an answer from a dictionary that is present. Nil means
	// the dictionary is present.
	Available func(policy cardpolicy.Policy) bool
}

// workSourceEvidence prepares a source-evidence word's material in the order
// ADR 0138 §1 gives: the payload gate, the dictionary entries, the Representative
// sentence and its passage, the evidence-assisted pass, then the outcome. A
// failed gate or a missing dictionary or provider waits; only an answer the
// evidence cannot defend is recorded as an omission, and a provider failure is
// returned for River to retry.
func (w *MaterialWorker) workSourceEvidence(ctx context.Context, owner string, material domain.StudyMaterial, policy cardpolicy.Policy) error {
	evidence := w.Evidence
	if evidence == nil || evidence.Lexical == nil || evidence.MaySend == nil || (evidence.Available != nil && !evidence.Available(policy)) {
		return river.JobSnooze(notConfiguredDelay)
	}
	if err := evidence.MaySend(material.Language); err != nil {
		// Nothing is sent; the word waits until its assessment is recorded.
		return river.JobSnooze(notConfiguredDelay)
	}
	resolution, found := sourceevidence.Resolve(ctx, evidence.Lexical, policy, material.CanonicalLemma, nil)
	if !found {
		return w.unpreparable(ctx, owner, material.ID, material.StudyIdentity, domain.StudyOmissionNoDictionaryEntry)
	}
	senses := passSenses(policy, resolution)
	if len(senses) == 0 || strings.TrimSpace(resolution.Pin) == "" {
		// The entry has nothing the Gloss could cite.
		return w.unpreparable(ctx, owner, material.ID, material.StudyIdentity, domain.StudyOmissionMeaning)
	}
	choice, found, err := w.representativeSentence(ctx, owner, material.StudyIdentity)
	if err != nil {
		return err
	}
	if !found {
		return w.unpreparable(ctx, owner, material.ID, material.StudyIdentity, domain.StudyOmissionSentenceQuality)
	}
	passage, err := sourceevidence.ResolvePassageOfBook(ctx, evidence.Translations, policy, owner, choice.occurrence.BookID, choice.citations)
	if err != nil {
		return err
	}
	meaning, err := w.Enricher.EnrichEvidenceAssistedPass(ctx, enrichment.Candidate{
		Language: material.Language, CanonicalLemma: material.CanonicalLemma, UPOS: material.UPOS,
		TargetWord:      choice.target,
		ExampleSentence: choice.sentence,
		CandidateSenses: senses,
	}, passage.Text)
	if errors.Is(err, enrichment.ErrExternalNotConfigured) {
		return river.JobSnooze(notConfiguredDelay)
	}
	if err != nil {
		return err
	}
	citedIDs, citable, validCitations := nativeCitedIDs(policy, resolution, meaning.CitedSenseIDs)
	cited := citationsOf(citable, citedIDs)
	if !validCitations || meaning.InvalidCitation || !meaning.Resolved() || strings.TrimSpace(meaning.SentenceTranslation) == "" || len(cited.SenseIDs) == 0 {
		return w.unpreparable(ctx, owner, material.ID, material.StudyIdentity, domain.StudyOmissionMeaning)
	}
	return w.Store.RecordStudyMaterialReady(ctx, owner, material.ID, domain.StudyMaterialReadyContent{
		BookID: choice.occurrence.BookID, AnalysisRunID: choice.occurrence.AnalysisRunID, CorpusID: choice.occurrence.CorpusID,
		SentenceOrdinal: choice.occurrence.SentenceOrdinal, TokenOrdinal: choice.occurrence.TokenOrdinal,
		Sentence: choice.sentence, TargetWord: choice.target,
		Gloss: meaning.Gloss, SentenceTranslation: meaning.SentenceTranslation,
		SentenceTranslationTargets: meaning.SentenceTranslationTargets,
		Cited:                      cited,
		Passage:                    domain.StudyPassage{Text: passage.Text, Citation: passage.Citation, TranslationPin: passage.Pin},
	})
}

// passSenses are the pinned dictionary senses the pass may cite across every
// resolved entry in source order. A Greek sense's evidence ID is its native
// sense ID; a Latin sense's names its entry and its sense, so a cited sense
// resolves to exactly one of each across stacked homographs.
func passSenses(policy cardpolicy.Policy, resolution sourceevidence.Resolution) []enrichment.LexicalSense {
	if policy.Entries != cardpolicy.EntryLSJ {
		return sourceevidence.LatinPassSenses(resolution.Entries)
	}
	var senses []enrichment.LexicalSense
	for _, entry := range resolution.Entries {
		senses = append(senses, sourceevidence.GreekPassSenses(entry.LSJ)...)
	}
	return senses
}

// nativeCitedIDs turns the evidence IDs a pass answer cited into the native
// sense IDs the stored citations use, with the resolution those citations are
// made against. A Latin answer's IDs each name an entry and a sense, so the
// returned resolution holds only the cited senses of the entries that name them:
// a sense ID two stacked homographs share is recorded under the entry that was
// cited. An ID the resolved entries do not hold makes the answer invalid.
func nativeCitedIDs(policy cardpolicy.Policy, resolution sourceevidence.Resolution, cited []string) ([]string, sourceevidence.Resolution, bool) {
	if policy.Entries == cardpolicy.EntryLSJ {
		return cited, resolution, true
	}
	native, ok := sourceevidence.LatinCitedSenseIDs(resolution.Entries, cited)
	if !ok {
		return nil, resolution, false
	}
	narrowed := resolution
	narrowed.Entries = make([]sourceevidence.Entry, len(resolution.Entries))
	for i, entry := range resolution.Entries {
		narrowed.Entries[i] = entry
		narrowed.Entries[i].Senses = slices.DeleteFunc(slices.Clone(entry.Senses), func(sense sourceevidence.Sense) bool {
			return !slices.Contains(cited, enrichment.LatinEvidenceID(entry.ID, sense.ID))
		})
	}
	return native, narrowed, true
}

// citationsOf names the entries the cited senses belong to, in the order the
// entries resolved, with the dictionary pin they were made against. A cited ID
// that no resolved entry holds makes the citations empty: the Gloss must cite
// only senses that exist (ADR 0138 §2).
func citationsOf(resolution sourceevidence.Resolution, citedIDs []string) domain.StudyCitations {
	var cited domain.StudyCitations
	if !allSensesExist(resolution, citedIDs) {
		return cited
	}
	for _, entry := range resolution.Entries {
		held := false
		for _, sense := range entry.Senses {
			if slices.Contains(citedIDs, sense.ID) && !slices.Contains(cited.SenseIDs, sense.ID) {
				cited.SenseIDs = append(cited.SenseIDs, sense.ID)
				held = true
			}
		}
		if held {
			cited.EntryIDs = append(cited.EntryIDs, entry.ID)
		}
	}
	if len(cited.SenseIDs) > 0 {
		cited.DictionaryPin = resolution.Pin
	}
	return cited
}

// allSensesExist reports whether every cited ID is a sense of a resolved entry.
func allSensesExist(resolution sourceevidence.Resolution, citedIDs []string) bool {
	known := map[string]bool{}
	for _, entry := range resolution.Entries {
		for _, sense := range entry.Senses {
			known[sense.ID] = true
		}
	}
	return !slices.ContainsFunc(citedIDs, func(id string) bool { return !known[id] })
}
