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
	got := routeNames(routeProbesFor(transcoding.NVENC, []string{"h264", "hevc"}, nil, false, true, true, false, nil))
	want := []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRouteBridge, transcoding.HWRoutePassthrough}
	if !slices.Equal(got, want) {
		t.Fatalf("nvenc routes = %v, want %v", got, want)
	}
	got = routeNames(routeProbesFor(transcoding.VAAPI, []string{"h264"}, []string{transcoding.FeatureTonemapOpenCL}, true, false, false, false, nil))
	want = []string{transcoding.HWRouteSDR, transcoding.HWRouteTonemap, transcoding.HWRouteBridge, transcoding.HWRouteDecoupled}
	if !slices.Equal(got, want) {
		t.Fatalf("vaapi routes = %v, want %v", got, want)
	}
	// An AV1 encoder that passed the real-encode probe gets its own builder
	// route (av1_<backend> on the SDR mire); without it, no AV1 route.
	got = routeNames(routeProbesFor(transcoding.QSV, []string{"h264", "hevc", "av1"}, nil, false, false, false, false, nil))
	want = []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteAV1, transcoding.HWRouteBridge, transcoding.HWRouteDecoupled}
	if !slices.Equal(got, want) {
		t.Fatalf("qsv+av1 routes = %v, want %v", got, want)
	}
	// The QSV deinterlace route only with the probed verdict, on the woven TFF
	// H.264 mire, through the builder with the session-level filter; per-frame
	// backends never get one even when asked.
	got = routeNames(routeProbesFor(transcoding.QSV, []string{"h264"}, nil, false, false, false, true, nil))
	want = []string{transcoding.HWRouteSDR, transcoding.HWRouteBridge, transcoding.HWRouteDecoupled, transcoding.HWRouteDeinterlace}
	if !slices.Equal(got, want) {
		t.Fatalf("qsv+deint routes = %v, want %v", got, want)
	}
	if got := routeNames(routeProbesFor(transcoding.NVENC, []string{"h264"}, nil, false, false, false, true, nil)); slices.Contains(got, transcoding.HWRouteDeinterlace) {
		t.Fatalf("nvenc must not grow a session-level deinterlace route: %v", got)
	}
	for _, p := range routeProbesFor(transcoding.QSV, []string{"h264"}, nil, false, false, false, true, nil) {
		if p.route != transcoding.HWRouteDeinterlace {
			continue
		}
		if p.mire.ext() != "h264" || !p.mire.interlaced {
			t.Fatalf("deint route mire must be the interlaced H.264 clip, got %+v", p.mire)
		}
		argv := strings.Join(BuildFFmpegArgs(routeProbeOpts(transcoding.QSV, "/dev/dri/renderD128", nil, "/bench/route-tff.h264", "/tmp/out", p)), " ")
		if !strings.Contains(argv, "-hwaccel qsv -hwaccel_output_format qsv -qsv_device /dev/dri/renderD128 -i /bench/route-tff.h264 -map 0:v:0 -vf deinterlace_qsv=mode=advanced,scale_qsv=") {
			t.Fatalf("deint route argv does not run the production QSV deinterlacer:\n%s", argv)
		}
	}
	// RKMPP: every shape goes through the same proofs - SDR/HEVC zero-copy,
	// the OpenCL tone-map only on its probed feature, the bridge, the
	// decoupled upload; never a session-level deinterlacer (RGA has none).
	got = routeNames(routeProbesFor(transcoding.RKMPP, []string{"h264", "hevc"}, []string{transcoding.FeatureTonemapOpenCL}, true, true, false, true, nil))
	want = []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRouteTonemap, transcoding.HWRouteBridge, transcoding.HWRouteDecoupled}
	if !slices.Equal(got, want) {
		t.Fatalf("rkmpp routes = %v, want %v", got, want)
	}
	if got := routeNames(routeProbesFor(transcoding.RKMPP, []string{"h264"}, nil, true, false, false, false, nil)); slices.Contains(got, transcoding.HWRouteTonemap) {
		t.Fatalf("rkmpp without the OpenCL feature has no GPU tone-map route: %v", got)
	}
	for _, p := range routeProbesFor(transcoding.NVENC, []string{"h264", "av1"}, nil, false, false, false, false, nil) {
		if p.route != transcoding.HWRouteAV1 {
			continue
		}
		argv := strings.Join(BuildFFmpegArgs(routeProbeOpts(transcoding.NVENC, "", nil, "/bench/route-sdr8.hevc", "/tmp/out", p)), " ")
		if !strings.Contains(argv, "scale_cuda=384:-2:format=nv12 -c:v av1_nvenc -preset p4 -forced-idr 1") {
			t.Fatalf("AV1 route argv does not run the production av1_nvenc shape:\n%s", argv)
		}
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
	for _, p := range routeProbesFor(transcoding.NVENC, []string{"h264", "hevc"}, features, false, true, true, false, nil) {
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

// An exotic decode class the GPU measured gets its own end-to-end route probe;
// the canonical 4:2:0 classes are covered by sdr/src10 and get none.
func TestRouteProbesFor_ExoticDecodeClassesGetARoute(t *testing.T) {
	got := routeNames(routeProbesFor(transcoding.NVENC, []string{"h264"}, nil, false, false, false, false,
		[]string{"h264", "hevc", transcoding.HWDecodeHEVC10, "hevc:yuv444p", "hevc:yuv420p12le", "hevc:nope"}))
	want := []string{transcoding.HWRouteSDR, transcoding.HWRouteBridge, "src:yuv444p", "src:yuv420p12le"}
	if !slices.Equal(got, want) {
		t.Fatalf("routes = %v, want %v", got, want)
	}
}

// resolveTonemapCaps treats the native VPP and VAAPI/OpenCL probes as
// INDEPENDENT: OpenCL alone certifies HWTonemap and adds its feature (the fix
// for the gating bug that dropped VAAPI's OpenCL tone-map whenever the
// fixed-function VPP aborted, as it did on prod-host). Native alone still counts.
func TestResolveTonemapCaps_NativeAndOpenCLIndependent(t *testing.T) {
	cases := []struct {
		name           string
		native, opencl bool
		wantTonemap    bool
		wantOpenCL     bool
	}{
		{"opencl only", false, true, true, true}, // prod-host: VPP aborts, OpenCL validated 3.26x
		{"native only", true, false, true, false},
		{"both", true, true, true, true},
		{"neither", false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tonemap, features := resolveTonemapCaps("vaapi", nil, tc.native, tc.opencl)
			if tonemap != tc.wantTonemap {
				t.Fatalf("hwTonemap = %v, want %v", tonemap, tc.wantTonemap)
			}
			if got := slices.Contains(features, transcoding.FeatureTonemapOpenCL); got != tc.wantOpenCL {
				t.Fatalf("opencl feature = %v, want %v", got, tc.wantOpenCL)
			}
		})
	}
}

// tonemapRouteArgv builds the production argv for the tone-map route probe of a
// backend calibrated with the given (features, hwTonemap), or fails if the
// route is not even proposed.
func tonemapRouteArgv(t *testing.T, accel transcoding.HWAccelType, features []string, tonemap bool) string {
	t.Helper()
	for _, p := range routeProbesFor(accel, []string{"h264"}, features, tonemap, false, false, false, nil) {
		if p.route != transcoding.HWRouteTonemap {
			continue
		}
		opts := routeProbeOpts(accel, "/dev/dri/renderD128", features, "/bench/route-hdr10.hevc", "/tmp/out", p)
		if err := opts.Validate(); err != nil {
			t.Fatalf("tone-map route opts invalid: %v", err)
		}
		return strings.Join(BuildFFmpegArgs(opts), " ")
	}
	t.Fatalf("no tone-map route proposed for %s (features=%v tonemap=%v)", accel, features, tonemap)
	return ""
}

// With OpenCL certified while the native VPP failed, the tone-map route is
// still proposed AND the production builder emits the OpenCL chain for it (so
// runRouteProbe certifies the REAL pipeline). With only the native VPP, the
// same route emits the fixed-function tonemap_vaapi. This is what the resolver
// (TonemapBackend) then reads - the fix's end-to-end shape, pinned as a golden.
func TestTonemapRoute_OpenCLIndependentOfNativeVPP(t *testing.T) {
	// OpenCL-only: exactly what probeBackendCap builds when native=false, opencl=true.
	tonemap, features := resolveTonemapCaps("vaapi", nil, false, true)
	routes := routeNames(routeProbesFor(transcoding.VAAPI, []string{"h264"}, features, tonemap, false, false, false, nil))
	if !slices.Contains(routes, transcoding.HWRouteTonemap) {
		t.Fatalf("OpenCL-only must still propose the tone-map route, got %v", routes)
	}
	if argv := tonemapRouteArgv(t, transcoding.VAAPI, features, tonemap); !strings.Contains(argv, "tonemap_opencl=format=nv12") {
		t.Fatalf("OpenCL tone-map route must emit the OpenCL chain:\n%s", argv)
	}
	// Native-only inverse: HWTonemap still true, route still proposed, VPP filter.
	tonemap, features = resolveTonemapCaps("vaapi", nil, true, false)
	if argv := tonemapRouteArgv(t, transcoding.VAAPI, features, tonemap); !strings.Contains(argv, "tonemap_vaapi=format=nv12") {
		t.Fatalf("native-only route must emit the fixed-function VPP filter:\n%s", argv)
	}
}
