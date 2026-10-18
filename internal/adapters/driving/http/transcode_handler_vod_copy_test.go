package httpapi

// Tests for the copy-rung routing guard isCopyRequest, in particular the disk
// guard (fix c): the whole-file strategy duplicates ~the source size on disk, so
// a source that cannot fit must fall back to the segment-bounded transcode
// ladder instead of risking ENOSPC on a small box.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// copyEligibleStreams is one H.264 High@L4.0 8-bit video stream - the source a
// copy rung accepts (sourceCopyVideoCodec returns a non-empty avc1.640028).
func copyEligibleStreams() []*domainlibrary.MediaStream {
	w, hgt, level := 1920, 1080, 40
	return []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo,
		Codec:      "h264",
		Profile:    "High",
		Width:      &w,
		Height:     &hgt,
		Level:      &level,
		Keyframes:  []int64{0, 6000, 12000}, // keyframe-backfilled → copy-eligible
	}}
}

// newCopyTestHandler builds a minimal hlsVODServe with the copy path wired
// (copyProducer non-nil) and a real on-disk SegmentCache so FreeBytes() statfs
// returns the temp filesystem's real free space.
func newCopyTestHandler(t *testing.T) *hlsVODServe {
	t.Helper()
	cache, err := apptranscoding.NewSegmentCache(t.TempDir())
	require.NoError(t, err)
	// copyProducer only needs to be non-nil for isCopyRequest's "copy path wired"
	// gate; the disk guard reads cache.FreeBytes(), never the producer. A copy-mode
	// SoftwareProducer mirrors production (process/source nil - not exercised here).
	copyProd := apptranscoding.NewSoftwareProducer(cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil), nil)
	return &hlsVODServe{
		cache:        cache,
		copyProducer: copyProd,
	}
}

func TestIsCopyRequest_DiskGuard(t *testing.T) {
	h := newCopyTestHandler(t)
	dec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	streams := copyEligibleStreams()

	// A small source fits in the (large, real) temp filesystem → copy allowed.
	assert.True(t, h.isCopyRequest(dec, streams, 1<<20), "1MB source fits → copy rung")
	// Unknown size (0) → fail-open so behaviour matches the pre-guard path.
	assert.True(t, h.isCopyRequest(dec, streams, 0), "unknown size → fail-open allows copy")
	// A source larger than any real free space → guard refuses → transcode ladder.
	assert.False(t, h.isCopyRequest(dec, streams, 1<<60), "source can't fit on disk → fallback transcode")
}

func TestIsCopyRequest_NonCopyDecisionUnaffected(t *testing.T) {
	h := newCopyTestHandler(t)
	// A transcode decision is never a copy request, regardless of disk space.
	got := h.isCopyRequest(domaintranscoding.PlaybackDecision{VideoDecision: "transcode"}, copyEligibleStreams(), 1<<20)
	assert.False(t, got)
}

// Safety for the now-on-by-default rung: a copy-eligible codec/level but NO
// keyframe index (not yet keyframe-backfilled) must NOT route to copy - the plan
// would be empty and break playback. It falls back to transcode instead.
func TestIsCopyRequest_NoKeyframes_FallsBackToTranscode(t *testing.T) {
	h := newCopyTestHandler(t)
	dec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	w, hgt, level := 1920, 1080, 40
	unbackfilled := []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo,
		Codec:      "h264",
		Profile:    "High",
		Width:      &w,
		Height:     &hgt,
		Level:      &level,
		// Keyframes: nil - level is known but keyframes_ms not yet backfilled.
	}}
	assert.False(t, h.isCopyRequest(dec, unbackfilled, 1<<20),
		"eligible codec but no keyframe index → transcode, not copy (an empty plan would break)")
}

