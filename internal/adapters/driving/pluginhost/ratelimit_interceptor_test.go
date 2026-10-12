package pluginhost

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRateLimitInterceptor_TripsUnderFlood(t *testing.T) {
	h := &HostServer{rpcLimiter: ratelimit.New(60)} // burst 60, refill 1/s
	interc := h.makeRateLimitInterceptor(&pluginEntry{pluginID: "plg-x"})
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }

	allowed, limited := 0, 0
	for range 500 {
		_, err := interc(context.Background(), nil, nil, handler)
		if status.Code(err) == codes.ResourceExhausted {
			limited++
		} else {
			allowed++
		}
	}
	assert.Positive(t, allowed, "legitimate calls must pass")
	assert.Positive(t, limited, "a flood must be rate-limited")
}

func TestRateLimitInterceptor_PerPluginIsolated(t *testing.T) {
	h := &HostServer{rpcLimiter: ratelimit.New(60)}
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }

	// Flood plugin A until it trips.
	intA := h.makeRateLimitInterceptor(&pluginEntry{pluginID: "plg-a"})
	for range 200 {
		_, _ = intA(context.Background(), nil, nil, handler)
	}
	// Plugin B has its own bucket → its first call still passes.
	intB := h.makeRateLimitInterceptor(&pluginEntry{pluginID: "plg-b"})
	_, err := intB(context.Background(), nil, nil, handler)
	assert.NoError(t, err, "one plugin's flood must not starve another's quota")
}
