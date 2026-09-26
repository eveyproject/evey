package transcoding

// picture_resolver.go - the player's picture decision for a producer that is
// not a player session: the sync-preview clip an operator judges, the reduced
// download a phone keeps. Both used to keep their own copy of the tone-map
// string and none of the other corrections (SAR, interlace verdict, telecine),
// and neither ever saw the GPU. Here they get the SAME route derivation the
// player gets - the backend pick, the tone-map placement, the per-title block
// (resolveRoute) - for a fixed target: one small SDR H.264 picture, the one
// shape every browser and phone decodes. No ladder, no admission pin, no
// fingerprint: a clip takes no session slot, so admission never spills it.

import (
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// pictureCodec is the one codec a browser-side picture is drawn in.
const pictureCodec = "h264"

// PictureFor is the software picture decision for these streams: the source
// corrections the player applies, no accelerator. What a host without a
// transcoder (a resolver not wired) still draws correctly.
func PictureFor(streams []*library.MediaStream) transcoding.PicturePlan {
	snap := SnapshotOf(PlaybackRequest{Streams: streams})
	return picturePlanOf(snap, ResolvedRoute{Codec: pictureCodec, Topology: softwareTopology(snap.IsHDR)})
}

func softwareTopology(tonemap bool) transcoding.Topology {
	if tonemap {
		return transcoding.TopologySoftwareHDR
	}
	return transcoding.TopologySoftware
}

// ResolvePicture is PictureFor with the player's route: the accelerator that
// serves this source on this host (or none), decided by the same pick a
// playback session gets. maxHeight is the tallest picture the consumer draws
// (the rung the backend must reach); 0 = the source's.
func (pr *PlanResolver) ResolvePicture(mediaID string, streams []*library.MediaStream, maxHeight int) transcoding.PicturePlan {
	req := PlaybackRequest{MediaID: mediaID, Streams: streams}
	req.Source = SnapshotOf(req)
	req.Policy = pr.hwPolicy()
	decision := transcoding.PlaybackDecision{
		PlayMethod:        "transcode",
		VideoDecision:     "transcode",
		AudioDecision:     "transcode",
		SubtitleDecision:  "none",
		VideoCodec:        pictureCodec,
		ClientVideoCodecs: []string{pictureCodec},
		MaxHeight:         maxHeight,
		TranscodeReasons:  []string{"picture"},
	}
	// The tone-map reason is what routes an HDR source to a GPU tone-mapper or
	// the CPU bridge (naturalRoute reads it); the player's Decide adds it for
	// every HDR source a non-HDR client plays, and this consumer never is one.
	if req.Source.IsHDR {
		decision.TranscodeReasons = append(decision.TranscodeReasons, "hdr_tonemapping_needed")
	}
	route := pr.resolveRoute(req, decision, false)
	return picturePlanOf(req.Source, route)
}

// picturePlanOf projects a resolved route and the source snapshot onto the
// picture plan, field for field as fingerprintFor projects them onto the
// player's fingerprint.
func picturePlanOf(snap SourceSnapshot, route ResolvedRoute) transcoding.PicturePlan {
	hw := route.HW()
	p := transcoding.PicturePlan{
		Codec:          route.Codec,
		Topology:       route.Topology,
		ToneMap:        route.Topology.ToneMap != transcoding.ToneMapNone,
		HDRTransfer:    strings.ToLower(snap.HDRRange),
		DeinterlaceAll: snap.DeinterlaceAll,
		Telecine:       snap.Telecine,
		HWDeinterlace:  route.Deinterlace && hw,
		SourceSAR:      snap.SAR,
	}
	if p.Codec == "" {
		p.Codec = pictureCodec
	}
	if hw {
		p.Accel = transcoding.HWAccelType(route.Backend.Name)
		p.Device = route.Backend.Device
		p.Features = route.Backend.Features
	}
	return p
}
