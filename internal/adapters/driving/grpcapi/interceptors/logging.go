package interceptors

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// identityAttrs appends user_id / profile_id / is_admin to log attrs if claims
// are in ctx. Returns the attrs slice so LoggingUnary/Stream can emit
// consistent audit context alongside request_id.
//
// profile_id is here because user_id alone does not identify WHO on a shared
// household account: every profile of one account logs under the same user_id,
// which makes a log search unable to separate them.
func identityAttrs(ctx context.Context, attrs []any) []any {
	if c := ClaimsFromContext(ctx); c != nil {
		if c.UserID != "" {
			attrs = append(attrs, slog.String("user_id", c.UserID))
		}
		if c.ProfileID != "" {
			attrs = append(attrs, slog.String("profile_id", c.ProfileID))
		}
		if c.IsAdmin {
			attrs = append(attrs, slog.Bool("admin", true))
		}
	}
	return attrs
}

func LoggingUnary(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// RequestIDUnary (first in the chain) normally stamped it already; keep
		// that id - with no gateway header, requestIDFor MINTS a random one and
		// a second mint would split this request across two ids in the logs.
		// Kept as a fallback so hand-built chains without the first interceptor
		// (tests, embedded uses) still get request-scoped logs.
		if eveylogger.RequestIDFromCtx(ctx) == "" {
			ctx = eveylogger.WithRequestID(ctx, requestIDFor(ctx))
		}
		start := time.Now()
		resp, err := handler(ctx, req)
		st, _ := status.FromError(err)
		attrs := []any{
			slog.String("method", info.FullMethod),
			slog.Duration("duration", time.Since(start)),
			slog.String("status", st.Code().String()),
		}
		// ClaimsFromContext is populated by AuthInterceptor (chained earlier);
		// ctx may not have claims on unauthenticated endpoints - that's fine.
		attrs = identityAttrs(ctx, attrs)
		if err != nil {
			attrs = append(attrs, slog.String("error", st.Message()))
		}
		log.InfoContext(ctx, "grpc unary", attrs...)
		return resp, err
	}
}

func LoggingStream(log *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Same keep-if-stamped rule as LoggingUnary (and reuse the gateway id
		// via requestIDFor instead of always minting - this path predated it).
		ctx := ss.Context()
		if eveylogger.RequestIDFromCtx(ctx) == "" {
			ctx = eveylogger.WithRequestID(ctx, requestIDFor(ctx))
		}
		wrapped := &wrappedStream{ServerStream: ss, ctx: ctx}
		start := time.Now()
		err := handler(srv, wrapped)
		st, _ := status.FromError(err)
		attrs := []any{
			slog.String("method", info.FullMethod),
			slog.Duration("duration", time.Since(start)),
			slog.String("status", st.Code().String()),
		}
		attrs = identityAttrs(ctx, attrs)
		log.InfoContext(ctx, "grpc stream", attrs...)
		return err
	}
}

// requestIDFor reuses the id the HTTP leg already assigned (the gateway
// forwards it as x-request-id metadata) so both halves of one user action share
// an id, and mints a fresh one only for calls that arrive straight over gRPC.
// Inbound values are bounded and filtered to printable ASCII: this ends up in
// log lines, and an unbounded header is a log-injection vector.
func requestIDFor(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return newRequestID()
	}
	vals := md.Get("x-request-id")
	if len(vals) == 0 {
		return newRequestID()
	}
	id := vals[0]
	if id == "" || len(id) > 128 {
		return newRequestID()
	}
	for _, r := range id {
		if r < 0x20 || r > 0x7e {
			return newRequestID()
		}
	}
	return id
}

func newRequestID() string {
	id, err := uuid.NewRandom()
	if err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", id[:8])
}
