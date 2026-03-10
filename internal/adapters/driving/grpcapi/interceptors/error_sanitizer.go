package interceptors

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorSanitizerUnary replaces verbose Internal error messages with a generic
// "internal error" to prevent leaking implementation details to clients.
func ErrorSanitizerUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			err = sanitizeError(err, info.FullMethod)
		}
		return resp, err
	}
}

func ErrorSanitizerStream() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		err := handler(srv, ss)
		if err != nil {
			err = sanitizeError(err, info.FullMethod)
		}
		return err
	}
}

func sanitizeError(err error, method string) error {
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	if st.Code() == codes.Internal {
		slog.Warn("sanitized internal error", slog.String("method", method), slog.String("original", st.Message()))
		return status.Error(codes.Internal, "internal error")
	}
	return err
}
