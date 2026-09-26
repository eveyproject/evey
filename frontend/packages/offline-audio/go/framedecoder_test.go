package offlineaudio

import (
	"bytes"
	"testing"

	"github.com/gravity-zero/ac3go/ac3"
	"github.com/gravity-zero/ac3go/pcm"
)

// referencePCM decodes frames back to back with one plain decoder, the
// output the player's frame-at-a-time path must reproduce byte for byte.
func referencePCM(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	dec := ac3.NewDecoder()
	if err := dec.SetDownmix(pcm.LayoutStereo); err != nil {
		t.Fatal(err)
	}
	var out []byte
	for _, f := range frames {
		if err := dec.DecodeFrame(f); err != nil {
			t.Fatal(err)
		}
		out = appendStereoS16(out, dec.Samples(0), dec.Samples(1))
	}
	return out
}

func newFrameDecoder(t *testing.T) *FrameDecoder {
	t.Helper()
	d, err := NewFrameDecoder(2)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFrameDecoderMatchesContinuousDecode(t *testing.T) {
	for _, fixture := range []string{"testdata/tone_48k_5p1_448k.ac3", "testdata/tone_48k_stereo_192k.ac3", "testdata/tones_48k_5p1_384k.eac3"} {
		frames := ac3Frames(t, fixture)
		d := newFrameDecoder(t)
		var got []byte
		for _, f := range frames {
			pcm, err := d.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if len(pcm) != ac3.BlocksPerFrame*ac3.SamplesPerBlock*4 {
				t.Fatalf("%s: frame gave %d bytes, want one frame of stereo s16", fixture, len(pcm))
			}
			got = append(got, pcm...)
		}
		if !bytes.Equal(got, referencePCM(t, frames)) {
			t.Fatalf("%s: frame-at-a-time PCM differs from the continuous decode", fixture)
		}
		if d.SampleRate() != 48000 || d.BadFrames() != 0 {
			t.Fatalf("%s: rate %d bad %d", fixture, d.SampleRate(), d.BadFrames())
		}
	}
}

// An E-AC-3 MP4 sample can carry several syncframes; every one of them plays.
func TestFrameDecoderSeveralFramesInOneUnit(t *testing.T) {
	frames := ac3Frames(t, "testdata/tone_48k_5p1_448k.ac3")
	d := newFrameDecoder(t)
	got, err := d.Decode(append(append([]byte(nil), frames[0]...), frames[1]...))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, referencePCM(t, frames[:2])) {
		t.Fatal("a two-frame unit did not decode as its two frames")
	}
}

// A damaged frame keeps its duration as silence, so the audio clock stays on
// the video's, and the next frame decodes again.
func TestFrameDecoderDamagedFrameIsSilence(t *testing.T) {
	frames := ac3Frames(t, "testdata/tone_48k_5p1_448k.ac3")
	var hdr ac3.Header
	if err := ac3.ParseHeader(frames[1], &hdr); err != nil {
		t.Fatal(err)
	}
	damaged := append([]byte(nil), frames[1]...)
	for i := hdr.AudioStartBit/8 + 1; i < len(damaged); i++ {
		damaged[i] = 0xFF
	}

	d := newFrameDecoder(t)
	const frameBytes = ac3.BlocksPerFrame * ac3.SamplesPerBlock * 4
	for i, f := range [][]byte{frames[0], damaged, frames[2]} {
		pcm, err := d.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		if len(pcm) != frameBytes {
			t.Fatalf("frame %d gave %d bytes, want %d", i, len(pcm), frameBytes)
		}
		silent := bytes.Equal(pcm, make([]byte, frameBytes))
		if silent != (i == 1) {
			t.Fatalf("frame %d: silent=%v", i, silent)
		}
	}
	if d.BadFrames() != 1 {
		t.Fatalf("bad frames = %d, want 1", d.BadFrames())
	}
}

