package transcoding

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// expressibility_expected.go - the cells the shadow's exit criterion may
// require (ADR-0030, 6), TEMPORARY like the shadow.
//
// "Reachable" is decided by the resolver itself, not by a hand-written rule
// or a product of proofs: a small matrix of synthetic sources x clients is
// run through the real Decide + resolveRoute under the host's live
// capability, route costs and policy, and the GPU cells those routes name are
// the expected set. A backend the pick never chooses (the dearer of two for a
// shape), a bridge a GPU tone-mapper makes moot, a deinterlacer no backend
// proved - none of them can show up as missing, because none of them is ever
// resolved. Flip a policy toggle and the set follows.

// ExpectedExpressibilityCells enumerates the {backend, topology, needs} cells
// this resolver resolves for the probe matrix. Nil-safe on an unwired resolver
// (no capability = no GPU cell).
func ExpectedExpressibilityCells(pr *PlanResolver) []transcoding.ExpressibilityCell {
	if pr == nil {
		return nil
	}
	seen := map[transcoding.ExpressibilityCell]bool{}
	var out []transcoding.ExpressibilityCell
	ctx := context.Background()
	for _, src := range probeSources() {
		for _, cl := range probeClients() {
			req := PlaybackRequest{
				MediaID: "expressibility-probe", SourcePath: "/expressibility-probe.mkv", Container: "matroska",
				Streams: src.streams, Client: cl, Prefs: DefaultTranscodingPreferences(),
				AudioTrack: 0, SubtitleBurnInTrack: src.burnIn,
				// A copyable source is forced to a transcode, as the corpus does
				// (vd=transcode): the copy rung reaches no GPU cell.
				PinnedVideoDecision: "transcode",
				SourceBitrateKbps:   src.kbps,
			}
			req.Source = SnapshotOf(req)
			req.Policy = pr.hwPolicy()
			decision := pr.Decide(ctx, req)
			if decision.VideoDecision != "transcode" {
				continue
			}
			route := pr.resolveRoute(req, decision, false)
			if !route.HW() {
				continue
			}
			cell := transcoding.ExpressibilityCell{
				Backend:  route.Backend.Name,
				Topology: route.Topology.Name(),
				Needs:    transcoding.RouteNeedsLabel(decision.SubtitleDecision == "burn", route.Overlay, route.Deinterlace),
			}
			if !seen[cell] {
				seen[cell] = true
				out = append(out, cell)
			}
		}
	}
	return out
}

type probeSource struct {
	streams []*library.MediaStream
	kbps    int64
	burnIn  int // subtitle-relative index to burn, -1 = none
}

// probeSources are the source shapes that reach distinct topologies: the
// plain 8-bit H.264 (zero-copy), HDR10 and HLG HEVC (GPU tone-map / bridge /
// passthrough), a codec no GPU decodes (decoupled), a 10-bit H.264 and a
// 4:2:2 HEVC (decoupled or software), an interlaced H.264 (the session-level
// deinterlacer), and an H.264 with a bitmap subtitle (burn-in / overlay).
func probeSources() []probeSource {
	video := func(codec string, w, h int, mut func(*library.MediaStream)) *library.MediaStream {
		s := &library.MediaStream{StreamType: library.StreamTypeVideo, Codec: codec, Width: &w, Height: &h, Profile: "High", Level: new(41)}
		s.ColorTransfer = new("bt709")
		s.PixelFormat = new("yuv420p")
		s.Keyframes = []int64{0, 3000, 6000}
		if mut != nil {
			mut(s)
		}
		return s
	}
	audio := &library.MediaStream{StreamType: library.StreamTypeAudio, Codec: "aac", Channels: new(2), IsDefault: true}
	hdr := func(transfer string) func(*library.MediaStream) {
		return func(s *library.MediaStream) {
			s.Profile, s.BitDepth = "Main 10", new(10)
			s.ColorTransfer, s.ColorSpace, s.PixelFormat = new(transfer), new("bt2020nc"), new("yuv420p10le")
		}
	}
	pgs := &library.MediaStream{StreamType: library.StreamTypeSubtitle, Codec: "hdmv_pgs_subtitle", IsDefault: true}
	return []probeSource{
		{[]*library.MediaStream{video("h264", 1920, 1080, nil), audio}, 8000, -1},
		{[]*library.MediaStream{video("hevc", 3840, 2160, hdr("smpte2084")), audio}, 40000, -1},
		{[]*library.MediaStream{video("hevc", 3840, 2160, hdr("arib-std-b67")), audio}, 30000, -1},
		{[]*library.MediaStream{video("mpeg4", 720, 576, func(s *library.MediaStream) { s.Profile, s.Level = "Advanced Simple", nil }), audio}, 1500, -1},
		{[]*library.MediaStream{video("h264", 1920, 1080, func(s *library.MediaStream) {
			s.Profile, s.BitDepth, s.PixelFormat = "High 10", new(10), new("yuv420p10le")
		}), audio}, 6000, -1},
		{[]*library.MediaStream{video("hevc", 1920, 1080, func(s *library.MediaStream) {
			s.Profile, s.BitDepth, s.PixelFormat = "Main 4:2:2 10", new(10), new("yuv422p10le")
		}), audio}, 20000, -1},
		{[]*library.MediaStream{video("h264", 1920, 1080, func(s *library.MediaStream) { s.IdetInterlaced = new(true) }), audio}, 12000, -1},
		{[]*library.MediaStream{video("h264", 1920, 1080, nil), audio, pgs}, 8000, 0},
	}
}

// probeClients: an H.264-only SDR client, an HEVC-capable SDR client (the
// HEVC upgrade), an HDR client (the passthrough).
func probeClients() []transcoding.ClientProfile {
	sdr := transcoding.ClientProfile{VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}, Containers: []string{"mp4"}, MaxAudioChannels: 6}
	hevc := sdr
	hevc.VideoCodecs = []string{"h264", "hevc"}
	hdrTV := hevc
	hdrTV.SupportsHDR = true
	hdrTV.AudioCodecs = []string{"aac", "ac3", "eac3"}
	return []transcoding.ClientProfile{sdr, hevc, hdrTV}
}
