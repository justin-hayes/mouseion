package domain

import (
	"math"
	"regexp"
	"strings"
	"time"
)

// PositionSource is where a Position report came from. Manual entry is the only
// source so far; device sources arrive with their own specs.
type PositionSource string

const PositionSourceManual PositionSource = "manual"

// ReadingPosition is a Book's latest known place in it: its latest Position
// report. Fraction is the fraction of the whole Book from 0 to 1, held to six
// decimal places. A Book with no report has no Reading position, which is not
// the same as 0%.
type ReadingPosition struct {
	Fraction   float64
	Source     PositionSource
	RecordedAt time.Time
}

// Percent is the whole percentage shown for the position, rounded down so a
// position of 99.6% never reads as 100%.
func (p ReadingPosition) Percent() int {
	// The stored fraction has six decimals, so scaling to millionths is exact
	// and the integer division floors without float error (0.29 stays 29).
	millionths := int64(math.Round(p.Fraction * 1e6))
	return int(millionths / 10000)
}

// PositionRefusalError is the stable reason a learner's position input is refused.
type PositionRefusalError string

const (
	// ErrPositionMissing: nothing was entered.
	ErrPositionMissing PositionRefusalError = "missing"
	// ErrPositionNotANumber: the input is not a number with at most one
	// decimal place.
	ErrPositionNotANumber PositionRefusalError = "not_a_number"
	// ErrPositionOutOfRange: the percentage is below 0 or above 100.
	ErrPositionOutOfRange PositionRefusalError = "out_of_range"
)

func (r PositionRefusalError) Error() string { return "reading position refused: " + string(r) }

var percentagePattern = regexp.MustCompile(`^([+-]?)(\d+)(?:\.(\d+))?$`)

// maxPercentageDigits bounds the whole-number digits read, so an absurdly long
// number is out of range rather than an overflow.
const maxPercentageDigits = 6

// PositionFromPercentage turns a learner's percentage, from 0 to 100 with at
// most one decimal place, into a fraction from 0 to 1. A trailing percent sign
// is allowed.
func PositionFromPercentage(input string) (float64, error) {
	text := strings.TrimSpace(input)
	if text == "" {
		return 0, ErrPositionMissing
	}
	text = strings.TrimSpace(strings.TrimSuffix(text, "%"))
	match := percentagePattern.FindStringSubmatch(text)
	if match == nil {
		return 0, ErrPositionNotANumber
	}
	negative := match[1] == "-"
	whole := strings.TrimLeft(match[2], "0")
	decimals := strings.TrimRight(match[3], "0")
	if len(decimals) > 1 {
		return 0, ErrPositionNotANumber
	}
	if len(whole) > maxPercentageDigits {
		return 0, ErrPositionOutOfRange
	}
	tenths := 0
	for _, digit := range whole + decimals + strings.Repeat("0", 1-len(decimals)) {
		tenths = tenths*10 + int(digit-'0')
	}
	if (negative && tenths > 0) || tenths > 1000 {
		return 0, ErrPositionOutOfRange
	}
	return float64(tenths) / 1000, nil
}