func TestFrameDecoderIgnoresNonSyncframeBytes(t *testing.T) {
	d := newFrameDecoder(t)
	got, err := d.Decode([]byte("not an ac-3 syncframe at all"))
	if err != nil || len(got) != 0 || d.SampleRate() != 0 {
		t.Fatalf("got %d bytes, err %v, rate %d", len(got), err, d.SampleRate())
	}
	frames := ac3Frames(t, "testdata/tone_48k_stereo_192k.ac3")
	got, err = d.Decode(frames[0][:len(frames[0])-1])
	if err != nil || len(got) != 0 {
		t.Fatalf("a truncated frame gave %d bytes, err %v", len(got), err)
	}
}

// A 5.1 stream on an output that takes six channels comes out whole, each
// speaker in the slot Android's CHANNEL_OUT_5POINT1 gives it.
func TestFrameDecoderKeeps51InAndroidOrder(t *testing.T) {
	for _, fixture := range []string{"testdata/tone_48k_5p1_448k.ac3", "testdata/tones_48k_5p1_384k.eac3"} {
		frames := ac3Frames(t, fixture)
		d, err := NewFrameDecoder(8)
		if err != nil {
			t.Fatal(err)
		}
		ref := ac3.NewDecoder()
		for fi, f := range frames {
			got, err := d.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := ref.DecodeFrame(f); err != nil {
				t.Fatal(err)
			}
			if d.Channels() != 6 {
				t.Fatalf("%s: %d output channels, want 6", fixture, d.Channels())
			}
			layout := ref.OutputLayout()
			n := len(got) / 12
			if n != ac3.BlocksPerFrame*ac3.SamplesPerBlock {
				t.Fatalf("%s frame %d: %d samples per channel", fixture, fi, n)
			}
			// The 5.1 surrounds are ac3go's Ls/Rs, Android's back pair.
			source := pcm.Layout{pcm.ChannelLeft, pcm.ChannelRight, pcm.ChannelCenter, pcm.ChannelLFE, pcm.ChannelLeftSurround, pcm.ChannelRightSurround}
			for ch, name := range source {
				plane := ref.Samples(layout.Index(name))
				for i := 0; i < n; i += 97 {
					want := s16le(plane[i])
					have := uint16(got[(i*6+ch)*2]) | uint16(got[(i*6+ch)*2+1])<<8
					if have != want {
						t.Fatalf("%s frame %d: %s slot %d sample %d = %d, want %d", fixture, fi, name, ch, i, have, want)
					}
				}
			}
		}
	}
}

// A stereo stream stays stereo on a surround output: nothing to spread.
func TestFrameDecoderStereoStreamStaysStereo(t *testing.T) {
	frames := ac3Frames(t, "testdata/tone_48k_stereo_192k.ac3")
	d, err := NewFrameDecoder(8)
	if err != nil {
		t.Fatal(err)
	}
	var got []byte
	for _, f := range frames {
		pcm, err := d.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, pcm...)
	}
	if d.Channels() != 2 || !bytes.Equal(got, referencePCM(t, frames)) {
		t.Fatalf("channels %d, stereo output differs from the stereo decode", d.Channels())
	}
}

