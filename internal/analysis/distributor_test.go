package analysis

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/stretchr/testify/assert"
)

func TestDropDistributorSentencesDropsOverlappingSpansOnly(t *testing.T) {
	result := analyzer.Result{Sentences: []analyzer.Sentence{
		{Text: "preamble", Location: analyzer.SourceLocation{SourceDocumentID: "header", StartOffset: 0, EndOffset: 8}},
		{Text: "marker crossing", Location: analyzer.SourceLocation{SourceDocumentID: "header", StartOffset: 9, EndOffset: 25}},
		{Text: "book", Location: analyzer.SourceLocation{SourceDocumentID: "header", StartOffset: 26, EndOffset: 30}},
		{Text: "licence", Location: analyzer.SourceLocation{SourceDocumentID: "footer", StartOffset: 5, EndOffset: 15}},
		{Text: "after licence", Location: analyzer.SourceLocation{SourceDocumentID: "footer", StartOffset: 40, EndOffset: 50}},
		{Text: "other unit", Location: analyzer.SourceLocation{SourceDocumentID: "chapter", StartOffset: 0, EndOffset: 10}},
	}}
	dropDistributorSentences(&result, []epub.UnitTextRange{
		{UnitID: "header", StartOffset: 0, EndOffset: 24},
		{UnitID: "footer", StartOffset: 10, EndOffset: 40},
	})

	assert.Equal(t, []analyzer.Sentence{
		{Text: "book", Location: analyzer.SourceLocation{SourceDocumentID: "header", StartOffset: 26, EndOffset: 30}},
		{Text: "after licence", Location: analyzer.SourceLocation{SourceDocumentID: "footer", StartOffset: 40, EndOffset: 50}},
		{Text: "other unit", Location: analyzer.SourceLocation{SourceDocumentID: "chapter", StartOffset: 0, EndOffset: 10}},
	}, result.Sentences, "retained source locations are unchanged")
}

func TestSnapshotConfigIdentityForNamesEachSelectionRule(t *testing.T) {
	trimmed := epub.MainTextSelection{Distributor: epub.DistributorSelection{
		Recognised:    true,
		TrimmedRanges: []epub.UnitTextRange{{UnitID: "header", EndOffset: 10}},
	}}
	landmark := epub.MainTextSelection{Identified: true, LandmarkApplies: true}

	assert.Equal(t, snapshotConfigIdentity, snapshotConfigIdentityFor(epub.MainTextSelection{}))
	assert.Equal(t, snapshotConfigIdentity, snapshotConfigIdentityFor(epub.MainTextSelection{
		Distributor: epub.DistributorSelection{Recognised: true},
	}), "recognition without an exclusion is a no-op")
	assert.Equal(t, mainTextConfigIdentity, snapshotConfigIdentityFor(landmark))
	assert.Equal(t, distributorConfigIdentity, snapshotConfigIdentityFor(trimmed))
	both := trimmed
	both.Identified = true
	both.LandmarkApplies = true
	assert.Equal(t, mainTextDistributorConfigIdentity, snapshotConfigIdentityFor(both))
}

func TestScopeSnapshotUnitsHonoursTheRunIdentity(t *testing.T) {
	units := []snapshotUnit{{UnitID: "front"}, {UnitID: "book"}, {UnitID: "licence"}, {UnitID: "back"}}
	decision := epub.MainTextSelection{
		Identified:      true,
		BodyMatterStart: 1,
		BackMatterStart: 2,
		LandmarkApplies: true,
		SelectedUnitIDs: []string{"book"},
		Distributor: epub.DistributorSelection{
			Recognised:      true,
			ExcludedUnitIDs: []string{"back"},
			TrimmedRanges:   []epub.UnitTextRange{{UnitID: "book", EndOffset: 3}},
		},
	}

	selected, trims := scopeSnapshotUnits(units, decision, snapshotConfigIdentity)
	assert.Equal(t, units, selected)
	assert.Empty(t, trims)

	selected, trims = scopeSnapshotUnits(units, decision, mainTextConfigIdentity)
	assert.Equal(t, []snapshotUnit{{UnitID: "book"}}, selected)
	assert.Empty(t, trims)

	selected, trims = scopeSnapshotUnits(units, decision, distributorConfigIdentity)
	assert.Equal(t, []snapshotUnit{{UnitID: "front"}, {UnitID: "book"}, {UnitID: "licence"}}, selected)
	assert.Equal(t, decision.Distributor.TrimmedRanges, trims)

	selected, trims = scopeSnapshotUnits(units, decision, mainTextDistributorConfigIdentity)
	assert.Equal(t, []snapshotUnit{{UnitID: "book"}}, selected)
	assert.Equal(t, decision.Distributor.TrimmedRanges, trims)
}
