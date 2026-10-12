package interceptors

import (
	"context"
	"log/slog"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// MethodLimiter associates a gRPC full method name with a per-IP limiter.
type MethodLimiter struct {
	Limiter        *ratelimit.Limiter
	RetryAfterSecs int
}

// DefaultPublicMethodLimiters returns per-method limiters for endpoints
// prone to brute-force or abuse. Most entries are public (no auth needed
// to hit), but a few require auth - we still rate-limit them because an
// already-authenticated attacker can grind brute-force targets too
// (QuickConnect code guessing, invite redemption, etc.). Each limiter
// is per-IP.
func DefaultPublicMethodLimiters() map[string]*MethodLimiter {
	return map[string]*MethodLimiter{
		"/evey.v1.UserService/Login":                {Limiter: ratelimit.NewWithBurst(5, 5), RetryAfterSecs: 12},
		"/evey.v1.UserService/RefreshToken":         {Limiter: ratelimit.NewWithBurst(30, 30), RetryAfterSecs: 2},
		"/evey.v1.UserService/RequestPasswordReset": {Limiter: ratelimit.NewWithBurst(3, 3), RetryAfterSecs: 20},
		"/evey.v1.UserService/ResetPassword":        {Limiter: ratelimit.NewWithBurst(5, 5), RetryAfterSecs: 12},
		"/evey.v1.UserService/RegisterWithInvite":   {Limiter: ratelimit.NewWithBurst(10, 10), RetryAfterSecs: 6},
		"/evey.v1.UserService/InitiateQuickConnect": {Limiter: ratelimit.NewWithBurst(10, 10), RetryAfterSecs: 6},
		"/evey.v1.UserService/CheckQuickConnect":    {Limiter: ratelimit.NewWithBurst(30, 30), RetryAfterSecs: 2},
		// Authenticated but a brute-force target: the QC short code is a
		// 6-char hex derivation (2²⁴ ≈ 16M). An authenticated attacker
		// binding a victim's pending code to their own user_id turns
		// the victim's next CheckQuickConnect poll into token issuance
		// against the ATTACKER'S account (device hijack). Budget tight
		// enough that grinding 16M codes is infeasible: 5/min per IP.
		"/evey.v1.UserService/AuthorizeQuickConnect": {Limiter: ratelimit.NewWithBurst(5, 5), RetryAfterSecs: 12},
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
			recordRejection(ctx, info.FullMethod, ip)
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
			recordRejection(ctx, info.FullMethod, ip)
			sendRetryAfter(ctx, ml.RetryAfterSecs)
			return status.Error(codes.ResourceExhausted, "too many requests")
		}
		return handler(srv, ss)
	}
}

// recordRejection emits the ops signals for a throttled request: a Prometheus
// counter (labeled by method) and a structured warning. Both are necessary
// because the rate limiter runs before the logging interceptor, so a throttled
// brute-force attempt is otherwise invisible. slog.String escapes the IP value,
// so an attacker-supplied X-Forwarded-For cannot forge log lines here.
func recordRejection(ctx context.Context, method, ip string) {
	metrics.RateLimitRejected.WithLabelValues(method).Inc()
	attrs := []any{
		slog.String("method", method),
		slog.String("ip", ip),
	}
	// The bucket key alone is not diagnosable. Production showed every external
	// client refused under ip=127.0.0.1 on 2026-07-29, and nothing in the line
	// said whether that was "the proxy is not trusted so its XFF was dropped" or
	// "no XFF ever arrived" - two different fixes. The resolution carries both.
	if res, ok := ClientIPResolutionFromCtx(ctx); ok {
		attrs = append(attrs,
			slog.String("peer_ip", res.PeerIP),
			slog.Bool("peer_trusted", res.PeerTrusted),
			slog.String("xff", res.XFF),
			slog.Bool("xff_honored", res.XFFHonored))
	}
	slog.WarnContext(ctx, "ratelimit: request rejected", attrs...)
}

func sendRetryAfter(ctx context.Context, secs int) {
	if secs <= 0 {
		return
	}
	_ = grpc.SetHeader(ctx, metadata.Pairs("retry-after", strconv.Itoa(secs)))
}
