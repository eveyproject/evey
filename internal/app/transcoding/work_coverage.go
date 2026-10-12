package transcoding

import (
	"context"

	"github.com/eveyproject/evey/internal/pkg/bgtasks"
)

// The system-health view's library-wide progress per worker. Each store that
// can count its own work list (the Postgres one) is asked to; test doubles
// that cannot are simply left out of the view.

func registerKeyframesCoverage(store any) {
	if c, ok := store.(interface {
		KeyframesCoverage(context.Context, int16) (bgtasks.Coverage, error)
	}); ok {
		bgtasks.SetCoverage(keyframeBackfillTask, func(ctx context.Context) (bgtasks.Coverage, error) {
			return c.KeyframesCoverage(ctx, KeyframeProbeAlgoVersion)
		})
	}
}

func registerLevelCoverage(store any) {
	if c, ok := store.(interface {
		LevelCoverage(context.Context) (bgtasks.Coverage, error)
	}); ok {
		bgtasks.SetCoverage(levelBackfillTask, c.LevelCoverage)
	}
}

func registerComplexityCoverage(repo any) {
	if c, ok := repo.(interface {
		ComplexityCoverage(context.Context) (bgtasks.Coverage, error)
	}); ok {
		bgtasks.SetCoverage(complexityTask, c.ComplexityCoverage)
	}
}
