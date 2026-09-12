package ffmpeg

import (
	"slices"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func routeNames(ps []routeProbe) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.route)
	}
	return out
}

// The route set follows the filter-level facts: no HEVC encoder ⇒ no hevc /
// passthrough route; no GPU tone-mapper ⇒ no tonemap route but always the
// bridge; NVENC has no upload chain ⇒ no decoupled route.
func TestRouteProbesFor_FollowsTheProbedFacts(t *testing.T) {
	got := routeNames(routeProbesFor(transcoding.NVENC, []string{"h264", "hevc"}, nil, false, true, true))
	want := []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRouteBridge, transcoding.HWRoutePassthrough}
	if !slices.Equal(got, want) {
		t.Fatalf("nvenc routes = %v, want %v", got, want)
	}
	got = routeNames(routeProbesFor(transcoding.VAAPI, []string{"h264"}, []string{transcoding.FeatureTonemapOpenCL}, true, false, false))
	want = []string{transcoding.HWRouteSDR, transcoding.HWRouteTonemap, transcoding.HWRouteBridge, transcoding.HWRouteDecoupled}
	if !slices.Equal(got, want) {
		t.Fatalf("vaapi routes = %v, want %v", got, want)
	}
}

// The probe runs the PRODUCTION builder: its argv carries the very filters a
// session of that route emits (the bridge's hwdownload/bwdif/tonemapx/upload,
// the passthrough's p010 scaler and main10 profile, the decoupled upload).
func TestRouteProbeOpts_RunTheProductionBuilder(t *testing.T) {
	prevTonemapx := tonemapxAvailable("ffmpeg")
	tonemapxOK = true // the bundled jellyfin-ffmpeg carries tonemapx
	t.Cleanup(func() { tonemapxOK = prevTonemapx })
	features := []string{"rc_lookahead", "bframes"}
	byRoute := map[string][]string{}
	for _, p := range routeProbesFor(transcoding.NVENC, []string{"h264", "hevc"}, features, false, true, true) {
		opts := routeProbeOpts(transcoding.NVENC, "", features, "/bench/route-"+p.mire.name+".hevc", "/tmp/out", p)
		if err := opts.Validate(); err != nil {
			t.Fatalf("route %s: opts invalid: %v", p.route, err)
		}
		byRoute[p.route] = BuildFFmpegArgs(opts)
	}
	has := func(route, sub string) {
		t.Helper()
		if !strings.Contains(strings.Join(byRoute[route], " "), sub) {
			t.Errorf("route %s argv lacks %q:\n%s", route, sub, strings.Join(byRoute[route], " "))
		}
	}
	has(transcoding.HWRouteSDR, "-hwaccel cuda -hwaccel_output_format cuda -i /bench/route-sdr8.hevc")
	has(transcoding.HWRouteSDR, "yadif_cuda=deint=interlaced,scale_cuda=384:-2:format=nv12 -c:v h264_nvenc -preset p4 -forced-idr 1 -rc-lookahead 20 -bf 3")
	has(transcoding.HWRouteHEVC, "-c:v hevc_nvenc")
	has(transcoding.HWRouteSource10Bit, "-i /bench/route-sdr10.hevc")
	has(transcoding.HWRouteBridge, "-i /bench/route-hdr10.hevc -map 0:v:0 -vf hwdownload,format=p010le,bwdif=mode=send_frame:deint=interlaced,setparams=")
	has(transcoding.HWRouteBridge, "hwupload_cuda,scale_cuda=384:-2:format=nv12 -c:v h264_nvenc")
	has(transcoding.HWRoutePassthrough, "scale_cuda=384:-2:format=p010 -c:v hevc_nvenc")
	has(transcoding.HWRoutePassthrough, "-profile:v main10 -color_primaries:v bt2020 -color_trc:v smpte2084")
	for route, argv := range byRoute {
		s := strings.Join(argv, " ")
		if strings.Contains(s, "-map 0:a") {
			t.Errorf("route %s maps audio the mire does not carry", route)
		}
		if !strings.Contains(s, "-hls_segment_type fmp4") || !strings.Contains(s, "/tmp/out/stream_0_%03d.m4s") {
			t.Errorf("route %s does not write the production HLS layout", route)
		}
	}
}

func TestProgressFrames(t *testing.T) {
	if got := progressFrames("frame=1\nfps=0\nframe=12\nprogress=continue\nframe=24\nprogress=end\n"); got != 24 {
		t.Fatalf("progressFrames = %d, want 24", got)
	}
	if got := progressFrames("progress=end\n"); got != 0 {
		t.Fatalf("no frame line should read 0, got %d", got)
	}
}
