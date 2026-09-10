package ffmpeg

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
)

func TestParseKeyframeCSV(t *testing.T) {
	// ffprobe emits packets in DECODE order, so keyframe pts_time is non-monotonic
	// (B-frames jump around); the parser must sort. Keyframe = 'K' as the FIRST
	// flag char. N/A / non-K / malformed lines are skipped. Duplicate times deduped.
	in := []byte("0.000000,K__\n" +
		"0.083000,___\n" +
		"6.548000,K__\n" + // out of order vs the next keyframe
		"3.253000,K__\n" +
		"3.253000,K_\n" + // duplicate time, shorter flags
		"9.843000,___\n" + // not a keyframe
		"N/A,K__\n" + // unparseable pts
		"12.0,__K\n" + // K not first → not a keyframe packet
		"\n" + // blank
		"13.138000,K__\n")
	got := parseKeyframeCSV(in)
	assert.Equal(t, []int64{0, 3253, 6548, 13138}, got)
}

func TestParseKeyframeCSV_Empty(t *testing.T) {
	assert.Empty(t, parseKeyframeCSV(nil))
	assert.Empty(t, parseKeyframeCSV([]byte("0.5,___\n1.0,___\n")), "no keyframes ⇒ empty")
}

func TestParseKeyframeCSV_Rounding(t *testing.T) {
	// 0.0835 s → 84 ms (round half up), not 83.
	assert.Equal(t, []int64{84}, parseKeyframeCSV([]byte("0.083500,K__\n")))
}

func TestParseFrameRate_Valid(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"24000/1001", 24000.0 / 1001.0},
		{"30/1", 30.0},
		{"25/1", 25.0},
		{"60/1", 60.0},
		{"50/1", 50.0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseFrameRate(tc.input)
			assert.InDelta(t, tc.expected, got, 0.001)
		})
	}
}

func TestParseFrameRate_Invalid(t *testing.T) {
	cases := []struct {
		input string
	}{
		{"0/0"},
		{""},
		{"abc"},
		{"24/0"},
		{"abc/1"},
		{"24/abc"},
		{"nodivider"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseFrameRate(tc.input)
			assert.InDelta(t, 0.0, got, 0.001)
		})
	}
}

