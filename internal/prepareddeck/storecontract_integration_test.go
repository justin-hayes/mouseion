//go:build integration

package prepareddeck

import (
	"context"
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/storecontract"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"
)

// postgresHarness runs the shared store contracts against PostgreSQL, with the
// River-backed deck service that admission decisions are made through.
type postgresHarness struct {
	ctx     context.Context
	store   *persistence.PostgresStore
	service *Service
	owner   string
	other   string
	books   int
}

// newPostgresHarness gives each scenario its own cloned database and learners.
func newPostgresHarness(t *testing.T) storecontract.Harness {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "contract-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "contract-other-owner", false)
	require.NoError(t, err)
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
	return &postgresHarness{ctx: ctx, store: store, service: NewService(store, client), owner: owner.ID, other: other.ID}
}

func TestStoreContracts(t *testing.T) {
	storecontract.Run(t, newPostgresHarness)
}

func (h *postgresHarness) GetBookDetail(owner, bookID string) (domain.MyBook, error) {
	return h.store.GetBookDetail(h.ctx, owner, bookID)
}

func (h *postgresHarness) TransitionBookDisposition(owner, bookID string, expectedRevision int64, disposition domain.BookDisposition) (bool, error) {
	return h.store.TransitionBookDisposition(h.ctx, owner, bookID, expectedRevision, disposition)
}

func (h *postgresHarness) SetBookDisposition(owner, bookID string, disposition domain.BookDisposition) error {
	return h.store.SetBookDisposition(h.ctx, owner, bookID, disposition)
}

func (h *postgresHarness) RemoveBookFromMyBooks(owner, bookID string) error {
	return h.store.RemoveBookFromMyBooks(h.ctx, owner, bookID)
}

func (h *postgresHarness) ImportPreviouslyRead(owner, bookID string) (domain.ReadingCompletion, error) {
	return h.store.ImportPreviouslyRead(h.ctx, owner, bookID)
}

func (h *postgresHarness) MarkBookPreviouslyRead(owner, bookID string) error {
	return h.store.MarkBookPreviouslyRead(h.ctx, owner, bookID)
}

func (h *postgresHarness) ResolveOrCreateBookForAcquisition(owner, sourceIdentifier, language, title string) (string, error) {
	return h.store.ResolveOrCreateBookForAcquisition(h.ctx, owner, sourceIdentifier, language, title)
}

func (h *postgresHarness) RecordEPUBAcquisitionFailure(record domain.EPUBAcquisitionFailureRecord) error {
	return h.store.RecordEPUBAcquisitionFailure(h.ctx, record)
}

func (h *postgresHarness) ClearEPUBAcquisitionFailure(owner, bookID string) error {
	return h.store.ClearEPUBAcquisitionFailure(h.ctx, owner, bookID)
}

func (h *postgresHarness) Store() storecontract.Store                      { return h }
func (h *postgresHarness) LemmaReview() storecontract.LemmaReviewStore     { return h }
func (h *postgresHarness) BookState() storecontract.BookStateStore         { return h }
func (h *postgresHarness) Classical() storecontract.ClassicalStore         { return h }
func (h *postgresHarness) Catalogue() storecontract.CatalogueStore         { return h }
func (h *postgresHarness) Study() storecontract.StudyStore                 { return h }
func (h *postgresHarness) StudyProgress() storecontract.StudyProgressStore { return h }
func (h *postgresHarness) StudyFluency() storecontract.StudyFluencyStore   { return h }
func (h *postgresHarness) Seeds() storecontract.Seeder                     { return h }

func (h *postgresHarness) BrowseCatalogueEntries(owner, connectionID string, query domain.CatalogueQuery) (domain.CataloguePage, error) {
	return h.store.BrowseCatalogueEntries(h.ctx, owner, connectionID, query)
}

func (h *postgresHarness) AddCatalogueEntry(owner, connectionID, entryIdentifier string) (domain.CatalogueAddResult, error) {
	return h.store.AddCatalogueEntry(h.ctx, owner, connectionID, entryIdentifier)
}