// Phase H1: the HEVC copy rung. Eligibility mirrors CopyRungEligible - the
// handler-level wiring (sourceCopyVideoCodec → CopySource projection) must
// thread the HDR ground-truth fields, or every HEVC title silently falls back
// to transcode (or worse, a DoVi P5 slips through).
func TestIsCopyRequest_HEVC(t *testing.T) {
	h := newCopyTestHandler(t)
	dec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	w, hgt, level := 3840, 2160, 153
	pq := "smpte2084"
	hevc := func(mutate func(*domainlibrary.MediaStream)) []*domainlibrary.MediaStream {
		s := &domainlibrary.MediaStream{
			StreamType:    domainlibrary.StreamTypeVideo,
			Codec:         "hevc",
			Profile:       "Main 10",
			Width:         &w,
			Height:        &hgt,
			Level:         &level,
			ColorTransfer: &pq,
			Keyframes:     []int64{0, 6000, 12000},
		}
		if mutate != nil {
			mutate(s)
		}
		return []*domainlibrary.MediaStream{s}
	}

	assert.True(t, h.isCopyRequest(dec, hevc(nil), 1<<20),
		"HEVC Main 10 with probed ground truth routes to copy")
	assert.False(t, h.isCopyRequest(dec, hevc(func(s *domainlibrary.MediaStream) { s.ColorTransfer = nil }), 1<<20),
		"unprobed color_transfer gates the HEVC copy off")
	assert.False(t, h.isCopyRequest(dec, hevc(func(s *domainlibrary.MediaStream) { s.DoViProfile = new(5) }), 1<<20),
		"DoVi profile 5 never routes to copy")
	assert.True(t, h.isCopyRequest(dec, hevc(func(s *domainlibrary.MediaStream) { s.DoViProfile = new(8); s.DoViBLCompat = new(1) }), 1<<20),
		"DoVi 8.1 (HDR10-compatible base) routes to copy")
	assert.False(t, h.isCopyRequest(dec, hevc(func(s *domainlibrary.MediaStream) { s.Keyframes = nil }), 1<<20),
		"no keyframe index → transcode (empty plan would break)")
}

// The VFR gate must survive the handler-level projection: CopyRungEligible
// refuses vfr sources (HLS copy segments drift A/V on them), but the refusal
// only bites if sourceCopyVideoCodec threads FrameRateMode into CopySource.
func TestIsCopyRequest_VFRSourceNeverRoutesToCopy(t *testing.T) {
	h := newCopyTestHandler(t)
	dec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	w, hgt, level := 1920, 1080, 40
	vfr := func(mode string) []*domainlibrary.MediaStream {
		return []*domainlibrary.MediaStream{{
			StreamType:    domainlibrary.StreamTypeVideo,
			Codec:         "h264",
			Profile:       "High",
			Width:         &w,
			Height:        &hgt,
			Level:         &level,
			Keyframes:     []int64{0, 6000, 12000},
			FrameRateMode: mode,
		}}
	}
	assert.True(t, h.isCopyRequest(dec, vfr(""), 1<<20),
		"unknown frame-rate mode keeps today's behavior (copy allowed)")
	assert.True(t, h.isCopyRequest(dec, vfr("cfr"), 1<<20),
		"cfr routes to copy")
	assert.False(t, h.isCopyRequest(dec, vfr("vfr"), 1<<20),
		"vfr must fall back to transcode (CFR output), never copy")
}

// Regression lock (caught on prod bytes, SATAVG 3.1): an HDR-capable client
// (hdr=1) suppresses the decision-level tonemap, but a transcoded rung is SDR
// by contract - when the request will NOT be served as a raw copy (keyframes
// not yet backfilled, copy off, non-wired profile), the re-encode MUST
// tonemap or the PQ source renders washed-out.
func TestMakeDecision_HDRClientStillTonemapsWhenCopyUnavailable(t *testing.T) {
	w, hgt, level := 3840, 2160, 153
	pq := "smpte2084"
	hevcHDR := func(keyframes []int64) []*domainlibrary.MediaStream {
		return []*domainlibrary.MediaStream{
			{
				StreamType:    domainlibrary.StreamTypeVideo,
				Codec:         "hevc",
				Profile:       "Main 10",
				Width:         &w,
				Height:        &hgt,
				Level:         &level,
				ColorTransfer: &pq,
				ColorSpace:    func() *string { s := "bt2020nc"; return &s }(),
				Keyframes:     keyframes,
			},
			{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Channels: func() *int { c := 2; return &c }(), IsDefault: true},
		}
	}
	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4&hdr=1", nil)
		// makeDecision reads the auth middleware's claims; inject like prod does.
		return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
			&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	}

	// Copy rung AVAILABLE: the raw HDR serve needs no tonemap.
	h := newCopyTestHandler(t)
	d := h.makeDecision(context.Background(), hevcHDR([]int64{0, 6000, 12000}), "mp4", "", newReq())
	assert.Equal(t, "copy", d.VideoDecision)
	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed",
		"raw HDR copy to an HDR client must not tonemap")

	// Copy rung UNAVAILABLE (no keyframe index yet): the re-encode must tonemap.
	d = h.makeDecision(context.Background(), hevcHDR(nil), "mp4", "", newReq())
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed",
		"an HDR source that will be transcoded must tonemap even for an hdr=1 client")

	// Copy producer OFF entirely: same forced tonemap.
	hOff := &hlsVODServe{cache: h.cache}
	d = hOff.makeDecision(context.Background(), hevcHDR([]int64{0, 6000}), "mp4", "", newReq())
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed")

	// SDR source: never gains a tonemap reason.
	bt709 := "bt709"
	sdr := []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Profile: "Main",
		Width: &w, Height: &hgt, Level: &level, ColorTransfer: &bt709,
	}}
	d = hOff.makeDecision(context.Background(), sdr, "mp4", "", newReq())
	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
}

