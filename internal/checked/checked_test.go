package checked

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegerConversionsRejectOverflow(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{name: "int32 high", call: func() error { _, err := Int32FromInt(math.MaxInt); return err }},
		{name: "uint64 high", call: func() error { _, err := Int64FromUint64(math.MaxUint64); return err }},
		{name: "uint32 negative", call: func() error { _, err := Uint32FromInt64(-1); return err }},
		{name: "uint32 high", call: func() error { _, err := Uint32FromInt64(math.MaxInt64); return err }},
		{name: "uint32 int high", call: func() error { _, err := Uint32FromInt(math.MaxInt); return err }},
		{name: "uint64 negative", call: func() error { _, err := Uint64FromInt64(-1); return err }},
		{name: "uint64 int negative", call: func() error { _, err := Uint64FromInt(-1); return err }},
		{name: "float NaN", call: func() error { _, err := IntFromFloat64(math.NaN()); return err }},
		{name: "float fractional", call: func() error { _, err := IntFromFloat64(1.5); return err }},
	}
	if strconv.IntSize == 32 {
		tests = append(tests, struct {
			name string
			call func() error
		}{name: "int64 to int high", call: func() error { _, err := IntFromInt64(math.MaxInt64); return err }})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.call(), ErrOverflow)
		})
	}
}

func TestIntegerConversionsAllowBounds(t *testing.T) {
	got32, err := Int32FromInt(math.MaxInt32)
	require.NoError(t, err)
	require.Equal(t, int32(math.MaxInt32), got32)
	got64, err := Int64FromUint64(math.MaxInt64)
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), got64)
	gotUint32, err := Uint32FromInt64(math.MaxUint32)
	require.NoError(t, err)
	require.Equal(t, uint32(math.MaxUint32), gotUint32)
	gotInt, err := IntFromInt64(42)
	require.NoError(t, err)
	require.Equal(t, 42, gotInt)
	gotFloat, err := IntFromFloat64(42)
	require.NoError(t, err)
	require.Equal(t, 42, gotFloat)
	gotUint32FromInt, err := Uint32FromInt(42)
	require.NoError(t, err)
	require.Equal(t, uint32(42), gotUint32FromInt)
	gotUint64FromInt, err := Uint64FromInt(42)
	require.NoError(t, err)
	require.Equal(t, uint64(42), gotUint64FromInt)
}
