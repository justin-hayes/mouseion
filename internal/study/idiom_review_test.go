package study

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errReviewStore = errors.New("store failed")

// reviewFakeStore is idiomReviewStore over fixed answers, recording what is
// written. failing names the one method that returns errReviewStore.
type reviewFakeStore struct {
	candidates []domain.StudyCandidate
	pending    int64
	material   domain.StudyMaterial
	current    []domain.StudyMaterial
	tokens     map[int64][]analyzer.Token
	refs       []persistence.StudyIdiomSentenceRef

	failing    string
	missing    string
	superseded []string
	withdrawn  []bool
	restored   int
}

func (s *reviewFakeStore) fail(method string) error {
	switch s.failing {
	case method:
		return errReviewStore
	}
	if s.missing == method {
		return domain.ErrNotFound
	}
	return nil
}

func (s *reviewFakeStore) ListStudyIdiomCandidates(context.Context, string, string, string) ([]domain.StudyCandidate, int64, error) {
	return s.candidates, s.pending, s.fail("candidates")
}

func (s *reviewFakeStore) GetStudyMaterial(context.Context, string, string) (domain.StudyMaterial, error) {
	return s.material, s.fail("material")
}

func (s *reviewFakeStore) ListCurrentStudyMaterial(context.Context, string, string) ([]domain.StudyMaterial, error) {
	return s.current, s.fail("current")
}

func (s *reviewFakeStore) ListStudyEffectiveTokens(context.Context, string, string, string, string, []int64) (map[int64][]analyzer.Token, error) {
	return s.tokens, s.fail("tokens")
}

func (s *reviewFakeStore) SupersedeStudyMaterial(_ context.Context, _, id string) error {
	s.superseded = append(s.superseded, id)
	return s.fail("supersede")
}

func (s *reviewFakeStore) SetStudyWordWithdrawn(_ context.Context, _, _ string, withdrawn bool, _ time.Time) error {
	s.withdrawn = append(s.withdrawn, withdrawn)
	return s.fail("withdraw")
}

func (s *reviewFakeStore) ListStudyIdiomSentenceRefs(context.Context, string, string, string, []string) ([]persistence.StudyIdiomSentenceRef, error) {
	return s.refs, s.fail("refs")
}

func (s *reviewFakeStore) RestoreStudyMaterial(context.Context, string, domain.StudyProgress) error {
	s.restored++
	return s.fail("restore")
}

var reviewNow = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

func reviewedIdiom() domain.StudyProgress {
	return domain.StudyProgress{ID: "p1", StudyItem: domain.IdiomItem("de", "mit dem haus"), MaterialID: "m1"}
}

// newReviewFixture is an Idiom with a candidate, introduced in a sentence that
// still holds it.
func newReviewFixture() *reviewFakeStore {
	sentence := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN"})
	return &reviewFakeStore{
		candidates: []domain.StudyCandidate{{StudyItem: domain.IdiomItem("de", "mit dem haus")}},
		material: domain.StudyMaterial{
			ID: "m1", StudyItem: domain.IdiomItem("de", "mit dem haus"), State: domain.StudyMaterialReady,
			BookID: "b1", AnalysisRunID: "r1", CorpusID: "c1", SentenceOrdinal: 4,
		},
		tokens: map[int64][]analyzer.Token{4: sentence.Tokens},
		refs:   []persistence.StudyIdiomSentenceRef{{BookID: "b1", AnalysisRunID: "r1", CorpusID: "c1", SentenceOrdinal: 4}},
	}
}

func reviewAvoidance() *IdiomAvoidance {
	return switchOn(&fakeIdiomIndex{entries: []idiom.Entry{mitDemHaus()}})
}

func review(store *reviewFakeStore, word domain.StudyProgress, active bool) (bool, error) {
	return reviewIdiom(context.Background(), store, reviewAvoidance(), active, "owner", word, reviewNow)
}

func TestReviewIdiomLeavesAWordAndAnIdiomWithoutAnIndexAlone(t *testing.T) {
	store := newReviewFixture()
	t.Run("a word is not an Idiom", func(t *testing.T) {
		changed, err := review(store, domain.StudyProgress{ID: "w", StudyItem: domain.WordItem("de", "haus", "NOUN", "")}, true)
		require.NoError(t, err)
		require.False(t, changed)
	})
	t.Run("a service without an idiom index changes nothing", func(t *testing.T) {
		service := &Service{}
		changed, err := service.ReviewIdiom(t.Context(), "owner", reviewedIdiom(), reviewNow)
		require.NoError(t, err)
		require.False(t, changed)
	})
}

