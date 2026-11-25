package classical

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatCitationRange(t *testing.T) {
	for _, test := range []struct {
		name      string
		citations []string
		want      string
	}{
		{name: "single", citations: []string{"1.2.17"}, want: "1.2.17"},
		{name: "consecutive pair", citations: []string{"1.2.17", "1.2.18"}, want: "1.2.17\u201318"},
		{name: "consecutive triple", citations: []string{"1.2.17", "1.2.18", "1.2.19"}, want: "1.2.17\u201319"},
		{name: "two runs", citations: []string{"1.2.1", "1.2.2", "1.2.5"}, want: "1.2.1\u20132, 1.2.5"},
		{name: "different chapter", citations: []string{"1.2.17", "1.3.17"}, want: "1.2.17, 1.3.17"},
		{name: "reversed input is ordered", citations: []string{"1.3.17", "1.2.17"}, want: "1.2.17, 1.3.17"},
		{name: "mixed component counts", citations: []string{"1.2.17", "1.2"}, want: "1.2, 1.2.17"},
		{name: "empty citation is listed verbatim", citations: []string{"1.2.17", ""}, want: "1.2.17, "},
		{name: "negative component is listed verbatim", citations: []string{"1.-2"}, want: "1.-2"},
		{name: "non-numeric component is listed verbatim", citations: []string{"1.x"}, want: "1.x"},
		{name: "unknown citation is listed verbatim", citations: []string{"1.2.17", "appendix"}, want: "1.2.17, appendix"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, FormatCitationRange(test.citations))
		})
	}
}

func TestCompareCitationsOrdersComponentsAndRefusesInvalidInput(t *testing.T) {
	order, err := CompareCitations("1.2.3", "1.10")
	require.NoError(t, err)
	assert.Equal(t, -1, order)
	order, err = CompareCitations("2", "2.0")
	require.NoError(t, err)
	assert.Zero(t, order)
	_, err = CompareCitations("1.x", "1.2")
	require.Error(t, err)
}
