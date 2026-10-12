package interceptors_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/pkg/demo"
)

// The demo's visitors are administrators: the sessions page, the presence
// list and the shared account's login history would show them each other's
// addresses. The interceptor blanks the named fields wherever they sit and
// leaves every other field alone.
func TestRedactInterceptor_BlanksNamedFieldsAtEveryDepth(t *testing.T) {
	ri := interceptors.NewRedactInterceptor(demo.RedactedFields...)

	resp, err := ri.Unary()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/evey.v1.AdminSessionService/ListActiveSessions"},
		func(ctx context.Context, req any) (any, error) {
			return &eveyv1.ListActiveSessionsResponse{Sessions: []*eveyv1.ActiveSession{
				{SessionId: "s1", DeviceName: "Living room TV", ClientIp: "203.0.113.7"},
				{SessionId: "s2", ClientIp: "198.51.100.9"},
			}}, nil
		})
	require.NoError(t, err)
	out := resp.(*eveyv1.ListActiveSessionsResponse)
	require.Len(t, out.Sessions, 2)
	require.Empty(t, out.Sessions[0].ClientIp, "repeated message: client_ip blanked")
	require.Equal(t, "Living room TV", out.Sessions[0].DeviceName, "unrelated fields untouched")
	require.Equal(t, "s1", out.Sessions[0].SessionId)
	require.Empty(t, out.Sessions[1].ClientIp)

	// A different message carrying the same field names, nested one level.
	resp, err = ri.Unary()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/evey.v1.ActivityService/GetLoginHistory"},
		func(ctx context.Context, req any) (any, error) {
			return &eveyv1.GetLoginHistoryResponse{Entries: []*eveyv1.LoginEntry{{IpAddress: "203.0.113.7", UserAgent: "curl", Success: true}}}, nil
		})
	require.NoError(t, err)
	hist := resp.(*eveyv1.GetLoginHistoryResponse)
	require.Empty(t, hist.Entries[0].IpAddress)
	require.Empty(t, hist.Entries[0].UserAgent)
	require.True(t, hist.Entries[0].Success)
}

// A handler error passes through untouched, and a nil response does not panic.
func TestRedactInterceptor_PassesErrorsAndNil(t *testing.T) {
	ri := interceptors.NewRedactInterceptor("client_ip")
	resp, err := ri.Unary()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x"},
		func(ctx context.Context, req any) (any, error) { return nil, context.Canceled })
	require.Nil(t, resp)
	require.ErrorIs(t, err, context.Canceled)
}
