package study

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// idiomSentence is a candidate sentence of "mit dem haus": the seeded sentence
// whose preposition is the given three-letter word, in the book at the ordinal.
func idiomSentence(book string, ordinal int64, preposition string, inReading bool) persistence.StudyIdiomSentence {
	seeded := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN"})
	seeded.Text = strings.Replace(seeded.Text, "mit", preposition, 1)
	seeded.Tokens[3].Surface, seeded.Tokens[3].RawLemma, seeded.Tokens[3].CanonicalLemma = preposition, preposition, preposition
	return persistence.StudyIdiomSentence{
		BookID: book, AnalysisRunID: "run-" + book, CorpusID: "corpus-" + book, SentenceOrdinal: ordinal,
		InCurrentReading: inReading, BookStartOffset: ordinal * 100,
		Sentence:  analyzer.Sentence{Text: seeded.Text, Tokens: seeded.Tokens},
		Effective: seeded.Tokens,
	}
}

func newIdiomWorker(store *fakeStore, enricher *fakeEnricher) *MaterialWorker {
	worker := newWorker(store, enricher)
	store.material = domain.StudyMaterial{ID: "m1", StudyItem: domain.IdiomItem("de", "mit dem haus"), State: domain.StudyMaterialPending}
	worker.Idioms = switchOn(&fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}})
	return worker
}

func confirmedIdiom() enrichment.ContextualMeaning {
	return enrichment.ContextualMeaning{Gloss: "at home", SentenceTranslation: "At home it is old.", SentenceTranslationTargets: []string{"at home"}}
}

func TestWorkIdiomStoresTheConfirmedGlossAndTheMatchedWords(t *testing.T) {
	store := &fakeStore{idiomSentences: []persistence.StudyIdiomSentence{idiomSentence(otherBook, 1, "mit", false), idiomSentence(readingBook, 2, "mit", true)}}
	enricher := &fakeEnricher{idiom: confirmedIdiom()}
	require.NoError(t, work(newIdiomWorker(store, enricher)))

	require.NotNil(t, store.ready)
	assert.Equal(t, readingBook, store.ready.BookID, "the Current reading's sentence wins")
	assert.Equal(t, int64(2), store.ready.SentenceOrdinal)
	assert.Equal(t, "at home", store.ready.Gloss)
	assert.Equal(t, "mit dem haus", strings.ToLower(store.ready.TargetWord), "the matched surface forms, joined by a space")
	assert.Equal(t, []int{3, 4, 5}, store.ready.MatchedTokenOrdinals)
	assert.Equal(t, int64(3), store.ready.TokenOrdinal, "the first matched word")
	require.Len(t, enricher.idiomCards, 1, "one request")
	assert.Equal(t, "mit dem haus", enricher.idiomCards[0].Headword)
	assert.Len(t, enricher.idiomCards[0].MatchedForms, 3)
	assert.Empty(t, enricher.requests, "the word contract is not used")
}

func TestWorkIdiomSkipsRetiredSentencesAndSentencesTheMatcherRejects(t *testing.T) {
	retired := idiomSentence(readingBook, 1, "mit", true)
	other := idiomSentence(otherBook, 2, "mit", false)
	notAnIdiom := idiomSentence(readingBook, 3, "auf", true)
	store := &fakeStore{
		idiomSentences: []persistence.StudyIdiomSentence{retired, notAnIdiom, other},
		retired:        []domain.StudyOccurrenceRef{{BookID: readingBook, AnalysisRunID: "run-" + readingBook, CorpusID: "corpus-" + readingBook, SentenceOrdinal: 1}},
	}
	require.NoError(t, work(newIdiomWorker(store, &fakeEnricher{idiom: confirmedIdiom()})))
	require.NotNil(t, store.ready)
	assert.Equal(t, otherBook, store.ready.BookID)
}

func TestWorkIdiomMatchesOnEffectiveLemmas(t *testing.T) {
	sentence := idiomSentence(otherBook, 1, "mit", false)
	// The persisted lemma of "haus" says another word; a correction made it "haus".
	sentence.Sentence.Tokens = append([]analyzer.Token(nil), sentence.Effective...)
	sentence.Sentence.Tokens[5].CanonicalLemma, sentence.Sentence.Tokens[5].Surface = "x", "x"
	sentence.Effective = append([]analyzer.Token(nil), sentence.Effective...)
	sentence.Effective[5].CanonicalLemma = "haus"
	store := &fakeStore{idiomSentences: []persistence.StudyIdiomSentence{sentence}}
	require.NoError(t, work(newIdiomWorker(store, &fakeEnricher{idiom: confirmedIdiom()})))
	require.NotNil(t, store.ready, "the corrected lemma makes the candidate")
}

