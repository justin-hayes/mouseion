package study

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSourceEvidenceFluencyFixture is a mature Greek word whose Learned meaning
// cites sense n100.1 of entry n100 on pin lsj-pin-1, with two other occurrences.
func newSourceEvidenceFluencyFixture(t *testing.T) (*FluencyWorker, *fluencyFakeStore, *fluencyFakeEnricher, *greekLexicon) {
	t.Helper()
	worker, store, enricher := newFluencyFixture()
	store.progress.Language, store.progress.CanonicalLemma = "grc", "haus"
	store.material.Language = "grc"
	store.progress.LearnedCitations = domain.StudyCitations{SenseIDs: []string{"n100.1"}, EntryIDs: []string{"n100"}, DictionaryPin: "lsj-pin-1"}
	store.addOccurrence(2, "hausa", map[string]string{"Number": "Plur"})
	store.addOccurrence(3, "hausb", map[string]string{"Case": "Gen"})
	lexicon := &greekLexicon{entry: syntheticLSJ(), found: true}
	worker.Evidence = &SourceEvidence{Lexical: lexicon, Translations: &fakeTranslations{}, MaySend: func(string) error { return nil }}
	worker.MaySendAgreement = func(string) error { return nil }
	return worker, store, enricher, lexicon
}

func TestSourceEvidenceFluencyCitedSensesReachTheAgreementCall(t *testing.T) {
	worker, store, enricher, _ := newSourceEvidenceFluencyFixture(t)
	enricher.passFor = map[string]enrichment.EvidenceAssistedMeaning{
		"hausa": {Gloss: "an account", SentenceTranslation: "An account.", CitedSenseIDs: []string{"n100.2"}},
	}

	require.NoError(t, fluencyWork(worker))

	require.Len(t, enricher.passes, 2)
	require.Len(t, enricher.variant, 2, "every cited context gets the agreement check")
	var first variantRequest
	for _, request := range enricher.variant {
		if request.candidate.TargetWord == "hausa" {
			first = request
		}
	}
	assert.Equal(t, []string{"word"}, first.learned, "the Learned meaning's cited senses carry their definitions")
	assert.Equal(t, []string{"account"}, first.occurred)
	require.Len(t, store.recorded, 2)
	for _, record := range store.recorded {
		assert.NotEmpty(t, record.Cited.SenseIDs, "each stored context keeps its citations")
		assert.Equal(t, "lsj-pin-1", record.Cited.DictionaryPin)
		assert.Equal(t, domain.FluencyAgrees, record.Agreement)
	}
}

// newLatinFluencyFixture is a mature Latin word whose Learned meaning cites
// sense "s1" of entry n9, with stacked homographs n9 and n10 that both hold a
// sense "s1" (Lewis & Short sense IDs are only unique within an entry).
func newLatinFluencyFixture(t *testing.T) (*FluencyWorker, *fluencyFakeStore, *fluencyFakeEnricher) {
	t.Helper()
	worker, store, enricher := newFluencyFixture()
	store.progress.Language, store.progress.CanonicalLemma = "la", "dico"
	store.material.Language = "la"
	store.progress.LearnedCitations = domain.StudyCitations{SenseIDs: []string{"s1"}, EntryIDs: []string{"n9"}, DictionaryPin: "ls-pin-1"}
	store.addOccurrence(2, "dicoa", map[string]string{"Number": "Plur"})
	store.addOccurrence(3, "dicob", map[string]string{"Case": "Gen"})
	entries := stackedDico()
	entries[0].Senses[0].ID, entries[1].Senses[0].ID = "s1", "s1"
	worker.Evidence = &SourceEvidence{Lexical: &latinLexicon{entries: entries}, Translations: &fakeTranslations{}, MaySend: func(string) error { return nil }}
	worker.MaySendAgreement = func(string) error { return nil }
	return worker, store, enricher
}

func TestLatinFluencyContextCitingAnotherHomographIsADifferentMeaning(t *testing.T) {
	worker, store, enricher := newLatinFluencyFixture(t)
	other := enrichment.EvidenceAssistedMeaning{Gloss: "to say", SentenceTranslation: "He says.", CitedSenseIDs: []string{enrichment.LatinEvidenceID("n10", "s1")}}
	enricher.passFor = map[string]enrichment.EvidenceAssistedMeaning{"dicoa": other, "dicob": other}

	require.NoError(t, fluencyWork(worker))

	assert.Empty(t, enricher.variant, "a different entry on the same pin makes no agreement call, though the sense ID is shared")
	require.Len(t, store.recorded, 2)
	for _, record := range store.recorded {
		assert.Equal(t, domain.FluencyReasonDifferentMeaning, record.Reason)
		assert.Equal(t, domain.FluencyDisagrees, record.Agreement)
		assert.Equal(t, domain.StudyCitations{SenseIDs: []string{"s1"}, EntryIDs: []string{"n10"}, DictionaryPin: "ls-pin-1"}, record.Cited)
	}
}

