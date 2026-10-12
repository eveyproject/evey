package mkvgo

// One-call triage over mkvgo's ops.Diagnose: seek-index health, audio start
// delays, where each track's content really ends, and declared-size coherence
// (a tolerant walk only when the size diverges). It distils the verdict into
// the domain Diagnosis so the repair UI can route to the right remedy -
// reindex/resync, retime, or re-acquire for a truncated or audio-short source
// - instead of trying blind.

import (
	"context"
	"fmt"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	return mapDiagnosis(d), nil
}

// mapDiagnosis distils mkvgo's triage into the domain Diagnosis. Pure - unit
// tests construct the input shapes the wild produces.
func mapDiagnosis(d *extmatroska.Diagnosis) *mkv.Diagnosis {
	out := &mkv.Diagnosis{Healthy: d.Healthy}
	for _, f := range d.Findings {
		out.Findings = append(out.Findings, mkv.DiagnosisFinding{
			Kind:    f.Kind,
			Detail:  f.Detail,
			Remedy:  f.Remedy,
			Track:   f.Track,
			DelayMs: f.DelayNs / 1_000_000,
		})
		// The CLASSIFIED finding, not the raw salvage-walk TruncatedTail bit:
		// mkvgo only calls a file truncated when its declared end exceeds the
		// real size AND the walk confirmed the cut. The raw bit is also true
		// when the walk merely ran off the end of TRAILING JUNK - relaying it
		// mislabeled a complete, playable file (mid-file damage + 9 junk
		// bytes) as an "incomplete download / re-acquire the source" (proven
		// on prod bytes, 2026-07-26).
		if f.Kind == "truncated" {
			out.Truncated = true
		}
	}
	// The MAGNITUDE of a cut routes the operator's decision (a 9 KB stub vs a
	// missing final scene); mkvgo measures it on both engines. Only meaningful
	// next to the classified finding above.
	if out.Truncated {
		out.MissingTailBytes = d.MissingTailBytes
	}
	if len(d.AudioDelaysNs) > 0 {
		out.AudioDelaysMs = make(map[uint64]int64, len(d.AudioDelaysNs))
		for tr, ns := range d.AudioDelaysNs {
			out.AudioDelaysMs[tr] = ns / 1_000_000
		}
	}
	// Where the audio really stops, against where the picture really stops -
	// TrackEnds reads statistics tags only when they describe THIS file and
	// walks the tail header-only otherwise. A shortfall is a lower bound when
	// that track's end is only walk-bound, which is still the right side to
	// err on for a "re-acquire" verdict.
	if ends := d.TrackEnds; ends != nil && ends.AudioShortfallMs > 0 {
		out.AudioShortfallMs = ends.AudioShortfallMs
	}
	if d.Damage != nil {
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
	return out
}