func TestIsCopyRequest_ProducerOffNeverCopies(t *testing.T) {
	// copyProducer nil (EVEY_REMUX_COPY off) → never a copy request, even with an
	// eligible source and ample disk.
	h := &hlsVODServe{}
	got := h.isCopyRequest(domaintranscoding.PlaybackDecision{VideoDecision: "copy"}, copyEligibleStreams(), 1<<20)
	assert.False(t, got)
}

// Phase A2: passthrough eligibility + the codec the master must advertise.
// The two MUST stay coherent - a master claiming mp4a over ac-3 segments (or
// the reverse) is refused by Safari.
func TestAudioCopyEligibilityAndServedCodec(t *testing.T) {
	copyDec := domaintranscoding.PlaybackDecision{AudioDecision: "copy"}
	transDec := domaintranscoding.PlaybackDecision{AudioDecision: "transcode", AudioCodec: "aac"}

	// A client that opted into MP3-in-fMP4 (our ExoPlayer apps) vs one that
	// merely DECODES mp3, which every web/iOS client declares unconditionally.
	mp3Client := domaintranscoding.ClientProfile{FMP4AudioPassthrough: []string{"mp3"}}
	plainClient := domaintranscoding.ClientProfile{AudioCodecs: []string{"aac", "mp3"}}

	cases := []struct {
		name      string
		dec       domaintranscoding.PlaybackDecision
		normalize bool
		profiles  int
		src       string
		client    domaintranscoding.ClientProfile
		eligible  bool
		served    string
	}{
		{"aac copy", copyDec, false, 1, "aac", plainClient, true, "aac"},
		{"ac3 passthrough (Safari-class client)", copyDec, false, 1, "ac3", plainClient, true, "ac3"},
		{"eac3 passthrough", copyDec, false, 1, "eac3", plainClient, true, "eac3"},
		{"dts never passes through", copyDec, false, 1, "dts", plainClient, false, ""},
		// The 2026-09-06 bug: an old DVD rip's MP3 track was re-encoded to AAC
		// for every client. Decoding mp3 is NOT the opt-in.
		{"mp3 stays re-encoded without the opt-in", copyDec, false, 1, "mp3", plainClient, false, ""},
		{"mp3 passes through for an opted-in client", copyDec, false, 1, "mp3", mp3Client, true, "mp3"},
		{"opt-in cannot unlock a codec we can't mux", copyDec, false, 1, "dts", domaintranscoding.ClientProfile{FMP4AudioPassthrough: []string{"dts"}}, false, ""},
		{"loudnorm forces re-encode", copyDec, true, 1, "ac3", plainClient, false, ""},
		{"multi-rung ladder forces re-encode", copyDec, false, 3, "aac", plainClient, false, ""},
		{"transcode verdict stays aac", transDec, false, 1, "ac3", plainClient, false, "aac"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := audioCopyEligible(c.dec, c.normalize, c.profiles, c.src, c.client)
			if got != c.eligible {
				t.Fatalf("eligible = %v, want %v", got, c.eligible)
			}
			if served := servedAudioCodec(got, c.src, c.dec); served != c.served {
				t.Fatalf("servedAudioCodec = %q, want %q", served, c.served)
			}
		})
	}
}

