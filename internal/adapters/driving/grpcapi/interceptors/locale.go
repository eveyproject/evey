package interceptors

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/eveyproject/evey/internal/pkg/i18n"
)

func LocaleUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = enrichLocale(ctx)
		return handler(ctx, req)
	}
}

func LocaleStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := enrichLocale(ss.Context())
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

func enrichLocale(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	// Priority: explicit x-locale header > JWT locale claim > Accept-Language header
	if vals := md.Get("x-locale"); len(vals) > 0 {
		return i18n.ContextWithLocale(ctx, i18n.ParseLocale(vals[0]))
	}
	if vals := md.Get("grpcgateway-x-locale"); len(vals) > 0 {
		return i18n.ContextWithLocale(ctx, i18n.ParseLocale(vals[0]))
	}
	if vals := md.Get("locale"); len(vals) > 0 {
		return i18n.ContextWithLocale(ctx, i18n.ParseLocale(vals[0]))
	}
	if vals := md.Get("accept-language"); len(vals) > 0 {
		return i18n.ContextWithLocale(ctx, i18n.ParseLocale(vals[0]))
	}
	if vals := md.Get("grpcgateway-accept-language"); len(vals) > 0 {
		return i18n.ContextWithLocale(ctx, i18n.ParseLocale(vals[0]))
	}
	return ctx
}
