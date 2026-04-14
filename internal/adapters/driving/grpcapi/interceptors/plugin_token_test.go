package interceptors

import (
	"context"
	"errors"
	"testing"
	"time"

	authpkg "github.com/evey-project/evey/internal/pkg/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
	scopes, ok := ScopeFromCtx(gotCtx)
	if !ok || len(scopes) != 1 || scopes[0] != "media:read" {
		t.Errorf("scopes = %v ok=%v", scopes, ok)
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

func TestRequireScope_NoClaims_OK(t *testing.T) {
	if err := RequireScope(context.Background(), "media:read"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestRequireScope_HasScope_OK(t *testing.T) {
	ctx := context.WithValue(context.Background(), pluginClaimsKey{}, &authpkg.PluginClaims{Scopes: []string{"media:read"}})
	if err := RequireScope(ctx, "media:read"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestRequireScope_MissingScope_PermissionDenied(t *testing.T) {
	ctx := context.WithValue(context.Background(), pluginClaimsKey{}, &authpkg.PluginClaims{Scopes: []string{"media:read"}})
	err := RequireScope(ctx, "media:write")
	if code := status.Code(err); code != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", code)
	}
}