func (h *postgresHarness) MarkCatalogueEntryPreviouslyRead(owner, connectionID, entryIdentifier string) (domain.CatalogueReadResult, error) {
	return h.store.MarkCatalogueEntryPreviouslyRead(h.ctx, owner, connectionID, entryIdentifier)
}

func (h *postgresHarness) AddClassicalReadingUnits(owner string, readingUnitKeys []string) (classical.AddReport, error) {
	return h.store.AddClassicalReadingUnits(h.ctx, owner, readingUnitKeys)
}

func (h *postgresHarness) MarkClassicalReadingUnitPreviouslyRead(owner, readingUnitKey string) (domain.CatalogueReadResult, error) {
	return h.store.MarkClassicalReadingUnitPreviouslyRead(h.ctx, owner, readingUnitKey)
}

func (h *postgresHarness) ListOwnedClassicalReadingUnits(owner string) ([]classical.OwnedReadingUnit, error) {
	return h.store.ListOwnedClassicalReadingUnits(h.ctx, owner)
}

func (h *postgresHarness) ListStudyLanguages(owner string) ([]domain.StudyLanguage, error) {
	return h.store.ListStudyLanguages(h.ctx, owner)
}

func (h *postgresHarness) SeedClassicalCatalog(t *testing.T) {
	t.Helper()
	require.NoError(t, h.store.ReconcileClassicalManifest(h.ctx, storecontract.ClassicalDefinitions()))
}

func (h *postgresHarness) SeedCatalogueConnection(t *testing.T, owner, name string) string {
	t.Helper()
	connection, err := h.store.CreateOpdsConnection(h.ctx, owner, domain.OpdsConnection{Name: name, URL: "https://contract.example/opds"})
	require.NoError(t, err)
	return connection.ID
}

func (h *postgresHarness) SeedCatalogueEntries(t *testing.T, owner, connectionID string, seeds []storecontract.CatalogueEntrySeed) {
	t.Helper()
	entries := make([]domain.CatalogueIndexEntry, 0, len(seeds))
	for _, seed := range seeds {
		entry := seed.Entry
		entry.ConnectionID = connectionID
		entries = append(entries, entry)
	}
	require.NoError(t, h.store.ReconcileCatalogueIndex(h.ctx, owner, connectionID, entries, time.Now()))
	for _, seed := range seeds {
		if seed.BookID == "" {
			continue
		}
		h.exec(t, `INSERT INTO book_aliases(owner_id,book_id,connection_id,alias_type,namespace,value)
			VALUES($1,$2,$3,$4,$5,$6)`, owner, seed.BookID, connectionID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, seed.Entry.EntryIdentifier)
	}
}

func (h *postgresHarness) SeedClassicalPreparationFailure(t *testing.T, owner, bookID string, stage domain.BookPreparationStage) {
	t.Helper()
	detail, err := h.store.GetBookDetail(h.ctx, owner, bookID)
	require.NoError(t, err)
	require.NotNil(t, detail.ReadingUnit, "a classical failure needs a Reading unit")
	require.NoError(t, h.store.PutClassicalPreparationFailure(h.ctx, domain.ClassicalPreparationFailureRecord{
		OwnerID:            owner,
		BookID:             bookID,
		ReadingUnitKey:     detail.ReadingUnit.ReadingUnitKey,
		DefinitionRevision: detail.ReadingUnit.Revision,
		Stage:              stage,
		Code:               domain.BookPreparationCode("contract_" + string(stage)),
	}))
}

func (h *postgresHarness) Owner() string      { return h.owner }
func (h *postgresHarness) OtherOwner() string { return h.other }

// exec runs one seeding statement and fails the test if it does not apply.
func (h *postgresHarness) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, err := h.store.Pool().Exec(h.ctx, sql, args...)
	require.NoError(t, err)
}

