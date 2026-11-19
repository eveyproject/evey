package mkvgo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

// withHeadJunk copies a fixture whose last element ahead of the first Cluster
// is a Void and turns that Void's last n bytes into junk: the Void is declared
// n bytes shorter, so a reader lands on bytes that do not parse before the
// Cluster. Nothing moves, every offset in the file stays true. It is the shape
// of a library file a television refused: "0000" and two NULs between a Void
// and the first Cluster, left by an in-place tag edit.
func withHeadJunk(t *testing.T, src string, n int) string {
	t.Helper()
	b, err := os.ReadFile(src)
	require.NoError(t, err)
	vint := func(p int) (v, l int) {
		for l = 1; b[p]&(0x80>>(l-1)) == 0; l++ {
		}
		v = int(b[p]) & (0x80>>(l-1) - 1)
		for i := 1; i < l; i++ {
			v = v<<8 | int(b[p+i])
		}
		return v, l
	}
	id := func(p int) (v, l int) {
		for l = 1; b[p]&(0x80>>(l-1)) == 0; l++ {
		}
		for i := range l {
			v = v<<8 | int(b[p+i])
		}
		return v, l
	}
	_, il := id(0)
	s, sl := vint(il)
	p := il + sl + s // past the EBML header
	_, il = id(p)
	_, sl = vint(p + il)
	p += il + sl // into the Segment
	voidAt, voidIDLen, voidSizeLen, voidSize := -1, 0, 0, 0
	for {
		eid, il := id(p)
		s, sl := vint(p + il)
		if eid == 0x1F43B675 {
			break
		}
		if eid == 0xEC {
			voidAt, voidIDLen, voidSizeLen, voidSize = p, il, sl, s
		} else {
			voidAt = -1
		}
		p += il + sl + s
	}
	require.NotEqual(t, -1, voidAt, "the fixture must carry a Void right before its first Cluster")
	require.Equal(t, 1, voidSizeLen, "a one-byte Void size is rewritten in place")
	require.GreaterOrEqual(t, voidSize, n)
	b[voidAt+voidIDLen] = byte(0x80 | (voidSize - n))
	end := voidAt + voidIDLen + voidSizeLen + voidSize
	junk := []byte("0000\x00\x00\x00\x00\x00\x00")[:n]
	copy(b[end-n:end], junk)
	out := filepath.Join(t.TempDir(), "head-junk.mkv")
	require.NoError(t, os.WriteFile(out, b, 0o600))
	return out
}

// The head junk is counted: mkvgo raises a damaged finding for it without a
// tolerant walk, so the byte count came back 0 and the file read as clean to
// the playback gate and to the resync repair.
func TestProbeContainerDamage_CountsHeadJunk(t *testing.T) {
	a := New()
	clean := filepath.Join("testdata", "two_editions.mkv")
	n, err := a.ProbeContainerDamage(t.Context(), clean)
	require.NoError(t, err)
	require.Zero(t, n, "the untouched fixture is clean")

	damaged := withHeadJunk(t, clean, 6)
	n, err = a.ProbeContainerDamage(t.Context(), damaged)
	require.NoError(t, err)
	require.EqualValues(t, 6, n)

	d, err := a.Diagnose(t.Context(), damaged)
	require.NoError(t, err)
	require.True(t, d.Damaged())
	require.EqualValues(t, 6, d.DamagedBytes, "the media-health scan sees the same count")
}

func TestProbeContainerDamage_IgnoresOtherContainers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.mp4")
	require.NoError(t, os.WriteFile(p, []byte("not matroska"), 0o600))
	n, err := New().ProbeContainerDamage(t.Context(), p)
	require.NoError(t, err)
	require.Zero(t, n)
}

// mkvgo's Diagnose reads the head only as far as it needs: on a file whose
// Cues sit before the junk it answers healthy (measured on a 5 MB Lavf file
// with the same six bytes). The full reader's count is what decides, and the
// diagnosis then says damaged with the resync remedy.
func TestCountHeadJunk_OverridesAHealthyDiagnosis(t *testing.T) {
	damaged := withHeadJunk(t, filepath.Join("testdata", "two_editions.mkv"), 6)
	d := &mkv.Diagnosis{Healthy: true}
	New().countHeadJunk(t.Context(), damaged, d)
	require.False(t, d.Healthy)
	require.True(t, d.Damaged())
	require.EqualValues(t, 6, d.DamagedBytes)

	clean := &mkv.Diagnosis{Healthy: true}
	New().countHeadJunk(t.Context(), filepath.Join("testdata", "two_editions.mkv"), clean)
	require.True(t, clean.Healthy)
	require.Zero(t, clean.DamagedBytes)
}
