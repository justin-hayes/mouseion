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

// DictionaryPin is the pinned revision of the dictionary the policy names, read
// from the loaded index, or empty when there is no dictionary or it does not say.
// A no-dictionary-entry omission is recorded against it, and Study preparation
// tries the word again when it changes.
func DictionaryPin(lexical enrichment.LexicalProvider, policy cardpolicy.Policy) string {
	switch policy.Entries {
	case cardpolicy.EntryLSJ:
		if pinned, ok := lexical.(interface{ AncientGreekLSJPin() string }); ok {
			return strings.TrimSpace(pinned.AncientGreekLSJPin())
		}
	case cardpolicy.EntryLewisShort:
		if pinned, ok := lexical.(interface{ LatinLSPin() string }); ok {
			return strings.TrimSpace(pinned.LatinLSPin())
		}
	case cardpolicy.EntryNone:
	}
	return ""
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
	resolution, found := sourceevidence.ResolveStudy(ctx, evidence.Lexical, policy, material.CanonicalLemma, nil)
	if !found {
		return w.unpreparableNoDictionaryEntry(ctx, owner, material.ID, material.StudyIdentity, DictionaryPin(evidence.Lexical, policy))
	}
	senses := sourceevidence.PassSenses(policy, resolution.Entries)
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
	refs, validCitations := citedSenses(resolution, meaning.CitedSenseIDs)
	cited := citationsOf(resolution, refs)
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

// citedSenses resolves the evidence IDs a pass answer cited to the (entry,
// sense) pairs they name, in the order cited, with no repeats. Every ID names an
// entry and a sense, so a sense ID two homograph entries share resolves to the
// entry that was cited. It reports false when a cited ID names no sense the
// resolved entries hold (ADR 0138 §2).
func citedSenses(resolution sourceevidence.Resolution, cited []string) ([]sourceevidence.SenseRef, bool) {
	refs := make([]sourceevidence.SenseRef, 0, len(cited))
	for _, id := range cited {
		entryID, senseID, ok := enrichment.SplitEntryEvidenceID(id)
		if !ok || !holdsSense(resolution, entryID, senseID) {
			return nil, false
		}
		if ref := (sourceevidence.SenseRef{EntryID: entryID, SenseID: senseID}); !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	return refs, true
}

// holdsSense reports whether a resolved entry holds the sense.
func holdsSense(resolution sourceevidence.Resolution, entryID, senseID string) bool {
	return slices.ContainsFunc(resolution.Entries, func(entry sourceevidence.Entry) bool {
		return entry.ID == entryID && slices.ContainsFunc(entry.Senses, func(sense sourceevidence.Sense) bool { return sense.ID == senseID })
	})
}

// citationsOf stores the cited senses as parallel sense and entry IDs, in the
// order the entries and senses resolved, with the dictionary pin they were made
// against. An empty cited set gives empty citations.
func citationsOf(resolution sourceevidence.Resolution, refs []sourceevidence.SenseRef) domain.StudyCitations {
	var cited domain.StudyCitations
	for _, entry := range resolution.Entries {
		for _, sense := range entry.Senses {
			ref := sourceevidence.SenseRef{EntryID: entry.ID, SenseID: sense.ID}
			if slices.Contains(refs, ref) && !cited.Has(entry.ID, sense.ID) {
				cited.SenseIDs = append(cited.SenseIDs, sense.ID)
				cited.EntryIDs = append(cited.EntryIDs, entry.ID)
			}
		}
	}
	if len(cited.SenseIDs) > 0 {
		cited.DictionaryPin = resolution.Pin
	}
	return cited
}
