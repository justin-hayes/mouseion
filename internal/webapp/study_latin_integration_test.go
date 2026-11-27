//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/study"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	latinStudyLemma      = "dico"
	latinStudyDictionary = "ls-study-pin-1"
)

func readyLatin() staticCapabilities {
	return staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "la", DisplayName: "Latin", Ready: true}}}}
}

// latinStudyLexicon is a synthetic Lewis & Short lexicon: two stacked entries
// under one lemma.
type latinStudyLexicon struct{}

func (latinStudyLexicon) Name() string    { return "latin-study-stub" }
func (latinStudyLexicon) Version() string { return "1" }
func (latinStudyLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (latinStudyLexicon) LatinLSPin() string { return latinStudyDictionary }

func (latinStudyLexicon) LookupLatinLS(_ context.Context, lemma string) ([]enrichment.LatinLSEntry, bool) {
	if lemma != latinStudyLemma {
		return nil, false
	}
	return []enrichment.LatinLSEntry{
		{EntryID: "n9", Headword: lemma, Homograph: 1, Pin: latinStudyDictionary, Senses: []enrichment.LatinLSSense{{ID: "n9.0", Level: 1, Definitions: []string{"to dedicate"}}}},
		{EntryID: "n10", Headword: lemma, Homograph: 2, Pin: latinStudyDictionary, Senses: []enrichment.LatinLSSense{{ID: "n10.0", Level: 1, Definitions: []string{"to say"}}}},
	}, true
}

// TestLatinStudyIntroducesAWordWithItsCitedEntry proves Latin Study is on in
// production (#2768): no override, the page offers the word, and taking it keeps
// the citation of the one stacked entry the pass cited.
func TestLatinStudyIntroducesAWordWithItsCitedEntry(t *testing.T) {
	s := newStudyHarnessWith(t, studyHarnessOptions{
		capabilities: readyLatin(),
		dictionary:   latinStudyLexicon{},
		evidence: func(store *persistence.PostgresStore) *study.SourceEvidence {
			return &study.SourceEvidence{Lexical: latinStudyLexicon{}, Translations: store, MaySend: func(string) error { return nil }}
		},
	})
	s.provider.citeLast = true
	s.seedBookIn("la", "gallic", testutil.CorpusWord{Lemma: latinStudyLemma, UPOS: "VERB", Occurrences: 3})
	require.Equal(t, http.StatusOK, getAs(t, s.h, "/la/study", s.cookies).Code)
	require.NoError(t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(t, "study River client", s.client.Stop)
	testutil.Eventually(t, 60*time.Second, "Latin material to settle", func() (bool, string) {
		counts, err := s.store.CountStudyMaterial(s.ctx, s.owner.ID, "la")
		require.NoError(t, err)
		return s.planJobs() == 0 && counts.Preparing == 0 && counts.Ready+counts.Unpreparable > 0, "the Latin material is still preparing"
	})

	material, err := s.store.ListCurrentStudyMaterial(s.ctx, s.owner.ID, "la")
	require.NoError(t, err)
	require.Len(t, material, 1)
	require.Equal(t, domain.StudyMaterialReady, material[0].State, "omission: %q", material[0].OmissionReason)
	assert.Equal(t, domain.StudyCitations{SenseIDs: []string{"n10.0"}, EntryIDs: []string{"n10"}, DictionaryPin: latinStudyDictionary}, material[0].Cited, "the second stacked entry is cited, not the first")

	page := getAs(t, s.h, "/la/study", s.cookies).Body.String()
	assert.NotContains(t, page, "Study isn't available for this language yet")
	assert.Contains(t, page, "Take these words")
	require.Equal(t, http.StatusSeeOther, s.post("/study/batches/take", url.Values{"language": {"la"}}).Code)
	progress, err := s.store.ListStudyProgress(s.ctx, s.owner.ID, "la")
	require.NoError(t, err)
	require.Len(t, progress, 1)
	assert.Equal(t, material[0].Cited, progress[0].LearnedCitations)
}
