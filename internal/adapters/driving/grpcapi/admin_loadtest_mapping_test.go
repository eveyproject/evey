package grpcapi

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"

	apploadtest "github.com/evey-project/evey/internal/app/loadtest"
)

// tierCosts is the wire mapping of the per-audience cost table: every field
// crosses with a distinct value so a swapped pair shows.
func TestTierCosts_MapsEveryField(t *testing.T) {
	in := []apploadtest.TierCost{{
		Viewers: 12, CPUPercent: 34.5, MemUsedBytes: 67, DiskReadBytesPerSec: 89, ServedBytesPerSec: 101,
		CPUPercentPerViewer: 2.5, MemBytesPerViewer: 7, Samples: 3,
	}}
	out := tierCosts(in)
	if len(out) != 1 {
		t.Fatalf("got %d tiers", len(out))
	}
	got := out[0]
	if got.Viewers != 12 || got.CpuPercent != 34.5 || got.MemUsedBytes != 67 || got.DiskReadBytesPerSec != 89 ||
		got.ServedBytesPerSec != 101 || got.CpuPercentPerViewer != 2.5 || got.MemBytesPerViewer != 7 || got.Samples != 3 {
		t.Errorf("tier = %+v", got)
	}
	if got := tierCosts(nil); got == nil || len(got) != 0 {
		t.Errorf("no tiers must be an empty slice, got %v", got)
	}
}

// The load test runs with the CALLER's bearer, read off the gateway-forwarded
// Authorization metadata; absent metadata or header reads as no credential.
func TestBearerFromContext(t *testing.T) {
	if got := bearerFromContext(context.Background()); got != "" {
		t.Errorf("no metadata: %q", got)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-other", "1"))
	if got := bearerFromContext(ctx); got != "" {
		t.Errorf("no authorization header: %q", got)
	}
	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer abc.def"))
	if got := bearerFromContext(ctx); got != "abc.def" {
		t.Errorf("got %q, want the bare token", got)
	}
}