// SeedBook creates a Book with a completed analysis and the given candidate
// identities, as analysis and selection would, and marks it To Read when asked.
// Identities are seeded as analysis tokens, which the Reading freeze projects;
// an imported classical Book's tokens take the shape the import writer
// produces.
func (h *postgresHarness) SeedBook(t *testing.T, owner string, seed storecontract.BookSeed) string {
	t.Helper()
	h.books++
	identifier := fmt.Sprintf("contract-book-%d", h.books)
	language := seed.Language
	if language == "" {
		language = storecontract.Language
	}
	languageState, languageTag := domain.LanguageChosen, language
	if seed.LanguageUnknown {
		languageState, languageTag = domain.LanguageUnknown, ""
	}
	if seed.MetadataOnly {
		book, err := h.store.CreateBook(h.ctx, domain.Book{OwnerID: owner, Title: "Contract Book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: languageState, LanguageTag: languageTag})
		require.NoError(t, err)
		h.seedDisposition(t, owner, book.ID, !seed.Inbox)
		if seed.SourceIdentifier != "" {
			require.NoError(t, h.store.AddBookAlias(h.ctx, owner, book.ID, domain.AliasStrongBibliographic, domain.NamespaceSourceIdentifier, seed.SourceIdentifier))
		}
		return book.ID
	}
	source, err := h.store.PutSourceMaterialWithExtractedUnits(h.ctx, domain.SourceMaterial{OwnerID: owner, Language: language, SourceIdentifier: identifier, Title: "Contract Book", MediaType: "application/epub+zip", Content: []byte("text"), FullText: "text"}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "text", EndOffset: 4}}})
	require.NoError(t, err)
	book, err := h.store.CreateBook(h.ctx, domain.Book{OwnerID: owner, Title: source.Title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: languageState, LanguageTag: languageTag})
	require.NoError(t, err)
	require.NoError(t, h.store.LinkSourceToBook(h.ctx, owner, book.ID, source.ID))
	runID, corpusID := h.completeAnalysis(t, owner, book.ID, source.ID, identifier, language, seed.Imported)
	if len(seed.Occurrences) > 0 {
		h.seedLemmaOccurrences(t, owner, book.ID, source.ID, runID, corpusID, language, seed.Occurrences)
	}
	h.seedDisposition(t, owner, book.ID, !seed.Inbox)
	var imported []domain.SnapshotIdentity
	for _, identity := range seed.Vocabulary {
		if seed.Imported {
			imported = append(imported, identity)
			continue
		}
		testutil.SeedCorpusWords(t, h.ctx, h.store.Pool(), owner, corpusID, identity.Language, testutil.CorpusWord{
			Lemma: identity.CanonicalLemma, UPOS: identity.UPOS,
			LexicalDiscriminator: identity.LexicalDiscriminator, Headword: identity.Headword,
			Fragment: seed.DeclaresUnusable(identity), Occurrences: 5,
		})
	}
	if len(imported) > 0 {
		h.seedImportedVocabulary(t, owner, book.ID, source.ID, runID, corpusID, seed, imported)
	}
	return book.ID
}

// seedDisposition sets the Book's disposition. A Book that is not To Read is the
// legacy shape, which no store operation creates, so the seed writes it
// directly.
func (h *postgresHarness) seedDisposition(t *testing.T, owner, bookID string, toRead bool) {
	t.Helper()
	if toRead {
		require.NoError(t, h.store.SetBookDisposition(h.ctx, owner, bookID, domain.BookDispositionToRead))
		return
	}
	// The store refuses the retired Inbox state, so only a direct write can
	// seed the legacy row.
	h.exec(t, `UPDATE book_dispositions SET disposition = 'inbox' WHERE owner_id = $1 AND book_id = $2`, owner, bookID)
}

