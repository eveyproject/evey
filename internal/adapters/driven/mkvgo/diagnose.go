package mkvgo

// One-call triage over mkvgo's ops.Diagnose: seek-index health, audio start
// delays, and declared-size coherence (a tolerant walk only when the size
// diverges). It distils the verdict into the domain Diagnosis so the repair UI
// can route to the right remedy - reindex/resync, retime, or re-download for a
// truncated source - instead of trying blind.

import (
	"context"
	"fmt"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"

	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// Diagnose implements the mkv.Diagnoser port. Matroska/WebM only: a
// non-EBML container (MP4/MOV) has no seek-index defect class (its sample
// table is its index), so it reports Healthy.
func (a *Adapter) Diagnose(ctx context.Context, path string) (*mkv.Diagnosis, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.Diagnose")
	defer span.End()

	if !IsEBMLContainer(path) {
		return &mkv.Diagnosis{Healthy: true}, nil
	}
	d, err := extmatroska.Diagnose(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("diagnose: %w", err)
	}
	out := &mkv.Diagnosis{Healthy: d.Healthy}
	for _, f := range d.Findings {
		out.Findings = append(out.Findings, mkv.DiagnosisFinding{
			Kind:    f.Kind,
			Detail:  f.Detail,
			Remedy:  f.Remedy,
			Track:   f.Track,
			DelayMs: f.DelayNs / 1_000_000,
		})
	}
	if len(d.AudioDelaysNs) > 0 {
		out.AudioDelaysMs = make(map[uint64]int64, len(d.AudioDelaysNs))
		for tr, ns := range d.AudioDelaysNs {
			out.AudioDelaysMs[tr] = ns / 1_000_000
		}
	}
	if d.Damage != nil {
		out.Truncated = d.Damage.TruncatedTail
		// Since mkvgo v0.25 the salvage walk counts surplus bytes past the
		// declared Segment end (trailing junk) in BytesSkipped too, yet that
		// media is complete - only a mid-Segment "damaged" finding means a
		// resync repair has something to skip. Relaying the raw byte count would
		// mislabel a whole file with trailing junk as corrupt. Its trailing-junk
		// finding is copied above and surfaced via Diagnosis.TrailingJunk().
		if out.Damaged() {
			out.DamagedBytes = d.Damage.BytesSkipped
		}
	}
	return out, nil
}