func TestReviewIdiomWithdrawsAnIdiomWithNoCandidateLeft(t *testing.T) {
	t.Run("it is withdrawn", func(t *testing.T) {
		store := newReviewFixture()
		store.candidates = nil
		changed, err := review(store, reviewedIdiom(), true)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []bool{true}, store.withdrawn)
	})
	t.Run("one already withdrawn stays as it is", func(t *testing.T) {
		store := newReviewFixture()
		store.candidates = nil
		word := reviewedIdiom()
		word.Withdrawn = true
		changed, err := review(store, word, true)
		require.NoError(t, err)
		require.False(t, changed)
		require.Empty(t, store.withdrawn)
	})
	t.Run("a Book whose counts are not ready is not counted as having none", func(t *testing.T) {
		store := newReviewFixture()
		store.candidates, store.pending = nil, 1
		changed, err := review(store, reviewedIdiom(), true)
		require.NoError(t, err)
		require.False(t, changed)
		require.Empty(t, store.withdrawn)
	})
	t.Run("a missing progress is not an error", func(t *testing.T) {
		store := newReviewFixture()
		store.candidates, store.missing = nil, "withdraw"
		_, err := review(store, reviewedIdiom(), true)
		require.NoError(t, err)
	})
}

func TestReviewIdiomKeepsItsReviewsWhileTheSwitchIsOff(t *testing.T) {
	store := newReviewFixture()
	store.tokens = map[int64][]analyzer.Token{4: {{Surface: "x", CanonicalLemma: "x"}}}
	changed, err := review(store, reviewedIdiom(), false)
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, store.superseded, "no replacement is started that could not be prepared")
}

func TestReviewIdiomReintroducesAnIdiomWhoseIntroducingSentenceNoLongerMatches(t *testing.T) {
	t.Run("the sentence still holds the headword", func(t *testing.T) {
		store := newReviewFixture()
		changed, err := review(store, reviewedIdiom(), true)
		require.NoError(t, err)
		require.False(t, changed)
		require.Empty(t, store.superseded)
	})
	t.Run("the sentence no longer holds the headword", func(t *testing.T) {
		store := newReviewFixture()
		store.tokens = map[int64][]analyzer.Token{4: {{Surface: "anna", CanonicalLemma: "anna"}}}
		changed, err := review(store, reviewedIdiom(), true)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []string{"m1"}, store.superseded)
	})
	t.Run("the sentence is no longer in a current analysis", func(t *testing.T) {
		store := newReviewFixture()
		store.refs = nil
		changed, err := review(store, reviewedIdiom(), true)
		require.NoError(t, err)
		require.True(t, changed)
	})
	t.Run("the sentence is gone", func(t *testing.T) {
		store := newReviewFixture()
		store.tokens = map[int64][]analyzer.Token{}
		changed, err := review(store, reviewedIdiom(), true)
		require.NoError(t, err)
		require.True(t, changed)
	})
	t.Run("material that is missing is left alone", func(t *testing.T) {
		store := newReviewFixture()
		store.missing = "material"
		changed, err := review(store, reviewedIdiom(), true)
		require.NoError(t, err)
		require.False(t, changed)
	})
	t.Run("a refresh that dropped the headword waits for the counts", func(t *testing.T) {
		store := newReviewFixture()
		changed, err := reviewIdiom(t.Context(), store, switchOn(&fakeIdiomIndex{}), true, "owner", reviewedIdiom(), reviewNow)
		require.NoError(t, err)
		require.False(t, changed)
	})
}

