package interceptors_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// headerStream records what the interceptor sets on the response headers.
type headerStream struct{ header metadata.MD }

func (s *headerStream) Method() string { return testMethod }
func (s *headerStream) SetHeader(md metadata.MD) error {
	s.header = metadata.Join(s.header, md)
	return nil
}
func (s *headerStream) SendHeader(md metadata.MD) error { return s.SetHeader(md) }
func (s *headerStream) SetTrailer(metadata.MD) error     { return nil }

// A refused call tells the client how long to wait: the gateway turns the
// header into HTTP Retry-After, and a login form that never got it retried at
// once into the same closed bucket.
func TestRateLimitUnary_RefusalCarriesRetryAfter(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.01), RetryAfterSecs: 12},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	call := func() (*headerStream, error) {
		hs := &headerStream{}
		ctx := grpc.NewContextWithServerTransportStream(ipCtx("7.7.7.7"), hs)
		_, err := interceptor(ctx, nil, info, handler)
		return hs, err
	}

	hs, err := call()
	require.NoError(t, err)
	assert.Empty(t, hs.header.Get("retry-after"), "an allowed call carries no Retry-After")

	hs, err = call()
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Equal(t, []string{"12"}, hs.header.Get("retry-after"))
}
