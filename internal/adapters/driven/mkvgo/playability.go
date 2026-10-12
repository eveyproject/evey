package mkvgo

import (
	"context"
	"fmt"

	"github.com/gravity-zero/mkvgo/mkv/ops"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// Playability implements apptranscoding.PlayabilityAnalyzer: mkvgo v0.17.0's
// ops.Playability, a head-only per-track playability verdict against target.
// It is the mkvgo side of the SHADOW MODE comparison in
// internal/app/transcoding/playability_shadow.go - never a replacement for
// the real decision engine (decision.go), which is unaware of this method.
//
// Only the containers ops.Playability itself supports head-only - Matroska/
// WebM (EBML) via IsEBMLContainer, and MP4/MOV (ISO-BMFF) via IsMP4Container -
// are evaluated; any other extension (AVI, MPEG-TS, ...) returns a nil report
// and a nil error rather than an error, since a container mkvgo simply cannot
// look at yet is not a failure for a caller that only wants to observe.
func (a *Adapter) Playability(ctx context.Context, path string, target apptranscoding.PlayabilityTarget) (*apptranscoding.PlayabilityReport, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.Playability")
	defer span.End()

	if !IsEBMLContainer(path) && !IsMP4Container(path) {
		return nil, nil
	}

	report, err := ops.Playability(ctx, path, toOpsTarget(target))
	if err != nil {
		return nil, fmt.Errorf("mkvgo playability: %w", err)
	}
	return fromOpsPlayabilityReport(report), nil
}

// toOpsTarget converts the app-owned PlayabilityTarget into mkvgo's ops.Target.
// MaxLevelHEVC and DolbyVision have no source (zero = no level cap / DoVi
// unsupported, mkvgo's strictest built-in default).
func toOpsTarget(t apptranscoding.PlayabilityTarget) ops.Target {
	name := t.Name
	if name == "" {
		name = "custom"
	}
	return ops.Target{
		Name:         name,
		Container:    t.Containers,
		VideoCodecs:  t.VideoCodecs,
		AudioCodecs:  t.AudioCodecs,
		MaxWidth:     t.MaxWidth,
		MaxHeight:    t.MaxHeight,
		HDR:          t.HDR,
		MaxLevelH264: t.MaxLevelH264,
		HEVCMain10:   t.HEVCMain10,
		VP9Profile2:  t.VP9Profile2,
	}
}

func fromOpsPlayabilityReport(r *ops.PlayabilityReport) *apptranscoding.PlayabilityReport {
	out := &apptranscoding.PlayabilityReport{
		Target:         r.Target,
		OverallVerdict: r.OverallVerdict,
		RemuxContainer: r.RemuxContainer,
	}
	for _, t := range r.Tracks {
		out.Tracks = append(out.Tracks, apptranscoding.PlayabilityTrackVerdict{
			TrackID: t.TrackID,
			Type:    t.Type,
			Verdict: t.Verdict,
			Reasons: t.Reasons,
		})
	}
	return out
}
