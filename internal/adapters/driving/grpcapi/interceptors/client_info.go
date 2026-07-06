package interceptors

import (
	"context"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type userAgentKey struct{}
type deviceNameKey struct{}
type clientIPKey struct{}
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
	ctx = context.WithValue(ctx, clientIPKey{}, resolveClientIP(ctx, md))
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

func resolveClientIP(ctx context.Context, md metadata.MD) string {
	// The gateway annotator resolves the real client IP (trusted-proxy aware);
	// prefer it over the raw x-forwarded-for chain / gateway peer.
	if ip := firstMD(md, "x-real-ip"); ip != "" {
		return ip
	}
	if vals := md.Get("x-forwarded-for"); len(vals) > 0 {
		parts := strings.Split(vals[0], ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		host, _, err := net.SplitHostPort(p.Addr.String())
		if err == nil {
			return host
		}
		return p.Addr.String()
	}
	return ""
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

func PersistentSessionFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(persistentSessionKey{}).(bool)
	return v
}