func TestCodecTypeToStreamType(t *testing.T) {
	cases := []struct {
		input    string
		expected library.StreamType
	}{
		{"video", library.StreamTypeVideo},
		{"audio", library.StreamTypeAudio},
		{"subtitle", library.StreamTypeSubtitle},
		{"attachment", library.StreamTypeAttachment},
		{"unknown", library.StreamTypeAttachment},
		{"data", library.StreamTypeAttachment},
		{"", library.StreamTypeAttachment},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := codecTypeToStreamType(tc.input)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestProbeStreams_InvalidBinary(t *testing.T) {
	p := NewProber("nonexistent_binary_xyz")
	_, err := p.ProbeStreams(context.Background(), "/tmp/fakefile.mkv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffprobe")
}

func TestProbeStreams_WithFakeScript(t *testing.T) {
	fakeJSON := `{"streams":[{"index":0,"codec_name":"h264","codec_type":"video","width":1920,"height":1080,"r_frame_rate":"24/1","disposition":{"default":1,"forced":0},"tags":{"language":"eng"}},{"index":1,"codec_name":"aac","codec_type":"audio","channels":6,"channel_layout":"5.1","sample_rate":"48000","disposition":{"default":1,"forced":0},"tags":{"language":"eng","title":"Surround"}}],"format":{"duration":"7200.5"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()

	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(7200500), result.Duration.Milliseconds())
	require.Len(t, result.Streams, 2)

	video := result.Streams[0]
	assert.Equal(t, 0, video.StreamIndex)
	assert.Equal(t, library.StreamTypeVideo, video.StreamType)
	assert.Equal(t, "h264", video.Codec)
	assert.Equal(t, "eng", video.Language)
	assert.True(t, video.IsDefault)
	assert.False(t, video.IsForced)
	require.NotNil(t, video.Width)
	assert.Equal(t, 1920, *video.Width)
	require.NotNil(t, video.Height)
	assert.Equal(t, 1080, *video.Height)
	require.NotNil(t, video.Framerate)
	assert.InDelta(t, 24.0, *video.Framerate, 0.001)

	audio := result.Streams[1]
	assert.Equal(t, 1, audio.StreamIndex)
	assert.Equal(t, library.StreamTypeAudio, audio.StreamType)
	assert.Equal(t, "aac", audio.Codec)
	assert.Equal(t, "eng", audio.Language)
	assert.Equal(t, "Surround", audio.Title)
	assert.True(t, audio.IsDefault)
	require.NotNil(t, audio.Channels)
	assert.Equal(t, 6, *audio.Channels)
	require.NotNil(t, audio.ChannelLayout)
	assert.Equal(t, "5.1", *audio.ChannelLayout)
	require.NotNil(t, audio.SampleRate)
	assert.Equal(t, 48000, *audio.SampleRate)
}

func TestProbeStreams_WithColorSpace(t *testing.T) {
	fakeJSON := `{"streams":[{"index":0,"codec_name":"hevc","codec_type":"video","width":3840,"height":2160,"r_frame_rate":"60/1","color_space":"bt2020nc","bits_per_raw_sample":"10","bit_rate":"20000000","disposition":{"default":1,"forced":0},"tags":{}}],"format":{"duration":"5400.0"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_hdr_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(5400000), result.Duration.Milliseconds())
	require.Len(t, result.Streams, 1)

	s := result.Streams[0]
	assert.Equal(t, "hevc", s.Codec)
	require.NotNil(t, s.ColorSpace)
	assert.Equal(t, "bt2020nc", *s.ColorSpace)
	require.NotNil(t, s.BitDepth)
	assert.Equal(t, 10, *s.BitDepth)
	require.NotNil(t, s.Bitrate)
	assert.Equal(t, int64(20000000), *s.Bitrate)
}

// color_transfer + the DOVI configuration record are the HDR ground truth the
// playback decision needs (PQ-vs-HLG, and the DoVi profile that decides
// transcode-vs-copy). The fake JSON mirrors real `ffprobe -show_streams`
// output for a Dolby Vision profile 8.1 (HDR10-compatible base layer) stream.
func TestProbeStreams_ColorTransferAndDoVi(t *testing.T) {
	fakeJSON := `{"streams":[{"index":0,"codec_name":"hevc","codec_type":"video","profile":"Main 10","level":153,"width":3840,"height":2160,"r_frame_rate":"24000/1001","color_space":"bt2020nc","color_transfer":"smpte2084","disposition":{"default":1,"forced":0},"tags":{},"side_data_list":[{"side_data_type":"DOVI configuration record","dv_version_major":1,"dv_version_minor":0,"dv_profile":8,"dv_level":6,"rpu_present_flag":1,"el_present_flag":0,"bl_present_flag":1,"dv_bl_signal_compatibility_id":1}]}],"format":{"duration":"5400.0"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_dovi_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.NoError(t, err)
	require.Len(t, result.Streams, 1)

	s := result.Streams[0]
	require.NotNil(t, s.ColorTransfer)
	assert.Equal(t, "smpte2084", *s.ColorTransfer)
	require.NotNil(t, s.Level)
	assert.Equal(t, 153, *s.Level)
	require.NotNil(t, s.DoViProfile)
	assert.Equal(t, 8, *s.DoViProfile)
	require.NotNil(t, s.DoViBLCompat)
	assert.Equal(t, 1, *s.DoViBLCompat)
}

// A stream without DoVi side data must leave the DoVi fields nil (nil = "no
// DoVi RPU", which the decision treats as plain HDR/SDR per color_transfer).
func TestProbeStreams_NoDoViSideData(t *testing.T) {
	fakeJSON := `{"streams":[{"index":0,"codec_name":"hevc","codec_type":"video","color_transfer":"arib-std-b67","disposition":{"default":1,"forced":0},"tags":{},"side_data_list":[{"side_data_type":"Content light level metadata"}]}],"format":{"duration":"60.0"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_nodovi_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.NoError(t, err)
	require.Len(t, result.Streams, 1)

	s := result.Streams[0]
	require.NotNil(t, s.ColorTransfer)
	assert.Equal(t, "arib-std-b67", *s.ColorTransfer)
	assert.Nil(t, s.DoViProfile)
	assert.Nil(t, s.DoViBLCompat)
}

func TestProbeStreams_EmptyStreams(t *testing.T) {
	fakeJSON := `{"streams":[],"format":{"duration":"0"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_empty_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/empty.mkv")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(0), result.Duration.Milliseconds())
	assert.Empty(t, result.Streams)
}

func TestProbeStreams_InvalidJSON(t *testing.T) {
	script, err := os.CreateTemp("", "fake_ffprobe_bad_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho 'not valid json'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	_, err = p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffprobe parse")
}

func TestParseIdetSummary(t *testing.T) {
	// Real ffmpeg output: an initial all-zero summary then the cumulative one.
	out := []byte(`[Parsed_idet_0 @ 0x1] Multi frame detection: TFF:     0 BFF:     0 Progressive:     0 Undetermined:     0
[Parsed_idet_0 @ 0x2] Multi frame detection: TFF:   200 BFF:     0 Progressive:     0 Undetermined:     0`)
	interlaced, ok, err := parseIdetSummary(out)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, interlaced, "TFF-dominant must read interlaced (last summary wins over the zero one)")

	// Progressive majority → not interlaced.
	prog := []byte(`x Multi frame detection: TFF:   104 BFF:   14 Progressive:   182 Undetermined:     0`)
	interlaced, ok, err = parseIdetSummary(prog)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, interlaced, "progressive majority must not flip to deint=all")

	// All undetermined (static / no motion) → ok=false, caller retries/leaves NULL.
	none := []byte(`x Multi frame detection: TFF:     0 BFF:     0 Progressive:     0 Undetermined:   300`)
	_, ok, err = parseIdetSummary(none)
	require.NoError(t, err)
	assert.False(t, ok, "zero determined frames ⇒ ok=false")

	// No summary line at all → error.
	_, _, err = parseIdetSummary([]byte("garbage"))
	require.Error(t, err)
}

// TestProbeStreams_ExtendedDispositions verifies that the 5 extended disposition
// fields (comment / hearing_impaired / visual_impaired / descriptions / original)
// are parsed from the ffprobe JSON and mapped to the corresponding MediaStream
// fields (Commentary / HearingImpaired / VisualImpaired / TextDescriptions /
// Original).
func TestProbeStreams_ExtendedDispositions(t *testing.T) {
	// Stream 0: audio with comment=1 (director's commentary).
	// Stream 1: subtitle with hearing_impaired=1, visual_impaired=1.
	// Stream 2: audio with descriptions=1 and original=1.
	fakeJSON := `{"streams":[` +
		`{"index":0,"codec_name":"aac","codec_type":"audio","channels":2,"sample_rate":"48000","disposition":{"default":0,"forced":0,"comment":1,"hearing_impaired":0,"visual_impaired":0,"descriptions":0,"original":0},"tags":{"language":"eng"}},` +
		`{"index":1,"codec_name":"subrip","codec_type":"subtitle","disposition":{"default":0,"forced":0,"comment":0,"hearing_impaired":1,"visual_impaired":1,"descriptions":0,"original":0},"tags":{"language":"eng"}},` +
		`{"index":2,"codec_name":"aac","codec_type":"audio","channels":2,"sample_rate":"48000","disposition":{"default":1,"forced":0,"comment":0,"hearing_impaired":0,"visual_impaired":0,"descriptions":1,"original":1},"tags":{"language":"eng"}}` +
		`],"format":{"duration":"3600.0"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_disp_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/multi.mkv")
	require.NoError(t, err)
	require.Len(t, result.Streams, 3)

	audioCommentary := result.Streams[0]
	assert.True(t, audioCommentary.Commentary, "comment=1 must set Commentary")
	assert.False(t, audioCommentary.HearingImpaired)
	assert.False(t, audioCommentary.VisualImpaired)
	assert.False(t, audioCommentary.TextDescriptions)
	assert.False(t, audioCommentary.Original)

	sub := result.Streams[1]
	assert.False(t, sub.Commentary)
	assert.True(t, sub.HearingImpaired, "hearing_impaired=1 must set HearingImpaired")
	assert.True(t, sub.VisualImpaired, "visual_impaired=1 must set VisualImpaired")
	assert.False(t, sub.TextDescriptions)
	assert.False(t, sub.Original)

	audioDesc := result.Streams[2]
	assert.False(t, audioDesc.Commentary)
	assert.False(t, audioDesc.HearingImpaired)
	assert.False(t, audioDesc.VisualImpaired)
	assert.True(t, audioDesc.TextDescriptions, "descriptions=1 must set TextDescriptions")
	assert.True(t, audioDesc.Original, "original=1 must set Original")
}

// normalizeContainer collapses ffprobe's comma-joined format_name to the single
// name the playback decision matches on. An empty/unmapped container reads as
// "container_incompatible" downstream, so the mapping must be right.
func TestNormalizeContainer(t *testing.T) {
	cases := map[string]string{
		"matroska,webm":           "matroska",
		"webm":                    "matroska",
		"MATROSKA":                "matroska",
		"mov,mp4,m4a,3gp,3g2,mj2": "mp4",
		"mp4":                     "mp4",
		"avi":                     "avi",
		"mpegts":                  "mpegts",
		"flv":                     "flv",
		"":                        "",
		"  matroska,webm  ":       "matroska",
	}
	for in, want := range cases {
		if got := normalizeContainer(in); got != want {
			t.Errorf("normalizeContainer(%q) = %q, want %q", in, got, want)
		}
	}
}

// A Matroska PNG cover stored as a plain video track carries no attached_pic
// disposition; it is still cover art when a real video stream sits beside it
// and must not reach the library as a second "video" stream.
func TestProbeStreams_PNGCoverTrackWithoutAttachedPicIsSkipped(t *testing.T) {
	fakeJSON := `{"streams":[{"index":0,"codec_name":"h264","codec_type":"video","width":1920,"height":804,"r_frame_rate":"24000/1001","disposition":{"default":1}},{"index":1,"codec_name":"ac3","codec_type":"audio","channels":6,"sample_rate":"48000","disposition":{"default":1}},{"index":2,"codec_name":"png","codec_type":"video","width":200,"height":258,"r_frame_rate":"90000/1","disposition":{"default":0,"attached_pic":0},"tags":{"title":"cover"}}],"format":{"duration":"6960"}}`
	p := NewProber(fakeFFprobe(t, fakeJSON))
	result, err := p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.NoError(t, err)
	require.Len(t, result.Streams, 2)
	assert.Equal(t, "h264", result.Streams[0].Codec)
	assert.Equal(t, library.StreamTypeAudio, result.Streams[1].StreamType)
}

// Camera footage whose only video stream is MJPEG keeps it: the still-image
// codec is cover art only next to a real video stream.
func TestProbeStreams_SoleMJPEGVideoIsKept(t *testing.T) {
	fakeJSON := `{"streams":[{"index":0,"codec_name":"mjpeg","codec_type":"video","width":1280,"height":720,"r_frame_rate":"30/1","disposition":{"default":1}},{"index":1,"codec_name":"pcm_s16le","codec_type":"audio","channels":2,"sample_rate":"48000","disposition":{"default":1}}],"format":{"duration":"12"}}`
	p := NewProber(fakeFFprobe(t, fakeJSON))
	result, err := p.ProbeStreams(context.Background(), "/fake/clip.avi")
	require.NoError(t, err)
	require.Len(t, result.Streams, 2)
	assert.Equal(t, "mjpeg", result.Streams[0].Codec)
	assert.Equal(t, library.StreamTypeVideo, result.Streams[0].StreamType)
}

func fakeFFprobe(t *testing.T, fakeJSON string) string {
	t.Helper()
	script, err := os.CreateTemp("", "fake_ffprobe_*.sh")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(script.Name()) })
	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	require.NoError(t, script.Close())
	require.NoError(t, os.Chmod(script.Name(), 0755))
	return script.Name()
}

func TestParseIdetVerdict_Telecine(t *testing.T) {
	pulldown := []byte("[Parsed_idet_0] Repeated Fields: Neither:   180 Top:    60 Bottom:    60\n[Parsed_idet_0] Multi frame detection: TFF:   200 BFF:     0 Progressive:   100 Undetermined:     0\n")
	v, ok, err := parseIdetVerdict(pulldown)
	if err != nil || !ok || !v.Interlaced || !v.Telecine {
		t.Fatalf("3:2 pulldown = %+v ok=%v err=%v, want interlaced+telecine", v, ok, err)
	}
	interlaced := []byte("[Parsed_idet_0] Repeated Fields: Neither:   298 Top:     1 Bottom:     1\n[Parsed_idet_0] Multi frame detection: TFF:   290 BFF:     0 Progressive:    10 Undetermined:     0\n")
	v, _, _ = parseIdetVerdict(interlaced)
	if !v.Interlaced || v.Telecine {
		t.Errorf("clean interlaced = %+v, want interlaced only", v)
	}
}
