// Package checked contains explicit conversions across integer widths.
package checked

import (
	"errors"
	"math"
)

var ErrOverflow = errors.New("integer conversion overflow")

func Int32FromInt(value int) (int32, error) {
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, ErrOverflow
	}
	// The range check above makes this narrowing conversion safe.
	return int32(value), nil
}

func Int64FromUint64(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, ErrOverflow
	}
	// The range check above makes this narrowing conversion safe.
	return int64(value), nil
}

func Uint32FromInt64(value int64) (uint32, error) {
	if value < 0 || value > math.MaxUint32 {
		return 0, ErrOverflow
	}
	// The range check above makes this narrowing conversion safe.
	return uint32(value), nil
}

func Uint32FromInt(value int) (uint32, error) {
	if value < 0 || uint64(value) > math.MaxUint32 {
		return 0, ErrOverflow
	}
	// The range check above makes this narrowing conversion safe.
	return uint32(value), nil
}

func Uint64FromInt64(value int64) (uint64, error) {
	if value < 0 {
		return 0, ErrOverflow
	}
	// The range check above makes this sign conversion safe.
	return uint64(value), nil
}

func Uint64FromInt(value int) (uint64, error) {
	if value < 0 {
		return 0, ErrOverflow
	}
	// The range check above makes this sign conversion safe.
	return uint64(value), nil
}
