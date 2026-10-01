package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"google.golang.org/grpc"
)

type clientInfoProbe struct {
	eveyv1.UnimplementedUserServiceServer
	ip, ua string
}

func (p *clientInfoProbe) GetCurrentUser(ctx context.Context, _ *eveyv1.GetCurrentUserRequest) (*eveyv1.User, error) {
	p.ip, p.ua = interceptors.ClientIPFromCtx(ctx), interceptors.UserAgentFromCtx(ctx)
	return &eveyv1.User{}, nil
}

// A browser can send Grpc-Metadata-<key>; for keys the gateway itself stamps
// (client IP, user agent) the forged value used to arrive first and win.
func TestGatewayIgnoresForgedInternalMetadata(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	probe := &clientInfoProbe{}
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptors.ClientInfoUnary()))
	eveyv1.RegisterUserServiceServer(srv, probe)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	mux, conn := registerGateway(lis.Addr().String())
	if mux == nil {
		t.Fatal("gateway not built")
	}
	defer conn.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("User-Agent", "real-agent")
	req.Header.Set("Grpc-Metadata-X-Real-Ip", "1.2.3.4")
	req.Header.Set("Grpc-Metadata-X-Forwarded-For", "1.2.3.4")
	req.Header.Set("Grpc-Metadata-X-Real-User-Agent", "forged-agent")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if probe.ip != "203.0.113.9" {
		t.Errorf("client IP seen by gRPC = %q, want 203.0.113.9", probe.ip)
	}
	if probe.ua != "real-agent" {
		t.Errorf("user agent seen by gRPC = %q, want real-agent", probe.ua)
	}
}

func TestGatewayHeaderMatcherRefusesInternalKeys(t *testing.T) {
	for _, h := range []string{
		"Grpc-Metadata-X-Real-Ip", "Grpc-Metadata-X-Forwarded-For", "Grpc-Metadata-X-Forwarded-Host",
		"Grpc-Metadata-X-Real-User-Agent", "Grpc-Metadata-X-Request-Id", "Grpc-Metadata-Grpcgateway-User-Agent",
	} {
		if k, ok := gatewayHeaderMatcher(h); ok {
			t.Errorf("%s forwarded as %q, want refused", h, k)
		}
	}
	for h, want := range map[string]string{
		"Accept-Language":             "accept-language",
		"X-Locale":                    "x-locale",
		"Grpc-Metadata-X-Device-Name": "x-device-name",
		"User-Agent":                  "grpcgateway-user-agent",
	} {
		if k, ok := gatewayHeaderMatcher(h); !ok || k != want {
			t.Errorf("%s -> %q,%v want %q", h, k, ok, want)
		}
	}
}
