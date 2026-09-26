package library

import (
	"context"

	"github.com/evey-project/evey/internal/pkg/bgtasks"
)

// The system-health view's library-wide progress per worker; see the
// transcoding package's twin. The trickplay count is the boot sweep's own
// "missing" list (version 0): an outdated manifest the sweep never redoes
// must not hold the view below 100 %.

func registerTrickplayCoverage(repo any) {
	if c, ok := repo.(interface {
		TrickplayCoverage(context.Context, int) (bgtasks.Coverage, error)
	}); ok {
		bgtasks.SetCoverage(trickplayTask, func(ctx context.Context) (bgtasks.Coverage, error) {
			return c.TrickplayCoverage(ctx, 0)
		})
	}
}

func registerIntroCoverage(repo any) {
	if c, ok := repo.(interface {
		IntroCoverage(context.Context, int, int) (bgtasks.Coverage, error)
	}); ok {
		bgtasks.SetCoverage(introDetectionTask, func(ctx context.Context) (bgtasks.Coverage, error) {
			return c.IntroCoverage(ctx, introDetectionAlgoVersion, maxIntroDetectionAttempts)
		})
	}
}
