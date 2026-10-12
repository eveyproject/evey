package interceptors_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
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

// defaultTrustedProxies mirrors ratelimit's package-default trusted set, so
// tests that narrow it via ratelimit.SetTrustedProxies can restore it without
// leaking state into sibling tests in this package.
var defaultTrustedProxies = []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}

func resolveViaInterceptor(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	interceptor := interceptors.ClientInfoUnary()
	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	return captured
}

// This is the exact production failure mode (2026-07-29): distinct clients
// behind the SAME trusted peer must resolve to distinct IPs, or they'd share
// one rate-limit bucket. If resolveClientIPReasoned dropped the x-real-ip
// reasoning (e.g. always returned the peer address), ipA and ipB below would
// be identical and this test goes red.
func TestClientInfoUnary_DistinctXRealIPsGetSeparateBuckets(t *testing.T) {
	trustedPeer := &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 443}}
	resolve := func(clientIP string) string {
		md := metadata.Pairs("x-real-ip", clientIP)
		ctx := metadata.NewIncomingContext(context.Background(), md)
		ctx = peer.NewContext(ctx, trustedPeer)
		return interceptors.ClientIPFromCtx(resolveViaInterceptor(t, ctx))
	}

	ipA := resolve("198.51.100.11")
	ipB := resolve("198.51.100.22")
	require.NotEqual(t, ipA, ipB, "two distinct clients behind the same trusted peer must resolve to distinct IPs")

	lim := ratelimit.NewWithBurst(1, 0)
	assert.True(t, lim.Allow(ipA))
	assert.True(t, lim.Allow(ipB), "a second distinct client must not be blocked by the first client's exhausted bucket")
	assert.False(t, lim.Allow(ipA), "the first client's own bucket should now be exhausted")
}

// The resolution reasoning must be reachable for callers explaining a
// rate-limit decision, not just the winning IP.
func TestClientInfoUnary_ClientIPResolutionFromCtx(t *testing.T) {
	md := metadata.Pairs("x-forwarded-for", "198.51.100.5")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 443}})

	captured := resolveViaInterceptor(t, ctx)
	res, ok := interceptors.ClientIPResolutionFromCtx(captured)
	require.True(t, ok)
	assert.Equal(t, "198.51.100.5", res.IP)
	assert.Equal(t, "10.0.0.5", res.PeerIP)
	assert.True(t, res.PeerTrusted)
	assert.True(t, res.XFFHonored)
	assert.Equal(t, "198.51.100.5", res.XFF)
}

func TestClientIPResolutionFromCtx_Absent(t *testing.T) {
	_, ok := interceptors.ClientIPResolutionFromCtx(context.Background())
	assert.False(t, ok)
}

// The misconfiguration this warns about (2026-07-29): the peer is loopback
// but EVEY_TRUSTED_PROXIES was narrowed to a set that no longer covers it, so
// its X-Forwarded-For is ignored and every remote client resolves to the SAME
// 127.0.0.1 bucket.
func TestClientInfoUnary_WarnsOnLoopbackDespiteForwardedFor(t *testing.T) {
	var buf bytes.Buffer
	ratelimit.SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { ratelimit.SetLogger(nil) })
	ratelimit.ResetMisconfigWarnForTest()
	t.Cleanup(ratelimit.ResetMisconfigWarnForTest)

	ratelimit.SetTrustedProxies([]string{"203.0.113.0/24"})
	t.Cleanup(func() { ratelimit.SetTrustedProxies(defaultTrustedProxies) })

	md := metadata.Pairs("x-forwarded-for", "198.51.100.5")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}})

	resolveViaInterceptor(t, ctx)

	require.Contains(t, buf.String(), "EVEY_TRUSTED_PROXIES", "a loopback client with an ignored X-Forwarded-For must warn about the trusted-proxy config")
}

// Correctly configured (peer covered by the trusted set): no warning.
func TestClientInfoUnary_SilentWhenTrustedProxyCoversPeer(t *testing.T) {
	var buf bytes.Buffer
	ratelimit.SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { ratelimit.SetLogger(nil) })
	ratelimit.ResetMisconfigWarnForTest()
	t.Cleanup(ratelimit.ResetMisconfigWarnForTest)

	md := metadata.Pairs("x-forwarded-for", "198.51.100.5")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}) // loopback IS trusted by default

	resolveViaInterceptor(t, ctx)

	if strings.Contains(buf.String(), "EVEY_TRUSTED_PROXIES") {
		t.Fatalf("correctly configured trusted proxy must not warn: %s", buf.String())
	}
}