func TestLatinFluencyContextCitingTheLearnedHomographGoesToTheAgreementCall(t *testing.T) {
	worker, store, enricher := newLatinFluencyFixture(t)
	same := enrichment.EvidenceAssistedMeaning{Gloss: "to dedicate", SentenceTranslation: "He dedicates.", CitedSenseIDs: []string{enrichment.LatinEvidenceID("n9", "s1")}}
	enricher.passFor = map[string]enrichment.EvidenceAssistedMeaning{"dicoa": same, "dicob": same}

	require.NoError(t, fluencyWork(worker))

	require.Len(t, enricher.variant, 2)
	learned := []string{"to dedicate"}
	for _, request := range enricher.variant {
		assert.Equal(t, learned, request.learned, "the Learned meaning's sense is the learned homograph's, not the other's")
		assert.Equal(t, learned, request.occurred)
	}
	require.Len(t, store.recorded, 2)
	for _, record := range store.recorded {
		assert.Equal(t, []string{"n9"}, record.Cited.EntryIDs)
		assert.Equal(t, domain.FluencyAgrees, record.Agreement)
	}
}

func TestLatinFluencyRejectsACitationOfASenseTheNamedEntryDoesNotHold(t *testing.T) {
	worker, store, enricher := newLatinFluencyFixture(t)
	bad := enrichment.EvidenceAssistedMeaning{Gloss: "to say", SentenceTranslation: "He says.", CitedSenseIDs: []string{enrichment.LatinEvidenceID("n9", "nope")}}
	enricher.passFor = map[string]enrichment.EvidenceAssistedMeaning{"dicoa": bad, "dicob": bad}

	require.NoError(t, fluencyWork(worker))

	assert.Empty(t, enricher.variant)
	require.Len(t, store.recorded, 2)
	for _, record := range store.recorded {
		assert.Equal(t, domain.FluencyReasonUndefendable, record.Reason)
		assert.True(t, record.Cited.Empty())
	}
}

func TestSourceEvidenceFluencyADifferentPinGoesToTheAgreementCheck(t *testing.T) {
	worker, store, enricher, lexicon := newSourceEvidenceFluencyFixture(t)
	lexicon.entry.EntryID = "n200"
	store.progress.LearnedCitations.DictionaryPin = "lsj-pin-0"

	require.NoError(t, fluencyWork(worker))

	assert.Len(t, enricher.variant, 2, "an entry mismatch across pins is not a rejection")
	require.Len(t, store.recorded, 2)
	assert.Equal(t, domain.FluencyReasonSameMeaning, store.recorded[0].Reason)
}

func TestSourceEvidenceFluencyAnUncitedContextIsUndefendable(t *testing.T) {
	cases := map[string]enrichment.EvidenceAssistedMeaning{
		"context only":                   {Gloss: "a word", SentenceTranslation: "A translation.", ContextOnly: true},
		"abstention":                     {SentenceTranslation: "A translation.", ContextOnly: true, UnresolvedReason: "no sense fits"},
		"invalid citation":               {InvalidCitation: true},
		"invalid citation with its text": {InvalidCitation: true, Gloss: "a word", SentenceTranslation: "A translation."},
		"unknown sense":                  {Gloss: "a word", SentenceTranslation: "A translation.", CitedSenseIDs: []string{"n999.9"}},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			worker, store, enricher, _ := newSourceEvidenceFluencyFixture(t)
			enricher.passFor = map[string]enrichment.EvidenceAssistedMeaning{"hausa": answer, "hausb": answer}

			require.NoError(t, fluencyWork(worker))

			assert.Empty(t, enricher.variant, "nothing to compare, so no agreement call")
			require.Len(t, store.recorded, 2)
			for _, record := range store.recorded {
				assert.Equal(t, domain.FluencyReasonUndefendable, record.Reason)
				assert.Equal(t, domain.FluencyDisagrees, record.Agreement)
				assert.True(t, record.Cited.Empty())
				assert.Equal(t, answer.Gloss, record.Gloss, "whatever Gloss came back is kept")
				assert.Equal(t, answer.SentenceTranslation, record.SentenceTranslation)
			}
		})
	}
}

