package interceptors

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorSanitizerUnary replaces verbose Internal error messages with a generic
// "internal error" to prevent leaking implementation details to clients.
func ErrorSanitizerUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			err = sanitizeError(err, info.FullMethod)
		}
		return resp, err
	}
}

func ErrorSanitizerStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		err := handler(srv, ss)
		if err != nil {
			err = sanitizeError(err, info.FullMethod)
		}
		return err
	}
}

func sanitizeError(err error, method string) error {
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	}
	st, ok := status.FromError(err)
	if !ok {
		// A non-status error escaped a handler - typically a repo/service error
		// wrapped with a "fooHandler.Bar: %w" prefix. gRPC surfaces these to the
		// client verbatim as codes.Unknown, leaking handler names and internal
		// messages, so treat them exactly like an Internal error.
		slog.Warn("sanitized non-status error", slog.String("method", method), slog.String("original", err.Error()))
		return status.Error(codes.Internal, "internal error")
	}
	if st.Code() == codes.Canceled {
		return err
	}
	// Internal and Unknown are never deliberately-coded, user-facing errors:
	// genericise their messages. Every other code (NotFound, InvalidArgument,
	// PermissionDenied, …) carries an intentional - often i18n-translated -
	// message and is passed through untouched.
	if st.Code() == codes.Internal || st.Code() == codes.Unknown {
		if strings.Contains(st.Message(), "context canceled") {
			return status.Error(codes.Canceled, "request canceled")
		}
		slog.Warn("sanitized internal error", slog.String("method", method), slog.String("original", st.Message()))
		return status.Error(codes.Internal, "internal error")
	}
	return err
}
