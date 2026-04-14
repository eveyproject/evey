package interceptors_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testMethod = "/evey.v1.UserService/Login"

func ipCtx(ip string) context.Context {
	md := metadata.Pairs("x-forwarded-for", ip)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	// run through ClientInfoUnary so ClientIPFromCtx works
	ci := interceptors.ClientInfoUnary()
	var out context.Context
	h := func(c context.Context, _ any) (any, error) { out = c; return nil, nil }
	_, _ = ci(ctx, nil, &grpc.UnaryServerInfo{FullMethod: testMethod}, h)
	return out
}

func TestRateLimitUnary_AllowsUnderLimit(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(3, 3), RetryAfterSecs: 10},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	ctx := ipCtx("1.2.3.4")
	for i := 0; i < 3; i++ {
		resp, err := interceptor(ctx, nil, info, handler)
		require.NoError(t, err, "call %d", i)
		assert.Equal(t, "ok", resp)
	}
}

func TestRateLimitUnary_BlocksAfterBurst(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(2, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	ctx := ipCtx("5.5.5.5")
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	_, err = interceptor(ctx, nil, info, handler)
	require.NoError(t, err)

	_, err = interceptor(ctx, nil, info, handler)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestRateLimitUnary_PerIPIsolation(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	_, err := interceptor(ipCtx("1.1.1.1"), nil, info, handler)
	require.NoError(t, err)
	_, err = interceptor(ipCtx("1.1.1.1"), nil, info, handler)
	require.Error(t, err)
	// different IP still allowed
	_, err = interceptor(ipCtx("2.2.2.2"), nil, info, handler)
	require.NoError(t, err)
}

func TestRateLimitUnary_TokenRefill(t *testing.T) {
	// 60 req/min = 1 token/s
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 60), RetryAfterSecs: 1},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: testMethod}

	ctx := ipCtx("7.7.7.7")
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	_, err = interceptor(ctx, nil, info, handler)
	require.Error(t, err)

	time.Sleep(1100 * time.Millisecond)
	_, err = interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
}

func TestRateLimitUnary_UnregisteredMethodPasses(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitUnary(limiters)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.UserService/Me"}

	ctx := ipCtx("9.9.9.9")
	for i := 0; i < 5; i++ {
		_, err := interceptor(ctx, nil, info, handler)
		require.NoError(t, err)
	}
}

func TestRateLimitStream_Blocks(t *testing.T) {
	limiters := map[string]*interceptors.MethodLimiter{
		testMethod: {Limiter: ratelimit.NewWithBurst(1, 0.01), RetryAfterSecs: 60},
	}
	interceptor := interceptors.RateLimitStream(limiters)
	handler := func(srv any, ss grpc.ServerStream) error { return nil }
	info := &grpc.StreamServerInfo{FullMethod: testMethod}

	ctx := ipCtx("8.8.8.8")
	ss := &fakeServerStream{ctx: ctx}
	require.NoError(t, interceptor(nil, ss, info, handler))
	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestDefaultPublicMethodLimiters_CoversAudit(t *testing.T) {
	m := interceptors.DefaultPublicMethodLimiters()
	required := []string{
		"/evey.v1.UserService/Login",
		"/evey.v1.UserService/RefreshToken",
		"/evey.v1.UserService/RequestPasswordReset",
		"/evey.v1.UserService/ResetPassword",
		"/evey.v1.UserService/RegisterWithInvite",
		"/evey.v1.UserService/InitiateQuickConnect",
		"/evey.v1.UserService/CheckQuickConnect",
	}
	for _, k := range required {
		_, ok := m[k]
		assert.True(t, ok, "missing limiter for %s", k)
	}
}
