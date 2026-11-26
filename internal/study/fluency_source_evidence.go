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
	learnedSenses []enrichment.FluencySense
}

// sourceEvidence prepares a source-evidence word's evidence. It reports false,
// and nothing is sent, when the pass or the agreement variant is not recorded
// for the language, the dictionary or its source is missing, the lemma resolves
// to nothing, or the Learned meaning cites no sense to compare an occurrence
// against (ADR 0138 §4).
func (w *FluencyWorker) sourceEvidence(ctx context.Context, word domain.StudyProgress, policy cardpolicy.Policy) (*fluencyEvidence, bool) {
	source := w.Evidence
	if source == nil || source.Lexical == nil || source.MaySend == nil || w.MaySendAgreement == nil ||
		(source.Available != nil && !source.Available(policy)) || word.LearnedCitations.Empty() {
		return nil, false
	}
	if source.MaySend(word.Language) != nil || w.MaySendAgreement(word.Language) != nil {
		return nil, false
	}
	resolution, found := sourceevidence.Resolve(ctx, source.Lexical, policy, word.CanonicalLemma, nil)
	if !found {
		return nil, false
	}
	senses := passSenses(policy, resolution)
	if len(senses) == 0 || strings.TrimSpace(resolution.Pin) == "" {
		return nil, false
	}
	learnedSenses := citedSenseDefinitions(resolution, word.LearnedCitations.SenseIDs)
	if len(learnedSenses) == 0 {
		// The dictionary no longer holds the senses the Learned meaning cited;
		// there is nothing to compare an occurrence against.
		return nil, false
	}
	return &fluencyEvidence{source: source, policy: policy, resolution: resolution, senses: senses, learned: word.LearnedCitations, learnedSenses: learnedSenses}, true
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
	cited := citationsOf(evidence.resolution, meaning.CitedSenseIDs)
	answer := fluencyAnswer{Cited: cited}
	answer.Reason, answer.Gloss = enrichment.FluencyReasonUndefendable, meaning.Gloss
	answer.SentenceTranslation, answer.SentenceTranslationTargets = meaning.SentenceTranslation, meaning.SentenceTranslationTargets
	if meaning.InvalidCitation || !meaning.Resolved() || len(cited.SenseIDs) == 0 {
		answer.Cited = domain.StudyCitations{}
		return answer, nil
	}
	if cited.DictionaryPin == evidence.learned.DictionaryPin && !sharesEntry(cited.EntryIDs, evidence.learned.EntryIDs) {
		answer.Reason = enrichment.FluencyReasonDifferentMeaning
		return answer, nil
	}
	agreement, err := w.Enricher.EnrichSourceEvidenceFluencyAgreement(ctx, candidate, word.LearnedMeaning,
		evidence.learnedSenses, citedSenseDefinitions(evidence.resolution, cited.SenseIDs))
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

// citedSenseDefinitions are the cited senses the resolved entries hold, with
// their definitions, in the entries' order. A cited sense the entries no longer
// hold, because the dictionary moved on, has no definition to send and is left
// out.
func citedSenseDefinitions(resolution sourceevidence.Resolution, ids []string) []enrichment.FluencySense {
	var senses []enrichment.FluencySense
	for _, entry := range resolution.Entries {
		for _, sense := range entry.Senses {
			if definition := strings.Join(sense.Definitions, "; "); slices.Contains(ids, sense.ID) && strings.TrimSpace(definition) != "" {
				senses = append(senses, enrichment.FluencySense{ID: sense.ID, Definition: definition})
			}
		}
	}
	return senses
}