// completeAnalysis runs the Book's analysis to completion as analysis does,
// makes it the Book's current analysis, and returns its run and corpus IDs.
func (h *postgresHarness) completeAnalysis(t *testing.T, owner, bookID, sourceID, identifier, language string, imported bool) (string, string) {
	t.Helper()
	analysisRiver, err := analysis.NewClient(h.store.Pool(), &analyzertest.Fake{}, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(h.store))
	require.NoError(t, err)
	handle, err := analysis.NewService(h.store.Pool(), analysisRiver).SubmitAnalysis(h.ctx, owner, sourceID)
	require.NoError(t, err)
	artifactHash := "sha256:" + identifier
	profile, version := "casefold", "1"
	if imported {
		profile, version = "ancient-greek/1", "1"
	}
	h.exec(t, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,$2,'1',$3,$4,'fake','1')`, artifactHash, language, profile, version)
	corpusID := uuid.NewString()
	h.exec(t, `INSERT INTO corpora(id,owner_id,source_material_id,artifact_hash,analysis_run_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count) VALUES($1,$2,$3,$4,$5,'complete',0,0,1,1,0,1,1,0)`, corpusID, owner, sourceID, artifactHash, handle.RunID)
	h.exec(t, `UPDATE analysis_runs SET state='completed',corpus_id=$2,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$3`, owner, corpusID, handle.RunID)
	h.exec(t, `UPDATE analysis_run_attempts SET state='completed',finalized_at=now() WHERE run_id=$1`, handle.RunID)
	h.exec(t, `UPDATE analysis_jobs SET corpus_id=$2,progress=100,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$3`, owner, corpusID, handle.RunID)
	if imported {
		h.exec(t, `UPDATE analysis_runs SET syntax_state='absent' WHERE owner_id=$1 AND id=$2`, owner, handle.RunID)
	}
	h.exec(t, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner, bookID, sourceID, handle.RunID)
	return handle.RunID, corpusID
}

// seedImportedVocabulary stores each imported identity's three occurrences in
// the Book's single extracted unit, then builds the Book's vocabulary count
// projection. Three in-Book occurrences qualify every identity for the Reading
// freeze, and the tokens carry the identity's source discriminator and headword.
// An identity the seed declares unusable gets fragments instead of gate-passing
// sentences.
func (h *postgresHarness) seedImportedVocabulary(t *testing.T, owner, bookID, sourceID, runID, corpusID string, seed storecontract.BookSeed, identities []domain.SnapshotIdentity) {
	t.Helper()
	unitID := domain.EPUBUnitID(0, "unit")
	ordinal, start := int64(0), int64(0)
	language := identities[0].Language
	for _, identity := range identities {
		word := testutil.CorpusWord{
			Lemma: identity.CanonicalLemma, UPOS: identity.UPOS,
			LexicalDiscriminator: identity.LexicalDiscriminator, Headword: identity.Headword,
			Fragment: seed.DeclaresUnusable(identity),
		}
		sentence := testutil.CorpusSentence(language, word)
		for range 3 {
			ordinal, start = testutil.SeedCorpusSentence(t, h.ctx, h.store.Pool(), owner, corpusID, language, runID, unitID, ordinal, start, sentence)
		}
	}
	tx, err := h.store.Pool().Begin(h.ctx)
	require.NoError(t, err)
	require.NoError(t, persistence.BuildVocabularyBrowseCountsTx(h.ctx, tx, owner, bookID, sourceID, runID, corpusID, language))
	require.NoError(t, tx.Commit(h.ctx))
}

// seedLemmaOccurrences stores the scenario's occurrences as analysis would, one
// sentence each in the Book's extracted unit, then builds the Book's vocabulary
// count projection so lemma review previews are ready.
func (h *postgresHarness) seedLemmaOccurrences(t *testing.T, owner, bookID, sourceID, runID, corpusID, language string, seeds []storecontract.OccurrenceSeed) {
	t.Helper()
	unitID := domain.EPUBUnitID(0, "unit")
	start := int64(0)
	for ordinal, seed := range seeds {
		end := start + int64(utf8.RuneCountInString(seed.Surface))
		h.exec(t, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, owner, runID, corpusID, unitID, ordinal, seed.Surface, start, end)
		h.exec(t, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,0,$6,$7,$7,$8,'root',0,'{}',$9,$10)`, owner, language, runID, corpusID, ordinal, seed.Surface, seed.Lemma, seed.UPOS, start, end)
		start = end + 1
	}
	tx, err := h.store.Pool().Begin(h.ctx)
	require.NoError(t, err)
	require.NoError(t, persistence.BuildVocabularyBrowseCountsTx(h.ctx, tx, owner, bookID, sourceID, runID, corpusID, language))
	require.NoError(t, tx.Commit(h.ctx))
}