// Mid-play decision pinning - regression lock for the prod "green frames on
// Chromecast" incident (2026-07-07): a playback started as
// TRANSCODE; ~22 min in, the keyframe backfill worker indexed the file, the
// copy rung became eligible, and the next EVENT-playlist poll of the SAME
// URLs flipped the served bitstream to the source h264 under the transcode
// init/grid the player had already consumed → hardware decoders output
// garbage (green), audio survived. The first playlist of a playback stamps
// vd=<decision> into every child URL; makeDecision honors the pin so a live
// session's decision can never flip under it.
func TestMakeDecision_PinnedVideoDecision(t *testing.T) {
	h := newCopyTestHandler(t)
	streams := copyEligibleStreams()
	newReq := func(vd string) *http.Request {
		url := "/api/v1/media/m1/hls/stream_0.m3u8?videoCodecs=h264&audioCodecs=aac&containers=mp4"
		if vd != "" {
			url += "&vd=" + vd
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
			&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	}

	// Precondition: with keyframes backfilled the fresh decision is COPY.
	d := h.makeDecision(context.Background(), streams, "matroska", "", newReq(""))
	require.Equal(t, "copy", d.VideoDecision)

	// The session started BEFORE the backfill (its URLs carry vd=transcode):
	// the pin holds even though the fresh decision would now say copy.
	d = h.makeDecision(context.Background(), streams, "matroska", "", newReq("transcode"))
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "decision_pinned")
	assert.NotEmpty(t, d.VideoCodec, "a pinned re-encode still needs an encoder")

	// vd=copy on a still-copy decision: plain no-op.
	d = h.makeDecision(context.Background(), streams, "matroska", "", newReq("copy"))
	assert.Equal(t, "copy", d.VideoDecision)

	// Garbage vd values are ignored.
	d = h.makeDecision(context.Background(), streams, "matroska", "", newReq("banana"))
	assert.Equal(t, "copy", d.VideoDecision)
}

// The pinned re-encode of an HDR source must still tonemap (the session's
// master advertised an SDR rung) - the pin runs BEFORE the tonemap re-add.
func TestMakeDecision_PinnedTranscodeStillTonemapsHDR(t *testing.T) {
	w, hgt, level := 3840, 2160, 153
	pq := "smpte2084"
	bt2020 := "bt2020nc"
	streams := []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Profile: "Main 10",
		Width: &w, Height: &hgt, Level: &level,
		ColorTransfer: &pq, ColorSpace: &bt2020,
		Keyframes: []int64{0, 6000, 12000},
	}}
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/media/m1/hls/stream_0.m3u8?videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4&hdr=1&vd=transcode", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))

	h := newCopyTestHandler(t)
	d := h.makeDecision(context.Background(), streams, "matroska", "", req)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed",
		"a pinned transcode of a PQ source must tonemap like any other re-encode")
}

// The pin stamp: the first playlist mints vd from its own decision; any
// request already carrying a pin passes it through VERBATIM (children must
// inherit the original stamp, never re-derive it from mutable DB state).
func TestPinnedVideoDecision_MintAndPassthrough(t *testing.T) {
	reqWith := func(rawQuery string) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/x?"+rawQuery, nil)
	}
	copyDec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	trDec := domaintranscoding.PlaybackDecision{VideoDecision: "transcode"}

	assert.Equal(t, "copy", pinnedVideoDecision(reqWith(""), copyDec, true), "first playlist mints its decision")
	assert.Equal(t, "transcode", pinnedVideoDecision(reqWith(""), trDec, false))
	assert.Equal(t, "transcode", pinnedVideoDecision(reqWith("vd=transcode"), copyDec, true),
		"an inherited pin wins over the fresh decision - that IS the fix")
	assert.Equal(t, "copy", pinnedVideoDecision(reqWith("vd=copy"), trDec, false))
	assert.Equal(t, "transcode", pinnedVideoDecision(reqWith("vd=banana"), trDec, false), "garbage pin is re-minted")
}

// The decision says copy from the codecs alone; the copy RUNG has gates of its
// own (keyframe index, level, colour truth) and the master serves a re-encode
// when one is closed. Pinning "copy" there let the children switch to copied
// segments under the transcode init the moment the gate opened mid-play (the
// keyframe backfill): the stamp must name what the master SERVES.
func TestPinnedVideoDecision_MintsWhatTheMasterServes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	copyDec := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	assert.Equal(t, "transcode", pinnedVideoDecision(req, copyDec, false),
		"a copy decision served as a re-encode is pinned transcode")
}

