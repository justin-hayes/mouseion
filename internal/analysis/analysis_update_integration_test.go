//go:build integration

package analysis

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pipelineFixture is a running analysis service whose normalization version
// the test moves, as an NLP deployment would.
type pipelineFixture struct {
	mu         sync.Mutex
	reported   string
	produced   string
	unreadable bool
	notReady   bool
	gate       chan error
}

func (p *pipelineFixture) set(reported, produced string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reported, p.produced = reported, produced
}

func (p *pipelineFixture) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unreadable {
		return analyzer.Capabilities{}, errors.New("NLP unreachable")
	}
	return analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{
		Language: "de", SupportedFeatures: []string{"tokenize", "pos", "lemma", "depparse"}, Ready: !p.notReady,
		NormalizationProfile: "casefold", NormalizationVersion: p.reported,
	}}}, nil
}

func (p *pipelineFixture) analyze(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
	p.mu.Lock()
	gate, produced := p.gate, p.produced
	p.mu.Unlock()
	if gate != nil {
		if err := <-gate; err != nil {
			return analyzer.Result{}, err
		}
	}
	length := uint64(len([]rune(req.Document.Text)))
	// The last token's source lemma follows the produced normalization version,
	// so a later version changes that one token's identity and no other.
	changing := "Ende"
	if produced != "1" {
		changing = "Ende-" + produced
	}
	token := func(surface, rawLemma, canonical string, start, end uint64) analyzer.Token {
		dependency := "dep" // every token depends on the first, which is the root
		if start == 0 {
			dependency = "root"
		}
		return analyzer.Token{Surface: surface, RawLemma: rawLemma, CanonicalLemma: canonical, UPOS: "NOUN", Dependency: dependency,
			Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, StartOffset: start, EndOffset: end}}
	}
	return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language,
		Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"},
		NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: produced},
		Sentences: []analyzer.Sentence{{Text: req.Document.Text, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: length}, Tokens: []analyzer.Token{
			token("Tag", "Tag", "tag", 0, 3),
			token("Guten", "Guten", "guten", 4, 9),
			token("Wort", "Wort", "wort", 10, 14),
			token("Ende", changing, "ende", 15, 19),
		}}}}, nil
}

type analysisUpdateHarness struct {
	store    *persistence.PostgresStore
	service  *Service
	pipeline *pipelineFixture
	owner    string
}

func newAnalysisUpdateHarness(t *testing.T, ctx context.Context, name string) analysisUpdateHarness {
	t.Helper()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, name, false)
	require.NoError(t, err)
	pipeline := &pipelineFixture{reported: "1", produced: "1"}
	client, err := NewClient(pool, &analyzertest.Fake{AnalyzeFunc: pipeline.analyze}, pipeline)
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	return analysisUpdateHarness{store: store, service: NewService(pool, client).WithCapabilities(pipeline), pipeline: pipeline, owner: owner.ID}
}

// analyzedToReadBook analyses a To Read Book through the worker under the
// pipeline's current identity and returns its published run.
func (h analysisUpdateHarness) analyzedToReadBook(t *testing.T, ctx context.Context, slug string) (domain.Book, domain.SourceMaterial, Handle) {
	t.Helper()
	book, err := h.store.CreateBook(ctx, domain.Book{OwnerID: h.owner, Title: "Update " + slug, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, h.store, h.owner, "update-"+slug, book.Title, "Tag Guten Wort Ende", "sha256:update-"+slug)
	require.NoError(t, err)
	require.NoError(t, h.store.LinkSourceToBook(ctx, h.owner, book.ID, source.ID))
	require.NoError(t, h.store.SetBookDisposition(ctx, h.owner, book.ID, domain.BookDispositionToRead))
	handle, err := h.service.SubmitToReadBookAnalysis(ctx, h.owner, book.ID, source.ID)
	require.NoError(t, err)
	status, err := h.service.Wait(ctx, h.owner, handle.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateCompleted, status.State, status.Error)
	require.Equal(t, handle.RunID, h.publishedRun(t, ctx, book.ID))
	return book, source, handle
}

func (h analysisUpdateHarness) publishedRun(t *testing.T, ctx context.Context, bookID string) string {
	t.Helper()
	var runID string
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, h.owner, bookID).Scan(&runID))
	return runID
}