func (h *postgresHarness) SeedKnownVocabulary(t *testing.T, owner string, identities []domain.SnapshotIdentity) {
	t.Helper()
	for _, identity := range identities {
		_, err := h.store.PutKnownVocabularyIdentity(h.ctx, owner, identity.Language, identity.CanonicalLemma, identity.UPOS, identity.LexicalDiscriminator)
		require.NoError(t, err)
	}
}

// SeedReadyPreparation submits the snapshot's deck and then records it as
// finished, the state a healthy ready deck is in.
func (h *postgresHarness) SeedReadyPreparation(t *testing.T, owner, bookID, snapshotID string) string {
	t.Helper()
	preparation, err := h.PrepareCurrentReadingDeck(owner, bookID, snapshotID)
	require.NoError(t, err)
	h.exec(t, `UPDATE deck_preparations SET state='ready',artifact=$3,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2`, owner, preparation.ID, []byte("deck"))
	return preparation.ID
}

func (h *postgresHarness) GetCurrentReading(owner, language string) (domain.CurrentReading, error) {
	return h.store.GetCurrentReading(h.ctx, owner, language)
}

func (h *postgresHarness) SeedCurrentReading(t *testing.T, owner, language, bookID string) {
	t.Helper()
	h.exec(t, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, owner, language, bookID)
}

func (h *postgresHarness) StartCurrentReading(owner, language, bookID string) (domain.CurrentReading, error) {
	return h.store.StartCurrentReading(h.ctx, owner, language, bookID)
}

func (h *postgresHarness) SwitchCurrentReading(owner, language, bookID, expectedBookID, expectedSnapshotID string) (domain.CurrentReading, error) {
	return h.store.SwitchCurrentReading(h.ctx, owner, language, bookID, expectedBookID, expectedSnapshotID)
}

func (h *postgresHarness) EndCurrentReading(owner, language, expectedBookID, expectedSnapshotID string) error {
	return h.store.EndCurrentReading(h.ctx, owner, language, expectedBookID, expectedSnapshotID)
}

func (h *postgresHarness) FinishCurrentReading(owner, language, expectedBookID, expectedSnapshotID string) (domain.CurrentReadingFinishResult, error) {
	return h.store.FinishCurrentReading(h.ctx, owner, language, expectedBookID, expectedSnapshotID)
}

func (h *postgresHarness) ListKnownVocabulary(owner, language string) ([]domain.KnownVocabulary, error) {
	return h.store.ListKnownVocabulary(h.ctx, owner, language)
}

func (h *postgresHarness) ListReservedVocabulary(owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	return h.store.ListReservedVocabulary(h.ctx, owner, language)
}

func (h *postgresHarness) ListLemmaReviewOccurrences(owner, bookID, surface string) ([]domain.LemmaReviewOccurrence, error) {
	return h.store.ListLemmaReviewOccurrences(h.ctx, owner, bookID, surface)
}

func (h *postgresHarness) SaveLemmaReviewFlags(flags []domain.LemmaReviewFlag) error {
	return h.store.SaveLemmaReviewFlags(h.ctx, flags)
}

func (h *postgresHarness) ReadLemmaReviewProposal(proposal domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error) {
	return h.store.ReadLemmaReviewProposal(h.ctx, proposal)
}

func (h *postgresHarness) PutLemmaDecisionProposal(proposal domain.LemmaReviewProposal, expectedFingerprint string) error {
	return h.store.PutLemmaDecisionProposal(h.ctx, proposal, expectedFingerprint)
}

func (h *postgresHarness) PrepareCurrentReadingDeck(owner, bookID, expectedSnapshotID string) (domain.DeckPreparation, error) {
	handle, err := h.service.PrepareCurrentReadingDeck(h.ctx, owner, bookID, expectedSnapshotID)
	return handle.Preparation, err
}

