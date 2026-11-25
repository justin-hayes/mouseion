package classical

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveRangeLayerNeedsItsArchiveFacts(t *testing.T) {
	layer := Layer{
		Key: "ola", Release: "v0.2.0", FetchKind: FetchKindArchiveRange, ArchiveURL: "https://example.test/a.zip",
		ArchiveSize: 10, Record: "10.5281/zenodo.15183688", LicenceClass: LicenceClassAnalysisResource,
		Restriction: RestrictionUnclear, DeployerRegisterRow: "row",
	}
	require.NoError(t, layer.Validate())
	for name, mutate := range map[string]func(*Layer){
		"no record": func(l *Layer) { l.Record = "" },
		"no size":   func(l *Layer) { l.ArchiveSize = 0 },
		"bad URL":   func(l *Layer) { l.ArchiveURL = "a.zip" },
		"unknown":   func(l *Layer) { l.FetchKind = "ftp" },
	} {
		broken := layer
		mutate(&broken)
		assert.Error(t, broken.Validate(), name)
	}
}

func TestAnOLASourceNeedsItsCitationMemberAndNoOtherSourceHasOne(t *testing.T) {
	work := Override{WorkKey: "w", WorkTitle: "t", WorkAuthor: "a", EditionKey: "e", EditionLabel: "l", Split: SplitWholeWork,
		Source: &SourceSpec{Layer: "ola", FileName: "x.conllu", Format: SourceFormatOLACoNLLU}}
	require.Error(t, work.Validate())
	work.Source.CitationFileName = "x.paula.xml"
	require.NoError(t, work.Validate())
	work.Source.Format = SourceFormatOGACoNLLU
	require.Error(t, work.Validate())
	work.Source.CitationFileName = ""
	require.NoError(t, work.Validate())

	unit := Override{Key: "w", ReviewerNote: "read"}
	require.Error(t, unit.Validate(), "a reviewer note clears a Work, never a unit")
}

func TestASpotCheckNamesAReviewerAndCitationPairsAndBelongsToAWork(t *testing.T) {
	check := SpotCheck{Reviewer: "Justin", Pairs: []string{"1.1", "2.3"}}
	require.NoError(t, check.Validate())
	require.Error(t, SpotCheck{Pairs: []string{"1.1"}}.Validate(), "a spot check needs a reviewer")
	require.Error(t, SpotCheck{Reviewer: "Justin"}.Validate(), "a spot check needs pairs")
	require.Error(t, SpotCheck{Reviewer: "Justin", Pairs: []string{"not a citation"}}.Validate())

	require.Error(t, Override{Key: "w", SpotCheck: &check}.Validate(), "a spot check admits a Work, never a unit")
}

func TestAnArchiveContainerBelongsToAnOLASourceOnly(t *testing.T) {
	source := SourceSpec{Layer: "ola", FileName: "a.conllu", CitationFileName: "a.xml", Format: SourceFormatOLACoNLLU, Container: "w/conllu.zip", CitationContainer: "w/paula.zip"}
	require.NoError(t, source.validateCitationMember("w"))
	greek := SourceSpec{Layer: "oga", FileName: "a.conllu", Format: SourceFormatOGACoNLLU, Container: "w/conllu.zip"}
	require.Error(t, greek.validateCitationMember("w"))
}
