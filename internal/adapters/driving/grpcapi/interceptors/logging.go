package interceptors

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

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
			slog.String("request_id", eveylogger.RequestIDFromCtx(ctx)),
		}
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
		log.InfoContext(ctx, "grpc stream",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", time.Since(start)),
			slog.String("status", st.Code().String()),
			slog.String("request_id", eveylogger.RequestIDFromCtx(ctx)),
		)
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
