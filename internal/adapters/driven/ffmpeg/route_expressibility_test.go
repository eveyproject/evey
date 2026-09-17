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
