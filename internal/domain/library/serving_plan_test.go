package library_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

func TestServingStrategyConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  library.ServingStrategy
		want string
	}{
		{"direct-play", library.ServingStrategyDirectPlay, "direct-play"},
		{"remux-hls", library.ServingStrategyRemuxHLS, "remux-hls"},
		{"transcode", library.ServingStrategyTranscode, "transcode"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if string(c.got) != c.want {
				t.Errorf("ServingStrategy %s = %q, want %q", c.name, c.got, c.want)
			}
		})
	}
}

func TestServingPlan_FieldsRoundTrip(t *testing.T) {
	t.Parallel()

	plan := &library.ServingPlan{
		MediaItemID:  "mi-1",
		Target:       "mse-generic",
		Strategy:     library.ServingStrategyRemuxHLS,
		NeedsReindex: true,
		PlanJSON:     []byte(`{"strategy":"remux-hls"}`),
	}
	if plan.MediaItemID != "mi-1" {
		t.Errorf("MediaItemID = %q, want mi-1", plan.MediaItemID)
	}
	if !plan.NeedsReindex {
		t.Errorf("NeedsReindex = false, want true")
	}
	if plan.Strategy != library.ServingStrategyRemuxHLS {
		t.Errorf("Strategy = %q, want remux-hls", plan.Strategy)
	}
}

// Compile-time check that serving_plan.go's exported interfaces stay
// satisfiable by a minimal implementation shape - catches an accidental
// breaking signature change at build time rather than at every adapter's
// call site.
var (
	_ library.IngestPlanner         = (*noopIngestPlanner)(nil)
	_ library.ServingPlanRepository = (*noopServingPlanRepo)(nil)
)

type noopIngestPlanner struct{}

func (noopIngestPlanner) Plan(_ context.Context, _, _ string) (*library.ServingPlan, error) {
	return nil, nil
}

type noopServingPlanRepo struct{}

func (noopServingPlanRepo) Upsert(_ context.Context, _ *library.ServingPlan) error { return nil }
func (noopServingPlanRepo) GetByMediaItemID(_ context.Context, _ shared.MediaItemID) (*library.ServingPlan, error) {
	return nil, nil
}
func (noopServingPlanRepo) DeleteByMediaItemID(_ context.Context, _ shared.MediaItemID) error {
	return nil
}
