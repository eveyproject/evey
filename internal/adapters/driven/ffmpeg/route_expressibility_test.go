package ffmpeg

import (
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The port states the builder's own gates (hwUsable) once: what it refuses
// here is exactly what BuildFFmpegArgs would have swapped for a software
// encode.
func TestExpressibility_MatchesTheBuilderGates(t *testing.T) {
	e := Expressibility{}
	vaapi := transcoding.VAAPI
	dev := "/dev/dri/renderD128"
	opencl := []string{transcoding.FeatureTonemapOpenCL}
	cases := []struct {
		name  string
		t     transcoding.Topology
		accel transcoding.HWAccelType
		dev   string
		feat  []string
		needs transcoding.RouteNeeds
		ok    bool
	}{
		{"software is always expressible", transcoding.TopologySoftwareHDR, "", "", nil, transcoding.RouteNeeds{BurnInBitmap: true}, true},
		{"zero-copy on vaapi", transcoding.TopologyZeroCopy, vaapi, dev, nil, transcoding.RouteNeeds{}, true},
		{"no accelerator named", transcoding.TopologyZeroCopy, "", dev, nil, transcoding.RouteNeeds{}, false},
		{"burn-in without a GPU compositor", transcoding.TopologyZeroCopy, vaapi, dev, nil, transcoding.RouteNeeds{BurnInBitmap: true}, false},
		{"burn-in with the compositor wired", transcoding.TopologyZeroCopy, vaapi, dev, nil, transcoding.RouteNeeds{BurnInBitmap: true, GPUOverlay: true}, true},
		{"decoupled on vaapi with a device", transcoding.TopologyDecoupled, vaapi, dev, nil, transcoding.RouteNeeds{}, true},
		{"decoupled on vaapi without a device", transcoding.TopologyDecoupled, vaapi, "", nil, transcoding.RouteNeeds{}, false},
		{"decoupled on nvenc: no upload chain", transcoding.TopologyDecoupled, transcoding.NVENC, "", nil, transcoding.RouteNeeds{}, false},
		{"GPU tone-map with the OpenCL chain", transcoding.TopologyGPUToneMap, vaapi, dev, opencl, transcoding.RouteNeeds{}, true},
		{"GPU tone-map on nvenc without tonemap_cuda", transcoding.TopologyGPUToneMap, transcoding.NVENC, "", nil, transcoding.RouteNeeds{}, false},
		{"bridge: the CPU tone-maps, always expressible on a GPU", transcoding.TopologyBridge, transcoding.NVENC, "", nil, transcoding.RouteNeeds{}, true},
		{"passthrough", transcoding.TopologyHDRPassthrough, transcoding.NVENC, "", nil, transcoding.RouteNeeds{}, true},
	}
	for _, tc := range cases {
		err := e.Expressible(tc.t, tc.accel, tc.dev, tc.feat, tc.needs)
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected refusal: %v", tc.name, err)
		}
		if !tc.ok {
			var nx transcoding.ErrRouteNotExpressible
			if !errors.As(err, &nx) || nx.Why == "" {
				t.Errorf("%s: want ErrRouteNotExpressible with a reason, got %v", tc.name, err)
			}
		}
	}
}

// The expected cells are what THIS host can reach, never a product: a backend
// without a compositor has no burn-in cell, only a QSV that proved its
// deinterlacer has a deint cell, a topology the backend did not prove is
// absent, and the codec plays no part (the builder's gates never read it).
func TestExpectedCells_OnlyReachableCells(t *testing.T) {
	e := Expressibility{}
	cap := transcoding.EncodeCapability{Backends: []transcoding.HWBackendCap{
		{Backend: "vaapi", Device: "/dev/dri/renderD128", Codecs: []string{"h264", "hevc"}, Features: []string{transcoding.FeatureTonemapOpenCL}, HWTonemap: true,
			Routes: []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteTonemap, transcoding.HWRouteBridge, transcoding.HWRouteDecoupled}},
		{Backend: "qsv", Device: "/dev/dri/renderD128", Codecs: []string{"h264", "hevc"}, HWOverlay: true, HWDeinterlace: true,
			Routes: []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteDeinterlace, transcoding.HWRoutePassthrough}},
		{Backend: "nvenc", Codecs: []string{"h264"}, Routes: []string{transcoding.HWRouteSDR, transcoding.HWRouteBridge, transcoding.HWRouteDecoupled}},
	}}
	got := map[string]bool{}
	for _, c := range e.ExpectedCells(cap) {
		got[c.Backend+"/"+c.Topology+"/"+c.Needs] = true
	}
	want := []string{
		"vaapi/zero-copy/plain", "vaapi/gpu-tonemap/plain", "vaapi/bridge/plain", "vaapi/decoupled/plain",
		"qsv/zero-copy/plain", "qsv/passthrough/plain", "qsv/zero-copy/burn-in+overlay", "qsv/zero-copy/deint",
		"nvenc/zero-copy/plain", "nvenc/bridge/plain",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("expected cell %s missing from %v", w, got)
		}
	}
	for _, no := range []string{
		"vaapi/passthrough/plain",         // not proven by the row
		"vaapi/zero-copy/burn-in+overlay", // no compositor probed
		"vaapi/zero-copy/deint",           // VAAPI deinterlaces per frame: never a session-level cell
		"nvenc/decoupled/plain",           // proven, but NVENC has no upload chain: not reachable
		"nvenc/zero-copy/burn-in",         // a CPU burn-in is never a GPU cell
	} {
		if got[no] {
			t.Errorf("%s must not be expected", no)
		}
	}
	if len(got) != len(want) {
		t.Errorf("expected exactly %d cells, got %d: %v", len(want), len(got), got)
	}
}
