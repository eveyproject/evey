package interceptors

import (
	"context"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

type userAgentKey struct{}
type deviceNameKey struct{}
type clientIPKey struct{}
type clientIPResolutionKey struct{}
type persistentSessionKey struct{}

// ClientInfoUnary extracts user-agent, x-device-name, and client IP from gRPC metadata into context.
func ClientInfoUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = enrichClientInfo(ctx)
		return handler(ctx, req)
	}
}

func ClientInfoStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := enrichClientInfo(ss.Context())
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

func enrichClientInfo(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	// Prefer the REAL client user-agent the gateway annotator captured; the raw
	// "user-agent" metadata is grpc-go's own ("grpc-go/x.y.z") once a request
	// crosses the HTTP->gRPC gateway.
	if ua := firstMD(md, "x-real-user-agent", "grpcgateway-user-agent", "user-agent"); ua != "" {
		ctx = context.WithValue(ctx, userAgentKey{}, ua)
	}
	if vals := md.Get("x-device-name"); len(vals) > 0 {
		ctx = context.WithValue(ctx, deviceNameKey{}, vals[0])
	}
	if vals := md.Get("x-persistent-session"); len(vals) > 0 && strings.EqualFold(vals[0], "true") {
		ctx = context.WithValue(ctx, persistentSessionKey{}, true)
	}
	resolved := resolveClientIPReasoned(ctx, md)
	ctx = context.WithValue(ctx, clientIPKey{}, resolved.IP)
	ctx = context.WithValue(ctx, clientIPResolutionKey{}, resolved)
	return ctx
}

// firstMD returns the first non-empty value across the given metadata keys.
func firstMD(md metadata.MD, keys ...string) string {
	for _, k := range keys {
		if vals := md.Get(k); len(vals) > 0 && vals[0] != "" {
			return vals[0]
		}
	}
	return ""
}

// resolveClientIPReasoned makes the same trust decision as the HTTP layer's
// clientIP(), but returns the reasoning behind it too (ratelimit.ProxyResolution)
// rather than just the winning IP - so a caller trying to explain a rate-limit
// decision (or WarnMisroutedProxy, below) doesn't have to re-derive it.
func resolveClientIPReasoned(ctx context.Context, md metadata.MD) ratelimit.ProxyResolution {
	// The immediate peer decides trust: proxy-supplied IP headers (x-real-ip,
	// x-forwarded-for) are only honored when the request actually arrived from a
	// trusted proxy (the reverse proxy, or the in-process HTTP->gRPC gateway).
	// Otherwise a client speaking gRPC directly could set x-forwarded-for to any
	// value and evade the per-IP rate limiters (login, password reset, quick
	// connect) - the exact bypass the HTTP layer's clientIP() already guards.
	peerIP := ""
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			peerIP = host
		} else {
			peerIP = p.Addr.String()
		}
	}
	// An unparseable peer address (nil) means an in-process transport (the
	// gateway/bufconn) - local and trusted. A parseable peer must be in the
	// trusted-proxy set for its forwarded headers to count.
	parsed := net.ParseIP(peerIP)
	res := ratelimit.ProxyResolution{
		IP:          peerIP,
		PeerIP:      peerIP,
		PeerTrusted: parsed == nil || ratelimit.IsTrustedProxy(parsed),
	}
	res.XFF = firstMD(md, "x-real-ip", "x-forwarded-for")
	if res.PeerTrusted && res.XFF != "" {
		if ip := strings.TrimSpace(strings.Split(res.XFF, ",")[0]); ip != "" {
			res.IP = ip
			res.XFFHonored = true
		}
	}
	ratelimit.WarnMisroutedProxy(res)
	return res
}

func UserAgentFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(userAgentKey{}).(string)
	return v
}

func DeviceNameFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(deviceNameKey{}).(string)
	return v
}

func ClientIPFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(clientIPKey{}).(string)
	return v
}

// ClientIPResolutionFromCtx returns the reasoning behind the resolved client
// IP - the peer address, whether it was trusted, and any forwarded-for value
// seen - for callers that need to explain a rate-limit decision, not just its
// outcome. See ratelimit.ProxyResolution.
func ClientIPResolutionFromCtx(ctx context.Context) (ratelimit.ProxyResolution, bool) {
	v, ok := ctx.Value(clientIPResolutionKey{}).(ratelimit.ProxyResolution)
	return v, ok
}

// WithClientIP stamps the resolved client IP onto ctx. The client-info
// interceptor sets it in production; exported so handlers/tests that build a
// context without going through the interceptor can supply it symmetrically
// with ClientIPFromCtx.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// WithUserAgent stamps a user agent onto ctx - same test/handler symmetry as
// WithClientIP above.
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, userAgentKey{}, ua)
}

func PersistentSessionFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(persistentSessionKey{}).(bool)
	return v
}