func TestWorkIdiomRecordsItsReasonWhenTheProviderDoesNotConfirm(t *testing.T) {
	for name, tc := range map[string]struct {
		kind   enrichment.ContextualGlossUnresolvedKind
		reason string
	}{
		"literal use": {enrichment.ContextualGlossUnresolvedLiteralUse, domain.StudyOmissionLiteralUse},
		"ambiguous":   {enrichment.ContextualGlossUnresolvedAmbiguous, domain.StudyOmissionMeaning},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{idiomSentences: []persistence.StudyIdiomSentence{idiomSentence(otherBook, 1, "mit", false)}}
			enricher := &fakeEnricher{idiom: enrichment.ContextualMeaning{SentenceTranslation: "At home.", UnresolvedKind: tc.kind, UnresolvedReason: "no"}}
			require.NoError(t, work(newIdiomWorker(store, enricher)))
			assert.Equal(t, tc.reason, store.unpreparable)
			assert.Nil(t, store.ready)
		})
	}
}

func TestWorkIdiomWithNoUsableSentenceCallsNoProvider(t *testing.T) {
	for name, sentences := range map[string][]persistence.StudyIdiomSentence{
		"none":            nil,
		"matcher rejects": {idiomSentence(otherBook, 1, "auf", false)},
		"only a fragment": {func() persistence.StudyIdiomSentence {
			s := idiomSentence(otherBook, 1, "mit", false)
			s.Sentence.Text = "Haus."
			return s
		}()},
		"all quality fail": {func() persistence.StudyIdiomSentence {
			s := idiomSentence(otherBook, 1, "mit", false)
			s.Sentence.Text = "x"
			return s
		}()},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{idiomSentences: sentences}
			enricher := &fakeEnricher{idiom: confirmedIdiom()}
			require.NoError(t, work(newIdiomWorker(store, enricher)))
			assert.Equal(t, domain.StudyOmissionSentenceQuality, store.unpreparable)
			assert.Empty(t, enricher.idiomCards)
		})
	}
}

func TestWorkIdiomNeverRecordsAProviderOmission(t *testing.T) {
	store := &fakeStore{idiomSentences: []persistence.StudyIdiomSentence{idiomSentence(otherBook, 1, "mit", false)}}
	require.Error(t, work(newIdiomWorker(store, &fakeEnricher{idiomErr: errors.New("provider unavailable")})))
	assert.Empty(t, store.unpreparable)
	assert.Nil(t, store.ready)

	err := work(newIdiomWorker(store, &fakeEnricher{idiomErr: enrichment.ErrExternalNotConfigured}))
	var snooze *rivertype.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
	assert.Empty(t, store.unpreparable)
}

func TestWorkIdiomPreparesNothingWhileTheSwitchIsOff(t *testing.T) {
	for name, idioms := range map[string]*IdiomAvoidance{
		"no switch":         nil,
		"language disabled": {Index: &fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}}, Enabled: func(string) bool { return false }},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{idiomSentences: []persistence.StudyIdiomSentence{idiomSentence(otherBook, 1, "mit", false)}}
			enricher := &fakeEnricher{idiom: confirmedIdiom()}
			worker := newIdiomWorker(store, enricher)
			worker.Idioms = idioms
			require.NoError(t, worker.Work(context.Background(), &river.Job[PrepareMaterialArgs]{Args: PrepareMaterialArgs{OwnerID: "owner", MaterialID: "m1"}}))
			assert.Nil(t, store.ready)
			assert.Empty(t, store.unpreparable)
			assert.Empty(t, enricher.idiomCards)
		})
	}
}

func TestWorkIdiomTriesTheNextSentenceWhenTheBestIsLiteral(t *testing.T) {
	store := &fakeStore{idiomSentences: []persistence.StudyIdiomSentence{idiomSentence(otherBook, 1, "mit", false), idiomSentence(otherBook, 2, "mit", false)}}
	enricher := &fakeEnricher{idiom: enrichment.ContextualMeaning{SentenceTranslation: "x", UnresolvedKind: enrichment.ContextualGlossUnresolvedLiteralUse, UnresolvedReason: "no"}}
	require.NoError(t, work(newIdiomWorker(store, enricher)))
	assert.Len(t, enricher.idiomCards, 2, "every candidate sentence is asked before the Idiom is given up")
	assert.Equal(t, domain.StudyOmissionLiteralUse, store.unpreparable)
}
