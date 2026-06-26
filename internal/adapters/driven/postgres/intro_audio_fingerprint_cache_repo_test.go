package postgres

// Internal test file: accesses unexported encodeUint32Slice / decodeUint32Slice
// directly. The DB integration test (Upsert→Get) is in the external test suite
// via testhelpers, which would create a circular import from this internal file
// (testhelpers imports the postgres package). The pure encode/decode tests below
// run without any DB and must pass everywhere.
//
// Run with: CGO_ENABLED=0 go test ./internal/adapters/driven/postgres/ -run AudioFingerprint

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudioFingerprintEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		values []uint32
	}{
		{
			name:   "empty slice",
			values: []uint32{},
		},
		{
			name:   "zero value",
			values: []uint32{0},
		},
		{
			name:   "max uint32",
			values: []uint32{math.MaxUint32},
		},
		{
			name:   "zero and max",
			values: []uint32{0, math.MaxUint32},
		},
		{
			name:   "typical chromaprint fingerprint",
			values: []uint32{0x1A2B3C4D, 0xDEADBEEF, 0x00000001, 0xFFFFFFFE, 42},
		},
		{
			name:   "boundary values",
			values: []uint32{0, 1, math.MaxUint32 - 1, math.MaxUint32},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := encodeUint32Slice(tc.values)

			// Buffer must be exactly 4 bytes per value.
			assert.Len(t, buf, 4*len(tc.values))

			got, err := decodeUint32Slice(buf)
			require.NoError(t, err)
			assert.Equal(t, tc.values, got)
		})
	}
}

func TestAudioFingerprintDecodeRejectsNonMultipleOf4(t *testing.T) {
	t.Parallel()

	for _, badLen := range []int{1, 2, 3, 5, 7, 9} {
		buf := make([]byte, badLen)
		_, err := decodeUint32Slice(buf)
		require.Errorf(t, err, "expected error for buffer len %d", badLen)
	}
}

func TestAudioFingerprintEncodeLittleEndian(t *testing.T) {
	t.Parallel()
	// 0x01020304 in little-endian = bytes [0x04, 0x03, 0x02, 0x01]
	buf := encodeUint32Slice([]uint32{0x01020304})
	require.Equal(t, []byte{0x04, 0x03, 0x02, 0x01}, buf)
}
