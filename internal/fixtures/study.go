package fixtures

// Contract status: contractual. Study candidate and Study material
// transitions must match internal/storecontract scenarios shared with
// PostgreSQL (ADR 0088).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListStudyCandidates derives the Study stream's candidates from the analyzed
// Books' seeded occurrences, applying the learner's lemma corrections and
// exclusions. Known and Reserved vocabulary are not consulted.
func (s *Store) ListStudyCandidates(_ context.Context, owner, language string) ([]domain.StudyCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.currentReadings[fixtureGoalKey(owner, language)]
	tallies := map[domain.StudyItem]*domain.StudyCandidate{}
	for _, book := range s.books {
		if book.Source.OwnerID != owner || book.Source.Language != language || book.BookID == "" {
			continue
		}
		inReading := current.IsActive() && current.BookID == book.BookID
		for _, occurrence := range s.lemmaOccurrences[lemmaOccurrenceKey(owner, book.BookID)] {
			identity, counts := s.effectiveStudyIdentityLocked(owner, book.BookID, language, occurrence)
			if !counts {
				continue
			}
			entry := tallies[identity]
			if entry == nil {
				entry = &domain.StudyCandidate{StudyItem: identity}
				tallies[identity] = entry
			}
			entry.PersonalOccurrences++
			entry.InCurrentReading = entry.InCurrentReading || inReading
		}
	}
	candidates := make([]domain.StudyCandidate, 0, len(tallies)+len(s.studyCandidates))
	for _, entry := range tallies {
		candidates = append(candidates, *entry)
	}
	return append(candidates, s.seededStudyCandidatesLocked(owner, language, tallies)...), nil
}

// ListStudyCandidatesAmong is ListStudyCandidates for the given identities only.
func (s *Store) ListStudyCandidatesAmong(ctx context.Context, owner, language string, identities []domain.StudyItem) ([]domain.StudyCandidate, error) {
	all, err := s.ListStudyCandidates(ctx, owner, language)
	if err != nil {
		return nil, err
	}
	wanted := make(map[domain.StudyItem]bool, len(identities))
	for _, identity := range identities {
		identity.Language = language
		wanted[identity] = true
	}
	var among []domain.StudyCandidate
	for _, candidate := range all {
		if wanted[candidate.StudyItem] {
			among = append(among, candidate)
		}
	}
	return among, nil
}

// seededStudyCandidatesLocked returns the seeded candidates the Books do not
// already contribute. The caller holds s.mu.
func (s *Store) seededStudyCandidatesLocked(owner, language string, tallies map[domain.StudyItem]*domain.StudyCandidate) []domain.StudyCandidate {
	var seeded []domain.StudyCandidate
	for _, item := range s.studyCandidates {
		if item.owner == owner && item.Language == language && tallies[item.StudyItem] == nil {
			seeded = append(seeded, item.StudyCandidate)
		}
	}
	return seeded
}

type seededStudyCandidate struct {
	owner string
	domain.StudyCandidate
}

// SeedStudyCandidate adds a word to the learner's Study stream without an
// analyzed Book behind it, so a browser fixture can offer a batch of several
// words. A word the Books already contribute is left to them.
func (s *Store) SeedStudyCandidate(owner string, candidate domain.StudyCandidate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.studyCandidates = append(s.studyCandidates, seededStudyCandidate{owner: owner, StudyCandidate: candidate})
}

// effectiveStudyIdentityLocked applies the learner's decision to one occurrence:
// an exclusion removes it and a correction replaces its lemma. The caller holds
// s.mu.
func (s *Store) effectiveStudyIdentityLocked(owner, bookID, language string, occurrence domain.LemmaReviewOccurrence) (domain.StudyItem, bool) {
	key := fixtureLemmaCorrectionKey(owner, bookID, occurrence.AnalysisRunID, occurrence.StartOffset, occurrence.EndOffset)
	if s.lemmaExclusions[key] {
		return domain.StudyItem{}, false
	}
	identity := domain.StudyItem{Language: language, CanonicalLemma: occurrence.CanonicalLemma, UPOS: strings.ToUpper(occurrence.UPOS), LexicalDiscriminator: occurrence.LexicalDiscriminator}
	if corrected := s.lemmaCorrections[key]; corrected != "" {
		identity.CanonicalLemma, identity.LexicalDiscriminator = corrected, ""
	}
	return identity, true
}