// A user audio resync (audioOffsetMs) can only be applied by re-encoding the
// audio from a shifted input: the decision must force audio transcode even
// when passthrough would otherwise win, with an explicit reason.
func TestMakeDecision_AudioOffsetForcesAudioTranscode(t *testing.T) {
	h := newCopyTestHandler(t)
	streams := copyEligibleStreams()
	streams = append(streams, &domainlibrary.MediaStream{
		StreamType: domainlibrary.StreamTypeAudio, Codec: "aac",
		Channels: func() *int { c := 2; return &c }(), IsDefault: true,
	})
	newReq := func(q string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264&audioCodecs=aac&containers=mp4"+q, nil)
		return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
			&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	}

	d := h.makeDecision(context.Background(), streams, "matroska", "", newReq("&audioOffsetMs=300"))
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_resync_requested")

	d = h.makeDecision(context.Background(), streams, "matroska", "", newReq(""))
	assert.NotContains(t, d.TranscodeReasons, "audio_resync_requested")

	// Clamp: an absurd crafted offset is bounded, not honored.
	assert.Equal(t, 30_000, audioOffsetMsQuery(newReq("&audioOffsetMs=999999999")))
	assert.Equal(t, -30_000, audioOffsetMsQuery(newReq("&audioOffsetMs=-999999999")))
	// Quantized to the 100ms UI grid so adjacent-millisecond sweeps cannot
	// fork a distinct ffmpeg session per value.
	assert.Equal(t, 300, audioOffsetMsQuery(newReq("&audioOffsetMs=337")))
	assert.Equal(t, 400, audioOffsetMsQuery(newReq("&audioOffsetMs=351")))
	assert.Equal(t, -300, audioOffsetMsQuery(newReq("&audioOffsetMs=-337")))
	assert.Equal(t, 0, audioOffsetMsQuery(newReq("&audioOffsetMs=49")))
}

// Audio-side decision pin (ad=), same class as the vd pin: an admin loudnorm
// toggle or enrichment can flip audio passthrough-eligibility between two
// polls of a live playlist, switching the served audio codec under the
// player. ad=transcode must hold; ad=copy on a no-longer-copy decision flips
// with a warning (re-encode is always serveable).
func TestMakeDecision_AudioDecisionPin(t *testing.T) {
	h := newCopyTestHandler(t)
	streams := copyEligibleStreams()
	streams = append(streams, &domainlibrary.MediaStream{
		StreamType: domainlibrary.StreamTypeAudio, Codec: "aac",
		Channels: func() *int { c := 2; return &c }(), IsDefault: true,
	})
	newReq := func(ad string) *http.Request {
		url := "/api/v1/media/m1/hls/stream_0.m3u8?videoCodecs=h264&audioCodecs=aac&containers=mp4"
		if ad != "" {
			url += "&ad=" + ad
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
			&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	}

	// Precondition: aac stereo against aac caps = audio passthrough.
	d := h.makeDecision(context.Background(), streams, "matroska", "", newReq(""))
	require.Equal(t, "copy", d.AudioDecision)

	// A session whose URLs were stamped ad=transcode holds transcode even
	// though the fresh decision would now say copy.
	d = h.makeDecision(context.Background(), streams, "matroska", "", newReq("transcode"))
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "decision_pinned")
	assert.NotEmpty(t, d.AudioCodec, "a pinned audio re-encode still needs an encoder")

	// ad=copy on a still-copy decision: no-op.
	d = h.makeDecision(context.Background(), streams, "matroska", "", newReq("copy"))
	assert.Equal(t, "copy", d.AudioDecision)

	// Garbage values are ignored.
	d = h.makeDecision(context.Background(), streams, "matroska", "", newReq("banana"))
	assert.Equal(t, "copy", d.AudioDecision)

	// The pin composes with the user resync force: offset wins over a copy pin
	// (shifted audio can only be served re-encoded).
	req := newReq("copy")
	q := req.URL.Query()
	q.Set("audioOffsetMs", "300")
	req.URL.RawQuery = q.Encode()
	d = h.makeDecision(context.Background(), streams, "matroska", "", req)
	assert.Equal(t, "transcode", d.AudioDecision)
}

