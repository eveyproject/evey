package interceptors_test

import (
	"context"
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorSanitizer_StripInternalDetails(t *testing.T) {
	interceptor := interceptors.ErrorSanitizerUnary()

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
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
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
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

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.Test/Method"}
	resp, err := interceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}
