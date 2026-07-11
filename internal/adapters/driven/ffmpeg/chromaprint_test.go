package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFpcalcRawOutput(t *testing.T) {
	t.Parallel()
	out := []byte(`DURATION=120
FINGERPRINT=1234567,890,42,-1,2147483647
`)
	fp, err := parseFpcalcRawOutput(out)
	require.NoError(t, err)
	require.Len(t, fp, 5)
	assert.Equal(t, uint32(1234567), fp[0])
	assert.Equal(t, uint32(890), fp[1])
	assert.Equal(t, uint32(42), fp[2])
	// -1 wraps to all-ones uint32.
	assert.Equal(t, uint32(0xFFFFFFFF), fp[3])
	assert.Equal(t, uint32(2147483647), fp[4])
}

func TestParseFpcalcRawOutput_MissingFingerprint(t *testing.T) {
	t.Parallel()
	_, err := parseFpcalcRawOutput([]byte("DURATION=120\n"))
	assert.ErrorContains(t, err, "FINGERPRINT line not found")
}
