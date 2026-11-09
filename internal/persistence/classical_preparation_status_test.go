package persistence

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassicalPreparationStatusResolution(t *testing.T) {
	t.Parallel()
	unit := &domain.BookReadingUnit{ReadingUnitKey: "greek-1", Revision: 2}
	tests := []struct {
		name string
		unit *domain.BookReadingUnit
		row  classicalPreparationRow
		want func(*testing.T, *domain.ClassicalPreparationStatus)
	}{
		{name: "no Reading unit", unit: nil, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			assert.Nil(t, status)
		}},
		{name: "prepared", unit: unit, row: classicalPreparationRow{prepared: true, jobState: "completed"}, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			assert.Nil(t, status)
		}},
		{name: "queued current revision", unit: unit, row: classicalPreparationRow{jobState: "available", jobRevision: 2}, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			require.NotNil(t, status)
			assert.Equal(t, domain.ClassicalPreparationQueued, status.State)
		}},
		{name: "queued older revision is stale", unit: unit, row: classicalPreparationRow{jobState: "available", jobRevision: 1}, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			assert.Nil(t, status)
		}},
		{name: "running current revision", unit: unit, row: classicalPreparationRow{jobState: "running", jobRevision: 2}, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			require.NotNil(t, status)
			assert.Equal(t, domain.ClassicalPreparationRunning, status.State)
		}},
		{name: "failure current revision", unit: unit, row: classicalPreparationRow{jobState: "discarded", jobRevision: 2, stage: "source", code: ancientgreek.CodePinMismatch, failureRevision: 2}, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			require.NotNil(t, status)
			assert.Equal(t, domain.ClassicalPreparationFailureSource, status.Failure)
			assert.Equal(t, ancientgreek.CodePinMismatch, status.Code)
			assert.True(t, status.Failed())
		}},
		{name: "failure cleared by newer revision", unit: unit, row: classicalPreparationRow{jobState: "discarded", jobRevision: 1, stage: "source", code: ancientgreek.CodePinMismatch, failureRevision: 1}, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			require.NotNil(t, status)
			assert.True(t, status.Cleared)
			assert.Empty(t, status.Failure)
			assert.False(t, status.Failed())
		}},
		{name: "job error fallback", unit: unit, row: classicalPreparationRow{jobState: "discarded", jobRevision: 2, jobError: "ancientgreek: import refused: " + ancientgreek.CodeOversize}, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			require.NotNil(t, status)
			assert.Equal(t, domain.ClassicalPreparationFailureSource, status.Failure)
			assert.Equal(t, ancientgreek.CodeOversize, status.Code)
		}},
		{name: "withdrawn", unit: &domain.BookReadingUnit{ReadingUnitKey: "greek-1", Revision: 2, Withdrawn: true}, row: classicalPreparationRow{}, want: func(t *testing.T, status *domain.ClassicalPreparationStatus) {
			t.Helper()
			require.NotNil(t, status)
			assert.True(t, status.Withdrawn)
			assert.Equal(t, domain.ClassicalPreparationFailureWithdrawn, status.Failure)
			assert.Equal(t, classical.WithdrawnCode, status.Code)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.want(t, classicalPreparationStatus(tt.unit, tt.row))
		})
	}
}

func TestValidatePreparationFailure(t *testing.T) {
	t.Parallel()
	valid := domain.ClassicalPreparationFailureRecord{
		OwnerID: "owner", BookID: "book", ReadingUnitKey: "greek-1", DefinitionRevision: 1,
		Stage: domain.ClassicalPreparationFailureSource, Code: ancientgreek.CodePinMismatch,
	}
	require.NoError(t, validatePreparationFailure(valid))
	for name, mutate := range map[string]func(*domain.ClassicalPreparationFailureRecord){
		"owner":    func(r *domain.ClassicalPreparationFailureRecord) { r.OwnerID = "" },
		"book":     func(r *domain.ClassicalPreparationFailureRecord) { r.BookID = "" },
		"unit":     func(r *domain.ClassicalPreparationFailureRecord) { r.ReadingUnitKey = "" },
		"revision": func(r *domain.ClassicalPreparationFailureRecord) { r.DefinitionRevision = 0 },
		"stage":    func(r *domain.ClassicalPreparationFailureRecord) { r.Stage = domain.ClassicalPreparationFailureNone },
		"code":     func(r *domain.ClassicalPreparationFailureRecord) { r.Code = "" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			record := valid
			mutate(&record)
			assert.Error(t, validatePreparationFailure(record))
		})
	}
}

func TestValidatePreparedDefinition(t *testing.T) {
	t.Parallel()
	valid := domain.ClassicalPreparedDefinition{
		OwnerID: "owner", BookID: "book", ReadingUnitKey: "greek-1", DefinitionRevision: 1,
		SourceFileSHA256: strings.Repeat("a", 64), InterpretationVersion: "oga-agdt-upos/5", AnalysisRunID: "run",
	}
	require.NoError(t, validatePreparedDefinition(valid))
	for name, mutate := range map[string]func(*domain.ClassicalPreparedDefinition){
		"owner":          func(r *domain.ClassicalPreparedDefinition) { r.OwnerID = "" },
		"book":           func(r *domain.ClassicalPreparedDefinition) { r.BookID = "" },
		"unit":           func(r *domain.ClassicalPreparedDefinition) { r.ReadingUnitKey = "" },
		"revision":       func(r *domain.ClassicalPreparedDefinition) { r.DefinitionRevision = 0 },
		"sha":            func(r *domain.ClassicalPreparedDefinition) { r.SourceFileSHA256 = "short" },
		"interpretation": func(r *domain.ClassicalPreparedDefinition) { r.InterpretationVersion = "" },
		"analysis run":   func(r *domain.ClassicalPreparedDefinition) { r.AnalysisRunID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			record := valid
			mutate(&record)
			assert.Error(t, validatePreparedDefinition(record))
		})
	}
}

func TestClassicalFailureStageFromCode(t *testing.T) {
	t.Parallel()
	cases := map[string]domain.ClassicalPreparationFailure{
		ancientgreek.CodeSourceDownloadFailed: domain.ClassicalPreparationFailureFetch,
		ancientgreek.CodePinMismatch:          domain.ClassicalPreparationFailureSource,
		ancientgreek.CodeOversize:             domain.ClassicalPreparationFailureSource,
		ancientgreek.CodeAdapterRefused:       domain.ClassicalPreparationFailureSource,
		classical.WithdrawnCode:               domain.ClassicalPreparationFailureWithdrawn,
		classical.LegacyRowsCode:              domain.ClassicalPreparationFailureCatalog,
		classical.PreparationFailedCode:       domain.ClassicalPreparationFailureImport,
	}
	for code, want := range cases {
		assert.Equal(t, want, classicalFailureStageFromCode(code), code)
	}
}
