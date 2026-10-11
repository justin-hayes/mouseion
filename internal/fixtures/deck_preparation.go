package fixtures

// Contract status: contractual. Deck preparation and admission transitions
// must match internal/storecontract scenarios shared with PostgreSQL (ADR
// 0088). Issue #1722 introduces those shared scenarios.

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
)

// SetGoalSnapshotVocabulary lets acceptance tests add a deterministic frozen
// snapshot without exposing the fixture store's internal maps.
func (s *Store) SetGoalSnapshotVocabulary(snapshotID string, vocabulary []domain.DeckPreparationVocabulary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goalSnapshotVocabulary[snapshotID] = append([]domain.DeckPreparationVocabulary(nil), vocabulary...)
}

// reservedDeckVocabularyLocked returns the active reading's frozen vocabulary.
// Callers hold s.mu.
func (s *Store) reservedDeckVocabularyLocked(owner, language string) []domain.DeckPreparationVocabulary {
	var result []domain.DeckPreparationVocabulary
	seen := make(map[string]struct{})
	goal := s.currentReadings[fixtureGoalKey(owner, language)]
	hasSnapshot := false
	if goal.IsActive() && goal.SnapshotID != "" {
		snapshot, snapshotExists := s.goalSnapshotVocabulary[goal.SnapshotID]
		hasSnapshot = snapshotExists
		for _, item := range snapshot {
			if item.Language == language {
				item.GraduatedAt = nil
				result = append(result, item)
				seen[item.Language+"\x00"+item.CanonicalLemma+"\x00"+item.UPOS] = struct{}{}
			}
		}
	}
	if !hasSnapshot && goal.IsActive() && goal.SourceMaterialID != "" {
		for _, preparation := range s.preps {
			if preparation.OwnerID != owner || preparation.SourceMaterialID != goal.SourceMaterialID || preparation.AnalysisRunID != goal.AnalysisRunID {
				continue
			}
			for _, item := range s.deckVocabularyFor(owner, preparation.ID) {
				if item.Language == language {
					item.GraduatedAt = nil
					result = append(result, item)
					seen[item.Language+"\x00"+item.CanonicalLemma+"\x00"+item.UPOS] = struct{}{}
				}
			}
		}
	}
	return result
}

func (s *Store) CountCurrentReadingVocabularyToAccept(_ context.Context, owner, language string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return domain.PlanSnapshotAcceptance(s.fixtureSnapshotIdentitiesLocked(owner, language), s.known).EligibleCount, nil
}

// fixtureSnapshotIdentitiesLocked returns the active reading's frozen identities.
func (s *Store) fixtureSnapshotIdentitiesLocked(owner, language string) []domain.SnapshotIdentity {
	snapshot := s.goalSnapshotVocabularyLocked(owner, language)
	identities := make([]domain.SnapshotIdentity, 0, len(snapshot))
	for _, item := range snapshot {
		identities = append(identities, domain.SnapshotIdentity{Language: item.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS})
	}
	return identities
}

func (s *Store) goalSnapshotVocabularyLocked(owner, language string) []domain.DeckPreparationVocabulary {
	goal := s.currentReadings[fixtureGoalKey(owner, language)]
	if !goal.IsActive() || goal.SnapshotID == "" {
		return nil
	}
	snapshot, exists := s.goalSnapshotVocabulary[goal.SnapshotID]
	if !exists {
		return nil
	}
	result := make([]domain.DeckPreparationVocabulary, 0, len(snapshot))
	for _, item := range snapshot {
		if item.Language == language {
			item.GraduatedAt = nil
			result = append(result, item)
		}
	}
	return result
}

// ListReservedVocabulary returns the vocabulary currently reserved by the
// owner's active reading snapshot, scoped to one language. It is the read seam
// used by the coverage service.
func (s *Store) ListReservedVocabulary(_ context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reservedDeckVocabularyLocked(owner, language), nil
}

func (s *Store) deckVocabularyFor(owner, preparationID string) []domain.DeckPreparationVocabulary {
	var result []domain.DeckPreparationVocabulary
	for _, item := range s.deckVocabulary {
		if item.OwnerID == owner && item.DeckPreparationID == preparationID {
			result = append(result, item)
		}
	}
	return result
}