func TestBuildMixRoutesSurrounds(t *testing.T) {
	gains := func(mix [][]mixTap, src pcm.Layout, to pcm.Channel) map[pcm.Channel]float32 {
		out := map[pcm.Channel]float32{}
		for _, tap := range mix[android51.Index(to)] {
			out[src[tap.src]] = tap.gain
		}
		return out
	}
	// 3/1: the mono surround feeds both back speakers at -3 dB.
	l31 := pcm.Layout{pcm.ChannelLeft, pcm.ChannelCenter, pcm.ChannelRight, pcm.ChannelMonoSurround}
	m := buildMix(l31, 6)
	for _, back := range []pcm.Channel{pcm.ChannelBackLeft, pcm.ChannelBackRight} {
		if g := gains(m, l31, back); g[pcm.ChannelMonoSurround] != minus3dB || len(g) != 1 {
			t.Fatalf("%s <- %v", back, g)
		}
	}
	if len(m[android51.Index(pcm.ChannelLFE)]) != 0 {
		t.Fatal("a stream without LFE fed the LFE slot")
	}
	// 7.1 on a 5.1 output: side and back share the back speakers.
	m = buildMix(android71, 6)
	if g := gains(m, android71, pcm.ChannelBackLeft); g[pcm.ChannelSideLeft] != minus3dB || g[pcm.ChannelBackLeft] != minus3dB {
		t.Fatalf("BL <- %v", g)
	}
	// 7.1 on a 7.1 output: one to one.
	m = buildMix(android71, 8)
	for i, taps := range m {
		if len(taps) != 1 || taps[0].src != i || taps[0].gain != 1 {
			t.Fatalf("slot %d <- %v", i, taps)
		}
	}
}

// A multichannel stream whose very first frame is damaged: the layout is not
// known from a decode yet, but the frame still owes its duration. It comes out
// as 5.1 silence, and the next frame decodes as a cold start would, so every
// frame after it sits where it belongs.
func TestFrameDecoderFirstFrameDamagedKeepsTimeline(t *testing.T) {
	for _, fixture := range []string{"testdata/tone_48k_5p1_448k.ac3", "testdata/tones_48k_5p1_384k.eac3"} {
		frames := ac3Frames(t, fixture)
		var hdr ac3.Header
		if err := ac3.ParseHeader(frames[0], &hdr); err != nil {
			t.Fatal(err)
		}
		damaged := append([]byte(nil), frames[0]...)
		for i := hdr.AudioStartBit/8 + 1; i < len(damaged); i++ {
			damaged[i] = 0xFF
		}
		d, err := NewFrameDecoder(8)
		if err != nil {
			t.Fatal(err)
		}
		const n = ac3.BlocksPerFrame * ac3.SamplesPerBlock
		first, err := d.Decode(damaged)
		if err != nil {
			t.Fatal(err)
		}
		if d.Channels() != 6 || len(first) != n*6*2 || !bytes.Equal(first, make([]byte, n*6*2)) {
			t.Fatalf("%s: damaged first frame gave %d bytes over %d channels, want %d bytes of 5.1 silence", fixture, len(first), d.Channels(), n*6*2)
		}
		// The frame after it decodes cold, exactly like a fresh decoder's first.
		ref := ac3.NewDecoder()
		if err := ref.DecodeFrame(frames[1]); err != nil {
			t.Fatal(err)
		}
		second, err := d.Decode(frames[1])
		if err != nil {
			t.Fatal(err)
		}
		if len(second) != n*6*2 || d.BadFrames() != 1 {
			t.Fatalf("%s: next frame gave %d bytes, bad=%d", fixture, len(second), d.BadFrames())
		}
		layout := ref.OutputLayout()
		source := pcm.Layout{pcm.ChannelLeft, pcm.ChannelRight, pcm.ChannelCenter, pcm.ChannelLFE, pcm.ChannelLeftSurround, pcm.ChannelRightSurround}
		for ch, name := range source {
			plane := ref.Samples(layout.Index(name))
			for i := 0; i < n; i += 97 {
				want := s16le(plane[i])
				have := uint16(second[(i*6+ch)*2]) | uint16(second[(i*6+ch)*2+1])<<8
				if have != want {
					t.Fatalf("%s: frame after the damage, %s sample %d = %d, want %d", fixture, name, i, have, want)
				}
			}
		}
		// The whole stream: one frame's worth of output per frame, no drift.
		total := len(first) + len(second)
		for _, f := range frames[2:] {
			out, err := d.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			total += len(out)
		}
		if total != len(frames)*n*6*2 {
			t.Fatalf("%s: %d bytes for %d frames, want %d", fixture, total, len(frames), len(frames)*n*6*2)
		}
	}
}
