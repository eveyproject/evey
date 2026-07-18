package interceptors_test

import (
	"context"
	"net"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func TestClientInfoUnary_ExtractsAllFields(t *testing.T) {
	interceptor := interceptors.ClientInfoUnary()

	md := metadata.Pairs(
		"user-agent", "EveyApp/1.0",
		"x-device-name", "Living Room TV",
		"x-forwarded-for", "203.0.113.5, 10.0.0.1",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	resp, err := interceptor(ctx, nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, "EveyApp/1.0", interceptors.UserAgentFromCtx(captured))
	assert.Equal(t, "Living Room TV", interceptors.DeviceNameFromCtx(captured))
	assert.Equal(t, "203.0.113.5", interceptors.ClientIPFromCtx(captured))
}

func TestClientInfoUnary_NoMetadata(t *testing.T) {
	interceptor := interceptors.ClientInfoUnary()

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Empty(t, interceptors.UserAgentFromCtx(captured))
	assert.Empty(t, interceptors.DeviceNameFromCtx(captured))
}

func TestClientInfoUnary_PeerAddress(t *testing.T) {
	interceptor := interceptors.ClientInfoUnary()

	md := metadata.Pairs()
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 54321},
	})

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "192.168.1.10", interceptors.ClientIPFromCtx(captured))
}

func TestClientInfoUnary_XForwardedForTakesPrecedence(t *testing.T) {
	interceptor := interceptors.ClientInfoUnary()

	md := metadata.Pairs("x-forwarded-for", "10.0.0.99")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 54321},
	})

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "10.0.0.99", interceptors.ClientIPFromCtx(captured))
}

func TestClientInfoUnary_SpoofedXFFFromUntrustedPeerIgnored(t *testing.T) {
	interceptor := interceptors.ClientInfoUnary()
	// A client speaking gRPC directly (a PUBLIC peer, not a trusted proxy) sets
	// x-forwarded-for to spoof its source IP. It must be ignored and the real
	// peer address used, so the per-IP rate limiters can't be evaded.
	md := metadata.Pairs("x-forwarded-for", "1.2.3.4")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 40000}, // TEST-NET-3, untrusted
	})

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.7", interceptors.ClientIPFromCtx(captured), "spoofed XFF from an untrusted peer must be ignored")
}

func TestClientInfoStream(t *testing.T) {
	interceptor := interceptors.ClientInfoStream()

	md := metadata.Pairs(
		"user-agent", "EveyTV/2.0",
		"x-device-name", "Bedroom",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	handler := func(srv any, stream grpc.ServerStream) error {
		captured = stream.Context()
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/test/Stream"}
	err := interceptor(nil, &fakeServerStream{ctx: ctx}, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "EveyTV/2.0", interceptors.UserAgentFromCtx(captured))
	assert.Equal(t, "Bedroom", interceptors.DeviceNameFromCtx(captured))
}

type plainAddr string

func (a plainAddr) Network() string { return "plain" }
func (a plainAddr) String() string  { return string(a) }

func TestClientInfoUnary_PeerWithoutPort(t *testing.T) {
	interceptor := interceptors.ClientInfoUnary()

	md := metadata.Pairs()
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: plainAddr("192.168.1.5")})

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "192.168.1.5", interceptors.ClientIPFromCtx(captured))
}

func TestContextAccessors_EmptyContext(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, interceptors.UserAgentFromCtx(ctx))
	assert.Empty(t, interceptors.DeviceNameFromCtx(ctx))
	assert.Empty(t, interceptors.ClientIPFromCtx(ctx))
	assert.False(t, interceptors.PersistentSessionFromCtx(ctx))
}

func TestClientInfoUnary_PersistentSession(t *testing.T) {
	interceptor := interceptors.ClientInfoUnary()
	md := metadata.Pairs("x-persistent-session", "true")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.True(t, interceptors.PersistentSessionFromCtx(captured))
}

func TestClientInfoUnary_PersistentSession_Absent(t *testing.T) {
	interceptor := interceptors.ClientInfoUnary()
	md := metadata.Pairs("user-agent", "EveyWeb/1.0")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.False(t, interceptors.PersistentSessionFromCtx(captured))
}