// prepareReintroductions prepares again the material of a word in study whose
// material a correction replaced, in place of the occurrence that no longer
// resolves.
func (s Study) prepareReintroductions(ctx context.Context, owner string, progress []domain.StudyProgress) error {
	for _, item := range progress {
		if item.Withdrawn {
			continue
		}
		id, created, err := s.Store.ReserveStudyMaterial(ctx, owner, item.StudyItem)
		if err != nil {
			return err
		}
		if created {
			if err := s.Store.RecordStudyMaterialReady(ctx, owner, id, s.Store.fixtureStudyContent(owner, item.StudyItem)); err != nil {
				return err
			}
		}
	}
	return nil
}

// fixtureStudyDefaultSource is the occurrence of a word the browser fixtures
// seed without an analyzed Book behind it.
var fixtureStudyDefaultSource = domain.StudyOccurrenceRef{BookID: BookID, AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", SentenceOrdinal: 1, TokenOrdinal: 2}

// ListCurrentStudyMaterial returns every word's current Study material.
func (s *Store) ListCurrentStudyMaterial(_ context.Context, owner, language string) ([]domain.StudyMaterial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	material := make([]domain.StudyMaterial, 0, len(s.studyMaterial))
	for _, item := range s.studyMaterial {
		if item.OwnerID == owner && item.Language == language && !s.studySuperseded[item.ID] {
			material = append(material, cloneStudyMaterial(item))
		}
	}
	return material, nil
}

// GetStudyMaterial returns one Study material record.
func (s *Store) GetStudyMaterial(_ context.Context, owner, id string) (domain.StudyMaterial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.studyMaterial {
		if item.OwnerID == owner && item.ID == id {
			return cloneStudyMaterial(item), nil
		}
	}
	return domain.StudyMaterial{}, errNotFound
}

// CountStudyMaterial reports the ready, preparing and unpreparable words.
func (s *Store) CountStudyMaterial(_ context.Context, owner, language string) (domain.StudyMaterialCounts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var counts domain.StudyMaterialCounts
	for _, item := range s.studyMaterial {
		if item.OwnerID != owner || item.Language != language || s.studySuperseded[item.ID] {
			continue
		}
		switch item.State {
		case domain.StudyMaterialReady:
			counts.Ready++
		case domain.StudyMaterialPending:
			counts.Preparing++
		case domain.StudyMaterialUnpreparable:
			counts.Unpreparable++
		}
	}
	return counts, nil
}

// ReserveStudyMaterial starts a word's material as pending, reporting false
// when the word already has current material.
func (s *Store) ReserveStudyMaterial(_ context.Context, owner string, identity domain.StudyItem) (string, bool, error) {
	identity.CanonicalLemma = strings.TrimSpace(identity.CanonicalLemma)
	identity.UPOS = strings.ToUpper(strings.TrimSpace(identity.UPOS))
	identity.LexicalDiscriminator = strings.TrimSpace(identity.LexicalDiscriminator)
	identity.Language = strings.ToLower(strings.TrimSpace(identity.Language))
	if identity.Language == "" || identity.CanonicalLemma == "" || (identity.UPOS == "") != identity.IsIdiom() || (identity.IsIdiom() && identity.LexicalDiscriminator != "") {
		return "", false, domain.ErrInvalidStudyMaterial
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.studyMaterial {
		if item.OwnerID == owner && item.StudyItem == identity && !s.studySuperseded[item.ID] {
			return "", false, nil
		}
	}
	id := fmt.Sprintf("fixture-study-material-%d", len(s.studyMaterial)+1)
	now := time.Now().UTC()
	s.studyMaterial = append(s.studyMaterial, domain.StudyMaterial{ID: id, OwnerID: owner, StudyItem: identity, State: domain.StudyMaterialPending, CreatedAt: now, UpdatedAt: now})
	return id, true, nil
}

// RecordStudyMaterialReady stores a pending word's prepared material.
func (s *Store) RecordStudyMaterialReady(_ context.Context, owner, id string, content domain.StudyMaterialReadyContent) error {
	if !content.Complete() {
		return domain.ErrInvalidStudyMaterial
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyMaterial {
		item := &s.studyMaterial[i]
		if item.OwnerID != owner || item.ID != id || item.State != domain.StudyMaterialPending {
			continue
		}
		item.State = domain.StudyMaterialReady
		item.BookID, item.AnalysisRunID, item.CorpusID = content.BookID, content.AnalysisRunID, content.CorpusID
		item.SentenceOrdinal, item.TokenOrdinal = content.SentenceOrdinal, content.TokenOrdinal
		item.Sentence, item.TargetWord, item.Gloss = content.Sentence, content.TargetWord, content.Gloss
		item.SentenceTranslation = content.SentenceTranslation
		item.SentenceTranslationTargets = append([]string{}, content.SentenceTranslationTargets...)
		item.Cited, item.Passage = content.Cited.Clone(), content.Passage
		item.UpdatedAt = time.Now().UTC()
		return nil
	}
	return errNotFound
}

// RecordStudyMaterialUnpreparable marks a pending word unpreparable.
func (s *Store) RecordStudyMaterialUnpreparable(_ context.Context, owner, id, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return domain.ErrInvalidStudyMaterial
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyMaterial {
		item := &s.studyMaterial[i]
		if item.OwnerID != owner || item.ID != id || item.State != domain.StudyMaterialPending {
			continue
		}
		item.State, item.OmissionReason, item.UpdatedAt = domain.StudyMaterialUnpreparable, strings.TrimSpace(reason), time.Now().UTC()
		return nil
	}
	return errNotFound
}

// RecordStudyMaterialNoDictionaryEntry marks a pending word unpreparable for
// having no dictionary entry, with the pin of the dictionary that was asked.
func (s *Store) RecordStudyMaterialNoDictionaryEntry(_ context.Context, owner, id, pin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyMaterial {
		item := &s.studyMaterial[i]
		if item.OwnerID != owner || item.ID != id || item.State != domain.StudyMaterialPending {
			continue
		}
		item.State, item.OmissionReason, item.OmissionDictionaryPin = domain.StudyMaterialUnpreparable, domain.StudyOmissionNoDictionaryEntry, strings.TrimSpace(pin)
		item.UpdatedAt = time.Now().UTC()
		return nil
	}
	return errNotFound
}

// ReopenStudyMaterialNoDictionaryEntry makes a no-dictionary-entry omission
// pending again when pin is not the pin it was recorded against.
func (s *Store) ReopenStudyMaterialNoDictionaryEntry(_ context.Context, owner, id, pin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyMaterial {
		item := &s.studyMaterial[i]
		if item.OwnerID != owner || item.ID != id || s.studySuperseded[item.ID] || !item.RetriesAgainstDictionary(pin) {
			continue
		}
		item.State, item.OmissionReason, item.OmissionDictionaryPin, item.EvidenceFingerprint = domain.StudyMaterialPending, "", "", ""
		item.UpdatedAt = time.Now().UTC()
		return nil
	}
	return errNotFound
}

func cloneStudyMaterial(item domain.StudyMaterial) domain.StudyMaterial {
	item.SentenceTranslationTargets = append([]string{}, item.SentenceTranslationTargets...)
	item.Cited = item.Cited.Clone()
	return item
}

// Study is the fixture server's Study preparation seam. It stands in for the
// study job package: the fixture server binds no provider or River, so it
// prepares each word's material at once from the word itself.
type Study struct{ Store *Store }

// EnqueueStudyPreparation prepares material for the next words in stream order
// that are not yet in study.
func (s Study) EnqueueStudyPreparation(ctx context.Context, owner, language string) error {
	candidates, err := s.Store.ListStudyCandidates(ctx, owner, language)
	if err != nil {
		return err
	}
	progress, err := s.Store.ListStudyProgress(ctx, owner, language)
	if err != nil {
		return err
	}
	material, err := s.Store.ListCurrentStudyMaterial(ctx, owner, language)
	if err != nil {
		return err
	}
	states := make(map[domain.StudyItem]domain.StudyMaterialState, len(material))
	for _, item := range material {
		states[item.StudyItem] = item.State
	}
	reports, err := s.Store.ListOpenStudyReports(ctx, owner, language, domain.StudyReportWrongWord)
	if err != nil {
		return err
	}
	ordered := domain.OrderStudyStream(domain.WithoutStudyProgress(candidates, progress, domain.StudyHeldIdentities(reports)))
	for _, target := range domain.StudyPreparationTargets(ordered, states) {
		id, created, err := s.Store.ReserveStudyMaterial(ctx, owner, target.StudyItem)
		if err != nil {
			return err
		}
		if created {
			if err := s.Store.RecordStudyMaterialReady(ctx, owner, id, s.Store.fixtureStudyContent(owner, target.StudyItem)); err != nil {
				return err
			}
		}
	}
	return s.prepareReintroductions(ctx, owner, progress)
}

// EnqueueFluencyPreparation prepares nothing: the fixture server seeds a mature
// word with its Fluency contexts up front, since it binds no provider or River.
func (s Study) EnqueueFluencyPreparation(context.Context, string, string) error { return nil }

// IdiomsActive is false: the fixture server has no idiom switch.
func (s Study) IdiomsActive(string) bool { return false }

// IdiomCandidates offers no Idiom: the fixture server has no idiom switch.
func (s Study) IdiomCandidates(context.Context, string, string) ([]domain.StudyCandidate, error) {
	return nil, nil
}

// ReviewIdiom changes nothing: the fixture server has no Idioms in study.
func (s Study) ReviewIdiom(context.Context, string, domain.StudyProgress, time.Time) (bool, error) {
	return false, nil
}

// fixtureStudyContent is the Study material the fixture server shows for a
// word: a short sentence around its lemma, taken from the first occurrence
// that resolves to the word under current evidence.
func (s *Store) fixtureStudyContent(owner string, identity domain.StudyItem) domain.StudyMaterialReadyContent {
	source := fixtureStudyDefaultSource
	if refs, err := s.ListStudyOccurrences(context.Background(), owner, identity); err == nil && len(refs) > 0 {
		source = refs[0]
	}
	// The target word is the form the occurrence has, so Lemma review finds it.
	target := identity.CanonicalLemma
	s.mu.Lock()
	for _, occurrence := range s.lemmaOccurrences[lemmaOccurrenceKey(owner, source.BookID)] {
		if occurrence.AnalysisRunID == source.AnalysisRunID && occurrence.SentenceOrdinal == source.SentenceOrdinal && occurrence.TokenOrdinal == source.TokenOrdinal {
			target = occurrence.Surface
		}
	}
	s.mu.Unlock()
	return domain.StudyMaterialReadyContent{
		BookID: source.BookID, AnalysisRunID: source.AnalysisRunID, CorpusID: source.CorpusID,
		SentenceOrdinal: source.SentenceOrdinal, TokenOrdinal: source.TokenOrdinal,
		Sentence:                   "Hier steht " + target + " im Satz.",
		TargetWord:                 target,
		Gloss:                      "the meaning of " + identity.CanonicalLemma,
		SentenceTranslation:        "Here " + identity.CanonicalLemma + " stands in the sentence.",
		SentenceTranslationTargets: []string{identity.CanonicalLemma},
	}
}
