package study

import (
	"context"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sourceevidence"
)

// fluencyAnswer is what one assessed occurrence yields: the agreement outcome
// and, for a source-evidence language, the dictionary senses its Gloss cites.
type fluencyAnswer struct {
	enrichment.FluencyAgreement
	Cited domain.StudyCitations
}

// fluencyEvidence is what one source-evidence word's job resolved once: the
// entries of its lemma, the senses a pass may cite, and the Learned meaning's
// cited senses with their definitions.
type fluencyEvidence struct {
	source     *SourceEvidence
	policy     cardpolicy.Policy
	resolution sourceevidence.Resolution
	senses     []enrichment.LexicalSense
	learned    domain.StudyCitations
	// learnedSenses are the Learned meaning's cited senses, with definitions.
	learnedSenses []string
}

// sourceEvidence prepares a source-evidence word's evidence. It reports not ready,
// and nothing is sent, when the pass or the agreement variant is not recorded
// for the language, the dictionary or its source is missing, the lemma resolves
// to nothing, or the Learned meaning cites no sense to compare an occurrence
// against (ADR 0138 §4).
func (w *FluencyWorker) sourceEvidence(ctx context.Context, word domain.StudyProgress, policy cardpolicy.Policy) (evidence *fluencyEvidence, ready, wait bool) {
	source := w.Evidence
	if source == nil || source.MaySend == nil || w.MaySendAgreement == nil || word.LearnedCitations.Empty() ||
		source.MaySend(word.Language) != nil || w.MaySendAgreement(word.Language) != nil {
		return nil, false, false
	}
	if source.Lexical == nil || (source.Available != nil && !source.Available(policy)) {
		return nil, false, true
	}
	resolution, found := sourceevidence.ResolveStudy(ctx, source.Lexical, policy, word.CanonicalLemma, nil)
	if !found {
		return nil, false, true
	}
	senses := sourceevidence.PassSenses(policy, resolution.Entries)
	if len(senses) == 0 || strings.TrimSpace(resolution.Pin) == "" {
		return nil, false, true
	}
	learnedSenses := learnedSenseDefinitions(resolution, word.LearnedCitations)
	return &fluencyEvidence{source: source, policy: policy, resolution: resolution, senses: senses, learned: word.LearnedCitations, learnedSenses: learnedSenses}, true, false
}

// assessSourceEvidence gives one occurrence its own evidence-assisted pass, then
// decides its agreement (ADR 0138 §4): an occurrence whose Gloss cites nothing
// valid, or that abstains, is undefendable; one whose cited entries share none
// with the Learned meaning's, on the same pin, is a different meaning with no
// agreement call; every other goes to the agreement variant.
func (w *FluencyWorker) assessSourceEvidence(ctx context.Context, word domain.StudyProgress, choice sentenceChoice, candidate enrichment.Candidate, evidence *fluencyEvidence) (fluencyAnswer, error) {
	passage, err := sourceevidence.ResolvePassageOfBook(ctx, evidence.source.Translations, evidence.policy, word.OwnerID, choice.occurrence.BookID, choice.citations)
	if err != nil {
		return fluencyAnswer{}, err
	}
	candidate.CandidateSenses = evidence.senses
	meaning, err := w.Enricher.EnrichEvidenceAssistedPass(ctx, candidate, passage.Text)
	if err != nil {
		return fluencyAnswer{}, err
	}
	refs, validCitations := citedSenses(evidence.resolution, meaning.CitedSenseIDs)
	cited := citationsOf(evidence.resolution, refs)
	answer := fluencyAnswer{Cited: cited}
	answer.Reason, answer.Gloss = enrichment.FluencyReasonUndefendable, meaning.Gloss
	answer.SentenceTranslation, answer.SentenceTranslationTargets = meaning.SentenceTranslation, meaning.SentenceTranslationTargets
	if !validCitations || meaning.InvalidCitation || !meaning.Resolved() || len(cited.SenseIDs) == 0 {
		answer.Cited = domain.StudyCitations{}
		return answer, nil
	}
	if cited.DictionaryPin == evidence.learned.DictionaryPin && !sharesEntry(cited.EntrySet(), evidence.learned.EntrySet()) {
		answer.Reason = enrichment.FluencyReasonDifferentMeaning
		return answer, nil
	}
	agreement, err := w.Enricher.EnrichSourceEvidenceFluencyAgreement(ctx, candidate, word.LearnedMeaning,
		evidence.learnedSenses, citedSenseDefinitions(evidence.resolution, cited))
	if err != nil {
		return fluencyAnswer{}, err
	}
	// The stored Gloss and translation are the pass's: they are what the cited
	// senses belong to. The check decides only the reason.
	answer.Reason = agreement.Reason
	return answer, nil
}

// sharesEntry reports whether two sets of native entry IDs have one in common.
func sharesEntry(a, b []string) bool {
	return slices.ContainsFunc(a, func(id string) bool { return slices.Contains(b, id) })
}

// learnedSenseDefinitions are the definitions of the Learned meaning's cited
// senses. On the current pin they are the dictionary's; on another pin the same
// IDs may name other senses, so none is looked up and none is sent. Nothing is
// returned when the current pin holds none of them.
func learnedSenseDefinitions(resolution sourceevidence.Resolution, learned domain.StudyCitations) []string {
	if learned.DictionaryPin != resolution.Pin {
		return nil
	}
	return citedSenseDefinitions(resolution, learned)
}

// citedSenseDefinitions are the definitions of the cited (entry, sense) pairs
// the resolved entries hold, in the entries' order. A cited sense the entries no
// longer hold, because the dictionary moved on, has no definition to send and is
// left out. A sense ID another entry shares is never taken for the cited one.
func citedSenseDefinitions(resolution sourceevidence.Resolution, cited domain.StudyCitations) []string {
	var senses []string
	for _, entry := range resolution.Entries {
		for _, sense := range entry.Senses {
			// The pass sends a sense's glosses when it has no definitions, and
			// uncapped Study can cite such a sense: agreement sees the same text.
			text := sense.Definitions
			if len(text) == 0 {
				text = sense.Glosses
			}
			if definition := strings.Join(text, "; "); cited.Has(entry.ID, sense.ID) && strings.TrimSpace(definition) != "" {
				senses = append(senses, definition)
			}
		}
	}
	return senses
}
