package mkvgo

import (
	"context"
	"sync/atomic"
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// DropSources forgets the plans of the named sources only: the next request
// for one of them builds again, the others stay cached.
func TestOnDemandPackager_DropSourcesForgetsOnlyThoseSources(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	var builds atomic.Int32
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		builds.Add(1)
		return &ondemandPlan{}, nil
	}
	a, b := stampedeSrc(t), stampedeSrc(t)
	ctx := context.Background()
	for _, src := range []string{a, b} {
		if _, err := p.Plan(ctx, src, apptranscoding.OnDemandOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	p.DropSources([]string{a})
	for _, src := range []string{a, b} {
		if _, err := p.Plan(ctx, src, apptranscoding.OnDemandOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := builds.Load(); got != 3 {
		t.Fatalf("builds = %d, want 3 (a twice, b once)", got)
	}
}
