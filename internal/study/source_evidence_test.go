package study

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sourceevidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const greekText = "ἄνθρωπος ὁρᾷ τὸν λόγον καὶ τρέχει ταχέως ἐκ τῆς πόλεως."

// greekLexicon is a synthetic LSJ lexicon: one entry for the lemma "λόγος".
type greekLexicon struct {
	entry enrichment.AncientGreekLSJEntry
	found bool
	calls int
	pin   string
	// homographs, when set, are every entry the Study lookup returns for any lemma.
	homographs []enrichment.AncientGreekLSJEntry
}

func (l *greekLexicon) Name() string    { return "stub" }
func (l *greekLexicon) Version() string { return "stub-v1" }
func (l *greekLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}

// AncientGreekLSJPin is the pin of the loaded index; it is empty when pin is.
func (l *greekLexicon) AncientGreekLSJPin() string { return l.pin }

func (l *greekLexicon) LookupAncientGreekLSJ(context.Context, string) (enrichment.AncientGreekLSJEntry, bool) {
	l.calls++
	return l.entry, l.found
}

func (l *greekLexicon) LookupAncientGreekLSJEntries(context.Context, string) ([]enrichment.AncientGreekLSJEntry, bool) {
	l.calls++
	if len(l.homographs) > 0 {
		return l.homographs, true
	}
	return []enrichment.AncientGreekLSJEntry{l.entry}, l.found
}

func syntheticLSJ() enrichment.AncientGreekLSJEntry {
	return enrichment.AncientGreekLSJEntry{
		EntryID: "n100", Headword: "λόγος", Pin: "lsj-pin-1",
		Senses: []enrichment.AncientGreekLSJSense{
			{ID: "n100.1", Number: "I", Level: 1, Definitions: []string{"word"}},
			{ID: "n100.2", Number: "II", Level: 1, Definitions: []string{"account"}},
		},
	}
}

type fakeTranslations struct {
	translations map[string]string
	pin          string
	err          error
	books        []string
}

func (f *fakeTranslations) BookReferenceTranslations(_ context.Context, _, bookID string) (map[string]string, string, error) {
	f.books = append(f.books, bookID)
	return f.translations, f.pin, f.err
}

type greekFixture struct {
	store    *fakeStore
	enricher *fakeEnricher
	lexicon  *greekLexicon
	trans    *fakeTranslations
	worker   *MaterialWorker
	gate     error
}

func newGreekFixture(t *testing.T) *greekFixture {
	t.Helper()
	f := &greekFixture{
		lexicon:  &greekLexicon{entry: syntheticLSJ(), found: true},
		trans:    &fakeTranslations{translations: map[string]string{"1.2": "The man sees the word."}, pin: "sha256:abc"},
		enricher: &fakeEnricher{pass: enrichment.EvidenceAssistedMeaning{Gloss: "a word", SentenceTranslation: "The man sees the word.", SentenceTranslationTargets: []string{"word"}, CitedSenseIDs: []string{enrichment.EntryEvidenceID("n100", "n100.1")}}},
	}
	f.store = &fakeStore{
		occurrences: []domain.ConcordanceResultOccurrence{{
			Surface: "λόγον", CanonicalLemma: "λόγοσ", UPOS: "NOUN", SentenceText: greekText,
			BookID: otherBook, AnalysisRunID: "run-1", CorpusID: "corpus-1", SentenceOrdinal: 3, BookStartOffset: 300,
		}},
		citations: []string{"1.2"},
	}
	f.worker = &MaterialWorker{Store: f.store, Enricher: f.enricher, Evidence: &SourceEvidence{
		Lexical: f.lexicon, Translations: f.trans,
		MaySend: func(string) error { return f.gate },
	}}
	f.store.material = domain.StudyMaterial{ID: "m1", Language: "grc", CanonicalLemma: "λόγοσ", UPOS: "NOUN", State: domain.StudyMaterialPending}
	return f
}

