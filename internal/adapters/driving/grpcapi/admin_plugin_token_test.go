package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stubIssuer struct {
	failWith error
}

func (s *stubIssuer) GeneratePluginToken(userID, pluginID, profileID string, scopes []string, ttl time.Duration) (string, time.Time, []string, error) {
	if s.failWith != nil {
		return "", time.Time{}, nil, s.failWith
	}
	return "tok-" + pluginID, time.Now().Add(ttl), scopes, nil
}

func newTokenHandler(t *testing.T, p *plugindomain.Plugin) (*adminPluginHandler, *pluginfakes.FakePluginRepository) {
	t.Helper()
	repo := pluginfakes.NewFakePluginRepository()
	if p != nil {
		if err := repo.Create(context.Background(), p); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	tokenSvc := appplugin.NewTokenService(repo, &stubIssuer{})
	return &adminPluginHandler{tokenSvc: tokenSvc, enabled: true}, repo
}

func ctxWithUser(userID string) context.Context {
	return interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: userID})
}

func TestIssuePluginToken_HappyPath(t *testing.T) {
	h, _ := newTokenHandler(t, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true,
		Permissions: []string{"media.read"},
	})
	resp, err := h.IssuePluginToken(ctxWithUser("user-1"), &eveyv1.IssuePluginTokenRequest{PluginId: "plg1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Token == "" {
		t.Error("empty token")
	}
	if len(resp.Scopes) != 1 || resp.Scopes[0] != "media.read" {
		t.Errorf("scopes = %v", resp.Scopes)
	}
	if resp.ExpiresAt == nil {
		t.Error("nil ExpiresAt")
	}
}

func TestIssuePluginToken_PluginNotFound(t *testing.T) {
	h, _ := newTokenHandler(t, nil)
	_, err := h.IssuePluginToken(ctxWithUser("u"), &eveyv1.IssuePluginTokenRequest{PluginId: "missing"})
	if code := status.Code(err); code != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", code)
	}
}

func TestIssuePluginToken_PluginDisabled(t *testing.T) {
	h, _ := newTokenHandler(t, &plugindomain.Plugin{
		ID: "plgD", Name: "d", Enabled: false,
		Permissions: []string{"media.read"},
	})
	_, err := h.IssuePluginToken(ctxWithUser("u"), &eveyv1.IssuePluginTokenRequest{PluginId: "plgD"})
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", code)
	}
}

func TestIssuePluginToken_Unauthenticated(t *testing.T) {
	h, _ := newTokenHandler(t, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true, Permissions: []string{"media.read"},
	})
	_, err := h.IssuePluginToken(context.Background(), &eveyv1.IssuePluginTokenRequest{PluginId: "plg1"})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", code)
	}
}

func TestIssuePluginToken_MissingPluginID(t *testing.T) {
	h, _ := newTokenHandler(t, nil)
	_, err := h.IssuePluginToken(ctxWithUser("u"), &eveyv1.IssuePluginTokenRequest{PluginId: ""})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
}

func TestIssuePluginToken_IssuerError(t *testing.T) {
	repo := pluginfakes.NewFakePluginRepository()
	if err := repo.Create(context.Background(), &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true, Permissions: []string{"media.read"},
	}); err != nil {
		t.Fatal(err)
	}
	tokenSvc := appplugin.NewTokenService(repo, &stubIssuer{failWith: errors.New("signer down")})
	h := &adminPluginHandler{tokenSvc: tokenSvc, enabled: true}
	_, err := h.IssuePluginToken(ctxWithUser("u"), &eveyv1.IssuePluginTokenRequest{PluginId: "plg1"})
	if code := status.Code(err); code != codes.Internal {
		t.Fatalf("code = %v, want Internal", code)
	}
}