func (s *Store) ListUnattachedGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.GeneratedVocabulary
	for _, item := range s.legacyGenerated {
		if item.OwnerID == owner && item.Language == language {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *Store) GetDeckPreparationForAnalysis(_ context.Context, owner, sourceMaterialID, analysisRunID string) (domain.DeckPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, preparation := range s.preps {
		if preparation.OwnerID == owner && preparation.SourceMaterialID == sourceMaterialID && preparation.AnalysisRunID == analysisRunID && preparation.SnapshotID == "" && preparation.RetiredAt == nil {
			preparation.VocabularyCount = len(s.deckVocabularyFor(owner, preparation.ID))
			return preparation, nil
		}
	}
	return domain.DeckPreparation{}, errNotFound
}

func (s *Store) GetDeckPreparationForSnapshot(_ context.Context, owner, snapshotID string) (domain.DeckPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, preparation := range s.preps {
		if preparation.OwnerID == owner && preparation.SnapshotID == snapshotID && preparation.RetiredAt == nil {
			preparation.VocabularyCount = len(s.deckVocabularyFor(owner, preparation.ID))
			return preparation, nil
		}
	}
	return domain.DeckPreparation{}, errNotFound
}

func (s *Store) ListDeckPreparationsForSourceMaterial(_ context.Context, owner, sourceMaterialID string) ([]domain.DeckPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.DeckPreparation
	for _, preparation := range s.preps {
		if preparation.OwnerID == owner && preparation.SourceMaterialID == sourceMaterialID {
			preparation.VocabularyCount = len(s.deckVocabularyFor(owner, preparation.ID))
			result = append(result, preparation)
		}
	}
	return result, nil
}

func (s *Store) GetExtractedUnitSnapshot(context.Context, string, string) (string, domain.ExtractedUnits, error) {
	text := "Haus. Ein kurzer deutscher Satz."
	second := "Ein sehr langer Beispielsatz mit vielen Wörtern für die Anzeige von realistischem Randinhalt im Browser."
	return "fixture-snapshot", domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "fixture-001"), Order: 0, SpineIndex: 0, ManifestID: "fixture-001", Title: "Chapter one", Text: text, StartOffset: 0, EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true},
		{ID: domain.EPUBUnitID(1, "fixture-002"), Order: 1, SpineIndex: 1, ManifestID: "fixture-002", Title: "Chapter two", Text: second, StartOffset: uint64(len([]rune(text)) + 2), EndOffset: uint64(len([]rune(text)) + 2 + len([]rune(second))), MediaType: "application/xhtml+xml", Linear: true},
	}}, nil
}

func (s *Store) ListCurrentReadingSnapshotVocabulary(_ context.Context, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, found := s.goalSnapshotVocabulary[snapshotID]
	if !found {
		return nil, nil
	}
	result := make([]domain.SelectionCandidate, 0, len(goal))
	for _, item := range goal {
		result = append(result, domain.SelectionCandidate{OwnerID: owner, Language: item.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS})
	}
	return result, nil
}

type PreparedDeck struct{ Store *Store }

// fixtureCurrentReadingFor returns the owner's active current reading for a Book
// and the Book's analysis identity. The fixture analysis is the reading's own
// identity, so an active reading always pins its analysis. The caller holds mu.
func (s *Store) fixtureCurrentReadingFor(owner, bookID string) (domain.CurrentReading, domain.CurrentAnalysis) {
	if bookID == "" {
		return domain.CurrentReading{}, domain.CurrentAnalysis{}
	}
	for _, current := range s.currentReadings {
		if current.OwnerID == owner && current.BookID == bookID {
			return current, domain.CurrentAnalysis{SourceMaterialID: current.SourceMaterialID, AnalysisRunID: current.AnalysisRunID, ContentRevisionID: current.ContentRevisionID, ContentSnapshotID: current.ContentSnapshotID, CorpusID: current.CorpusID}
		}
	}
	return domain.CurrentReading{}, domain.CurrentAnalysis{}
}

