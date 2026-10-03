package demo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/pkg/demo"
)

// tinyMatroska is an EBML header and a Segment holding an Info, a Cues and a
// Cluster, each with a one-byte size: the shape the defects walk.
func tinyMatroska(segmentSize []byte) []byte {
	children := []byte{
		0x15, 0x49, 0xA9, 0x66, 0x82, 'i', 'n', // Info, 2 bytes
		0x1C, 0x53, 0xBB, 0x6B, 0x8A, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, // Cues, 10 bytes
		0x1F, 0x43, 0xB6, 0x75, 0x83, 'a', 'b', 'c', // Cluster, 3 bytes
	}
	out := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x80} // EBML header, empty
	out = append(out, 0x18, 0x53, 0x80, 0x67)
	if segmentSize == nil {
		segmentSize = []byte{0x80 | byte(len(children))}
	}
	out = append(out, segmentSize...)
	return append(out, children...)
}

func writeTemp(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.mkv")
	require.NoError(t, os.WriteFile(p, b, 0o644))
	return p
}

func TestDefectNoSeekIndexTurnsCuesIntoVoidOnce(t *testing.T) {
	orig := tinyMatroska(nil)
	p := writeTemp(t, orig)

	changed, err := applyDefect(p, demo.DefectNoSeekIndex)
	require.NoError(t, err)
	require.True(t, changed)

	got, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Len(t, got, len(orig), "the file keeps its length")
	cues := bytes.Index(orig, []byte{0x1C, 0x53, 0xBB, 0x6B})
	require.False(t, bytes.Contains(got, []byte{0x1C, 0x53, 0xBB, 0x6B}), "no Cues left")
	require.Equal(t, []byte{0xEC, 0x01, 0, 0, 0, 0, 0, 0, 15 - 9}, got[cues:cues+9], "a Void spanning the 15 bytes of the Cues")
	require.Equal(t, orig[:cues], got[:cues], "everything before is untouched")
	require.Equal(t, orig[cues+15:], got[cues+15:], "everything after is untouched")

	changed, err = applyDefect(p, demo.DefectNoSeekIndex)
	require.NoError(t, err)
	require.False(t, changed, "a second boot finds nothing to do")
}

func TestDefectTrailingBytesAppendsOnce(t *testing.T) {
	orig := tinyMatroska(nil)
	p := writeTemp(t, orig)

	changed, err := applyDefect(p, demo.DefectTrailingBytes)
	require.NoError(t, err)
	require.True(t, changed)
	st, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, int64(len(orig)+trailingBytes), st.Size())

	changed, err = applyDefect(p, demo.DefectTrailingBytes)
	require.NoError(t, err)
	require.False(t, changed, "a second boot finds the bytes already there")
}

func TestDefectRefusesASegmentOfUnknownSize(t *testing.T) {
	orig := tinyMatroska([]byte{0xFF})
	p := writeTemp(t, orig)
	for _, d := range []demo.Defect{demo.DefectNoSeekIndex, demo.DefectTrailingBytes} {
		changed, err := applyDefect(p, d)
		require.Error(t, err)
		require.False(t, changed)
	}
	got, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, orig, got, "a refused file is left as it was")
}
