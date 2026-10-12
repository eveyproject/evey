package mkvgo

import (
	"context"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/ops"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// validate.go - streaming-readiness validation via mkvgo's ops.Validate. Beyond
// the structural checks it audits what seeking and on-demand HLS serving rely
// on: a missing Cues index, cue points referencing a non-video track, cue times
// matching no video keyframe (stale index), subtitle blocks without a
// BlockDuration, video tracks without DefaultDuration, AAC without its
// AudioSpecificConfig. Each finding's message names the fix. Matroska/WebM only
// (ISO-BMFF carries its index in the moov by construction).

// ValidateStreaming reports the streaming-readiness issues of an EBML file. An
// empty slice with a nil error means the file is ready. The read walks the
// clusters, so it is not head-only - call it per file, on demand, not across a
// whole library in the hot path.
func (a *Adapter) ValidateStreaming(ctx context.Context, path string) ([]mkv.ValidationIssue, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.ValidateStreaming")
	defer span.End()

	issues, err := ops.Validate(ctx, path)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	out := make([]mkv.ValidationIssue, 0, len(issues))
	for _, iss := range issues {
		sev := mkv.ValidationWarning
		if iss.Severity == extmkv.SeverityError {
			sev = mkv.ValidationError
		}
		out = append(out, mkv.ValidationIssue{Severity: sev, Message: iss.Message})
	}
	return out, nil
}