// fixtureLivePreparation returns a copy of the non-retired preparation bound to
// the snapshot, or nil. The caller holds mu.
func (s *Store) fixtureLivePreparation(owner, snapshotID string) *domain.DeckPreparation {
	if snapshotID == "" {
		return nil
	}
	for i := range s.preps {
		if s.preps[i].OwnerID == owner && s.preps[i].SnapshotID == snapshotID && s.preps[i].RetiredAt == nil {
			live := s.preps[i]
			return &live
		}
	}
	return nil
}

// PrepareCurrentReadingDeck applies the same admission rule as the service to the
// fixture store, then creates or returns the live preparation of the snapshot.
func (p PreparedDeck) PrepareCurrentReadingDeck(_ context.Context, owner, bookID, expectedSnapshotID string) (prepareddeck.Handle, error) {
	if p.Store == nil {
		return prepareddeck.Handle{JobID: 9}, nil
	}
	p.Store.mu.Lock()
	defer p.Store.mu.Unlock()
	current, analysis := p.Store.fixtureCurrentReadingFor(owner, bookID)
	live := p.Store.fixtureLivePreparation(owner, current.SnapshotID)
	admission := domain.DecideDeckAdmission(domain.DeckAdmissionFacts{Action: domain.DeckActionSubmit, BookID: bookID, Current: current, Analysis: analysis, ExpectedSnapshotID: expectedSnapshotID, Preparation: live})
	if err := admission.Err(); err != nil {
		return prepareddeck.Handle{}, err
	}
	if live != nil {
		return prepareddeck.Handle{Preparation: *live, JobID: 9}, nil
	}
	preparation := domain.DeckPreparation{ID: "fixture-goal-preparation-" + current.SnapshotID, OwnerID: owner, BookID: bookID, SourceMaterialID: current.SourceMaterialID, AnalysisRunID: current.AnalysisRunID, SnapshotID: current.SnapshotID, State: domain.DeckPreparationQueued}
	p.Store.preps = append(p.Store.preps, preparation)
	return prepareddeck.Handle{Preparation: preparation, JobID: 9}, nil
}

// CurrentReadingDeck reports the Book's current reading deck state and its
// Submit admission, as the service does.
func (p PreparedDeck) CurrentReadingDeck(_ context.Context, owner, bookID string) (prepareddeck.CurrentDeck, error) {
	if p.Store == nil {
		return prepareddeck.CurrentDeck{}, nil
	}
	p.Store.mu.Lock()
	defer p.Store.mu.Unlock()
	current, analysis := p.Store.fixtureCurrentReadingFor(owner, bookID)
	live := p.Store.fixtureLivePreparation(owner, current.SnapshotID)
	admission := domain.DecideDeckAdmission(domain.DeckAdmissionFacts{Action: domain.DeckActionSubmit, BookID: bookID, Current: current, Analysis: analysis, ExpectedSnapshotID: current.SnapshotID, Preparation: live})
	return prepareddeck.CurrentDeck{Current: current, Preparation: live, Admission: admission}, nil
}

// PreparationAdmissions reports which generation actions the preparation admits.
func (p PreparedDeck) PreparationAdmissions(ctx context.Context, owner, id string) (prepareddeck.PreparationAdmissions, error) {
	preparation, err := p.Get(ctx, owner, id)
	if err != nil {
		return prepareddeck.PreparationAdmissions{}, err
	}
	current, analysis := domain.CurrentReading{}, domain.CurrentAnalysis{}
	if p.Store != nil {
		p.Store.mu.Lock()
		current, analysis = p.Store.fixtureCurrentReadingFor(owner, preparation.BookID)
		p.Store.mu.Unlock()
	}
	facts := domain.DeckAdmissionFacts{BookID: preparation.BookID, Current: current, Analysis: analysis, ExpectedSnapshotID: preparation.SnapshotID, Preparation: &preparation}
	facts.Action = domain.DeckActionSubmit
	retry := domain.DecideDeckAdmission(facts)
	facts.Action = domain.DeckActionReprepare
	return prepareddeck.PreparationAdmissions{Retry: retry, Reprepare: domain.DecideDeckAdmission(facts)}, nil
}

