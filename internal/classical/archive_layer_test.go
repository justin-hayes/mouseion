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
