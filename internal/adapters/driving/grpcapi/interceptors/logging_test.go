package interceptors_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	eveylogger "github.com/eveyproject/evey/internal/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLoggingUnary_SetsRequestID(t *testing.T) {
	logger := slog.Default()
	interceptor := interceptors.LoggingUnary(logger)

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	resp, err := interceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.NotEmpty(t, eveylogger.RequestIDFromCtx(captured))
}

func TestLoggingUnary_PropagatesError(t *testing.T) {
	logger := slog.Default()
	interceptor := interceptors.LoggingUnary(logger)

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.NotFound, "not found")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLoggingStream_SetsRequestID(t *testing.T) {
	logger := slog.Default()
	interceptor := interceptors.LoggingStream(logger)

	var captured context.Context
	handler := func(srv any, stream grpc.ServerStream) error {
		captured = stream.Context()
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/test/Stream"}
	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, handler)

	require.NoError(t, err)
	assert.NotEmpty(t, eveylogger.RequestIDFromCtx(captured))
}

func TestLoggingStream_PropagatesError(t *testing.T) {
	logger := slog.Default()
	interceptor := interceptors.LoggingStream(logger)

	handler := func(srv any, stream grpc.ServerStream) error {
		return status.Error(codes.Internal, "boom")
	}

	info := &grpc.StreamServerInfo{FullMethod: "/test/Stream"}
	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, handler)

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
