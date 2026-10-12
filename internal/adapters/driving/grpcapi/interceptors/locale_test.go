package interceptors_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestLocaleUnary_XLocaleHeader(t *testing.T) {
	interceptor := interceptors.LocaleUnary()

	md := metadata.Pairs("x-locale", "fr")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "fr", i18n.LocaleFromCtx(captured))
}

func TestLocaleUnary_AcceptLanguageHeader(t *testing.T) {
	interceptor := interceptors.LocaleUnary()

	md := metadata.Pairs("accept-language", "fr-FR, en;q=0.9")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "fr", i18n.LocaleFromCtx(captured))
}

func TestLocaleUnary_NoLocaleHeader(t *testing.T) {
	interceptor := interceptors.LocaleUnary()

	md := metadata.Pairs("other-header", "val")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(ctx, nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, i18n.FallbackLocale, i18n.LocaleFromCtx(captured))
}

func TestLocaleUnary_NoMetadata(t *testing.T) {
	interceptor := interceptors.LocaleUnary()

	var captured context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		captured = ctx
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, i18n.FallbackLocale, i18n.LocaleFromCtx(captured))
}

func TestLocaleStream(t *testing.T) {
	interceptor := interceptors.LocaleStream()

	md := metadata.Pairs("x-locale", "fr")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	handler := func(srv any, stream grpc.ServerStream) error {
		captured = stream.Context()
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/test/Stream"}
	err := interceptor(nil, &fakeServerStream{ctx: ctx}, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "fr", i18n.LocaleFromCtx(captured))
}