func TestReviewIdiomReturnsAWithdrawnIdiomOnNewMaterial(t *testing.T) {
	withdrawn := reviewedIdiom()
	withdrawn.Withdrawn = true
	t.Run("it returns and its material is replaced", func(t *testing.T) {
		store := newReviewFixture()
		store.current = []domain.StudyMaterial{{ID: "m1", StudyItem: withdrawn.StudyItem, State: domain.StudyMaterialReady}}
		changed, err := review(store, withdrawn, true)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []bool{false}, store.withdrawn)
		require.Equal(t, []string{"m1"}, store.superseded)
	})
	t.Run("one with no current material returns and gets new material", func(t *testing.T) {
		store := newReviewFixture()
		changed, err := review(store, withdrawn, true)
		require.NoError(t, err)
		require.True(t, changed)
		require.Empty(t, store.superseded)
	})
	t.Run("an unpreparable one waits for sentences it has not seen", func(t *testing.T) {
		store := newReviewFixture()
		seen := []domain.StudyOccurrenceRef{{BookID: "b1", AnalysisRunID: "r1", CorpusID: "c1", SentenceOrdinal: 4}}
		store.current = []domain.StudyMaterial{{ID: "m2", StudyItem: withdrawn.StudyItem, State: domain.StudyMaterialUnpreparable, EvidenceFingerprint: domain.StudyEvidenceFingerprint(seen)}}
		changed, err := review(store, withdrawn, true)
		require.NoError(t, err)
		require.False(t, changed)
		require.Empty(t, store.withdrawn)
	})
	t.Run("an unpreparable one returns on new evidence", func(t *testing.T) {
		store := newReviewFixture()
		store.current = []domain.StudyMaterial{{ID: "m2", StudyItem: withdrawn.StudyItem, State: domain.StudyMaterialUnpreparable, EvidenceFingerprint: "another"}}
		changed, err := review(store, withdrawn, true)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []string{"m2"}, store.superseded)
	})
	t.Run("a progress that is gone is not an error", func(t *testing.T) {
		store := newReviewFixture()
		store.missing = "withdraw"
		_, err := review(store, withdrawn, true)
		require.NoError(t, err)
	})
}

func TestReviewIdiomReportsAStoreFailure(t *testing.T) {
	withdrawn := reviewedIdiom()
	withdrawn.Withdrawn = true
	cases := []struct {
		method string
		word   domain.StudyProgress
		mutate func(*reviewFakeStore)
	}{
		{"candidates", reviewedIdiom(), nil},
		{"withdraw", reviewedIdiom(), func(s *reviewFakeStore) { s.candidates = nil }},
		{"current", withdrawn, nil},
		{"withdraw", withdrawn, nil},
		{"supersede", withdrawn, func(s *reviewFakeStore) {
			s.current = []domain.StudyMaterial{{ID: "m1", StudyItem: withdrawn.StudyItem}}
		}},
		{"refs", withdrawn, func(s *reviewFakeStore) {
			s.current = []domain.StudyMaterial{{ID: "m2", StudyItem: withdrawn.StudyItem, State: domain.StudyMaterialUnpreparable}}
		}},
		{"material", reviewedIdiom(), nil},
		{"tokens", reviewedIdiom(), nil},
		{"refs", reviewedIdiom(), nil},
		{"supersede", reviewedIdiom(), func(s *reviewFakeStore) { s.refs = nil }},
	}
	for _, c := range cases {
		t.Run(c.method+" for "+c.word.ID, func(t *testing.T) {
			store := newReviewFixture()
			store.failing = c.method
			if c.mutate != nil {
				c.mutate(store)
			}
			_, err := review(store, c.word, true)
			require.ErrorIs(t, err, errReviewStore)
		})
	}
}

func TestIdiomEvidenceNamesTheSentencesTheIdiomHasNow(t *testing.T) {
	item := domain.IdiomItem("de", "mit dem haus")
	t.Run("a headword the index no longer has is no evidence", func(t *testing.T) {
		seen, err := idiomEvidence(t.Context(), newReviewFixture(), switchOn(&fakeIdiomIndex{}), "owner", item)
		require.NoError(t, err)
		require.Empty(t, seen)
	})
	t.Run("its candidate sentences are the evidence", func(t *testing.T) {
		seen, err := idiomEvidence(t.Context(), newReviewFixture(), reviewAvoidance(), "owner", item)
		require.NoError(t, err)
		assert.Equal(t, []domain.StudyOccurrenceRef{{BookID: "b1", AnalysisRunID: "r1", CorpusID: "c1", SentenceOrdinal: 4}}, seen)
	})
	t.Run("a store failure is reported", func(t *testing.T) {
		store := newReviewFixture()
		store.failing = "refs"
		_, err := idiomEvidence(t.Context(), store, reviewAvoidance(), "owner", item)
		require.ErrorIs(t, err, errReviewStore)
	})
}
