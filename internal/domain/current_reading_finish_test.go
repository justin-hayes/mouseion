package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func finishFacts() CurrentReadingFinishFacts {
	return CurrentReadingFinishFacts{
		Current:            CurrentReading{OwnerID: "owner", Language: "de", BookID: "book", SnapshotID: "snap"},
		ExpectedBookID:     "book",
		ExpectedSnapshotID: "snap",
	}
}

// Every snapshot identity is Reserved by the Current reading, so Reserved never
// excludes one; only Known does.
func TestPlanCurrentReadingFinishAcceptsOnlyIdentitiesNotKnown(t *testing.T) {
	facts := finishFacts()
	facts.Snapshot = []SnapshotIdentity{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "lesen", UPOS: "VERB"},
		{Language: "de", CanonicalLemma: "laufen", UPOS: "VERB"},
		{Language: "de", CanonicalLemma: "schnell", UPOS: "ADJ"},
	}
	facts.Known = []KnownVocabulary{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "laufen", UPOS: ""},
		{Language: "de", CanonicalLemma: "lesen", UPOS: "NOUN"},
		{Language: "it", CanonicalLemma: "schnell", UPOS: "ADJ"},
	}

	decision := PlanCurrentReadingFinish(facts)

	require.Equal(t, CurrentReadingFinishPlanned, decision.Outcome)
	assert.Equal(t, []SnapshotIdentity{
		{Language: "de", CanonicalLemma: "lesen", UPOS: "VERB"},
		{Language: "de", CanonicalLemma: "schnell", UPOS: "ADJ"},
	}, decision.Plan.Accept, "Known matches by exact UPOS or the lemma wildcard, within the language")
	assert.Equal(t, 4, decision.Plan.SnapshotCount)
	assert.Equal(t, 2, decision.Plan.EligibleCount)
	assert.Equal(t, 2, decision.Plan.NewlyKnownCount)
	assert.Equal(t, 2, decision.Plan.AlreadyKnownCount)
}

func TestPlanCurrentReadingFinishPlansEmptySnapshot(t *testing.T) {
	decision := PlanCurrentReadingFinish(finishFacts())

	require.Equal(t, CurrentReadingFinishPlanned, decision.Outcome)
	assert.Empty(t, decision.Plan.Accept)
	assert.Equal(t, CurrentReadingFinishPlan{}, decision.Plan)
}

func TestPlanCurrentReadingFinishAcceptsAllWhenNothingIsKnown(t *testing.T) {
	facts := finishFacts()
	facts.Snapshot = []SnapshotIdentity{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}}

	decision := PlanCurrentReadingFinish(facts)

	require.Equal(t, CurrentReadingFinishPlanned, decision.Outcome)
	assert.Equal(t, 1, decision.Plan.NewlyKnownCount)
	assert.Zero(t, decision.Plan.AlreadyKnownCount)
}

func TestPlanCurrentReadingFinishReplaysExistingCompletion(t *testing.T) {
	completion := CurrentReadingCompletion{BookID: "book", SnapshotID: "snap", SnapshotVocabularyCount: 3, GraduatedVocabularyCount: 2, AlreadyKnownVocabularyCount: 1}

	t.Run("after the current reading ended", func(t *testing.T) {
		facts := finishFacts()
		facts.Current = CurrentReading{}
		facts.Completion = &completion
		facts.Snapshot = []SnapshotIdentity{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}}

		decision := PlanCurrentReadingFinish(facts)

		require.Equal(t, CurrentReadingFinishReplayed, decision.Outcome)
		assert.Equal(t, completion, decision.Replay)
		assert.Empty(t, decision.Plan.Accept, "a replay accepts nothing new")
	})

	t.Run("while the current reading still exists", func(t *testing.T) {
		facts := finishFacts()
		facts.Completion = &completion

		decision := PlanCurrentReadingFinish(facts)

		require.Equal(t, CurrentReadingFinishReplayed, decision.Outcome)
		assert.Equal(t, completion, decision.Replay)
	})
}

func TestPlanCurrentReadingFinishRejectsStaleExpectations(t *testing.T) {
	completion := CurrentReadingCompletion{BookID: "book", SnapshotID: "snap"}
	cases := map[string]struct {
		mutate func(*CurrentReadingFinishFacts)
		reason CurrentReadingFinishRejection
	}{
		"missing book expectation": {func(f *CurrentReadingFinishFacts) { f.ExpectedBookID = "" }, CurrentReadingFinishStale},
		"other book is current":    {func(f *CurrentReadingFinishFacts) { f.ExpectedBookID = "other" }, CurrentReadingFinishStale},
		"other snapshot is current": {func(f *CurrentReadingFinishFacts) {
			f.ExpectedSnapshotID = "other"
		}, CurrentReadingFinishStale},
		"empty snapshot expectation against a frozen snapshot": {func(f *CurrentReadingFinishFacts) {
			f.ExpectedSnapshotID = ""
		}, CurrentReadingFinishStale},
		"completion belongs to another book": {func(f *CurrentReadingFinishFacts) {
			f.Current = CurrentReading{}
			f.ExpectedBookID = "other"
			f.Completion = &completion
		}, CurrentReadingFinishStale},
		"nothing current and nothing completed": {func(f *CurrentReadingFinishFacts) {
			f.Current = CurrentReading{}
		}, CurrentReadingFinishUnknown},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			facts := finishFacts()
			facts.Snapshot = []SnapshotIdentity{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}}
			tc.mutate(&facts)

			decision := PlanCurrentReadingFinish(facts)

			assert.Equal(t, CurrentReadingFinishRejected, decision.Outcome)
			assert.Equal(t, tc.reason, decision.Rejection)
			assert.Empty(t, decision.Plan.Accept)
		})
	}
}

func TestPlanCurrentReadingFinishAcceptsLegacyReadingWithoutSnapshot(t *testing.T) {
	facts := finishFacts()
	facts.Current.SnapshotID = ""
	facts.ExpectedSnapshotID = ""

	decision := PlanCurrentReadingFinish(facts)

	require.Equal(t, CurrentReadingFinishPlanned, decision.Outcome)
	assert.Equal(t, CurrentReadingFinishPlan{}, decision.Plan)
}
