package mkvgo

import (
	"context"
	"fmt"

	"github.com/gravity-zero/mkvgo/mkv/ops"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// defaultIngestTarget matches ops.IngestOptions' own default ("mse-generic"
// when Target is empty) - passed explicitly so a future default change
// upstream in mkvgo can't silently change what Evey judges without a code
// review surfacing it here.
const defaultIngestTarget = "mse-generic"

// Plan implements library.IngestPlanner over mkvgo's ops.Ingest: composes
// Playability + RecommendLadderFor (+ Analyze, always included for the
// playability/ladder detail it carries) into one head-only serving decision.
// It never writes to disk (Reindex is left false) - a caller wanting the
// plan+repair behaviour drives the mkv.Reindexer port separately and calls
// Plan again afterwards, mirroring app/maintenance/seek_index.go's own
// repair-then-refresh pattern. mkvgo's ops.ServingPlan never crosses this
// function's return: every field is translated into the domain type.
func (a *Adapter) Plan(ctx context.Context, path, target string) (*library.ServingPlan, error) {
	ctx, span := tracing.StartSpan(ctx, "mkvgo.Ingest")
	defer span.End()

	if target == "" {
		target = defaultIngestTarget
	}

	report, err := ops.Ingest(ctx, path, ops.IngestOptions{
		Target:          target,
		IncludeAnalysis: true,
	})
	if err != nil {
		return nil, fmt.Errorf("mkvgo ingest: %w", err)
	}

	return &library.ServingPlan{
		Target:       report.Target,
		Strategy:     library.ServingStrategy(report.Strategy),
		NeedsReindex: report.NeedsReindex,
	}, nil
}

var _ library.IngestPlanner = (*Adapter)(nil)
