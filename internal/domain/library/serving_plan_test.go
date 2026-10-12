package library_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
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
	plan := library.ServingPlan{Target: "mse-generic", Strategy: library.ServingStrategyRemuxHLS, NeedsReindex: true}
	if plan.Target != "mse-generic" || plan.Strategy != library.ServingStrategyRemuxHLS || !plan.NeedsReindex {
		t.Errorf("fields not kept: %+v", plan)
	}
}
