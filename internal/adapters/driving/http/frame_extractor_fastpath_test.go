package httpapi

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKF struct {
	packet []byte
	ext    string
	ptsMs  int64
	err    error
	calls  int
}

func (f *fakeKF) ExtractKeyframePacket(_ context.Context, _ string, _ int64) ([]byte, string, int64, error) {
	f.calls++
	return f.packet, f.ext, f.ptsMs, f.err
}

type fakeDec struct {
	out   []byte
	err   error
	calls int
}

func (d *fakeDec) DecodePacketToJPEG(_ context.Context, _ []byte, _ string, _ int) ([]byte, error) {
	d.calls++
	return d.out, d.err
}

type fakeFallback struct {
	out   []byte
	calls int
}

func (f *fakeFallback) ExtractFrame(_ context.Context, _ string, _ float64, _ int) ([]byte, error) {
	f.calls++
	return f.out, nil
}

func TestFastFrameExtractor_FastPathWhenClose(t *testing.T) {
	kf := &fakeKF{packet: []byte("pkt"), ext: ".h264", ptsMs: 9500} // req 10000ms → 500ms drift
	dec := &fakeDec{out: []byte("jpeg")}
	fb := &fakeFallback{out: []byte("fallback")}
	e := NewFastFrameExtractor(kf, dec, fb)

	out, err := e.ExtractFrame(context.Background(), "/m.mkv", 10.0, 640)
	require.NoError(t, err)
	assert.Equal(t, "jpeg", string(out), "close keyframe → fast path used")
	assert.Equal(t, 1, dec.calls)
	assert.Equal(t, 0, fb.calls, "fallback not called")
}

func TestFastFrameExtractor_FallbackWhenDistant(t *testing.T) {
	kf := &fakeKF{packet: []byte("pkt"), ext: ".h264", ptsMs: 3000} // req 10000ms → 7s drift > 2s
	dec := &fakeDec{out: []byte("jpeg")}
	fb := &fakeFallback{out: []byte("fallback")}
	e := NewFastFrameExtractor(kf, dec, fb)

	out, err := e.ExtractFrame(context.Background(), "/m.mkv", 10.0, 640)
	require.NoError(t, err)
	assert.Equal(t, "fallback", string(out), "distant keyframe → exact ffmpeg fallback")
	assert.Equal(t, 0, dec.calls, "no decode of a too-distant keyframe")
	assert.Equal(t, 1, fb.calls)
}

func TestFastFrameExtractor_FallbackForNonMatroska(t *testing.T) {
	kf := &fakeKF{packet: []byte("pkt"), ext: ".h264", ptsMs: 10000}
	dec := &fakeDec{out: []byte("jpeg")}
	fb := &fakeFallback{out: []byte("fallback")}
	e := NewFastFrameExtractor(kf, dec, fb)

	out, err := e.ExtractFrame(context.Background(), "/m.mp4", 10.0, 640)
	require.NoError(t, err)
	assert.Equal(t, "fallback", string(out))
	assert.Equal(t, 0, kf.calls, "mp4 never touches the EBML keyframe extractor")
}

func TestFastFrameExtractor_FallbackOnExtractError(t *testing.T) {
	kf := &fakeKF{err: errors.New("no Cues index")}
	dec := &fakeDec{out: []byte("jpeg")}
	fb := &fakeFallback{out: []byte("fallback")}
	e := NewFastFrameExtractor(kf, dec, fb)

	out, err := e.ExtractFrame(context.Background(), "/m.mkv", 10.0, 640)
	require.NoError(t, err)
	assert.Equal(t, "fallback", string(out), "extract error → fallback, never a failed snapshot")
	assert.Equal(t, 0, dec.calls)
	assert.Equal(t, 1, fb.calls)
}

func TestFastFrameExtractor_FallbackOnDecodeError(t *testing.T) {
	kf := &fakeKF{packet: []byte("pkt"), ext: ".ivf", ptsMs: 10000}
	dec := &fakeDec{err: errors.New("ffmpeg decode failed")}
	fb := &fakeFallback{out: []byte("fallback")}
	e := NewFastFrameExtractor(kf, dec, fb)

	out, err := e.ExtractFrame(context.Background(), "/m.webm", 10.0, 640)
	require.NoError(t, err)
	assert.Equal(t, "fallback", string(out), "decode error → fallback")
	assert.Equal(t, 1, dec.calls)
	assert.Equal(t, 1, fb.calls)
}
