package mkvgo

// One-call triage over mkvgo's ops.Diagnose: seek-index health, audio start
// delays, and declared-size coherence (a tolerant walk only when the size
// diverges). It distils the verdict into the domain Diagnosis so the repair UI
// can route to the right remedy - reindex/resync, retime, or re-download for a
// truncated source - instead of trying blind.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/gravity-zero/mkvgo/ebml"
	extmatroska "github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"

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
	out := mapDiagnosis(d)
	if out.Truncated {
		out.MissingTailBytes = missingTailBytes(path)
	}
	return out, nil
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
	if len(d.AudioDelaysNs) > 0 {
		out.AudioDelaysMs = make(map[uint64]int64, len(d.AudioDelaysNs))
		for tr, ns := range d.AudioDelaysNs {
			out.AudioDelaysMs[tr] = ns / 1_000_000
		}
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

// missingTailBytes measures HOW MUCH of the declared Segment is absent from
// the end of a truncated file: declared end (EBML + Segment headers, head-only
// read) minus the real size. Best-effort - 0 on any read hiccup; the finding
// itself already stands.
func missingTailBytes(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0
	}
	r := bufio.NewReaderSize(f, 4096)
	h1, n1, err := ebml.ReadElementHeader(r)
	if err != nil || h1.ID != ebml.IDEBMLHeader || h1.Size < 0 {
		return 0
	}
	if _, skipErr := io.CopyN(io.Discard, r, h1.Size); skipErr != nil {
		return 0
	}
	h2, n2, err := ebml.ReadElementHeader(r)
	if err != nil || h2.ID != extmkv.IDSegment || h2.Size < 0 {
		return 0
	}
	declaredEnd := int64(n1) + h1.Size + int64(n2) + h2.Size
	if missing := declaredEnd - st.Size(); missing > 0 {
		return missing
	}
	return 0
}
