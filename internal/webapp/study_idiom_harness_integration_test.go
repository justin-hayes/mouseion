//go:build integration

package webapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/study"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// The Study idiom harness. Tickets that test Idioms in Study drive it from any
// test file in this package:
//
//	lookup := newFakeIdiomLookup(germanIdiom("mit dem haus"))
//	h := newStudyHarnessWith(t, studyHarnessOptions{
//		capabilities: readyGerman(),
//		idioms:       germanIdiomAvoidance(t, lookup, true),
//	})
//	h.provider.answerIdioms("mit dem haus", idiomConfirmed)
//
// The switch is the production one (prepareddeck.IdiomSwitch) over a test-only
// assessment set that records German's idiom-card payload, so the ADR 0115 gate
// is exercised and the production set is not changed.

// fakeIdiomLookup is the dictionary's idiom lookup: it serves its entries by
// each of their component words, as the dictionary index does.
type fakeIdiomLookup struct {
	entries []idiom.Entry
}

func newFakeIdiomLookup(entries ...idiom.Entry) *fakeIdiomLookup {
	return &fakeIdiomLookup{entries: entries}
}

// IdiomEntries implements idiom.Index.
func (f *fakeIdiomLookup) IdiomEntries(_ context.Context, language, word string) ([]idiom.Entry, error) {
	var found []idiom.Entry
	for _, entry := range f.entries {
		if entry.Language != language {
			continue
		}
		for _, component := range entry.Words {
			if component == strings.ToLower(word) {
				found = append(found, entry)
				break
			}
		}
	}
	return found, nil
}

// germanIdiom is a dictionary entry whose headword is the space-separated
// words, with one sense a card can cite.
func germanIdiom(headword string) idiom.Entry {
	return idiom.Entry{
		Language: "de", Headword: headword, UPOS: "PHRASE", Words: strings.Fields(headword),
		Senses: []enrichment.LexicalSense{{Gloss: "the idiom " + headword}},
		Source: "test-dictionary", Version: "1",
	}
}

// germanIdiomAvoidance is the Study idiom switch for German over the lookup.
// With on false the operator has not enabled German, so Study selects exactly
// as it did before Idioms.
func germanIdiomAvoidance(t *testing.T, lookup idiom.Index, on bool) *study.IdiomAvoidance {
	t.Helper()
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assessments := enrichment.ProviderAssessmentSet{{
		Kind: enrichment.IdiomCardPayload, Language: "de", Provider: codec.ProviderName(),
		PromptVersion: codec.IdiomCardPromptContract(), Fields: enrichment.IdiomCardPayloadFields(),
		Disposition: "recorded for this test", Ticket: "#2751", Recorded: true,
	}}
	languages := map[string]bool{}
	if on {
		languages["de"] = true
	}
	return &study.IdiomAvoidance{Index: lookup, Enabled: prepareddeck.IdiomSwitch(languages, assessments, codec)}
}

// idiomAnswer is how the fake provider answers an idiom-card request.
type idiomAnswer string

const (
	// idiomConfirmed resolves the idiom: a Gloss citing the entry's first sense.
	idiomConfirmed idiomAnswer = "confirmed"
	// idiomLiteralUse says the sentence uses the words literally.
	idiomLiteralUse idiomAnswer = "literal_use"
	// idiomAmbiguous says no sense can be defended.
	idiomAmbiguous idiomAnswer = "ambiguous"
	// idiomFailure is a provider failure, which River retries.
	idiomFailure idiomAnswer = "failure"
)

// answerIdioms makes the provider answer the headword's idiom-card requests
// the given way. An unanswered headword is confirmed.
func (p *studyProvider) answerIdioms(headword string, answer idiomAnswer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.idiomAnswers == nil {
		p.idiomAnswers = map[string]idiomAnswer{}
	}
	p.idiomAnswers[headword] = answer
}

// idiomRequests are the idiom-card requests received so far.
func (p *studyProvider) idiomRequests() []enrichment.TranslationRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]enrichment.TranslationRequest(nil), p.idiom...)
}

func (p *studyProvider) answerIdiom(request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.idiom = append(p.idiom, request)
	answer, ok := p.idiomAnswers[request.IdiomHeadword]
	if !ok {
		answer = idiomConfirmed
	}
	switch answer {
	case idiomFailure:
		return enrichment.TranslationResponse{}, errors.New("provider unavailable")
	case idiomLiteralUse, idiomAmbiguous:
		kind := enrichment.ContextualGlossUnresolvedLiteralUse
		if answer == idiomAmbiguous {
			kind = enrichment.ContextualGlossUnresolvedAmbiguous
		}
		return enrichment.TranslationResponse{ContextOnly: true, UnresolvedKind: kind, UnresolvedReason: string(answer),
			SentenceTranslation: "Idiom: " + request.ExampleSentence, SentenceTranslationTargets: request.IdiomMatchedForms}, nil
	case idiomConfirmed:
	}
	response := enrichment.TranslationResponse{
		Translation: request.IdiomHeadword, Gloss: "the idiom " + request.IdiomHeadword,
		SentenceTranslation: "Idiom: " + request.ExampleSentence, SentenceTranslationTargets: request.IdiomMatchedForms,
	}
	if len(request.CandidateSenses) > 0 {
		response.EvidenceIDs = []string{request.CandidateSenses[0].EvidenceID}
	}
	return response, nil
}

// addSentenceWithPreposition adds one more sentence of "haus" to the Book's
// corpus whose preposition ("mit" in the seeded sentences) is the given
// three-letter word, so a headword such as "mit dem haus" matches the seeded
// sentences and not this one. It returns the sentence text.
func (s *studyHarness) addSentenceWithPreposition(key, preposition string) string {
	s.t.Helper()
	require.Len(s.t, preposition, 3, "a same-length word keeps every offset")
	sentence := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN"})
	sentence.Text = strings.Replace(sentence.Text, "mit", preposition, 1)
	sentence.Tokens[3].Surface, sentence.Tokens[3].RawLemma, sentence.Tokens[3].CanonicalLemma = preposition, preposition, preposition
	return s.appendSentence(key, sentence)
}

// appendSentence seeds the sentence after the Book's last one.
func (s *studyHarness) appendSentence(key string, sentence testutil.SeededSentence) string {
	s.t.Helper()
	var runID, corpusID string
	var ordinal, offset int64
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT analysis_run_id::text, corpus_id::text, max(sentence_ordinal)+1, max(end_offset)+1 FROM corpus_sentences WHERE owner_id=$1 GROUP BY analysis_run_id, corpus_id ORDER BY max(sentence_ordinal) DESC LIMIT 1`, s.owner.ID).Scan(&runID, &corpusID, &ordinal, &offset))
	testutil.SeedCorpusSentence(s.t, s.ctx, s.store.Pool(), s.owner.ID, corpusID, "de", runID, domain.EPUBUnitID(0, key), ordinal, offset, sentence)
	return sentence.Text
}
