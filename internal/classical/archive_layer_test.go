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
