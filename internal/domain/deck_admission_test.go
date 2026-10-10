package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func admissionFixture() DeckAdmissionFacts {
	analysis := CurrentAnalysis{SourceMaterialID: "source", AnalysisRunID: "run", ContentRevisionID: "rev", ContentSnapshotID: "content", CorpusID: "corpus"}
	return DeckAdmissionFacts{
		Action: DeckActionSubmit,
		BookID: "book",
		Current: CurrentReading{
			OwnerID: "owner", Language: "de", BookID: "book", SnapshotID: "snap", SnapshotSize: 2,
			SourceMaterialID: "source", AnalysisRunID: "run", ContentRevisionID: "rev", ContentSnapshotID: "content", CorpusID: "corpus",
		},
		Analysis:           analysis,
		ExpectedSnapshotID: "snap",
	}
}

func readyPreparation() *DeckPreparation {
	return &DeckPreparation{ID: "prep", BookID: "book", SnapshotID: "snap", State: DeckPreparationReady, CurrentRunID: "current-run"}
}

func TestDecideDeckAdmissionOutcomes(t *testing.T) {
	retired := time.Now()
	tests := []struct {
		name   string
		mutate func(*DeckAdmissionFacts)
		want   DeckAdmission
	}{
		{name: "submission without a preparation is admitted", want: DeckAdmit},
		{name: "queued preparation is retried", mutate: func(f *DeckAdmissionFacts) {
			f.Preparation = &DeckPreparation{BookID: "book", SnapshotID: "snap", State: DeckPreparationQueued}
		}, want: DeckAdmit},
		{name: "failed preparation is retried", mutate: func(f *DeckAdmissionFacts) {
			f.Preparation = &DeckPreparation{BookID: "book", SnapshotID: "snap", State: DeckPreparationFailed}
		}, want: DeckAdmit},
		{name: "ready deck that requires re-preparation rolls forward through submission", mutate: func(f *DeckAdmissionFacts) {
			p := readyPreparation()
			p.Error = DeckPreparationRequiresRepreparationError
			f.Preparation = p
		}, want: DeckAdmit},
		{name: "healthy ready deck is not resubmitted", mutate: func(f *DeckAdmissionFacts) { f.Preparation = readyPreparation() }, want: DeckAdmissionInvalidTransition},
		{name: "reprepare admits a ready deck", mutate: func(f *DeckAdmissionFacts) {
			f.Action = DeckActionReprepare
			f.Preparation = readyPreparation()
		}, want: DeckAdmit},
		{name: "reprepare refuses a failed deck", mutate: func(f *DeckAdmissionFacts) {
			f.Action = DeckActionReprepare
			f.Preparation = &DeckPreparation{BookID: "book", SnapshotID: "snap", State: DeckPreparationFailed}
		}, want: DeckAdmissionInvalidTransition},
		{name: "reprepare without a named preparation is an invalid transition", mutate: func(f *DeckAdmissionFacts) {
			f.Action = DeckActionReprepare
		}, want: DeckAdmissionInvalidTransition},
		{name: "rerender admits a ready deck with a current run", mutate: func(f *DeckAdmissionFacts) {
			f.Action = DeckActionRerender
			f.Preparation = readyPreparation()
		}, want: DeckAdmit},
		{name: "rerender refuses a ready deck without a current run", mutate: func(f *DeckAdmissionFacts) {
			f.Action = DeckActionRerender
			p := readyPreparation()
			p.CurrentRunID = ""
			f.Preparation = p
		}, want: DeckAdmissionInvalidTransition},
		{name: "retired preparation refuses every action", mutate: func(f *DeckAdmissionFacts) {
			p := readyPreparation()
			p.RetiredAt = &retired
			f.Action = DeckActionReprepare
			f.Preparation = p
		}, want: DeckAdmissionInvalidTransition},
		{name: "no active reading is not the current reading", mutate: func(f *DeckAdmissionFacts) {
			f.Current = CurrentReading{}
		}, want: DeckAdmissionNotCurrentReading},
		{name: "another Book is not the current reading", mutate: func(f *DeckAdmissionFacts) {
			f.BookID = "other-book"
		}, want: DeckAdmissionNotCurrentReading},
		{name: "a reading pinning an older analysis is not the current reading", mutate: func(f *DeckAdmissionFacts) {
			f.Analysis.AnalysisRunID = "newer-run"
		}, want: DeckAdmissionNotCurrentReading},
		{name: "a reading pinning older content is not the current reading", mutate: func(f *DeckAdmissionFacts) {
			f.Analysis.ContentRevisionID = "newer-rev"
		}, want: DeckAdmissionNotCurrentReading},
		{name: "a Book with no current analysis is not the current reading", mutate: func(f *DeckAdmissionFacts) {
			f.Analysis = CurrentAnalysis{}
		}, want: DeckAdmissionNotCurrentReading},
		{name: "a preparation bound to another snapshot is not the current reading", mutate: func(f *DeckAdmissionFacts) {
			p := readyPreparation()
			p.SnapshotID = "older-snap"
			f.Action = DeckActionReprepare
			f.Preparation = p
		}, want: DeckAdmissionNotCurrentReading},
		{name: "empty expected snapshot is stale", mutate: func(f *DeckAdmissionFacts) {
			f.ExpectedSnapshotID = ""
		}, want: DeckAdmissionStale},
		{name: "older expected snapshot is stale", mutate: func(f *DeckAdmissionFacts) {
			f.ExpectedSnapshotID = "older-snap"
		}, want: DeckAdmissionStale},
		{name: "empty current snapshot needs no deck", mutate: func(f *DeckAdmissionFacts) {
			f.Current.SnapshotSize = 0
		}, want: DeckAdmissionNotRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := admissionFixture()
			if tt.mutate != nil {
				tt.mutate(&facts)
			}
			assert.Equal(t, tt.want, DecideDeckAdmission(facts))
		})
	}
}

func TestDecideDeckAdmissionPrecedence(t *testing.T) {
	// Not-current outranks stale, stale outranks empty, and empty outranks the
	// state check: a request that fails several rules reports the first one.
	facts := admissionFixture()
	facts.BookID = "other-book"
	facts.ExpectedSnapshotID = "older-snap"
	facts.Current.SnapshotSize = 0
	facts.Preparation = &DeckPreparation{BookID: "book", SnapshotID: "snap", State: DeckPreparationReady}
	assert.Equal(t, DeckAdmissionNotCurrentReading, DecideDeckAdmission(facts))

	facts.BookID = "book"
	assert.Equal(t, DeckAdmissionStale, DecideDeckAdmission(facts))

	facts.ExpectedSnapshotID = "snap"
	assert.Equal(t, DeckAdmissionNotRequired, DecideDeckAdmission(facts))

	facts.Current.SnapshotSize = 2
	assert.Equal(t, DeckAdmissionInvalidTransition, DecideDeckAdmission(facts))
}

func TestDeckAdmissionErrMapsEachRefusal(t *testing.T) {
	require.NoError(t, DeckAdmit.Err())
	assert.True(t, DeckAdmit.Admitted())
	assert.True(t, errors.Is(DeckAdmissionStale.Err(), ErrDeckPreparationStale))
	assert.True(t, errors.Is(DeckAdmissionNotCurrentReading.Err(), ErrDeckPreparationNotCurrentReading))
	assert.True(t, errors.Is(DeckAdmissionNotRequired.Err(), ErrDeckPreparationNotRequired))
	assert.True(t, errors.Is(DeckAdmissionInvalidTransition.Err(), ErrDeckPreparationInvalidTransition))
	assert.False(t, DeckAdmissionStale.Admitted())
}
