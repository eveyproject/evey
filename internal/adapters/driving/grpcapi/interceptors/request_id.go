package interceptors

import (
	"context"

	"google.golang.org/grpc"

	eveylogger "github.com/eveyproject/evey/internal/pkg/logger"
)

// RequestIDUnary stamps the request id as the FIRST interceptor in the chain.
// LoggingUnary used to do it, but it runs after recovery, the error sanitizer
// and the rate limiter - so every log those three emitted carried no
// request_id, and migrating their log calls to the *Context variants was a
// no-op (the id landed in ctx six interceptors too late).
func RequestIDUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(eveylogger.WithRequestID(ctx, requestIDFor(ctx)), req)
	}
}

// RequestIDStream is the streaming twin of RequestIDUnary.
func RequestIDStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := eveylogger.WithRequestID(ss.Context(), requestIDFor(ss.Context()))
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}
