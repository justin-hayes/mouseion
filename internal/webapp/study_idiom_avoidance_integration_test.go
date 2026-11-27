//go:build integration

package webapp

import (
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// insideSentence is the seeded sentence of "haus"; the entry "mit dem haus"
	// makes it an Idiom candidate.
	insideSentence = "Anna ist nicht mit dem haus."
	// outsideSentence swaps the preposition, so it is outside that candidate, and
	// is weaker: its deictic "jetzt" scores lower than the seeded sentence.
	outsideSentence = "Anna ist jetzt auf dem haus."
)

func newIdiomStudyHarness(t *testing.T, headword string, on bool) *studyHarness {
	t.Helper()
	return newStudyHarnessWith(t, studyHarnessOptions{
		capabilities: readyGerman(),
		idioms:       germanIdiomAvoidance(t, newFakeIdiomLookup(germanIdiom(headword)), on),
	})
}

// preparedSentence prepares the Study material and returns the sentence the
// word "haus" was prepared with.
func (s *studyHarness) preparedSentence() string {
	s.t.Helper()
	require.Equal(s.t, http.StatusOK, getAs(s.t, s.h, "/de/study", s.cookies).Code)
	require.NoError(s.t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(s.t, "study River client", s.client.Stop)
	testutil.Eventually(s.t, 60*time.Second, "material to settle", func() (bool, string) { return s.settled() })
	material, err := s.store.ListCurrentStudyMaterial(s.ctx, s.owner.ID, "de")
	require.NoError(s.t, err)
	for _, item := range material {
		if item.CanonicalLemma == "haus" {
			require.Equal(s.t, domain.StudyMaterialReady, item.State)
			return item.Sentence
		}
	}
	s.t.Fatal("no Study material for haus")
	return ""
}

func TestStudyMaterialPrefersASentenceOutsideAnIdiomCandidate(t *testing.T) {
	s := newIdiomStudyHarness(t, "mit dem haus", true)
	s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 1})
	assert.Equal(t, outsideSentence, s.addSentenceWithPreposition("reading", "auf", true))

	assert.Equal(t, outsideSentence, s.preparedSentence(), "the sentence outside the candidate wins though the one inside comes first")
}

func TestStudyMaterialKeepsItsBestSentenceWhenEverySentenceIsInsideACandidate(t *testing.T) {
	s := newIdiomStudyHarness(t, "mit dem haus", true)
	s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 2})

	assert.Equal(t, insideSentence, s.preparedSentence())
}

func TestStudyMaterialIsUnchangedWhileTheIdiomSwitchIsOff(t *testing.T) {
	s := newIdiomStudyHarness(t, "mit dem haus", false)
	s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 1})
	s.addSentenceWithPreposition("reading", "auf", true)

	assert.Equal(t, insideSentence, s.preparedSentence())
}

func TestStudyFluencyPrefersOccurrencesOutsideAnIdiomCandidate(t *testing.T) {
	// "haeuser" is the only form the entry matches; Lena's sentence holds it and
	// would otherwise be the first one asked about.
	for _, tc := range []struct {
		name     string
		on       bool
		asksLena bool
	}{
		{name: "switch on", on: true, asksLena: false},
		{name: "switch off", on: false, asksLena: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newIdiomStudyHarness(t, "nicht mit dem haeuser", tc.on)
			// Lena's sentence, the idiom candidate, scores higher than every
			// other form's.
			s.weakerFluencyForms = map[string]bool{}
			for _, form := range fluencyForms {
				s.weakerFluencyForms[form.surface] = form.surface != "haeuser"
			}
			s.matureHaus()
			s.openFluency()

			targets := map[string]bool{}
			for _, item := range s.fluencyContexts() {
				targets[item.TargetWord] = true
			}
			assert.Equal(t, tc.asksLena, targets["haeuser"])
			assert.Len(t, targets, domain.FluencyContextsPerWord)
		})
	}
}

// The fake idiom answers are what later Idiom tickets drive; this pins what
// each one sends back.
func TestStudyHarnessFakeIdiomAnswers(t *testing.T) {
	provider := &studyProvider{FakeTranslationProvider: testutil.NewFakeTranslationProvider()}
	ask := func(headword string) (enrichment.TranslationResponse, error) {
		return provider.Translate(t.Context(), enrichment.TranslationRequest{
			Language: "de", TargetLanguage: "en", CanonicalLemma: headword, RequireIdiomCard: true,
			IdiomHeadword: headword, IdiomMatchedForms: []string{"mit", "dem", "haus"}, ExampleSentence: insideSentence,
			CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "sense"}},
		})
	}

	confirmed, err := ask("mit dem haus")
	require.NoError(t, err, "an unanswered headword is confirmed")
	assert.NotEmpty(t, confirmed.Gloss)
	assert.Equal(t, []string{"e1"}, confirmed.EvidenceIDs)

	provider.answerIdioms("mit dem haus", idiomLiteralUse)
	literal, err := ask("mit dem haus")
	require.NoError(t, err)
	assert.Empty(t, literal.Gloss)
	assert.Equal(t, enrichment.ContextualGlossUnresolvedLiteralUse, literal.UnresolvedKind)

	provider.answerIdioms("mit dem haus", idiomAmbiguous)
	ambiguous, err := ask("mit dem haus")
	require.NoError(t, err)
	assert.Equal(t, enrichment.ContextualGlossUnresolvedAmbiguous, ambiguous.UnresolvedKind)

	provider.answerIdioms("mit dem haus", idiomFailure)
	_, err = ask("mit dem haus")
	require.Error(t, err)

	assert.Len(t, provider.idiomRequests(), 4)
}

// The acceptance tests rely on the weakened sentence scoring below the seeded
// one; this pins that, so a change to the sentence-quality rules cannot make
// them pass vacuously.
func TestWeakenedSentenceScoresBelowTheSeededSentence(t *testing.T) {
	score := func(sentence testutil.SeededSentence) float64 {
		quality := sentencequality.ScoreSentenceQuality("de", sentence.Text, "haus", 0, sentence.Tokens, 0)
		require.True(t, quality.Accepted)
		return quality.GDEXScore + float64(quality.Score)
	}
	seeded := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN"})
	weaker := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN"})
	weakenSentence(&weaker)
	assert.Greater(t, score(seeded), score(weaker))
}
