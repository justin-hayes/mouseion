//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/study"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	greekStudyLemma      = "λόγοσ"
	greekStudyDictionary = "lsj-study-pin-1"
	// greekStudyTranslate is the reviewed Brownson file digest, so the register
	// credits the passage.
	greekStudyTranslate = "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7"
)

func readyGreek() staticCapabilities {
	return staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "grc", DisplayName: "Ancient Greek", Ready: true}}}}
}

// greekStudyLexicon is a synthetic LSJ lexicon: one entry for the study lemma.
// pin is its dictionary revision, which a test changes to move the pin.
type greekStudyLexicon struct{ pin *string }

func (greekStudyLexicon) Name() string    { return "greek-study-stub" }
func (greekStudyLexicon) Version() string { return "1" }
func (greekStudyLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}

func (l greekStudyLexicon) LookupAncientGreekLSJ(_ context.Context, lemma string) (enrichment.AncientGreekLSJEntry, bool) {
	if lemma != greekStudyLemma {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	return enrichment.AncientGreekLSJEntry{
		EntryID: "lsj-logos", Headword: lemma, Pin: *l.pin,
		Senses: []enrichment.AncientGreekLSJSense{
			{ID: "lsj-logos.0", Number: "A", Level: 1, Definitions: []string{"word", "speech"}},
			{ID: "lsj-logos.1", Number: "B", Level: 1, Definitions: []string{"account"}},
		},
	}, true
}

type greekStudyHarness struct {
	*studyHarness
	dictionaryPin *string
}

// newGreekStudyHarness serves Ancient Greek Study with its switch on for the
// test only (cardpolicy.OverrideStudyForTest); production leaves it off.
func newGreekStudyHarness(t *testing.T) *greekStudyHarness {
	t.Helper()
	t.Cleanup(cardpolicy.OverrideStudyForTest("grc", true))
	pin := greekStudyDictionary
	h := newStudyHarnessWith(t, studyHarnessOptions{
		capabilities: readyGreek(),
		dictionary:   greekStudyLexicon{pin: &pin},
		evidence: func(store *persistence.PostgresStore) *study.SourceEvidence {
			return &study.SourceEvidence{Lexical: greekStudyLexicon{pin: &pin}, Translations: store, MaySend: func(string) error { return nil }}
		},
	})
	return &greekStudyHarness{studyHarness: h, dictionaryPin: &pin}
}

// seedGreekBook gives the learner an analyzed Greek Book whose sentences cite
// section 1.2.17, with a pinned translation of it imported for the Book.
func (g *greekStudyHarness) seedGreekBook(words ...testutil.CorpusWord) {
	g.t.Helper()
	const key = "anabasis"
	book := g.seedBookIn("grc", key, words...)
	_, err := g.store.Pool().Exec(g.ctx, `UPDATE corpus_sentences SET citations = ARRAY['1.2.17'] WHERE owner_id=$1 AND analysis_run_id=(SELECT analysis_run_id FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2)`, g.owner.ID, book.ID)
	require.NoError(g.t, err)

	definition := classical.Definition{
		Key: "greek-study-" + key, WorkKey: "greek-study-work-" + key, WorkTitle: "Greek Study Work", WorkAuthor: "Author",
		EditionKey: "greek-study-edition-" + key, EditionLabel: "Edition", Language: classical.Language,
		ConstituentBooks: 1, CitationStart: "1.2.17", CitationEnd: "1.2.17", WordCount: 1, Opening: "ἄρχεται ὁ λόγος.",
		Source:                classical.SourcePin{Layer: "greek-study-layer", FileName: "study.conllu", Size: 10, SHA256: strings.Repeat("c", 64), Format: classical.SourceFormatOGACoNLLU},
		Translation:           &classical.TranslationPin{Layer: "greek-study-translation", FileName: "study.xml", Size: 10, SHA256: greekStudyTranslate, CitationStart: "1.2.17", CitationEnd: "1.2.17"},
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
	require.NoError(g.t, g.store.ReconcileClassicalManifest(g.ctx, []classical.Definition{definition}))
	_, err = g.store.Pool().Exec(g.ctx, `INSERT INTO book_reading_units(owner_id, book_id, reading_unit_id) SELECT $1, $2, id FROM reading_units WHERE definition_key=$3`, g.owner.ID, book.ID, definition.Key)
	require.NoError(g.t, err)
	_, err = g.store.PutReferenceTranslations(g.ctx, greekStudyTranslate, "grc", map[string]string{"1.2.17": "He halted his chariot in front of the phalanx."})
	require.NoError(g.t, err)
}

func (g *greekStudyHarness) greekMaterial() []domain.StudyMaterial {
	g.t.Helper()
	material, err := g.store.ListCurrentStudyMaterial(g.ctx, g.owner.ID, "grc")
	require.NoError(g.t, err)
	return material
}

func (g *greekStudyHarness) greekProgress() []domain.StudyProgress {
	g.t.Helper()
	progress, err := g.store.ListStudyProgress(g.ctx, g.owner.ID, "grc")
	require.NoError(g.t, err)
	return progress
}

// prepareGreek opens the page and runs the Study jobs until every word settles.
func (g *greekStudyHarness) prepareGreek() {
	g.t.Helper()
	require.Equal(g.t, http.StatusOK, getAs(g.t, g.h, "/grc/study", g.cookies).Code)
	require.NoError(g.t, g.client.Start(g.ctx))
	testutil.StopOnCleanup(g.t, "study River client", g.client.Stop)
	testutil.Eventually(g.t, 60*time.Second, "Greek material to settle", func() (bool, string) {
		counts, err := g.store.CountStudyMaterial(g.ctx, g.owner.ID, "grc")
		require.NoError(g.t, err)
		return g.planJobs() == 0 && counts.Preparing == 0 && counts.Ready+counts.Unpreparable > 0, "the Greek material is still preparing"
	})
}

func TestGreekStudyIntroducesAWordWithItsCitedMeaning(t *testing.T) {
	g := newGreekStudyHarness(t)
	g.seedGreekBook(testutil.CorpusWord{Lemma: greekStudyLemma, UPOS: "NOUN", Occurrences: 3})
	g.prepareGreek()

	material := g.greekMaterial()
	require.Len(t, material, 1)
	ready := material[0]
	require.Equal(t, domain.StudyMaterialReady, ready.State, "omission: %q", ready.OmissionReason)
	assert.Equal(t, "word (gloss)", ready.Gloss)
	assert.Equal(t, "He sees the word.", ready.SentenceTranslation)
	assert.Equal(t, domain.StudyCitations{SenseIDs: []string{"lsj-logos.0"}, EntryIDs: []string{"lsj-logos"}, DictionaryPin: greekStudyDictionary}, ready.Cited)
	assert.Equal(t, "He halted his chariot in front of the phalanx.", ready.Passage.Text)
	assert.Equal(t, "1.2.17", ready.Passage.Citation)
	assert.Equal(t, "sha256:"+greekStudyTranslate, ready.Passage.TranslationPin)

	requests := g.provider.passRequests()
	require.Len(t, requests, 1, "the evidence-assisted pass is the only provider call")
	assert.True(t, requests[0].RequireEvidenceAssistedPass)
	assert.Equal(t, "He halted his chariot in front of the phalanx.", requests[0].PassageContext)
	require.Len(t, requests[0].CandidateSenses, 2)
	assert.Equal(t, "lsj-logos.0", requests[0].CandidateSenses[0].EvidenceID)

	// Today offers the word; taking it introduces it with its citations.
	page := getAs(t, g.h, "/grc/study", g.cookies).Body.String()
	assert.Contains(t, page, "Take these words")
	response := g.post("/study/batches/take", url.Values{"language": {"grc"}})
	require.Equal(t, http.StatusSeeOther, response.Code)
	progress := g.greekProgress()
	require.Len(t, progress, 1)
	assert.Equal(t, "word (gloss)", progress[0].LearnedMeaning)
	assert.Equal(t, ready.Cited, progress[0].LearnedCitations, "the citations are copied at introduction")

	answer := getAs(t, g.h, "/grc/study?answer=1", g.cookies).Body.String()
	assert.Contains(t, answer, "word (gloss)", "the Meaning row")
	assert.Contains(t, answer, "He sees the word.", "the Translation row")
	assert.Contains(t, answer, `id="study-headword"`, "the Word row")

	assertGreekCardBack(t, answer)

	// A dictionary revision change never re-introduces the word: the pin stays
	// with the Learned meaning.
	*g.dictionaryPin = "lsj-study-pin-2"
	getAs(t, g.h, "/grc/study", g.cookies)
	again := g.greekProgress()
	require.Len(t, again, 1)
	assert.Equal(t, greekStudyDictionary, again[0].LearnedCitations.DictionaryPin)
	assert.False(t, again[0].Reintroducing)
	assert.Equal(t, progress[0].MaterialID, again[0].MaterialID)
}

func TestGreekStudyUndoRestoresTheLearnedMeaningsCitations(t *testing.T) {
	g := newGreekStudyHarness(t)
	g.seedGreekBook(testutil.CorpusWord{Lemma: greekStudyLemma, UPOS: "NOUN", Occurrences: 3})
	g.prepareGreek()
	require.Equal(t, http.StatusSeeOther, g.post("/study/batches/take", url.Values{"language": {"grc"}}).Code)
	word := g.greekProgress()[0]
	require.NotEmpty(t, word.LearnedCitations.SenseIDs)

	// Report the context as unsuitable; the new dictionary revision is used for
	// the replacement material.
	*g.dictionaryPin = "lsj-study-pin-2"
	form := url.Values{"language": {"grc"}, "progress": {word.ID}, "reps": {strconv.FormatInt(word.Memory.Reps, 10)}, "material": {word.MaterialID}, "kind": {"unsuitable_context"}, "note": {""}}
	require.Equal(t, http.StatusSeeOther, g.post("/study/reports", form).Code)
	reports, err := g.store.ListOpenStudyReports(g.ctx, g.owner.ID, "grc", domain.StudyReportUnsuitableContext)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, word.LearnedCitations, reports[0].PreviousLearnedCitations, "the report keeps what Undo restores")

	testutil.Eventually(t, 60*time.Second, "the word to be re-introduced", func() (bool, string) {
		getAs(t, g.h, "/grc/study", g.cookies)
		return g.greekProgress()[0].MaterialID != word.MaterialID, "the word is still waiting for new material"
	})
	moved := g.greekProgress()[0]
	assert.Equal(t, "lsj-study-pin-2", moved.LearnedCitations.DictionaryPin, "re-anchoring copies the new material's citations")

	require.Equal(t, http.StatusSeeOther, g.post("/study/reports/"+reports[0].ID+"/undo", url.Values{"language": {"grc"}}).Code)
	restored := g.greekProgress()
	require.Len(t, restored, 1)
	assert.Equal(t, word.MaterialID, restored[0].MaterialID, "the word returns to its material")
	assert.Equal(t, word.LearnedMeaning, restored[0].LearnedMeaning)
	assert.Equal(t, word.LearnedCitations, restored[0].LearnedCitations, "and to the citations and pin it had")
}

func TestStudyStaysUnavailableForGreekAndLatinInProduction(t *testing.T) {
	s := newStudyHarness(t)
	for _, language := range []string{"la", "grc"} {
		book, err := s.store.CreateBook(s.ctx, domain.Book{OwnerID: s.owner.ID, Title: "Book in " + language, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
		require.NoError(t, err)
		require.NoError(t, s.store.SetBookDisposition(s.ctx, s.owner.ID, book.ID, domain.BookDispositionToRead))
		page := getAs(t, s.h, "/"+language+"/study", s.cookies)
		require.Equal(t, http.StatusOK, page.Code, language)
		assert.Contains(t, page.Body.String(), "Study isn't available for this language yet", language)
	}
	assert.Zero(t, s.provider.Calls())
}

// assertGreekCardBack checks the revealed card of the Greek word: the usual
// rows, then the collapsed Dictionary and Passage context sections, and the
// source notices on the page.
func assertGreekCardBack(t *testing.T, page string) {
	t.Helper()
	assert.Regexp(t, `<details class="study-dictionary" id="study-dictionary"><summary>Dictionary</summary>`, page, "the Dictionary section starts collapsed")
	assert.Regexp(t, `<details class="study-passage" id="study-passage"><summary>Passage context</summary>`, page, "the Passage context section starts collapsed")
	assert.Contains(t, page, "Passage context: Brownson (1922), Xen. Anab. 1.2.17. Not a translation of this sentence")
	assert.Contains(t, page, "He halted his chariot in front of the phalanx.")
	assert.Equal(t, 1, strings.Count(page, "study-sense--cited"), "only the cited sense is marked")
	assert.Regexp(t, `study-sense study-sense--cited"[^>]*>.*?word; speech`, page)
	assert.NotContains(t, page, "provider-proposed")
	assert.Contains(t, page, cardpolicy.GreekSourceNotice)
	assert.Contains(t, page, "PerseusDL/lexica@"+greekStudyDictionary)
	assert.Contains(t, page, greekStudyTranslate, "the translation notice carries its pin")
}

func TestGreekStudyCardShowsTheOutlineWithoutMarksWhenThePinMoved(t *testing.T) {
	g := newGreekStudyHarness(t)
	g.seedGreekBook(testutil.CorpusWord{Lemma: greekStudyLemma, UPOS: "NOUN", Occurrences: 3})
	g.prepareGreek()
	require.Equal(t, http.StatusSeeOther, g.post("/study/batches/take", url.Values{"language": {"grc"}}).Code)

	*g.dictionaryPin = "lsj-study-pin-2"
	page := getAs(t, g.h, "/grc/study?answer=1", g.cookies).Body.String()
	assert.Contains(t, page, "account", "the outline is shown")
	assert.NotContains(t, page, "study-sense--cited", "no sense is marked against another revision")
	assert.Contains(t, page, "PerseusDL/lexica@lsj-study-pin-2")
}

func TestGreekStudyCardHasNoPassageSectionWithoutATranslation(t *testing.T) {
	g := newGreekStudyHarness(t)
	g.seedGreekBook(testutil.CorpusWord{Lemma: greekStudyLemma, UPOS: "NOUN", Occurrences: 3})
	_, err := g.store.Pool().Exec(g.ctx, `DELETE FROM book_reading_units`)
	require.NoError(t, err)
	g.prepareGreek()
	require.Equal(t, http.StatusSeeOther, g.post("/study/batches/take", url.Values{"language": {"grc"}}).Code)

	page := getAs(t, g.h, "/grc/study?answer=1", g.cookies).Body.String()
	assert.Contains(t, page, `id="study-dictionary"`)
	assert.NotContains(t, page, "study-passage")
	assert.Contains(t, page, cardpolicy.GreekSourceNotice, "the page still shows the source notices")
}