func TestSourceEvidenceWordIsReadyWithItsCitationsPinAndPassage(t *testing.T) {
	f := newGreekFixture(t)
	require.NoError(t, work(f.worker))

	require.NotNil(t, f.store.ready, "the cited Gloss is recorded")
	ready := f.store.ready
	assert.Equal(t, "a word", ready.Gloss)
	assert.Equal(t, "The man sees the word.", ready.SentenceTranslation)
	assert.Equal(t, domain.StudyCitations{SenseIDs: []string{"n100.1"}, EntryIDs: []string{"n100"}, DictionaryPin: "lsj-pin-1"}, ready.Cited)
	assert.Equal(t, "The man sees the word.", ready.Passage.Text)
	assert.Equal(t, "sha256:abc", ready.Passage.TranslationPin)
	assert.NotEmpty(t, ready.Passage.Citation)
	assert.Empty(t, f.store.unpreparable)

	require.Len(t, f.enricher.passRequests, 1)
	request := f.enricher.passRequests[0]
	assert.Equal(t, "λόγον", request.TargetWord)
	assert.Equal(t, greekText, request.ExampleSentence)
	assert.Equal(t, []enrichment.LexicalSense{
		{EvidenceID: enrichment.EntryEvidenceID("n100", "n100.1"), Source: "lsj", Gloss: "word"},
		{EvidenceID: enrichment.EntryEvidenceID("n100", "n100.2"), Source: "lsj", Gloss: "account"},
	}, request.CandidateSenses, "the pass cites the pinned dictionary senses by their entry and sense")
	assert.Equal(t, []string{"The man sees the word."}, f.enricher.passages)
	assert.Empty(t, f.enricher.requests, "the contextual-Gloss call is the modern path's")
	assert.Equal(t, []string{otherBook}, f.trans.books, "the passage comes from the sentence's Book")
}

func TestSourceEvidenceWordWithoutATranslationHasNoPassage(t *testing.T) {
	f := newGreekFixture(t)
	f.trans.translations, f.trans.pin = nil, ""
	require.NoError(t, work(f.worker))
	require.NotNil(t, f.store.ready)
	assert.True(t, f.store.ready.Passage.Empty(), "the passage columns are empty when no passage applies")
	assert.Equal(t, []string{""}, f.enricher.passages)
}

func TestSourceEvidenceOmitsAMeaningTheEvidenceCannotDefend(t *testing.T) {
	cases := map[string]enrichment.EvidenceAssistedMeaning{
		"context only":              {Gloss: "a word", SentenceTranslation: "The man sees the word.", ContextOnly: true},
		"abstention":                {SentenceTranslation: "The man sees the word.", ContextOnly: true, UnresolvedReason: "no sense fits"},
		"empty Gloss":               {SentenceTranslation: "The man sees the word.", CitedSenseIDs: []string{enrichment.EntryEvidenceID("n100", "n100.1")}},
		"no translation":            {Gloss: "a word", CitedSenseIDs: []string{enrichment.EntryEvidenceID("n100", "n100.1")}},
		"provider invalid citation": {InvalidCitation: true},
		"invalid citation":          {Gloss: "a word", SentenceTranslation: "The man sees the word.", CitedSenseIDs: []string{enrichment.EntryEvidenceID("n100", "n999.9")}},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			f := newGreekFixture(t)
			f.enricher.pass = answer
			require.NoError(t, work(f.worker))
			assert.Equal(t, domain.StudyOmissionMeaning, f.store.unpreparable)
			assert.Nil(t, f.store.ready)
			assert.Len(t, f.enricher.passRequests, 1)
		})
	}
}

func TestSourceEvidenceWordWithNoDictionaryEntryAsksNoProvider(t *testing.T) {
	f := newGreekFixture(t)
	f.lexicon.found, f.lexicon.pin = false, " lsj-pin-1 "
	require.NoError(t, work(f.worker))
	assert.Equal(t, domain.StudyOmissionNoDictionaryEntry, f.store.unpreparable)
	assert.Equal(t, "lsj-pin-1", f.store.omissionPin, "the omission records the dictionary pin it was made against")
	assert.Empty(t, f.enricher.passRequests, "no entry, no provider call")
	assert.Empty(t, f.enricher.requests)
	assert.Nil(t, f.store.ready)
}

