package interceptors_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorSanitizer_StripInternalDetails(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.Internal, "pq: connection refused at 10.0.0.5:5432")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Library/ListMedia"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message())
}

func TestErrorSanitizer_PassThroughOtherCodes(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	tests := []struct {
		name string
		code codes.Code
		msg  string
	}{
		{"NotFound", codes.NotFound, "media item not found"},
		{"InvalidArgument", codes.InvalidArgument, "title is required"},
		{"PermissionDenied", codes.PermissionDenied, "admin only"},
		{"Unauthenticated", codes.Unauthenticated, "missing token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := func(ctx context.Context, req any) (any, error) {
				return nil, status.Error(tt.code, tt.msg)
			}

			info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Test/Method"}
			_, err := interceptor(context.Background(), nil, info, handler)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, tt.code, st.Code())
			assert.Equal(t, tt.msg, st.Message())
		})
	}
}

func TestErrorSanitizer_NilError(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Test/Method"}
	resp, err := interceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestErrorSanitizer_ContextCanceled(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, context.Canceled
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Test/Method"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Canceled, st.Code())
	assert.Equal(t, "request canceled", st.Message())
}

func TestErrorSanitizer_InternalContextCanceled(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.Internal, "context canceled while processing")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Test/Method"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Canceled, st.Code())
}

func TestErrorSanitizer_GRPCCanceled_PassThrough(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.Canceled, "client cancelled")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Test/Method"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Canceled, st.Code())
	assert.Equal(t, "client cancelled", st.Message())
}

func TestErrorSanitizerStream_StripInternalDetails(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerStream()

	handler := func(srv any, stream grpc.ServerStream) error {
		return status.Error(codes.Internal, "pq: connection refused")
	}

	info := &grpc.StreamServerInfo{FullMethod: "/evey.v1.Test/Stream"}
	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message())
}

func TestErrorSanitizerStream_NilError(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerStream()

	handler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/evey.v1.Test/Stream"}
	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, handler)

	require.NoError(t, err)
}

func TestErrorSanitizer_DeadlineExceeded(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, context.DeadlineExceeded
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Test/Method"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.DeadlineExceeded, st.Code())
	assert.Equal(t, "deadline exceeded", st.Message())
}

// A non-status error escaping a handler (e.g. a repo error wrapped with the
// handler's name) must not reach the client verbatim - gRPC would surface it
// as codes.Unknown leaking "activityHandler.GetUserStats: ...".
func TestErrorSanitizer_WrappedPlainError_Genericised(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, fmt.Errorf("activityHandler.GetUserStats: %w", errors.New("pq: relation \"user_stats\" does not exist"))
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Activity/GetUserStats"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message())
	assert.NotContains(t, st.Message(), "activityHandler")
}

// An explicit codes.Unknown status is likewise genericised.
func TestErrorSanitizer_UnknownCode_Genericised(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.Unknown, "internal/foo.go:42 boom")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Test/Method"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message())
}

func TestErrorSanitizerStream_PassThrough(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerStream()

	handler := func(srv any, stream grpc.ServerStream) error {
		return status.Error(codes.NotFound, "not found")
	}

	info := &grpc.StreamServerInfo{FullMethod: "/evey.v1.Test/Stream"}
	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, handler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Equal(t, "not found", st.Message())
}