func TestSourceEvidenceFluencySendsNothingWhileEitherAssessmentIsNotRecorded(t *testing.T) {
	gates := map[string]func(*FluencyWorker){
		"variant": func(w *FluencyWorker) {
			w.MaySendAgreement = func(string) error { return enrichment.ErrProviderAssessmentNotRecorded }
		},
		"pass": func(w *FluencyWorker) {
			w.Evidence.MaySend = func(string) error { return enrichment.ErrProviderAssessmentNotRecorded }
		},
		"no evidence": func(w *FluencyWorker) { w.Evidence = nil },
		"no gate":     func(w *FluencyWorker) { w.MaySendAgreement = nil },
	}
	for name, gate := range gates {
		t.Run(name, func(t *testing.T) {
			worker, store, enricher, _ := newSourceEvidenceFluencyFixture(t)
			gate(worker)

			require.NoError(t, fluencyWork(worker))
			assert.Empty(t, enricher.passes, "no pass call either")
			assert.Empty(t, enricher.variant)
			assert.Empty(t, enricher.requests)
			assert.Empty(t, store.recorded)
		})
	}
}

func TestSourceEvidenceFluencyWaitsForAMissingDictionary(t *testing.T) {
	worker, store, enricher, _ := newSourceEvidenceFluencyFixture(t)
	worker.Evidence.Lexical = nil
	assert.True(t, isSnooze(fluencyWork(worker)), "a missing dictionary waits; it is never an answer")
	worker.Evidence.Lexical = &greekLexicon{}
	assert.True(t, isSnooze(fluencyWork(worker)), "a dictionary with no entry yet waits too")
	assert.Empty(t, enricher.passes)
	assert.Empty(t, store.recorded)
}

func TestSourceEvidenceFluencyDoesNotLookUpLearnedSensesOnAnotherPin(t *testing.T) {
	worker, store, enricher, _ := newSourceEvidenceFluencyFixture(t)
	store.progress.LearnedCitations = domain.StudyCitations{SenseIDs: []string{"n100.2"}, EntryIDs: []string{"n100"}, DictionaryPin: "lsj-pin-0"}

	require.NoError(t, fluencyWork(worker))

	require.NotEmpty(t, enricher.variant, "a dictionary pin change never stops the agreement check")
	assert.Empty(t, enricher.variant[0].learned, "an older pin's sense ID may name another sense now: no definition is guessed, and no ID is sent")
	assert.NotEmpty(t, enricher.variant[0].occurred)
	require.NotEmpty(t, store.recorded, "the answer is recorded, not retried forever")
	assert.Equal(t, domain.FluencyAgrees, store.recorded[0].Agreement)
}

func TestSourceEvidenceFluencyNeedsTheLearnedMeaningsCitations(t *testing.T) {
	worker, store, enricher, _ := newSourceEvidenceFluencyFixture(t)
	store.progress.LearnedCitations = domain.StudyCitations{}

	require.NoError(t, fluencyWork(worker))
	assert.Empty(t, enricher.passes)
	assert.Empty(t, store.recorded)
}

// onePageStore lists every occurrence on the first concordance page, so a test
// can offer more candidates than the page bound reads one by one.
type onePageStore struct{ *fluencyFakeStore }

func (s onePageStore) ListVocabularyConcordance(context.Context, string, string, domain.ConcordanceLookup) (domain.ConcordanceResult, error) {
	return domain.ConcordanceResult{Occurrences: s.occurrences}, nil
}

func TestSourceEvidenceFluencyCountsOnePassAndCheckAsOneAsk(t *testing.T) {
	worker, store, enricher, _ := newSourceEvidenceFluencyFixture(t)
	for ordinal := int64(4); ordinal < 4+maxFluencyAsks*2; ordinal++ {
		store.addOccurrence(ordinal, "haus"+string(rune('m'+ordinal%13))+string(rune('a'+ordinal/13)), map[string]string{"Case": "Case" + string(rune('A'+ordinal))})
	}
	// Every context disagrees, so only the ask bound stops the job.
	for _, item := range store.occurrences {
		enricher.disagree[item.Surface] = true
	}

	worker.Store = onePageStore{store}

	require.NoError(t, fluencyWork(worker))
	assert.Len(t, enricher.passes, maxFluencyAsks)
	assert.Len(t, enricher.variant, maxFluencyAsks, "a pass and its check are one ask")
}

func TestSourceEvidenceFluencyProviderFailureIsRetriedNotRecorded(t *testing.T) {
	worker, store, enricher, _ := newSourceEvidenceFluencyFixture(t)
	enricher.err = errors.New("provider unavailable")
	err := fluencyWork(worker)
	require.Error(t, err)
	assert.False(t, isSnooze(err))
	assert.Empty(t, store.recorded)

	enricher.err = enrichment.ErrExternalNotConfigured
	assert.True(t, isSnooze(fluencyWork(worker)))
}
