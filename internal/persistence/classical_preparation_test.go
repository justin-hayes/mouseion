package persistence

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validPreparedDefinition() domain.ClassicalPreparedDefinition {
	return domain.ClassicalPreparedDefinition{
		OwnerID: "owner-1", BookID: "book-1", ReadingUnitKey: "unit-1",
		DefinitionRevision: 1, SourceFileSHA256: strings.Repeat("a", 64),
		InterpretationVersion: "oga-agdt-upos/5", AnalysisRunID: "run-1",
	}
}

func TestValidatePreparedDefinitionRefusesIncompleteRecords(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*domain.ClassicalPreparedDefinition){
		"owner":         func(r *domain.ClassicalPreparedDefinition) { r.OwnerID = "" },
		"book":          func(r *domain.ClassicalPreparedDefinition) { r.BookID = "" },
		"reading unit":  func(r *domain.ClassicalPreparedDefinition) { r.ReadingUnitKey = "" },
		"revision":      func(r *domain.ClassicalPreparedDefinition) { r.DefinitionRevision = 0 },
		"source digest": func(r *domain.ClassicalPreparedDefinition) { r.SourceFileSHA256 = "short" },
		"interpretation": func(r *domain.ClassicalPreparedDefinition) {
			r.InterpretationVersion = ""
		},
		"analysis run": func(r *domain.ClassicalPreparedDefinition) { r.AnalysisRunID = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			record := validPreparedDefinition()
			mutate(&record)
			require.Error(t, validatePreparedDefinition(record))
		})
	}
	require.NoError(t, validatePreparedDefinition(validPreparedDefinition()))
}

func validClassicalSource() domain.ClassicalSource {
	return domain.ClassicalSource{
		SHA256: strings.Repeat("a", 64), Layer: "layer-1",
		FileName: "work.conllu", Size: 4, Content: []byte("data"),
	}
}

func TestValidateClassicalSourceRefusesUnpinnedBytes(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*domain.ClassicalSource){
		"digest":    func(s *domain.ClassicalSource) { s.SHA256 = "short" },
		"layer":     func(s *domain.ClassicalSource) { s.Layer = "" },
		"file name": func(s *domain.ClassicalSource) { s.FileName = "" },
		"size":      func(s *domain.ClassicalSource) { s.Size = 5 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := validClassicalSource()
			mutate(&source)
			require.Error(t, validateClassicalSource(source))
		})
	}
	source := validClassicalSource()
	require.NoError(t, validateClassicalSource(source))
	assert.Equal(t, int64(len(source.Content)), source.Size)
}
