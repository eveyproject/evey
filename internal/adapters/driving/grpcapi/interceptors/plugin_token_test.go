package interceptors

import (
	"context"
	"errors"
	"testing"
	"time"

	authpkg "github.com/eveyproject/evey/internal/pkg/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeParser struct {
	claims *authpkg.PluginClaims
	err    error
}

func (f *fakeParser) ParsePluginToken(raw string) (*authpkg.PluginClaims, error) {
	return f.claims, f.err
}

func TestPluginTokenInterceptor_ValidToken_PopulatesCtx(t *testing.T) {
	claims := &authpkg.PluginClaims{UserID: "u1", PluginID: "plg1", Scopes: []string{"media:read"}, TokenType: "plugin", ExpiresAt: time.Now().Add(time.Minute)}
	i := NewPluginTokenInterceptor(&fakeParser{claims: claims})
	md := metadata.New(map[string]string{"authorization": "Bearer abc.def.ghi"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var gotCtx context.Context
	_, err := i.Unary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/x/y"}, func(c context.Context, _ any) (any, error) {
		gotCtx = c
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := PluginClaimsFromContext(gotCtx)
	if got == nil || got.PluginID != "plg1" {
		t.Fatalf("claims not populated: %+v", got)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "media:read" {
		t.Errorf("scopes = %v", got.Scopes)
	}
}

func TestPluginTokenInterceptor_InvalidToken_Passthrough(t *testing.T) {
	i := NewPluginTokenInterceptor(&fakeParser{err: errors.New("not a plugin token")})
	md := metadata.New(map[string]string{"authorization": "Bearer user-jwt"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var gotCtx context.Context
	_, err := i.Unary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/x/y"}, func(c context.Context, _ any) (any, error) {
		gotCtx = c
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if PluginClaimsFromContext(gotCtx) != nil {
		t.Error("plugin claims should not be set")
	}
}

func TestPluginTokenInterceptor_NoAuthHeader_Passthrough(t *testing.T) {
	i := NewPluginTokenInterceptor(&fakeParser{})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(nil))
	_, err := i.Unary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/x/y"}, func(c context.Context, _ any) (any, error) {
		if PluginClaimsFromContext(c) != nil {
			t.Error("no claims expected")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestPluginTokenInterceptor_APIKeySkipped(t *testing.T) {
	// Parser would be called for "evey_..." without this short-circuit; ensure we skip.
	parser := &fakeParser{err: errors.New("should not be called")}
	i := NewPluginTokenInterceptor(parser)
	md := metadata.New(map[string]string{"authorization": "evey_apikey123"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := i.Unary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/x/y"}, func(c context.Context, _ any) (any, error) {
		if PluginClaimsFromContext(c) != nil {
			t.Error("no claims expected for api key")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

type streamStub struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *streamStub) Context() context.Context { return s.ctx }

// Stream interceptor injects claims into ss.Context() when a valid plugin token is present.
func TestPluginTokenInterceptor_Stream_ValidToken_PopulatesCtx(t *testing.T) {
	claims := &authpkg.PluginClaims{UserID: "u1", PluginID: "plg1", Scopes: []string{"media:read"}, TokenType: "plugin", ExpiresAt: time.Now().Add(time.Minute)}
	i := NewPluginTokenInterceptor(&fakeParser{claims: claims})
	md := metadata.New(map[string]string{"authorization": "Bearer abc.def.ghi"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var seen *authpkg.PluginClaims
	handler := func(_ any, ss grpc.ServerStream) error {
		if v, ok := ss.Context().Value(pluginClaimsKey{}).(*authpkg.PluginClaims); ok {
			seen = v
		}
		return nil
	}
	err := i.Stream()(nil, &streamStub{ctx: ctx}, nil, handler)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if seen == nil || seen.PluginID != "plg1" {
		t.Fatalf("stream claims not populated: %+v", seen)
	}
}

// Stream interceptor is a passthrough when no Authorization metadata is set.
func TestPluginTokenInterceptor_Stream_NoAuth_Passthrough(t *testing.T) {
	i := NewPluginTokenInterceptor(&fakeParser{err: errors.New("should not be called")})
	called := false
	handler := func(_ any, _ grpc.ServerStream) error { called = true; return nil }
	err := i.Stream()(nil, &streamStub{ctx: context.Background()}, nil, handler)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !called {
		t.Fatal("handler must be called on passthrough")
	}
}
