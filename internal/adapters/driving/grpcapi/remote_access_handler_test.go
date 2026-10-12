package grpcapi

import (
	"context"
	"errors"
	"sync"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/config"
	"google.golang.org/grpc/codes"
)

type fakeConfigStore struct {
	mu      sync.Mutex
	data    map[string]string
	setErr  error
	getErr  error
	setCall int
}

func newFakeConfigStore() *fakeConfigStore {
	return &fakeConfigStore{data: map[string]string{}}
}

func (f *fakeConfigStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.data[key], nil
}

func (f *fakeConfigStore) Set(_ context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCall++
	if f.setErr != nil {
		return f.setErr
	}
	f.data[key] = value
	return nil
}

func newTestRAHandler() (*remoteAccessHandler, *config.Config, *fakeConfigStore) {
	cfg := &config.Config{
		Server:       config.ServerConfig{BaseURL: "https://evey.example.com"},
		RemoteAccess: config.RemoteAccessConfig{Enabled: false, ExternalURL: "", MaxRemoteBitrateKbps: 0},
	}
	store := newFakeConfigStore()
	return &remoteAccessHandler{cfg: cfg, store: store}, cfg, store
}

func TestGetRemoteAccess_ReturnsCurrentConfigWithBaseURL(t *testing.T) {
	t.Parallel()
	h, cfg, _ := newTestRAHandler()
	cfg.RemoteAccess.Enabled = true
	cfg.RemoteAccess.ExternalURL = "https://public.example.com"
	cfg.RemoteAccess.LANNetworks = []string{"192.168.1.0/24"}
	cfg.RemoteAccess.MaxRemoteBitrateKbps = 8000

	resp, err := h.GetRemoteAccess(context.Background(), &eveyv1.GetRemoteAccessRequest{})
	if err != nil {
		t.Fatalf("GetRemoteAccess: %v", err)
	}
	if resp.ServerBaseUrl != "https://evey.example.com" {
		t.Errorf("ServerBaseUrl = %q", resp.ServerBaseUrl)
	}
	if !resp.Enabled || resp.ExternalUrl != "https://public.example.com" || resp.MaxRemoteBitrateKbps != 8000 {
		t.Errorf("unexpected config: %+v", resp)
	}
}

func TestUpdateRemoteAccess_ValidHTTPSURL(t *testing.T) {
	t.Parallel()
	h, cfg, store := newTestRAHandler()
	url := "https://public.example.com"
	enabled := true
	bitrate := int32(5000)

	resp, err := h.UpdateRemoteAccess(ctxWithClaims("admin-1", "p1", true), &eveyv1.UpdateRemoteAccessRequest{
		Enabled:              &enabled,
		ExternalUrl:          &url,
		LanNetworks:          []string{"10.0.0.0/8"},
		MaxRemoteBitrateKbps: &bitrate,
	})
	if err != nil {
		t.Fatalf("UpdateRemoteAccess: %v", err)
	}
	if resp.ExternalUrl != url || !resp.Enabled || resp.MaxRemoteBitrateKbps != 5000 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if cfg.RemoteAccess.ExternalURL != url {
		t.Errorf("config not mutated: %q", cfg.RemoteAccess.ExternalURL)
	}
	if store.setCall != 1 {
		t.Errorf("expected persist Set call, got %d", store.setCall)
	}
	if _, ok := store.data[remoteAccessCfgKey]; !ok {
		t.Error("expected remote_access key persisted")
	}
}

func TestUpdateRemoteAccess_RejectsNonHTTPS(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestRAHandler()

	cases := []struct{ name, url string }{
		{"http", "http://public.example.com"},
		{"javascript", "javascript:alert(1)"},
		{"ftp", "ftp://example.com"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := h.UpdateRemoteAccess(ctxWithClaims("admin-1", "p1", true), &eveyv1.UpdateRemoteAccessRequest{
				ExternalUrl: &tc.url,
			})
			if got := codeOf(t, err); got != codes.InvalidArgument {
				t.Errorf("url=%q code=%v, want InvalidArgument", tc.url, got)
			}
		})
	}
}

func TestUpdateRemoteAccess_RejectsPrivateIPs(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestRAHandler()

	cases := []struct{ name, url string }{
		{"loopback", "https://127.0.0.1"},
		{"rfc1918-192", "https://192.168.1.10"},
		{"rfc1918-10", "https://10.0.0.1"},
		{"localhost", "https://localhost"},
		{"link-local", "https://169.254.1.1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := h.UpdateRemoteAccess(ctxWithClaims("admin-1", "p1", true), &eveyv1.UpdateRemoteAccessRequest{
				ExternalUrl: &tc.url,
			})
			if got := codeOf(t, err); got != codes.InvalidArgument {
				t.Errorf("url=%q code=%v, want InvalidArgument", tc.url, got)
			}
		})
	}
}

func TestUpdateRemoteAccess_EmptyHost(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestRAHandler()
	url := "https://"
	_, err := h.UpdateRemoteAccess(ctxWithClaims("admin-1", "p1", true), &eveyv1.UpdateRemoteAccessRequest{ExternalUrl: &url})
	if got := codeOf(t, err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestUpdateRemoteAccess_NilURL_NoValidation(t *testing.T) {
	t.Parallel()
	h, cfg, _ := newTestRAHandler()
	cfg.RemoteAccess.ExternalURL = "https://kept.example.com"
	enabled := false

	resp, err := h.UpdateRemoteAccess(ctxWithClaims("admin-1", "p1", true), &eveyv1.UpdateRemoteAccessRequest{Enabled: &enabled})
	if err != nil {
		t.Fatalf("UpdateRemoteAccess: %v", err)
	}
	if resp.ExternalUrl != "https://kept.example.com" {
		t.Errorf("ExternalUrl mutated unexpectedly: %q", resp.ExternalUrl)
	}
	if resp.Enabled {
		t.Error("Enabled should be false")
	}
}

func TestUpdateRemoteAccess_PersistErrorDoesNotFail(t *testing.T) {
	t.Parallel()
	h, _, store := newTestRAHandler()
	store.setErr = errors.New("boom")
	url := "https://public.example.com"

	_, err := h.UpdateRemoteAccess(ctxWithClaims("admin-1", "p1", true), &eveyv1.UpdateRemoteAccessRequest{ExternalUrl: &url})
	if err != nil {
		t.Fatalf("UpdateRemoteAccess should not surface persist errors: %v", err)
	}
}

func TestGetNetworkInfo_ReturnsInterfaces(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestRAHandler()
	resp, err := h.GetNetworkInfo(context.Background(), &eveyv1.GetNetworkInfoRequest{})
	if err != nil {
		t.Fatalf("GetNetworkInfo: %v", err)
	}
	// At minimum loopback exists on any sane system.
	if resp == nil {
		t.Fatal("nil response")
	}
}

func TestUpdateRemoteAccess_Unauthenticated(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestRAHandler()
	enabled := true
	_, err := h.UpdateRemoteAccess(context.Background(), &eveyv1.UpdateRemoteAccessRequest{Enabled: &enabled})
	if got := codeOf(t, err); got != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", got)
	}
}

func TestUpdateRemoteAccess_NonAdmin_PermissionDenied(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestRAHandler()
	enabled := true
	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := h.UpdateRemoteAccess(ctx, &eveyv1.UpdateRemoteAccessRequest{Enabled: &enabled})
	if got := codeOf(t, err); got != codes.PermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", got)
	}
}