func TestSourceEvidenceWordWaitsWhenNoDictionaryIsConfigured(t *testing.T) {
	f := newGreekFixture(t)
	f.worker.Evidence.Lexical = nil
	err := work(f.worker)
	require.True(t, isSnooze(err), "a missing dictionary waits; it is never a permanent 'no entry': %v", err)
	assert.Empty(t, f.store.unpreparable)
	assert.Empty(t, f.enricher.passRequests)

	f.worker.Evidence = nil
	require.True(t, isSnooze(work(f.worker)), "a worker with no source-evidence dependencies waits")
	assert.Empty(t, f.store.unpreparable)
}

func TestSourceEvidenceWordSendsNothingWhileTheAssessmentIsNotRecorded(t *testing.T) {
	f := newGreekFixture(t)
	f.gate = enrichment.ErrProviderAssessmentNotRecorded
	err := work(f.worker)
	require.True(t, isSnooze(err), "a missing or stale assessment waits: %v", err)
	assert.Empty(t, f.enricher.passRequests, "nothing is sent")
	assert.Zero(t, f.lexicon.calls, "the gate comes before the dictionary")
	assert.Empty(t, f.store.unpreparable)
	assert.Nil(t, f.store.ready)
}

func TestSourceEvidenceWordWaitsWhenTheLoadedDictionaryLacksTheSource(t *testing.T) {
	f := newGreekFixture(t)
	f.worker.Evidence.Available = func(cardpolicy.Policy) bool { return false }
	require.True(t, isSnooze(work(f.worker)))
	assert.Empty(t, f.store.unpreparable, "a missing LSJ table is not a 'no entry' answer")
	assert.Zero(t, f.lexicon.calls)
}

func TestSourceEvidenceProviderFailureIsNeverRecorded(t *testing.T) {
	f := newGreekFixture(t)
	f.enricher.passErr = errors.New("provider unavailable")
	require.Error(t, work(f.worker))
	assert.Empty(t, f.store.evidence)
	assert.Empty(t, f.store.unpreparable)
	assert.Nil(t, f.store.ready)

	f.enricher.passErr = enrichment.ErrExternalNotConfigured
	require.True(t, isSnooze(work(f.worker)), "a missing provider waits")
	assert.Empty(t, f.store.unpreparable)
}

func TestSourceEvidenceWordWithNoUsableSentenceIsASentenceQualityOmission(t *testing.T) {
	f := newGreekFixture(t)
	f.store.occurrences[0].SentenceText = "λόγον."
	require.NoError(t, work(f.worker))
	assert.Equal(t, domain.StudyOmissionSentenceQuality, f.store.unpreparable)
	assert.Empty(t, f.enricher.passRequests)
}

func TestSourceEvidencePassageStoreFailureIsRetried(t *testing.T) {
	f := newGreekFixture(t)
	f.trans.err = errors.New("database unavailable")
	require.Error(t, work(f.worker))
	assert.Empty(t, f.store.unpreparable)
	assert.Empty(t, f.enricher.passRequests)
}

func TestSourceEvidenceCitesEntriesInOrderAndDropsNothingValid(t *testing.T) {
	resolution := resolutionOf(syntheticLSJ())
	refs, ok := citedSenses(resolution, []string{enrichment.EntryEvidenceID("n100", "n100.2"), enrichment.EntryEvidenceID("n100", "n100.1"), enrichment.EntryEvidenceID("n100", "n100.2")})
	require.True(t, ok)
	cited := citationsOf(resolution, refs)
	assert.Equal(t, []string{"n100.1", "n100.2"}, cited.SenseIDs, "senses follow the entry's source order, once each")
	assert.Equal(t, []string{"n100", "n100"}, cited.EntryIDs, "each sense names its entry")
	assert.Equal(t, "lsj-pin-1", cited.DictionaryPin)
	_, ok = citedSenses(resolution, []string{"nope"})
	assert.False(t, ok)
	_, ok = citedSenses(resolution, []string{enrichment.EntryEvidenceID("n100", "n100.1"), "nope"})
	assert.False(t, ok, "one unknown ID invalidates the citations")
	assert.True(t, citationsOf(resolution, nil).Empty())
}

