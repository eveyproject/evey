package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

func stampGuardStreams() []*domainlibrary.MediaStream {
	w, hgt, lvl, ch := 1920, 1080, 40, 2
	return []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Profile: "High", Width: &w, Height: &hgt, Level: &lvl, Keyframes: []int64{0}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Channels: &ch, IsDefault: true},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Channels: &ch},
	}
}

func serveStamped(h *hlsVODServe, streams []*domainlibrary.MediaStream, stamp string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/od_seg00001.m4s?src="+stamp, nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	rec := httptest.NewRecorder()
	h.serveOnDemandResource(rec, req, "m1", "od_seg00001.m4s", onDemandItem(), streams, noopSpan())
	return rec.Code
}

// The track a stamp names is a client-controlled integer, and every distinct
// one is its own packager cache key: trusted as is, each forged value cost a
// full plan build (16 build slots, a 256-plan LRU shared with every viewer)
// before the SourceID check answered 410. A track the source does not have is
// refused before anything is built.
func TestOnDemandResource_ForgedStampedTrackBuildsNothing(t *testing.T) {
	streams := stampGuardStreams()
	for _, tc := range []struct {
		name  string
		trim  bool
		stamp string
	}{
		{"track past the source's audio", true, "s1.2"},
		{"huge track", true, "s1.123456"},
		{"past the source's audio, trim off", false, "s1.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{sourceID: "s1", resources: map[string][]byte{"seg00001.m4s": []byte("x")}}}
			h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: tc.trim, libSvc: &testFakeStreamsGetter{streams: streams}}
			if code := serveStamped(h, streams, tc.stamp); code != http.StatusGone {
				t.Fatalf("status = %d, want 410", code)
			}
			if pkg.calls != 0 {
				t.Fatalf("%d plan(s) built for a forged stamp", pkg.calls)
			}
		})
	}
}

// The symmetric case: a track the source has, with the trim on, is served.
func TestOnDemandResource_StampedTrackInRangeIsServed(t *testing.T) {
	streams := stampGuardStreams()
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{sourceID: "s1", resources: map[string][]byte{"seg00001.m4s": []byte("x")}}}
	h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: true, libSvc: &testFakeStreamsGetter{streams: streams}}
	if code := serveStamped(h, streams, "s1.1"); code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if pkg.lastOpts.KeepAudioTrack == nil || *pkg.lastOpts.KeepAudioTrack != 1 {
		t.Fatalf("KeepAudioTrack = %v, want 1", pkg.lastOpts.KeepAudioTrack)
	}
}

// The delivery trims by itself when only one audio track can be carried
// verbatim (E-AC-3 beside TrueHD), with the operator's trim flag off: its
// master stamps that track, and the sub-resource must serve it. Prod
// 2026-10-18: every viewer of such a film got 410 on od_playlist.m3u8.
func TestOnDemandResource_StampedTrackServedWithTrimOff(t *testing.T) {
	streams := stampGuardStreams()
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{sourceID: "s1", resources: map[string][]byte{"seg00001.m4s": []byte("x")}}}
	h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: false, libSvc: &testFakeStreamsGetter{streams: streams}}
	if code := serveStamped(h, streams, "s1.0"); code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if pkg.lastOpts.KeepAudioTrack == nil || *pkg.lastOpts.KeepAudioTrack != 0 {
		t.Fatalf("KeepAudioTrack = %v, want 0", pkg.lastOpts.KeepAudioTrack)
	}
}