func fixturePreparationFor(owner, id string) domain.DeckPreparation {
	preparation := domain.DeckPreparation{ID: id, OwnerID: owner, BookID: BookID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German deck.apkg", DeckName: "Mouseion::de::Fixture", TotalCards: 3}
	switch id {
	case JourneyPrepID:
		preparation.SourceMaterialID = "fixture-empty"
		preparation.AnalysisRunID = "fixture-empty-run"
		preparation.DeckName = "Mouseion::it::Journey"
	case OutsidePrepID:
		preparation.SourceMaterialID = "fixture-failed"
		preparation.AnalysisRunID = "fixture-failed-run"
		preparation.DeckName = "Mouseion::de::Outside Journey"
	}
	return preparation
}

func (p PreparedDeck) Get(_ context.Context, owner, id string) (domain.DeckPreparation, error) {
	if p.Store != nil {
		p.Store.mu.Lock()
		defer p.Store.mu.Unlock()
		for _, preparation := range p.Store.preps {
			if preparation.OwnerID == owner && preparation.ID == id {
				preparation.VocabularyCount = len(p.Store.deckVocabularyFor(owner, id))
				return preparation, nil
			}
		}
	}
	return fixturePreparationFor(owner, id), nil
}

func (p PreparedDeck) GetForAnalysis(ctx context.Context, owner, sourceMaterialID, analysisRunID string) (domain.DeckPreparation, error) {
	if p.Store != nil {
		return p.Store.GetDeckPreparationForAnalysis(ctx, owner, sourceMaterialID, analysisRunID)
	}
	return fixturePreparationFor(owner, PrepID), nil
}

func (p PreparedDeck) GetForCurrentReadingSnapshot(ctx context.Context, owner, snapshotID string) (domain.DeckPreparation, error) {
	if p.Store != nil {
		return p.Store.GetDeckPreparationForSnapshot(ctx, owner, snapshotID)
	}
	return fixturePreparationFor(owner, PrepID), nil
}

func (PreparedDeck) Cancel(context.Context, string, string) (domain.DeckPreparation, error) {
	return domain.DeckPreparation{ID: PrepID, State: domain.DeckPreparationCancelled}, nil
}

// Reprepare admits a ready preparation of the exact current snapshot, as the
// service does, then reports the fixture generation.
func (p PreparedDeck) Reprepare(ctx context.Context, owner, id, expectedSnapshotID string) (prepareddeck.Handle, error) {
	if err := p.admitGeneration(ctx, owner, id, expectedSnapshotID, domain.DeckActionReprepare); err != nil {
		return prepareddeck.Handle{}, err
	}
	return prepareddeck.Handle{Preparation: domain.DeckPreparation{ID: "fixture-reprepared-deck", State: domain.DeckPreparationQueued}, JobID: 11}, nil
}

// Rerender admits a ready preparation of the exact current snapshot, as the
// service does, then reports the fixture job.
func (p PreparedDeck) Rerender(ctx context.Context, owner, id, expectedSnapshotID string) (prepareddeck.Handle, error) {
	if err := p.admitGeneration(ctx, owner, id, expectedSnapshotID, domain.DeckActionRerender); err != nil {
		return prepareddeck.Handle{}, err
	}
	return prepareddeck.Handle{JobID: 10}, nil
}

func (p PreparedDeck) admitGeneration(ctx context.Context, owner, id, expectedSnapshotID string, action domain.DeckAction) error {
	preparation, err := p.Get(ctx, owner, id)
	if err != nil {
		return err
	}
	if p.Store == nil {
		return nil
	}
	p.Store.mu.Lock()
	current, analysis := p.Store.fixtureCurrentReadingFor(owner, preparation.BookID)
	p.Store.mu.Unlock()
	return domain.DecideDeckAdmission(domain.DeckAdmissionFacts{Action: action, BookID: preparation.BookID, Current: current, Analysis: analysis, ExpectedSnapshotID: expectedSnapshotID, Preparation: &preparation}).Err()
}

func (p PreparedDeck) Download(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	preparation, err := p.Get(ctx, owner, id)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	preparation.State = domain.DeckPreparationReady
	preparation.Artifact = []byte("fixture")
	return preparation, nil
}