func TestModernWordNeverUsesTheSourceEvidencePath(t *testing.T) {
	store := &fakeStore{occurrences: []domain.ConcordanceResultOccurrence{occurrence(otherBook, goodText, 1)}}
	enricher := &fakeEnricher{meaning: resolved()}
	worker := newWorker(store, enricher)
	require.NoError(t, work(worker))
	require.NotNil(t, store.ready)
	assert.True(t, store.ready.Cited.Empty(), "a modern Learned meaning is a Gloss alone")
	assert.True(t, store.ready.Passage.Empty())
	assert.Empty(t, enricher.passRequests)
}

func TestTheStudySwitchIsOnForGreekAndLatinInProduction(t *testing.T) {
	assert.True(t, cardpolicy.For("grc").Study)
	assert.True(t, cardpolicy.For("la").Study)
}

func resolutionOf(entry enrichment.AncientGreekLSJEntry) sourceevidence.Resolution {
	resolution, _ := sourceevidence.ResolveStudy(context.Background(), &greekLexicon{entry: entry, found: true}, cardpolicy.For("grc"), "λόγοσ", nil)
	return resolution
}

type latinLexicon struct {
	greekLexicon
	entries []enrichment.LatinLSEntry
}

func (l *latinLexicon) LookupLatinLS(context.Context, string) ([]enrichment.LatinLSEntry, bool) {
	return l.entries, len(l.entries) > 0
}

func stackedDico() []enrichment.LatinLSEntry {
	return []enrichment.LatinLSEntry{
		{EntryID: "n9", Headword: "dico", Pin: "ls-pin-1", Senses: []enrichment.LatinLSSense{{ID: "n9.0", Level: 1, Definitions: []string{"to dedicate"}}}},
		{EntryID: "n10", Headword: "dico", Pin: "ls-pin-1", Senses: []enrichment.LatinLSSense{{ID: "n10.0", Level: 1, Definitions: []string{"to say"}}}},
	}
}

func latinFixture(t *testing.T, cited ...string) *greekFixture {
	t.Helper()
	f := newGreekFixture(t)
	f.store.material = domain.StudyMaterial{ID: "m1", Language: "la", CanonicalLemma: "dico", UPOS: "VERB", State: domain.StudyMaterialPending}
	f.enricher.pass.CitedSenseIDs = cited
	f.worker.Evidence.Lexical = &latinLexicon{entries: stackedDico()}
	return f
}

func TestLatinStudySendsEntryCarryingSensesAndResolvesACitationToOneEntry(t *testing.T) {
	f := latinFixture(t, enrichment.EntryEvidenceID("n10", "n10.0"))
	require.NoError(t, work(f.worker))
	require.Len(t, f.enricher.passRequests, 1)
	assert.Equal(t, []string{enrichment.EntryEvidenceID("n9", "n9.0"), enrichment.EntryEvidenceID("n10", "n10.0")}, evidenceIDsOf(f.enricher.passRequests[0].CandidateSenses))
	require.NotNil(t, f.store.ready)
	assert.Equal(t, domain.StudyCitations{SenseIDs: []string{"n10.0"}, EntryIDs: []string{"n10"}, DictionaryPin: "ls-pin-1"}, f.store.ready.Cited)
}