func (h *postgresHarness) RepreparePreparation(owner, id, expectedSnapshotID string) (domain.DeckPreparation, error) {
	handle, err := h.service.Reprepare(h.ctx, owner, id, expectedSnapshotID)
	return handle.Preparation, err
}

func (h *postgresHarness) ListStudyCandidates(owner, language string) ([]domain.StudyCandidate, error) {
	return h.store.ListStudyCandidates(h.ctx, owner, language)
}

func (h *postgresHarness) ListCurrentStudyMaterial(owner, language string) ([]domain.StudyMaterial, error) {
	return h.store.ListCurrentStudyMaterial(h.ctx, owner, language)
}

func (h *postgresHarness) GetStudyMaterial(owner, id string) (domain.StudyMaterial, error) {
	return h.store.GetStudyMaterial(h.ctx, owner, id)
}

func (h *postgresHarness) CountStudyMaterial(owner, language string) (domain.StudyMaterialCounts, error) {
	return h.store.CountStudyMaterial(h.ctx, owner, language)
}

func (h *postgresHarness) ReserveStudyMaterial(owner string, identity domain.StudyIdentity) (string, bool, error) {
	return h.store.ReserveStudyMaterial(h.ctx, owner, identity)
}

func (h *postgresHarness) RecordStudyMaterialReady(owner, id string, content domain.StudyMaterialReadyContent) error {
	return h.store.RecordStudyMaterialReady(h.ctx, owner, id, content)
}

func (h *postgresHarness) ListStudyProgress(owner, language string) ([]domain.StudyProgress, error) {
	return h.store.ListStudyProgress(h.ctx, owner, language)
}

func (h *postgresHarness) StudyBatchLedger(owner, language, studyDay string) (domain.StudyBatchLedger, error) {
	return h.store.StudyBatchLedger(h.ctx, owner, language, studyDay)
}

func (h *postgresHarness) TakeStudyBatch(owner, language, studyDay string, now time.Time, batchLimit int, words []domain.StudyBatchWord) ([]domain.StudyProgress, error) {
	return h.store.TakeStudyBatch(h.ctx, owner, language, studyDay, now, batchLimit, words)
}

func (h *postgresHarness) DeclineStudyBatch(owner, language, studyDay string, batchLimit int) error {
	return h.store.DeclineStudyBatch(h.ctx, owner, language, studyDay, batchLimit)
}

func (h *postgresHarness) NextStudyCard(owner, language string, now time.Time) (domain.StudyCard, error) {
	return h.store.NextStudyCard(h.ctx, owner, language, now)
}

func (h *postgresHarness) RecordStudyGrade(owner string, record domain.StudyGradeRecord) error {
	return h.store.RecordStudyGrade(h.ctx, owner, record)
}

func (h *postgresHarness) ListStudyReviewLog(owner, progressID string) ([]domain.StudyReviewLog, error) {
	return h.store.ListStudyReviewLog(h.ctx, owner, progressID)
}

func (h *postgresHarness) RecordStudyMaterialUnpreparable(owner, id, reason string) error {
	return h.store.RecordStudyMaterialUnpreparable(h.ctx, owner, id, reason)
}

func (h *postgresHarness) ListStudyFluencyContexts(owner, language string) ([]domain.FluencyContext, error) {
	return h.store.ListStudyFluencyContexts(h.ctx, owner, language)
}

func (h *postgresHarness) RecordStudyFluencyContext(owner string, record domain.FluencyContextRecord) error {
	return h.store.RecordStudyFluencyContext(h.ctx, owner, record)
}

func (h *postgresHarness) RecordStudyFluencyResponse(owner string, record domain.FluencyResponseRecord) error {
	return h.store.RecordStudyFluencyResponse(h.ctx, owner, record)
}

func (h *postgresHarness) CountStudyFluencyResponses(owner, contextID string) (int, error) {
	responses, err := h.store.ListStudyFluencyResponses(h.ctx, owner, contextID)
	return len(responses), err
}