func (h analysisUpdateHarness) runCount(t *testing.T, ctx context.Context, sourceID string) int {
	t.Helper()
	var count int
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, h.owner, sourceID).Scan(&count))
	return count
}

func (h analysisUpdateHarness) classification(t *testing.T, ctx context.Context, bookID string) domain.BookEvidenceClassification {
	t.Helper()
	books, err := h.store.ListMyBooksWithEvidence(ctx, h.owner)
	require.NoError(t, err)
	for _, book := range books {
		if book.Book.ID == bookID {
			return book.Classification()
		}
	}
	t.Fatalf("Book %s not listed", bookID)
	return domain.BookEvidenceClassification{}
}

func TestAnalysisUpdateReplacesOutdatedAnalysisThroughOneConfirmedRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	h := newAnalysisUpdateHarness(t, ctx, "analysis-update")
	book, source, original := h.analyzedToReadBook(t, ctx, "replace")
	currentKey := domain.AnalysisUpdateTarget{AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion, NormalizationProfile: "casefold", NormalizationVersion: "1"}.Key()

	offers, err := h.service.AnalysisUpdateCandidates(ctx, h.owner, []string{source.ID})
	require.NoError(t, err)
	assert.Empty(t, offers, "a current analysis is not offered an update")
	noop, err := h.service.SubmitAnalysisUpdate(ctx, h.owner, book.ID, source.ID, original.RunID, currentKey)
	require.NoError(t, err)
	assert.Equal(t, original.RunID, noop.RunID, "submitting under the current identity returns the existing run")
	again, err := h.service.SubmitToReadBookAnalysis(ctx, h.owner, book.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, original.RunID, again.RunID)
	assert.Equal(t, 1, h.runCount(t, ctx, source.ID))

	var corpusID string
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM analysis_runs WHERE id=$1`, original.RunID).Scan(&corpusID))
	var documentID string
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT unit_id FROM corpus_sentences WHERE owner_id=$1 AND analysis_run_id=$2`, h.owner, original.RunID).Scan(&documentID))
	// Four decisions on the published analysis: a correction that carries, one
	// the new token already has (absorbed), an exclusion that carries, and a
	// correction on the token whose source lemma the update changes.
	_, err = h.store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded)
		VALUES($1,$2,$3,$4,$5,0,3,'tagen','casefold','1',false),
		      ($1,$2,$3,$4,$5,4,9,'guten','casefold','1',false),
		      ($1,$2,$3,$4,$5,10,14,NULL,NULL,NULL,true),
		      ($1,$2,$3,$4,$5,15,19,'enden','casefold','1',false)`, h.owner, book.ID, corpusID, original.RunID, documentID)
	require.NoError(t, err)

	h.pipeline.set("2", "2")
	offers, err = h.service.AnalysisUpdateCandidates(ctx, h.owner, []string{source.ID, "not-a-uuid"})
	require.NoError(t, err)
	require.Contains(t, offers, source.ID)
	offer := offers[source.ID]
	assert.Equal(t, original.RunID, offer.PublishedRunID)
	assert.Equal(t, 3, offer.LemmaCorrections)
	assert.Equal(t, 1, offer.OccurrenceExclusions)
	assert.Equal(t, "2", offer.Target.NormalizationVersion)

	// The service moved on again before the learner confirmed.
	h.pipeline.set("3", "3")
	_, err = h.service.SubmitAnalysisUpdate(ctx, h.owner, book.ID, source.ID, offer.PublishedRunID, offer.Target.Key())
	require.ErrorIs(t, err, ErrAnalysisUpdateChanged)
	h.pipeline.set("2", "2")
	_, err = h.service.SubmitAnalysisUpdate(ctx, h.owner, book.ID, source.ID, "00000000-0000-0000-0000-000000000000", offer.Target.Key())
	require.ErrorIs(t, err, ErrAnalysisUpdateChanged, "a changed published analysis is revalidated too")
	assert.Equal(t, 1, h.runCount(t, ctx, source.ID), "a changed offer submits nothing")

	gate := make(chan error)
	h.pipeline.mu.Lock()
	h.pipeline.gate = gate
	h.pipeline.mu.Unlock()
	var wg sync.WaitGroup
	handles := make([]Handle, 2)
	errs := make([]error, 2)
	for i := range handles {
		wg.Go(func() {
			handles[i], errs[i] = h.service.SubmitAnalysisUpdate(ctx, h.owner, book.ID, source.ID, offer.PublishedRunID, offer.Target.Key())
		})
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, handles[0].RunID, handles[1].RunID, "double submit converges on one run")
	assert.NotEqual(t, original.RunID, handles[0].RunID)
	assert.Equal(t, 2, h.runCount(t, ctx, source.ID))
	update := handles[0]

	during := h.classification(t, ctx, book.ID)
	assert.Equal(t, domain.BookAnalyzed, during.Evidence, "the published analysis stays in effect while the update runs")
	assert.Equal(t, domain.CurrentReadingEligible, during.Eligibility)
	assert.Equal(t, original.RunID, h.publishedRun(t, ctx, book.ID))

	gate <- errors.New("NLP unavailable")
	failed, err := h.service.Wait(ctx, h.owner, update.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateDiscarded, failed.State)
	afterFailure := h.classification(t, ctx, book.ID)
	assert.Equal(t, domain.BookAnalyzed, afterFailure.Evidence, "a failed update leaves the evidence analyzed")
	assert.Equal(t, domain.CurrentReadingEligible, afterFailure.Eligibility)
	assert.Equal(t, original.RunID, h.publishedRun(t, ctx, book.ID))

	retried, err := h.service.Retry(ctx, h.owner, update.ID)
	require.NoError(t, err)
	assert.Equal(t, update.RunID, retried.RunID, "job retry reruns the same update run")
	gate <- nil
	completed, err := h.service.Wait(ctx, h.owner, update.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateCompleted, completed.State, completed.Error)
	require.Eventually(t, func() bool { return h.publishedRun(t, ctx, book.ID) == update.RunID }, 10*time.Second, 25*time.Millisecond, "the update publishes and replaces the old evidence")
	assert.Equal(t, 2, h.runCount(t, ctx, source.ID))

	offers, err = h.service.AnalysisUpdateCandidates(ctx, h.owner, []string{source.ID})
	require.NoError(t, err)
	assert.Empty(t, offers, "the replacement is current")
	type decision struct {
		Start, End int
		Lemma      string
		Excluded   bool
	}
	decisionsOn := func(runID string) []decision {
		rows, err := h.store.Pool().Query(ctx, `SELECT start_offset, end_offset, COALESCE(canonical_lemma,''), excluded FROM occurrence_lemma_corrections
			WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND source_document_id=$4 ORDER BY start_offset`, h.owner, book.ID, runID, documentID)
		require.NoError(t, err)
		defer rows.Close()
		var found []decision
		for rows.Next() {
			var d decision
			require.NoError(t, rows.Scan(&d.Start, &d.End, &d.Lemma, &d.Excluded))
			found = append(found, d)
		}
		require.NoError(t, rows.Err())
		return found
	}
	assert.Equal(t, []decision{{0, 3, "tagen", false}, {10, 14, "", true}}, decisionsOn(update.RunID),
		"matched decisions carry; the absorbed correction and the changed-lemma correction do not")
	assert.Equal(t, []decision{{0, 3, "tagen", false}, {4, 9, "guten", false}, {10, 14, "", true}, {15, 19, "enden", false}}, decisionsOn(original.RunID),
		"decisions on the earlier analysis are not rewritten")
	var newCorpusID, copiedProfile, copiedVersion string
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT r.corpus_id::text, o.normalization_profile, o.normalization_version FROM analysis_runs r
		JOIN occurrence_lemma_corrections o ON o.owner_id=r.owner_id AND o.analysis_run_id=r.id AND o.start_offset=0
		WHERE r.owner_id=$1 AND r.id=$2`, h.owner, update.RunID).Scan(&newCorpusID, &copiedProfile, &copiedVersion))
	assert.NotEqual(t, corpusID, newCorpusID)
	assert.Equal(t, "casefold", copiedProfile, "the copy keeps the decision's normalization metadata")
	assert.Equal(t, "1", copiedVersion)
	var carriedCount, totalCount int
	var previous string
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT carried, total, previous_analysis_run_id::text FROM analysis_decision_carry_overs WHERE owner_id=$1 AND analysis_run_id=$2`,
		h.owner, update.RunID).Scan(&carriedCount, &totalCount, &previous))
	assert.Equal(t, 3, carriedCount, "two copied and one absorbed")
	assert.Equal(t, 4, totalCount)
	assert.Equal(t, original.RunID, previous)
	var tagen, tag int
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT count(*) FILTER (WHERE canonical_lemma='tagen'), count(*) FILTER (WHERE canonical_lemma='tag') FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`,
		h.owner, book.ID, update.RunID).Scan(&tagen, &tag))
	assert.Equal(t, 1, tagen, "the new run's Browse counts already reflect the copied correction")
	assert.Zero(t, tag)
	var excluded int
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT count(*) FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND canonical_lemma='wort'`,
		h.owner, book.ID, update.RunID).Scan(&excluded))
	assert.Zero(t, excluded, "and the copied exclusion")
	replayed, err := h.service.SubmitAnalysisUpdate(ctx, h.owner, book.ID, source.ID, offer.PublishedRunID, offer.Target.Key())
	require.ErrorIs(t, err, ErrAnalysisUpdateChanged, "a replayed confirmation sees the refreshed state")
	assert.Zero(t, replayed)
}

func TestAnalysisUpdateCandidatesNothingWhenTheCurrentIdentityIsUnknown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	h := newAnalysisUpdateHarness(t, ctx, "analysis-update-unknown")
	book, source, original := h.analyzedToReadBook(t, ctx, "unknown")
	h.pipeline.set("2", "2")
	target := domain.AnalysisUpdateTarget{AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion, NormalizationProfile: "casefold", NormalizationVersion: "2"}.Key()

	for name, configure := range map[string]func(){
		"unreachable": func() { h.pipeline.unreadable = true },
		"not ready":   func() { h.pipeline.notReady = true },
	} {
		t.Run(name, func(t *testing.T) {
			h.pipeline.mu.Lock()
			h.pipeline.unreadable, h.pipeline.notReady = false, false
			configure()
			h.pipeline.mu.Unlock()
			offers, err := h.service.AnalysisUpdateCandidates(ctx, h.owner, []string{source.ID})
			require.NoError(t, err)
			assert.Empty(t, offers)
			_, err = h.service.SubmitAnalysisUpdate(ctx, h.owner, book.ID, source.ID, original.RunID, target)
			require.ErrorIs(t, err, ErrAnalysisUpdateChanged)
		})
	}
	without := NewService(h.store.Pool(), h.service.client)
	offers, err := without.AnalysisUpdateCandidates(ctx, h.owner, []string{source.ID})
	require.NoError(t, err)
	assert.Empty(t, offers, "a service without capabilities treats every Book as current")
	assert.Equal(t, 1, h.runCount(t, ctx, source.ID))
}

func TestAnalysisUpdateCandidatesDeriveTheSnapshotSelection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	h := newAnalysisUpdateHarness(t, ctx, "analysis-update-selection")

	_, regularSource, regular := h.analyzedToReadBook(t, ctx, "regular")
	assert.Equal(t, snapshotConfigIdentity, h.runConfigIdentity(t, ctx, regular.RunID))

	book, err := h.store.CreateBook(ctx, domain.Book{OwnerID: h.owner, Title: "Gutenberg selection", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	texts := []string{
		"Preamble.\n\n*** START OF THE PROJECT GUTENBERG EBOOK X ***\n\nBook one.",
		"Book two.",
		"*** END OF THE PROJECT GUTENBERG EBOOK X ***\n\nLicence.",
		"back",
	}
	manifestIDs := []string{"pg-header", "chapter", "pg-footer", "illustration"}
	fullText := strings.Join(texts, "\n\n")
	units := make([]domain.ExtractedUnit, len(texts))
	var start uint64
	for i, text := range texts {
		end := start + uint64(len([]rune(text)))
		units[i] = domain.ExtractedUnit{
			ID: domain.EPUBUnitID(uint64(i), manifestIDs[i]), Order: uint64(i), SpineIndex: uint64(i),
			ManifestID: manifestIDs[i], SourceHref: manifestIDs[i] + ".xhtml", ResolvedHref: manifestIDs[i] + ".xhtml",
			Text: text, StartOffset: start, EndOffset: end, MediaType: "application/xhtml+xml", Linear: true,
		}
		start = end + 2
	}
	source, err := putAnalysisSourceWithUnits(ctx, h.store, h.owner, "gutenberg-selection", book.Title, fullText, "sha256:gutenberg-selection", units)
	require.NoError(t, err)
	require.NoError(t, h.store.LinkSourceToBook(ctx, h.owner, book.ID, source.ID))
	require.NoError(t, h.store.SetBookDisposition(ctx, h.owner, book.ID, domain.BookDispositionToRead))
	handle, err := h.service.SubmitToReadBookAnalysis(ctx, h.owner, book.ID, source.ID)
	require.NoError(t, err)
	status, err := h.service.Wait(ctx, h.owner, handle.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateCompleted, status.State, status.Error)
	assert.Equal(t, distributorConfigIdentity, h.runConfigIdentity(t, ctx, handle.RunID))

	// Simulate a Book published before Distributor selection existed.
	_, err = h.store.Pool().Exec(ctx, `UPDATE analysis_runs SET config_identity=$3 WHERE owner_id=$1 AND id=$2`, h.owner, handle.RunID, snapshotConfigIdentity)
	require.NoError(t, err)

	offers, err := h.service.AnalysisUpdateCandidates(ctx, h.owner, []string{source.ID, regularSource.ID})
	require.NoError(t, err)
	require.Contains(t, offers, source.ID, "the Gutenberg snapshot now selects differently")
	assert.Equal(t, handle.RunID, offers[source.ID].PublishedRunID)
	assert.NotContains(t, offers, regularSource.ID, "a non-Gutenberg snapshot keeps the default identity")
}

func (h analysisUpdateHarness) runConfigIdentity(t *testing.T, ctx context.Context, runID string) string {
	t.Helper()
	var identity string
	require.NoError(t, h.store.Pool().QueryRow(ctx, `SELECT config_identity FROM analysis_runs WHERE owner_id=$1 AND id=$2`, h.owner, runID).Scan(&identity))
	return identity
}

func TestAnalysisUpdateIsRefusedOutsideNonCurrentToReadBooks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	h := newAnalysisUpdateHarness(t, ctx, "analysis-update-eligibility")
	current, currentSource, currentRun := h.analyzedToReadBook(t, ctx, "current")
	inbox, inboxSource, inboxRun := h.analyzedToReadBook(t, ctx, "inbox")
	h.pipeline.set("2", "2")
	target := domain.AnalysisUpdateTarget{AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion, NormalizationProfile: "casefold", NormalizationVersion: "2"}.Key()

	_, err := h.store.StartCurrentReading(ctx, h.owner, "de", current.ID)
	require.NoError(t, err)
	_, err = h.service.SubmitAnalysisUpdate(ctx, h.owner, current.ID, currentSource.ID, currentRun.RunID, target)
	require.ErrorIs(t, err, ErrCurrentBook)
	require.NoError(t, h.store.MarkBookPreviouslyRead(ctx, h.owner, inbox.ID))
	_, err = h.service.SubmitAnalysisUpdate(ctx, h.owner, inbox.ID, inboxSource.ID, inboxRun.RunID, target)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = h.service.SubmitAnalysisUpdate(ctx, h.owner, current.ID, inboxSource.ID, inboxRun.RunID, target)
	require.Error(t, err, "a source of another Book is refused")
	assert.Equal(t, 1, h.runCount(t, ctx, currentSource.ID))
	assert.Equal(t, 1, h.runCount(t, ctx, inboxSource.ID))

	offers, err := h.service.AnalysisUpdateCandidates(ctx, h.owner, []string{currentSource.ID, inboxSource.ID})
	require.NoError(t, err)
	assert.Len(t, offers, 2, "offers are raw identity differences; the classification decides eligibility")
}