func TestLatinStudySendsEverySenseAndAcceptsOnePastTheOldCap(t *testing.T) {
	entries := stackedDico()
	for i := 1; i < 34; i++ {
		entries[1].Senses = append(entries[1].Senses, enrichment.LatinLSSense{ID: fmt.Sprintf("n10.%d", i), Level: 1, Definitions: []string{"to say"}})
	}
	f := latinFixture(t, enrichment.EntryEvidenceID("n10", "n10.33"))
	f.worker.Evidence.Lexical = &latinLexicon{entries: entries}
	require.NoError(t, work(f.worker))
	require.Len(t, f.enricher.passRequests, 1)
	sent := evidenceIDsOf(f.enricher.passRequests[0].CandidateSenses)
	require.Len(t, sent, 35, "every sense of every stacked entry, past enrichment.DefaultMaxCandidateSenses")
	assert.Equal(t, enrichment.EntryEvidenceID("n9", "n9.0"), sent[0], "entry order")
	require.NotNil(t, f.store.ready, "a sense past the old cap is ready material")
	assert.Equal(t, domain.StudyCitations{SenseIDs: []string{"n10.33"}, EntryIDs: []string{"n10"}, DictionaryPin: "ls-pin-1"}, f.store.ready.Cited)
}

func TestLatinStudyOmitsACitationOfASenseTheNamedEntryDoesNotHold(t *testing.T) {
	f := latinFixture(t, enrichment.EntryEvidenceID("n9", "n10.0"))
	require.NoError(t, work(f.worker))
	assert.Nil(t, f.store.ready)
	assert.Equal(t, domain.StudyOmissionMeaning, f.store.unpreparable)
}

func TestLatinStudySendsNothingWhileTheLatinPassIsNotRecorded(t *testing.T) {
	f := latinFixture(t, enrichment.EntryEvidenceID("n9", "n9.0"))
	f.gate = enrichment.ErrProviderAssessmentNotRecorded
	require.Error(t, work(f.worker), "the word waits")
	assert.Empty(t, f.enricher.passRequests)
	assert.Nil(t, f.store.ready)
}

func evidenceIDsOf(senses []enrichment.LexicalSense) []string {
	ids := make([]string, 0, len(senses))
	for _, sense := range senses {
		ids = append(ids, sense.EvidenceID)
	}
	return ids
}

func TestLatinStudyRecordsTheCitedEntryWhenHomographsShareASenseID(t *testing.T) {
	entries := stackedDico()
	entries[1].Senses[0].ID = entries[0].Senses[0].ID
	f := latinFixture(t, enrichment.EntryEvidenceID("n10", "n9.0"))
	f.worker.Evidence.Lexical = &latinLexicon{entries: entries}
	require.NoError(t, work(f.worker))
	require.NotNil(t, f.store.ready)
	assert.Equal(t, []string{"n9.0"}, f.store.ready.Cited.SenseIDs)
	assert.Equal(t, []string{"n10"}, f.store.ready.Cited.EntryIDs)
}

func TestDictionaryPinIsTheLoadedIndexsPinForThePoliciesDictionary(t *testing.T) {
	lexicon := &greekLexicon{pin: " lsj-pin-1 "}
	assert.Equal(t, "lsj-pin-1", DictionaryPin(lexicon, cardpolicy.For("grc")))
	assert.Empty(t, DictionaryPin(lexicon, cardpolicy.For("la")), "an index that does not say has no pin")
	assert.Empty(t, DictionaryPin(lexicon, cardpolicy.For("de")), "a modern language has no dictionary")
	assert.Empty(t, DictionaryPin(nil, cardpolicy.For("grc")))
}

type pinnedLatinLexicon struct{ greekLexicon }

func (*pinnedLatinLexicon) LatinLSPin() string { return " ls-pin-1 " }

func TestDictionaryPinOfLatinIsTheLewisAndShortPin(t *testing.T) {
	assert.Equal(t, "ls-pin-1", DictionaryPin(&pinnedLatinLexicon{}, cardpolicy.For("la")))
	assert.Empty(t, DictionaryPin(&pinnedLatinLexicon{}, cardpolicy.For("grc")), "Greek reads LSJ's pin, which this index does not give")
}

