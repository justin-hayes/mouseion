package domain_test

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPositionFromPercentage(t *testing.T) {
	accepted := []struct {
		input    string
		fraction float64
	}{
		{"0", 0},
		{"35", 0.35},
		{"29", 0.29},
		{"99.6", 0.996},
		{"12.5", 0.125},
		{"100", 1},
		{"100.0", 1},
		{" 42 ", 0.42},
		{"42%", 0.42},
		{"007", 0.07},
		{"35.50", 0.355},
		{"-0", 0},
	}
	for _, tc := range accepted {
		t.Run("accepts "+tc.input, func(t *testing.T) {
			fraction, err := domain.PositionFromPercentage(tc.input)
			require.NoError(t, err)
			assert.InDelta(t, tc.fraction, fraction, 1e-12)
		})
	}

	refused := []struct {
		input  string
		reason domain.PositionRefusal
	}{
		{"", domain.PositionRefusalMissing},
		{"   ", domain.PositionRefusalMissing},
		{"abc", domain.PositionRefusalNotANumber},
		{"1e2", domain.PositionRefusalNotANumber},
		{"35.", domain.PositionRefusalNotANumber},
		{".5", domain.PositionRefusalNotANumber},
		{"12.34", domain.PositionRefusalNotANumber},
		{"35 %%", domain.PositionRefusalNotANumber},
		{"100.1", domain.PositionRefusalOutOfRange},
		{"101", domain.PositionRefusalOutOfRange},
		{"-1", domain.PositionRefusalOutOfRange},
		{"-0.1", domain.PositionRefusalOutOfRange},
		{"99999999999999999999", domain.PositionRefusalOutOfRange},
	}
	for _, tc := range refused {
		t.Run("refuses "+tc.input, func(t *testing.T) {
			_, err := domain.PositionFromPercentage(tc.input)
			require.ErrorIs(t, err, tc.reason)
		})
	}
}

func TestReadingPositionDisplayPercent(t *testing.T) {
	cases := []struct {
		fraction float64
		percent  int
	}{
		{0, 0},
		{0.35, 35},
		{0.29, 29},
		{0.57, 57},
		{0.996, 99},
		{0.999999, 99},
		{1, 100},
		{0.3529411, 35},
	}
	for _, tc := range cases {
		position := domain.ReadingPosition{Fraction: tc.fraction}
		assert.Equal(t, tc.percent, position.Percent(), "fraction %v", tc.fraction)
	}
}
