package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func framedRequest(w, h int, sar string) PlaybackRequest {
	req := ladderRequest(h)
	req.Streams = []*library.MediaStream{{StreamType: library.StreamTypeVideo, Codec: "h264", Width: &w, Height: &h}}
	if sar != "" {
		req.Streams[0].SampleAspectRatio = &sar
	}
	return req
}

// The backlog symptom through the resolver: a 640x480 source capped by the
// client to 360 gets one rung, framed 480x360 - inside the cap, at the
// source's aspect - and the cache key, the encoder's profiles and the master
// all read that one frame.
func TestShapeLadder_NarrowSourceUnderHeightCapIsFramedInsideIt(t *testing.T) {
	req := framedRequest(640, 480, "")
	req.Client.MaxHeight = 360
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}

	plan := (&PlanResolver{}).planFor(context.Background(), req, decision)
	require.NotEmpty(t, plan.Ladder)
	for _, p := range plan.Ladder {
		w, h := p.Frame()
		assert.True(t, p.Framed(), "rung %s carries its frame", p.Name)
		assert.LessOrEqual(t, p.Height, 360, "box of %s", p.Name)
		assert.LessOrEqual(t, h, 360, "frame of %s", p.Name)
		assert.Equal(t, w*3, h*4, "4:3 kept in %s (%dx%d)", p.Name, w, h)
	}

	// The fingerprint carries the frame to the encoder (profilesToDomain).
	enc := profilesToDomain(plan.Fingerprint.Profiles)
	require.Len(t, enc, len(plan.Ladder))
	for i := range enc {
		assert.Equal(t, plan.Ladder[i].FrameWidth, enc[i].FrameWidth)
		assert.Equal(t, plan.Ladder[i].FrameHeight, enc[i].FrameHeight)
	}

	master, err := NewManifestGenerator().Master(baseInput(plan.Ladder, 10))
	require.NoError(t, err)
	assert.Contains(t, master, "RESOLUTION=480x360")
	assert.NotContains(t, master, "RESOLUTION=640x360")
}

// The symmetric case: a 16:9 source's frame IS its box, so its cache hash is
// the one it had before frames existed; a scope source keeps the 1280x534 the
// width pin always drew, and now says so in the master.
func TestShapeLadder_FramesWideSourcesAsTheyWereDrawn(t *testing.T) {
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}

	wide := (&PlanResolver{}).planFor(context.Background(), framedRequest(1920, 1080, ""), decision)
	require.NotEmpty(t, wide.Ladder)
	for _, p := range wide.Ladder {
		w, h := p.Frame()
		assert.Equal(t, [2]int{p.Width, p.Height}, [2]int{w, h}, "a 16:9 frame is its box (%s)", p.Name)
	}
	unframed := wide.Fingerprint
	unframed.Profiles = make([]ProfileFingerprint, len(wide.Fingerprint.Profiles))
	for i, p := range wide.Fingerprint.Profiles {
		p.FrameWidth, p.FrameHeight = 0, 0
		unframed.Profiles[i] = p
	}
	assert.Equal(t, unframed.Hash(), wide.Fingerprint.Hash(), "a 16:9 source keeps its cache")

	scope := (&PlanResolver{}).planFor(context.Background(), framedRequest(1920, 800, ""), decision)
	require.NotEmpty(t, scope.Ladder)
	top := scope.Ladder[len(scope.Ladder)-1]
	w, h := top.Frame()
	assert.Equal(t, 1280, w, "premise: the software ceiling tops this ladder at the 720p box")
	assert.Equal(t, 534, h, "the height scale=1280:-2 drew from 1920x800")
	master, err := NewManifestGenerator().Master(baseInput(scope.Ladder, 10))
	require.NoError(t, err)
	assert.Contains(t, master, "RESOLUTION=1280x534")
}

// An anamorphic source is framed on its DISPLAY aspect: a 16:9 PAL DVD
// (720x576, SAR 64:45) draws 640x360 in the 360p box, a 4:3 one (16:15)
// 480x360.
func TestShapeLadder_AnamorphicSourceIsFramedOnItsDisplayAspect(t *testing.T) {
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	for _, tt := range []struct {
		sar          string
		wantW, wantH int
	}{{"64:45", 640, 360}, {"16:15", 480, 360}} {
		plan := (&PlanResolver{}).planFor(context.Background(), framedRequest(720, 576, tt.sar), decision)
		require.NotEmpty(t, plan.Ladder)
		w, h := plan.Ladder[0].Frame()
		assert.Equal(t, [2]int{tt.wantW, tt.wantH}, [2]int{w, h}, "SAR %s", tt.sar)
	}
}

// A frame other than the box, or any frame of an anamorphic source, names
// other bytes than the width pin did: it forks the cache.
func TestDecisionFingerprint_FrameForksOnlyChangedPictures(t *testing.T) {
	mk := func(fw, fh int, sar string) DecisionFingerprint {
		return DecisionFingerprint{
			MediaID: "m1", SourcePath: "/x.mkv", VideoCodec: "h264", AudioCodec: "aac", SourceSAR: sar,
			Profiles: []ProfileFingerprint{{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 128, FrameWidth: fw, FrameHeight: fh}},
		}
	}
	legacy := mk(0, 0, "").Hash()
	assert.Equal(t, legacy, mk(640, 360, "").Hash(), "a frame equal to its box is the legacy picture")
	assert.NotEqual(t, legacy, mk(480, 360, "").Hash(), "a height-pinned frame is other bytes")

	anamorphic := mk(0, 0, "64:45").Hash()
	assert.NotEqual(t, anamorphic, mk(640, 360, "64:45").Hash(), "the GPU scalers ignored the SAR: other bytes")
}

func TestSnapshotFacts_CarriesTheSampleAspect(t *testing.T) {
	f := SnapshotOf(framedRequest(720, 576, "128:90")).Facts()
	assert.Equal(t, [2]int{64, 45}, [2]int{f.SARNum, f.SARDen}, "reduced")
	sq := SnapshotOf(framedRequest(1920, 1080, "1:1")).Facts()
	assert.Zero(t, sq.SARNum, "square pixels stay unset")
}

func TestMPD_DeclaresTheFramedPicture(t *testing.T) {
	in := dashInput(23.976, 60)
	in.Profiles = []transcoding.HLSProfile{
		{Name: "720p", Width: 1280, Height: 720, FrameWidth: 1280, FrameHeight: 534, VideoBitrate: 2800, AudioBitrate: 192},
	}
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Contains(t, mpd, `width="1280" height="534"`)
	assert.NotContains(t, mpd, `height="720"`)
}
