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

func (h *postgresHarness) Store() storecontract.Store                  { return h }
func (h *postgresHarness) LemmaReview() storecontract.LemmaReviewStore { return h }
func (h *postgresHarness) Seeds() storecontract.Seeder                 { return h }

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
func (h *postgresHarness) SeedBook(t *testing.T, owner string, seed storecontract.BookSeed) string {
	t.Helper()
	h.books++
	identifier := fmt.Sprintf("contract-book-%d", h.books)
	source, err := h.store.PutSourceMaterialWithExtractedUnits(h.ctx, domain.SourceMaterial{OwnerID: owner, Language: storecontract.Language, SourceIdentifier: identifier, Title: "Contract Book", MediaType: "application/epub+zip", Content: []byte("text"), FullText: "text"}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "text", EndOffset: 4}}})
	require.NoError(t, err)
	book, err := h.store.CreateBook(h.ctx, domain.Book{OwnerID: owner, Title: source.Title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: storecontract.Language})
	require.NoError(t, err)
	require.NoError(t, h.store.LinkSourceToBook(h.ctx, owner, book.ID, source.ID))
	runID, corpusID := h.completeAnalysis(t, owner, book.ID, source.ID, identifier)
	if len(seed.Occurrences) > 0 {
		h.seedLemmaOccurrences(t, owner, book.ID, source.ID, runID, corpusID, seed.Occurrences)
	}
	if seed.ToRead {
		require.NoError(t, h.store.SetBookDisposition(h.ctx, owner, book.ID, domain.BookDispositionToRead))
	}
	for _, identity := range seed.Vocabulary {
		_, err = h.store.PutSelectionCandidate(h.ctx, domain.SelectionCandidate{OwnerID: owner, CorpusID: corpusID, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS, OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`)})
		require.NoError(t, err)
	}
	return book.ID
}

// completeAnalysis runs the Book's analysis to completion as analysis does,
// makes it the Book's current analysis, and returns its run and corpus IDs.
func (h *postgresHarness) completeAnalysis(t *testing.T, owner, bookID, sourceID, identifier string) (string, string) {
	t.Helper()
	analysisRiver, err := analysis.NewClient(h.store.Pool(), &analyzertest.Fake{}, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(h.store))
	require.NoError(t, err)
	handle, err := analysis.NewService(h.store.Pool(), analysisRiver).SubmitAnalysis(h.ctx, owner, sourceID)
	require.NoError(t, err)
	artifactHash := "sha256:" + identifier
	h.exec(t, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,'de','1','casefold','1','fake','1')`, artifactHash)
	corpusID := uuid.NewString()
	h.exec(t, `INSERT INTO corpora(id,owner_id,source_material_id,artifact_hash,analysis_run_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count) VALUES($1,$2,$3,$4,$5,'complete',0,0,1,1,0,1,1,0)`, corpusID, owner, sourceID, artifactHash, handle.RunID)
	h.exec(t, `UPDATE analysis_runs SET state='completed',corpus_id=$2,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$3`, owner, corpusID, handle.RunID)
	h.exec(t, `UPDATE analysis_run_attempts SET state='completed',finalized_at=now() WHERE run_id=$1`, handle.RunID)
	h.exec(t, `UPDATE analysis_jobs SET corpus_id=$2,progress=100,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$3`, owner, corpusID, handle.RunID)
	h.exec(t, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner, bookID, sourceID, handle.RunID)
	return handle.RunID, corpusID
}

// seedLemmaOccurrences stores the scenario's occurrences as analysis would, one
// sentence each in the Book's extracted unit, then builds the Book's vocabulary
// count projection so lemma review previews are ready.
func (h *postgresHarness) seedLemmaOccurrences(t *testing.T, owner, bookID, sourceID, runID, corpusID string, seeds []storecontract.OccurrenceSeed) {
	t.Helper()
	unitID := domain.EPUBUnitID(0, "unit")
	start := int64(0)
	for ordinal, seed := range seeds {
		end := start + int64(utf8.RuneCountInString(seed.Surface))
		h.exec(t, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, owner, runID, corpusID, unitID, ordinal, seed.Surface, start, end)
		h.exec(t, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,0,$6,$7,$7,$8,'root',0,'{}',$9,$10)`, owner, storecontract.Language, runID, corpusID, ordinal, seed.Surface, seed.Lemma, seed.UPOS, start, end)
		start = end + 1
	}
	tx, err := h.store.Pool().Begin(h.ctx)
	require.NoError(t, err)
	require.NoError(t, persistence.BuildVocabularyBrowseCountsTx(h.ctx, tx, owner, bookID, sourceID, runID, corpusID, storecontract.Language))
	require.NoError(t, tx.Commit(h.ctx))
}

func (h *postgresHarness) SeedKnownVocabulary(t *testing.T, owner string, identities []domain.SnapshotIdentity) {
	t.Helper()
	for _, identity := range identities {
		_, err := h.store.PutKnownVocabulary(h.ctx, owner, identity.Language, identity.CanonicalLemma, identity.UPOS)
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

func (h *postgresHarness) ClearSnapshotID(t *testing.T, owner, language string) {
	t.Helper()
	h.exec(t, `UPDATE primary_goals SET snapshot_id=NULL WHERE owner_id=$1 AND language=$2`, owner, language)
}

func (h *postgresHarness) GetCurrentReading(owner, language string) (domain.CurrentReading, error) {
	return h.store.GetCurrentReading(h.ctx, owner, language)
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
