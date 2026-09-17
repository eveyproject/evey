package ffmpeg

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// Every shape the resolver can emit (RuntimeRouteShapes, derived from
// EncodeRoute.CostKey) must have a calibration strategy, or sit in
// unmeasuredShapes with its reason. Admission refuses a named route it has no
// evidence for, so a shape measured nowhere is a GPU stream spilled to
// software - the HEVC decoupled/bridge/tone-map variants were exactly that
// until the plan was derived from the same source as the resolver.
func TestRouteEvidencePlan_CoversEveryRuntimeShape(t *testing.T) {
	// A capability proving every route on a VAAPI that encodes every codec.
	cap := &transcoding.EncodeCapability{
		Backends: []transcoding.HWBackendCap{{
			Backend:  "vaapi",
			Device:   "/dev/dri/renderD128",
			Codecs:   []string{"h264", "hevc", "av1"},
			Features: []string{transcoding.FeatureTonemapOpenCL},
			Routes: []string{
				transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteAV1,
				transcoding.HWRouteTonemap, transcoding.HWRouteBridge, transcoding.HWRoutePassthrough, transcoding.HWRouteDecoupled,
			},
			HWTonemap: true,
		}},
	}
	planned := map[transcoding.RouteShape]bool{}
	for _, er := range routeEvidencePlan(cap) {
		planned[transcoding.RouteShape{Route: er.route, Codec: codecOrH264(er), HW: er.hw()}] = true
	}
	runtime := map[transcoding.RouteShape]bool{}
	for _, sh := range transcoding.RuntimeRouteShapes() {
		runtime[sh] = true
		_, excused := unmeasuredShapes[sh]
		switch {
		case planned[sh] && excused:
			t.Errorf("%+v is both measured and listed as unmeasured - drop it from unmeasuredShapes", sh)
		case !planned[sh] && !excused:
			t.Errorf("runtime shape %+v has no calibration strategy: admission will refuse it for lack of evidence (add it to routeShapeOpts / backendServesShape, or to unmeasuredShapes with a reason)", sh)
		}
	}
	for sh, why := range unmeasuredShapes {
		if !runtime[sh] {
			t.Errorf("unmeasuredShapes lists %+v (%q) which the runtime never emits - stale entry", sh, why)
		}
		if why == "" {
			t.Errorf("unmeasuredShapes %+v has no reason", sh)
		}
	}
	for sh := range planned {
		if !runtime[sh] {
			t.Errorf("the plan measures %+v, a shape the runtime never emits - wasted boot time", sh)
		}
	}
}

// A backend measures only what it proved: no HEVC variants on an H.264-only
// GPU, no tone-map route without a GPU tone-mapper, no decoupled route on an
// accelerator with no upload chain (NVENC).
func TestRouteEvidencePlan_FollowsTheBackendProofs(t *testing.T) {
	cap := &transcoding.EncodeCapability{
		Backends: []transcoding.HWBackendCap{
			{Backend: "nvenc", Codecs: []string{"h264"}, Routes: []string{transcoding.HWRouteSDR, transcoding.HWRouteDecoupled, transcoding.HWRouteBridge}},
			{Backend: "qsv", Codecs: []string{"h264", "hevc"}, Routes: []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteDecoupled, transcoding.HWRouteTonemap}},
		},
	}
	got := map[string]bool{}
	for _, er := range routeEvidencePlan(cap) {
		if er.hw() {
			got[er.backend+"/"+codecOrH264(er)+"/"+string(er.route)] = true
		}
	}
	want := []string{"nvenc/h264/sdr", "nvenc/h264/bridge", "qsv/h264/sdr", "qsv/hevc/sdr", "qsv/h264/decoupled", "qsv/hevc/decoupled"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	for _, no := range []string{"nvenc/hevc/sdr", "nvenc/h264/decoupled", "nvenc/h264/tonemap", "qsv/h264/tonemap", "qsv/hevc/bridge", "qsv/av1/sdr"} {
		if got[no] {
			t.Errorf("%s must not be planned: the backend did not prove it", no)
		}
	}
}

// The software chain is measured on every host, HEVC included on both the
// SDR and the tone-map route (a client preferring HEVC with no GPU lands on
// x265, tone-mapped or not); software AV1 is the documented exception.
func TestRouteEvidencePlan_SoftwareRoutes(t *testing.T) {
	got := map[string]bool{}
	for _, er := range routeEvidencePlan(&transcoding.EncodeCapability{}) {
		if !er.hw() {
			got[codecOrH264(er)+"/"+string(er.route)] = true
		}
	}
	for _, w := range []string{"h264/sdr", "h264/sw-hdr", "hevc/sdr", "hevc/sw-hdr"} {
		if !got[w] {
			t.Errorf("software route %s not planned: %v", w, got)
		}
	}
	if got["av1/sdr"] || got["av1/sw-hdr"] {
		t.Error("software AV1 is deliberately unmeasured (see unmeasuredShapes)")
	}
}