// sourceSubtitleCodec resolves the burn-in subtitle track by its
// SUBTITLE-relative index (ffmpeg 0:s:N), skipping video/audio streams.
func TestSourceSubtitleCodec_ResolvesByRelativeIndex(t *testing.T) {
	ch := func(n int) *int { return &n }
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Channels: ch(2)},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip"},            // s:0 (text)
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "HDMV_PGS_SUBTITLE"}, // s:1 (bitmap)
	}
	assert.Equal(t, "subrip", sourceSubtitleCodec(streams, 0))
	assert.Equal(t, "hdmv_pgs_subtitle", sourceSubtitleCodec(streams, 1), "must lowercase and skip non-subtitle streams")
	assert.Empty(t, sourceSubtitleCodec(streams, -1), "no burn-in track")
	assert.Empty(t, sourceSubtitleCodec(streams, 9), "out of range")
}

// End-to-end wiring: selecting a BITMAP subtitle for burn-in must (a) force a
// video transcode and (b) thread the subtitle codec into the fingerprint, so
// the ffmpeg builder's burn-in gate (IsImageSubtitle) actually fires. Before
// this wiring the codec was never populated in production, so bitmap subtitles
// were silently dropped while the pipeline paid the transcode cost (audit HIGH).
func TestBuildFingerprint_BitmapBurnIn_ThreadsCodecAndForcesTranscode(t *testing.T) {
	h := newCopyTestHandler(t)
	ch := func(n int) *int { return &n }
	w, hgt, level := 1920, 1080, 40
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Profile: "High", Width: &w, Height: &hgt, Level: &level, Keyframes: []int64{0, 6000, 12000}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Channels: ch(2), IsDefault: true},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "hdmv_pgs_subtitle"}, // s:0, bitmap
	}
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0.m3u8?videoCodecs=h264&audioCodecs=aac&containers=mp4&subtitleTrack=0", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))

	fp, err := h.buildFingerprint(req, "m1", item, streams)
	require.NoError(t, err)
	assert.Equal(t, 0, fp.SubtitleBurnInTrack)
	assert.Equal(t, "hdmv_pgs_subtitle", fp.SubtitleBurnInCodec, "the bitmap codec must be threaded so IsImageSubtitle fires in the builder")

	// The decision layer must have forced a video transcode to burn the bitmap
	// subtitle in (a copy rung cannot overlay).
	d := h.makeDecision(req.Context(), streams, "matroska", "", req)
	assert.Equal(t, "transcode", d.VideoDecision, "a bitmap burn-in must force a video transcode")
	assert.Equal(t, "burn", d.SubtitleDecision)

	// And the threaded codec actually activates the builder's burn-in gate.
	assert.True(t, domaintranscoding.IsImageSubtitle(fp.SubtitleBurnInCodec), "the threaded codec must be recognised as a bitmap subtitle")
}

// The wasm-audio path serves VIDEO-ONLY ffmpeg segments (the AC-3/E-AC-3 audio is
// decoded client-side from a separate rendition). The video fingerprint must
// therefore drop the audio track (AudioTrack = -1) whenever the request carries
// ?wasmAudio=1, and fork the cache off the muxed fingerprint - otherwise a
// variant playlist's segment URI, sealed video-only, would re-derive a muxed
// fingerprint and 410. Guards the ffmpeg video-transcode wasm-audio path.
func TestBuildFingerprint_WasmAudio_ForcesVideoOnly(t *testing.T) {
	h := newCopyTestHandler(t)
	ch := func(n int) *int { return &n }
	w, hgt, level := 1920, 1080, 40
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Profile: "Main", Width: &w, Height: &hgt, Level: &level, Keyframes: []int64{0, 6000, 12000}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "eac3", Channels: ch(6), IsDefault: true},
	}
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur}}
	mk := func(q string) apptranscoding.DecisionFingerprint {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0.m3u8?"+q, nil)
		req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
		fp, err := h.buildFingerprint(req, "m1", item, streams)
		require.NoError(t, err)
		return fp
	}

	base := "videoCodecs=h264&audioCodecs=aac&containers=mp4&audioTrack=0"
	muxed := mk(base)
	wasm := mk(base + "&wasmAudio=1")

	assert.Equal(t, 0, muxed.AudioTrack, "muxed path keeps the selected audio track")
	assert.Equal(t, -1, wasm.AudioTrack, "wasm-audio path drops the audio track (video-only segments)")
	assert.NotEqual(t, muxed.Hash(), wasm.Hash(), "video-only must fork the segment cache off the muxed fingerprint")
}
