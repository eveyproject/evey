package interceptors

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// MethodLimiter associates a gRPC full method name with a per-IP limiter.
type MethodLimiter struct {
	Limiter         *ratelimit.Limiter
	RetryAfterSecs  int
}

// DefaultPublicMethodLimiters returns per-method limiters for public (unauthenticated)
// endpoints that are prone to brute-force or abuse. Each limiter is per-IP.
func DefaultPublicMethodLimiters() map[string]*MethodLimiter {
	return map[string]*MethodLimiter{
		"/evey.v1.UserService/Login":                 {Limiter: ratelimit.NewWithBurst(5, 5), RetryAfterSecs: 12},
		"/evey.v1.UserService/RefreshToken":          {Limiter: ratelimit.NewWithBurst(30, 30), RetryAfterSecs: 2},
		"/evey.v1.UserService/RequestPasswordReset":  {Limiter: ratelimit.NewWithBurst(3, 3), RetryAfterSecs: 20},
		"/evey.v1.UserService/ResetPassword":         {Limiter: ratelimit.NewWithBurst(5, 5), RetryAfterSecs: 12},
		"/evey.v1.UserService/RegisterWithInvite":    {Limiter: ratelimit.NewWithBurst(10, 10), RetryAfterSecs: 6},
		"/evey.v1.UserService/InitiateQuickConnect":  {Limiter: ratelimit.NewWithBurst(10, 10), RetryAfterSecs: 6},
		"/evey.v1.UserService/CheckQuickConnect":     {Limiter: ratelimit.NewWithBurst(30, 30), RetryAfterSecs: 2},
	}
}

// RateLimitUnary returns a unary interceptor enforcing per-method per-IP rate limits.
// Must be chained AFTER ClientInfoUnary so the client IP is available in the context.
func RateLimitUnary(limiters map[string]*MethodLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ml, ok := limiters[info.FullMethod]
		if !ok {
			return handler(ctx, req)
		}
		ip := ClientIPFromCtx(ctx)
		if ip == "" {
			ip = "unknown"
		}
		if !ml.Limiter.Allow(ip) {
			sendRetryAfter(ctx, ml.RetryAfterSecs)
			return nil, status.Error(codes.ResourceExhausted, "too many requests")
		}
		return handler(ctx, req)
	}
}

// RateLimitStream returns a stream interceptor enforcing per-method per-IP rate limits.
func RateLimitStream(limiters map[string]*MethodLimiter) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ml, ok := limiters[info.FullMethod]
		if !ok {
			return handler(srv, ss)
		}
		ctx := ss.Context()
		ip := ClientIPFromCtx(ctx)
		if ip == "" {
			ip = "unknown"
		}
		if !ml.Limiter.Allow(ip) {
			sendRetryAfter(ctx, ml.RetryAfterSecs)
			return status.Error(codes.ResourceExhausted, "too many requests")
		}
		return handler(srv, ss)
	}
}

func sendRetryAfter(ctx context.Context, secs int) {
	if secs <= 0 {
		return
	}
	_ = grpc.SetHeader(ctx, metadata.Pairs("retry-after", strconv.Itoa(secs)))
}