// The untrusted-peer anti-spoof guard (TestClientInfoUnary_SpoofedXFFFromUntrustedPeerIgnored,
// above) must not trip this warning either: a public peer is not loopback.
func TestClientInfoUnary_SpoofedXFFFromUntrustedPeerDoesNotWarn(t *testing.T) {
	var buf bytes.Buffer
	ratelimit.SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { ratelimit.SetLogger(nil) })
	ratelimit.ResetMisconfigWarnForTest()
	t.Cleanup(ratelimit.ResetMisconfigWarnForTest)

	md := metadata.Pairs("x-forwarded-for", "1.2.3.4")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 40000}})

	resolveViaInterceptor(t, ctx)

	if buf.Len() != 0 {
		t.Fatalf("an untrusted non-loopback peer must not trigger the misconfig warning: %s", buf.String())
	}
}

// A peer address that is PRESENT but does not parse as an IP must not be
// trusted. It used to be, because the code tested `net.ParseIP(peerIP) == nil`
// to mean "in-process transport" - which is also true for any non-IP address.
// Trusting it hands the caller the per-IP rate limiters via a forged header.
func TestResolveClientIP_UnparseablePeerAddressIsNotTrusted(t *testing.T) {
	ratelimit.SetTrustedProxies([]string{"10.0.0.0/8"})
	t.Cleanup(func() { ratelimit.SetTrustedProxies(defaultTrustedProxies) })

	md := metadata.Pairs("x-forwarded-for", "203.0.113.9")
	ctx := peer.NewContext(metadata.NewIncomingContext(context.Background(), md),
		&peer.Peer{Addr: fakeAddr{network: "weird", addr: "not-an-ip"}})

	var got context.Context
	_, _ = interceptors.ClientInfoUnary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: testMethod},
		func(c context.Context, _ any) (any, error) { got = c; return nil, nil })

	res, ok := interceptors.ClientIPResolutionFromCtx(got)
	require.True(t, ok)
	assert.False(t, res.PeerTrusted, "a present-but-unparseable peer must not be trusted")
	assert.False(t, res.XFFHonored, "its forwarded header must be ignored")
	assert.Equal(t, "not-an-ip", res.IP, "it must be keyed on the peer, not on the header")
}

// A unix socket has no remote to distrust - it stays local and trusted, so the
// fix above does not break an in-process or socket-based transport.
func TestResolveClientIP_UnixSocketPeerStaysTrusted(t *testing.T) {
	ratelimit.SetTrustedProxies([]string{"10.0.0.0/8"})
	t.Cleanup(func() { ratelimit.SetTrustedProxies(defaultTrustedProxies) })

	md := metadata.Pairs("x-forwarded-for", "203.0.113.9")
	ctx := peer.NewContext(metadata.NewIncomingContext(context.Background(), md),
		&peer.Peer{Addr: fakeAddr{network: "unix", addr: "/run/evey.sock"}})

	var got context.Context
	_, _ = interceptors.ClientInfoUnary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: testMethod},
		func(c context.Context, _ any) (any, error) { got = c; return nil, nil })

	res, ok := interceptors.ClientIPResolutionFromCtx(got)
	require.True(t, ok)
	assert.True(t, res.PeerTrusted)
	assert.Equal(t, "203.0.113.9", res.IP, "a local socket may forward a real client address")
}

type fakeAddr struct{ network, addr string }

func (f fakeAddr) Network() string { return f.network }
func (f fakeAddr) String() string  { return f.addr }

// The trusted hop appends its own x-real-ip / x-real-user-agent AFTER anything
// the client smuggled in, so the last value is the one the proxy vouches for.
func TestClientInfoUnary_TrustedHopValueWinsOverSmuggledOne(t *testing.T) {
	md := metadata.MD{
		"x-real-ip":         {"1.2.3.4", "203.0.113.9"},
		"x-real-user-agent": {"forged-agent", "real-agent"},
	}
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 40000}})
	got := resolveViaInterceptor(t, ctx)
	assert.Equal(t, "203.0.113.9", interceptors.ClientIPFromCtx(got))
	assert.Equal(t, "real-agent", interceptors.UserAgentFromCtx(got))
}

// Login's per-IP lockout reads the IP this interceptor resolved, not the raw
// metadata: a forged x-real-ip from an untrusted peer must not reach it.
func TestClientInfoUnary_HandsResolvedIPToLogin(t *testing.T) {
	md := metadata.Pairs("x-real-ip", "1.2.3.4")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 40000}})
	assert.Equal(t, "203.0.113.7", appuser.LoginClientIP(resolveViaInterceptor(t, ctx)))
}
