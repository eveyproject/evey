package ebml

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadVINT_SingleByte(t *testing.T) {
	// 1xxx xxxx — 1 byte, value bits = lower 7
	// 0x81 = 1000_0001 -> value = 0x81
	r := bytes.NewReader([]byte{0x81})
	val, n, err := ReadVINT(r)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, uint64(0x81), val)
}

func TestReadVINT_TwoBytes(t *testing.T) {
	// 01xx xxxx xxxx xxxx — 2 bytes
	// 0x40 0x01 -> raw value = 0x4001
	r := bytes.NewReader([]byte{0x40, 0x01})
	val, n, err := ReadVINT(r)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, uint64(0x4001), val)
}

func TestReadVINT_ThreeBytes(t *testing.T) {
	// 001x xxxx ... — 3 bytes
	// 0x20 0x00 0x01
	r := bytes.NewReader([]byte{0x20, 0x00, 0x01})
	val, n, err := ReadVINT(r)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, uint64(0x200001), val)
}

func TestReadVINT_FourBytes(t *testing.T) {
	// 0001 xxxx ... — 4 bytes
	// 0x10 0x00 0x00 0x01
	r := bytes.NewReader([]byte{0x10, 0x00, 0x00, 0x01})
	val, n, err := ReadVINT(r)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, uint64(0x10000001), val)
}

func TestReadVINT_ZeroByte(t *testing.T) {
	r := bytes.NewReader([]byte{0x00})
	_, _, err := ReadVINT(r)
	assert.ErrorContains(t, err, "invalid VINT")
}

func TestReadVINT_Empty(t *testing.T) {
	r := bytes.NewReader([]byte{})
	_, _, err := ReadVINT(r)
	assert.ErrorIs(t, err, io.EOF)
}

func TestReadDataSize(t *testing.T) {
	// 0x82 = 1000_0010 -> 1-byte width, marker bit = 0x80, value = 0x82 & ^0x80 = 2
	r := bytes.NewReader([]byte{0x82})
	size, n, err := ReadDataSize(r)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, int64(2), size)
}

func TestReadDataSize_UnknownSize(t *testing.T) {
	// 0xFF = all 1s for 1-byte VINT → unknown size
	r := bytes.NewReader([]byte{0xFF})
	size, n, err := ReadDataSize(r)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, int64(-1), size)
}

func TestReadDataSize_TwoBytes(t *testing.T) {
	// 0x40 0x80 -> 2 bytes, marker = 0x4000, value = 0x4080 & ^0x4000 = 0x80 = 128
	r := bytes.NewReader([]byte{0x40, 0x80})
	size, n, err := ReadDataSize(r)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, int64(128), size)
}

func TestReadElementID(t *testing.T) {
	// EBML Header ID: 0x1A45DFA3 — 4 bytes
	r := bytes.NewReader([]byte{0x1A, 0x45, 0xDF, 0xA3})
	id, n, err := ReadElementID(r)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, uint32(IDEBMLHeader), id)
}

func TestReadElementID_Segment(t *testing.T) {
	r := bytes.NewReader([]byte{0x18, 0x53, 0x80, 0x67})
	id, n, err := ReadElementID(r)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, uint32(IDSegment), id)
}

func TestOpenRealMKV(t *testing.T) {
	testFile := "/home/gravity/side_project/evey-media/testdata/test.mkv"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test.mkv not found, skipping real MKV test")
	}

	reader := NewReader()
	c, err := reader.Open(context.Background(), testFile)
	require.NoError(t, err)
	require.NotNil(t, c)

	assert.Equal(t, testFile, c.Path)
	assert.NotEmpty(t, c.Info.MuxingApp)
	assert.GreaterOrEqual(t, len(c.Tracks), 1, "expected at least 1 track, got %d", len(c.Tracks))

	hasVideo := false
	for _, tr := range c.Tracks {
		if tr.Type == "video" {
			hasVideo = true
			assert.NotNil(t, tr.Width)
			assert.NotNil(t, tr.Height)
			if tr.Width != nil {
				assert.Equal(t, uint32(320), *tr.Width)
			}
		}
	}
	assert.True(t, hasVideo, "expected video track")
}

func TestReadTracksRealMKV(t *testing.T) {
	testFile := "/home/gravity/side_project/evey-media/testdata/test.mkv"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test.mkv not found")
	}

	reader := NewReader()
	tracks, err := reader.ReadTracks(context.Background(), testFile)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tracks), 1)
}