func TestSourceEvidenceWordStillWithoutAnEntryIsRecordedAgainstTheNewPin(t *testing.T) {
	f := newGreekFixture(t)
	f.lexicon.found, f.lexicon.pin = false, "lsj-pin-1"
	require.NoError(t, work(f.worker))
	require.Equal(t, "lsj-pin-1", f.store.omissionPin)

	f.lexicon.pin = "lsj-pin-2"
	require.NoError(t, work(f.worker))
	assert.Equal(t, "lsj-pin-2", f.store.omissionPin, "a retry that still finds no entry is unpreparable under the new pin")
	assert.Empty(t, f.enricher.passRequests)
}

func TestSourceEvidenceNoEntryOmissionIsNotRecordedWhenTheAttemptCannotBeRecorded(t *testing.T) {
	f := newGreekFixture(t)
	f.lexicon.found = false
	f.store.occurrencesErr = errors.New("database down")
	require.ErrorContains(t, work(f.worker), "database down")
	assert.Empty(t, f.store.unpreparable, "River retries the job; nothing is recorded")
}

func twoLegoEntries() []enrichment.AncientGreekLSJEntry {
	return []enrichment.AncientGreekLSJEntry{
		{EntryID: "n1", Headword: "λέγω", Pin: "lsj-pin-1", Senses: []enrichment.AncientGreekLSJSense{{ID: "s0", Number: "A", Level: 1, Definitions: []string{"lay"}}}},
		{EntryID: "n2", Headword: "λέγω", Pin: "lsj-pin-1", Senses: []enrichment.AncientGreekLSJSense{{ID: "s0", Number: "A", Level: 1, Definitions: []string{"say"}}}},
	}
}

// TestGreekStudyOfAnLSJHomographIsReadyWithTheCitedEntry proves a lemma with two
// LSJ entries is no longer a "no dictionary entry" omission: the pass is offered
// both entries' senses, and a cited sense, even one whose native ID the entries
// share, records the entry that was cited.
func TestGreekStudyOfAnLSJHomographIsReadyWithTheCitedEntry(t *testing.T) {
	f := newGreekFixture(t)
	f.lexicon.homographs = twoLegoEntries()
	f.store.material.CanonicalLemma = "λέγω"
	f.enricher.pass.CitedSenseIDs = []string{enrichment.EntryEvidenceID("n2", "s0")}
	require.NoError(t, work(f.worker))

	assert.Empty(t, f.store.unpreparable)
	require.NotNil(t, f.store.ready)
	assert.Equal(t, domain.StudyCitations{SenseIDs: []string{"s0"}, EntryIDs: []string{"n2"}, DictionaryPin: "lsj-pin-1"}, f.store.ready.Cited)
	require.Len(t, f.enricher.passRequests, 1)
	assert.Equal(t, []string{enrichment.EntryEvidenceID("n1", "s0"), enrichment.EntryEvidenceID("n2", "s0")}, evidenceIDsOf(f.enricher.passRequests[0].CandidateSenses))
}

func TestGreekStudyOmitsACitationOfAnotherEntrysSense(t *testing.T) {
	f := newGreekFixture(t)
	f.lexicon.homographs = twoLegoEntries()
	f.lexicon.homographs[1].Senses[0].ID = "s1"
	f.enricher.pass.CitedSenseIDs = []string{enrichment.EntryEvidenceID("n1", "s1")}
	require.NoError(t, work(f.worker))
	assert.Nil(t, f.store.ready)
	assert.Equal(t, domain.StudyOmissionMeaning, f.store.unpreparable)
}

func TestGreekStudySendsNothingWhileTheGreekEntryPassIsNotRecorded(t *testing.T) {
	f := newGreekFixture(t)
	f.lexicon.homographs = twoLegoEntries()
	f.gate = enrichment.ErrProviderAssessmentNotRecorded
	require.True(t, isSnooze(work(f.worker)), "the word waits")
	assert.Empty(t, f.enricher.passRequests)
	assert.Empty(t, f.store.unpreparable)
}
