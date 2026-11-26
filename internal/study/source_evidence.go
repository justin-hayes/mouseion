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
}

// workSourceEvidence prepares a source-evidence word's material in the order
// ADR 0138 §1 gives: the payload gate, the dictionary entries, the Representative
// sentence and its passage, the evidence-assisted pass, then the outcome. A
// failed gate or a missing dictionary or provider waits; only an answer the
// evidence cannot defend is recorded as an omission, and a provider failure is
// returned for River to retry.
func (w *MaterialWorker) workSourceEvidence(ctx context.Context, owner string, material domain.StudyMaterial, policy cardpolicy.Policy) error {
	evidence := w.Evidence
	if evidence == nil || evidence.Lexical == nil || evidence.MaySend == nil {
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
	cited := citationsOf(resolution, meaning.CitedSenseIDs)
	if !meaning.Resolved() || strings.TrimSpace(meaning.SentenceTranslation) == "" || len(cited.SenseIDs) == 0 {
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

// passSenses are the pinned dictionary senses the pass may cite, each with its
// native sense ID as its evidence ID, across every resolved entry in source
// order.
func passSenses(policy cardpolicy.Policy, resolution sourceevidence.Resolution) []enrichment.LexicalSense {
	var senses []enrichment.LexicalSense
	for _, entry := range resolution.Entries {
		if policy.Entries == cardpolicy.EntryLSJ {
			senses = append(senses, sourceevidence.GreekPassSenses(entry.LSJ)...)
			continue
		}
		for _, sense := range entry.Senses {
			gloss := strings.Join(sense.Definitions, "; ")
			if strings.TrimSpace(sense.ID) != "" && strings.TrimSpace(gloss) != "" {
				senses = append(senses, enrichment.LexicalSense{EvidenceID: sense.ID, Source: "ls", Gloss: gloss})
			}
		}
	}
	return senses
}

// citationsOf names the entries the cited senses belong to, in the order the
// entries resolved, with the dictionary pin they were made against. A cited ID
// that no resolved entry holds is dropped, so a citation always names a real
// sense.
func citationsOf(resolution sourceevidence.Resolution, citedIDs []string) domain.StudyCitations {
	var cited domain.StudyCitations
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
