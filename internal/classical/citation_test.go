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

func TestParseCitationAcceptsNumbersSuffixesAndNamedSections(t *testing.T) {
	for _, value := range []string{"17", "1.2.3", "331a", "1.pr.1", "1.ep", "0", "pr"} {
		parsed, ok := ParseCitation(value)
		require.True(t, ok, value)
		assert.Equal(t, value, parsed.String())
		assert.True(t, IsCitation(value))
	}
}

func TestParseCitationRefusesUnknownNamesLeadingZerosAndMalformedInput(t *testing.T) {
	for _, value := range []string{"", "1.", ".1", "1..2", "01", "1.02", "00", "1.x", "1.-2", "331A", "331ab", "a331", "1.praef", "1.PR.1", "+1", "1 .2", "pr1"} {
		assert.False(t, IsCitation(value), value)
		_, ok := ParseCitation(value)
		assert.False(t, ok, value)
	}
}

func TestCompareCitationsOrdersSuffixesAndNamedSections(t *testing.T) {
	for _, test := range []struct {
		a, b string
		want int
	}{
		{"1.pr.1", "1.1.1", -1},
		{"1.1.1", "1.pr.1", 1},
		{"1.pr.1", "1.pr.2", -1},
		{"1.9.9", "1.ep", -1},
		{"1.ep", "1.9.9", 1},
		{"1.pr", "1", -1},
		{"331", "331a", -1},
		{"331a", "331b", -1},
		{"331b", "332", -1},
		{"1.2", "1.2.0", 0},
		{"1.pr.1", "1.pr.1", 0},
	} {
		order, err := CompareCitations(test.a, test.b)
		require.NoError(t, err)
		assert.Equal(t, test.want, order, "%s vs %s", test.a, test.b)
	}
	_, err := CompareCitations("1.01", "1.2")
	require.Error(t, err)
	_, err = CompareCitations("1.2", "1.foo")
	require.Error(t, err)
}

func TestCitationWithinRangeFollowsTheWidenedOrder(t *testing.T) {
	parse := func(value string) Citation {
		parsed, ok := ParseCitation(value)
		require.True(t, ok, value)
		return parsed
	}
	assert.True(t, parse("1.pr.2").Within(parse("1.pr.1"), parse("1.1.1")))
	assert.True(t, parse("1.1.1").Within(parse("1.pr.1"), parse("1.1.1")))
	assert.False(t, parse("1.2.1").Within(parse("1.pr.1"), parse("1.1.9")))
	assert.False(t, parse("1.pr.1").Within(parse("1.1.1"), parse("1.2.1")))
	assert.True(t, parse("331a").Within(parse("331"), parse("332")))
	assert.False(t, parse("331a").Within(parse("331b"), parse("332")))
	assert.True(t, parse("1.ep").Within(parse("1.9"), parse("2")))
}

func TestFormatCitationRangeHandlesSuffixesAndNamedSections(t *testing.T) {
	assert.Equal(t, "1.pr.1–2, 1.1.1", FormatCitationRange([]string{"1.1.1", "1.pr.2", "1.pr.1"}))
	assert.Equal(t, "331, 331a, 331b", FormatCitationRange([]string{"331b", "331", "331a"}))
	assert.Equal(t, "1.pr, 1.ep", FormatCitationRange([]string{"1.ep", "1.pr"}))
}

func TestCitationContains(t *testing.T) {
	cases := map[string]struct {
		outer, inner string
		want         bool
	}{
		"equal":                  {"1.2", "1.2", true},
		"ancestor":               {"1.2", "1.2.3", true},
		"named section ancestor": {"1.pr", "1.pr.1", true},
		"longer number":          {"1.2", "1.20", false},
		"suffixed section":       {"1.2", "1.2a", false},
		"descendant":             {"1.2.3", "1.2", false},
		"empty":                  {"", "1", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, CitationContains(tc.outer, tc.inner))
		})
	}
}
