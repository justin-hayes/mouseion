package ancientgreek

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ogaFixture(t *testing.T) ([]byte, domain.ReadingUnitDefinition) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return data, domain.ReadingUnitDefinition{
		Key: "test-work-1", Language: Language, WorkTitle: "Test Work",
		WorkAuthor: "Author", EditionLabel: "Edition",
		CitationStart: "1.1.1", CitationEnd: "1.1.2",
		SourceFileName: "work.conllu", SourceFileSize: int64(len(data)),
		SourceFileSHA256:      hex.EncodeToString(sum[:]),
		InterpretationVersion: "oga-agdt-upos/5",
	}
}

// TestParsedWorkCutUnitReturnsOnlyTheUnitText proves preparation stores the
// Reading unit's text, not the whole work file, and keeps the unit key as the
// Book's source identifier.
func TestParsedWorkCutUnitReturnsOnlyTheUnitText(t *testing.T) {
	t.Parallel()
	data, definition := ogaFixture(t)
	parsed, err := ParseOgaSource(data)
	require.NoError(t, err)
	unit, err := parsed.CutUnit(definition)
	require.NoError(t, err)

	assert.Equal(t, "test-work-1", unit.UnitKey)
	assert.Equal(t, domain.ClassicalUnitID("test-work-1"), unit.UnitID)
	assert.Contains(t, unit.FullText, "λέγει")
	assert.NotContains(t, unit.FullText, "Ἀρταξέρξης", "Book 2's sentence is outside this Reading unit")
	assert.Equal(t, []byte(unit.FullText), unit.SourceBytes)
	assert.Less(t, len(unit.SourceBytes), len(data), "only the unit text is stored, not the whole work file")
	assert.Equal(t, 2, unit.Report.Sentences)
	assert.NotEmpty(t, unit.Result.Sentences)
}

// TestParsedWorkCutUnitReusesTheSameParse proves one parsed work can produce
// two Book units, which is how the serial worker parses a moved work once.
func TestParsedWorkCutUnitReusesTheSameParse(t *testing.T) {
	t.Parallel()
	data, first := ogaFixture(t)
	parsed, err := ParseOgaSource(data)
	require.NoError(t, err)
	second := first
	second.Key = "test-work-2"
	second.CitationStart, second.CitationEnd = "2.1.1", "2.1.1"

	unitOne, err := parsed.CutUnit(first)
	require.NoError(t, err)
	unitTwo, err := parsed.CutUnit(second)
	require.NoError(t, err)
	assert.Contains(t, unitOne.FullText, "λέγει")
	assert.NotContains(t, unitOne.FullText, "Ἀρταξέρξης")
	assert.Contains(t, unitTwo.FullText, "Ἀρταξέρξης")
	assert.NotContains(t, unitTwo.FullText, "λέγει")
}

func TestParseOgaSourceRefusesMalformedBytes(t *testing.T) {
	t.Parallel()
	_, err := ParseOgaSource([]byte("not a conllu file"))
	var importErr *ImportError
	require.ErrorAs(t, err, &importErr)
	assert.Equal(t, CodeMalformed, importErr.Code)
}

func TestParsedWorkCutUnitRefusesInvalidInput(t *testing.T) {
	t.Parallel()
	data, valid := ogaFixture(t)
	parsed, err := ParseOgaSource(data)
	require.NoError(t, err)

	_, err = (*ParsedWork)(nil).CutUnit(valid)
	require.Error(t, err)

	invalid := valid
	invalid.Key = ""
	_, err = parsed.CutUnit(invalid)
	require.Error(t, err)

	badCitation := valid
	badCitation.CitationStart = "not-a-citation"
	_, err = parsed.CutUnit(badCitation)
	var importErr *ImportError
	require.ErrorAs(t, err, &importErr)
	assert.Equal(t, CodeUnknownInterpret, importErr.Code)
}
