package domain

import "testing"

func TestCoverageBandUsesExactBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		known int64
		total int64
		want  CoverageBand
	}{
		{name: "99 percent exactly", known: 99, total: 100, want: CoverageBand99Plus},
		{name: "just below 99 percent", known: 9899, total: 10000, want: CoverageBand97To99},
		{name: "97 percent exactly", known: 97, total: 100, want: CoverageBand97To99},
		{name: "just below 97 percent", known: 9699, total: 10000, want: CoverageBand95To97},
		{name: "95 percent exactly", known: 95, total: 100, want: CoverageBand95To97},
		{name: "just below 95 percent", known: 9499, total: 10000, want: CoverageBandBelow95},
		{name: "zero denominator", known: 0, total: 0, want: CoverageBandNoComparison},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CoverageBandFor(test.known, test.total); got != test.want {
				t.Fatalf("CoverageBandFor(%d, %d) = %q, want %q", test.known, test.total, got, test.want)
			}
		})
	}
}
