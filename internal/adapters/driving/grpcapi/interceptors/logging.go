package interceptors

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// identityAttrs appends user_id / is_admin to log attrs if claims are in ctx.
// Returns the attrs slice so LoggingUnary/Stream can emit consistent audit
// context alongside request_id.
func identityAttrs(ctx context.Context, attrs []any) []any {
	if c := ClaimsFromContext(ctx); c != nil {
		if c.UserID != "" {
			attrs = append(attrs, slog.String("user_id", c.UserID))
		}
		if c.IsAdmin {
			attrs = append(attrs, slog.Bool("admin", true))
		}
	}
	return attrs
}

func LoggingUnary(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = eveylogger.WithRequestID(ctx, newRequestID())
		start := time.Now()
		resp, err := handler(ctx, req)
		st, _ := status.FromError(err)
		attrs := []any{
			slog.String("method", info.FullMethod),
			slog.Duration("duration", time.Since(start)),
			slog.String("status", st.Code().String()),
		}
		// ClaimsFromContext is populated by AuthInterceptor (chained earlier);
		// ctx may not have claims on unauthenticated endpoints — that's fine.
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
		ctx := eveylogger.WithRequestID(ss.Context(), newRequestID())
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

func newRequestID() string {
	id, err := uuid.NewRandom()
	if err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", id[:8])
}
