package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The probe counts what the line actually sent, cut-off streams included, and
// a service that refuses is not a speed.
func TestMeasureUplink(t *testing.T) {
	var received atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		received.Add(n)
	}))
	defer srv.Close()
	up := measureUplink(context.Background(), srv.Client(), srv.URL+"/__up", 300*time.Millisecond)
	if !up.Measured || up.BytesPerSec <= 0 {
		t.Fatalf("got %+v, want a measured rate", up)
	}
	if !strings.HasPrefix(up.Service, "127.0.0.1:") {
		t.Fatalf("service = %q, want the probe's host", up.Service)
	}
	if got := float64(up.BytesPerSec) * 0.3; got < float64(received.Load())/2 {
		t.Fatalf("rate %d B/s over 0.3 s is far below the %d bytes received", up.BytesPerSec, received.Load())
	}

	refuse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer refuse.Close()
	if up := measureUplink(context.Background(), refuse.Client(), refuse.URL, 300*time.Millisecond); up.Measured {
		t.Fatalf("a refusing service gave %+v, want unmeasured", up)
	}
	if up := measureUplink(context.Background(), http.DefaultClient, "http://127.0.0.1:1/__up", 200*time.Millisecond); up.Measured {
		t.Fatalf("nothing listening gave %+v, want unmeasured", up)
	}
}

// The audience outside is the server's or the line's, whichever is smaller,
// with the line kept a fifth short of full.
func TestRemoteAudience(t *testing.T) {
	line := func(mbps int64) Uplink { return Uplink{BytesPerSec: mbps * 1_000_000 / 8, Measured: true} }
	// 93 viewers pulling 500 Mb/s: 5.38 Mb/s each.
	served := int64(500_000_000 / 8)
	for _, tc := range []struct {
		name      string
		sustained int
		up        Uplink
		want      int
		byLine    bool
	}{
		{"a 700 Mb/s line carries 104 of them", 93, line(700), 93, false},
		{"a 300 Mb/s line carries 44", 93, line(300), 44, true},
		{"line not measured", 93, Uplink{}, 0, false},
		{"no audience held", 0, line(700), 0, false},
	} {
		got, byLine := remoteAudience(tc.sustained, served, tc.up)
		if got != tc.want || byLine != tc.byLine {
			t.Errorf("%s: got %d byLine=%v, want %d byLine=%v", tc.name, got, byLine, tc.want, tc.byLine)
		}
	}
}

// PeerHandler names the immediate peer, never a forwarded header.
func TestPeerHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, LoadTestPeerPath, nil)
	req.RemoteAddr = "192.168.1.201:51234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	rec := httptest.NewRecorder()
	PeerHandler(rec, req)
	var body struct{ Peer string }
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Peer != "192.168.1.201" {
		t.Fatalf("peer = %q (err %v), want 192.168.1.201", body.Peer, err)
	}
}

// peerServer answers the peer route with peer, and counts its requests.
func peerServer(t *testing.T, peer string, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		p := peer
		if p == "" {
			p, _, _ = net.SplitHostPort(r.RemoteAddr)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"peer": p})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// routerRunner is a runner whose public name, public.test, is dialed at the
// router's address until the run redirects it.
func routerRunner(router string) (*Runner, *DialRedirect) {
	redirect := &DialRedirect{}
	base := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if strings.HasPrefix(addr, "public.test:") {
			addr = router
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	client := &http.Client{Transport: &http.Transport{DialContext: redirect.Wrap(base)}}
	return New(client, nil, func() int { return 0 }, testLogger()).WithDialRedirect(redirect), redirect
}

// A proxy on the local network that answers at its local address is dialed
// there: the router's turn is skipped, the same proxy serves.
func TestSkipRouterLoop_LocalProxy(t *testing.T) {
	var routerHits, proxyHits atomic.Int64
	proxy := peerServer(t, "", &proxyHits)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(proxy.URL, "http://"))
	router := peerServer(t, "127.0.0.1", &routerHits)
	r, _ := routerRunner(strings.TrimPrefix(router.URL, "http://"))
	public := "http://public.test:" + port

	got := r.skipRouterLoop(context.Background(), public, "tok")
	if got != "127.0.0.1:"+port {
		t.Fatalf("proxy address = %q, want 127.0.0.1:%s", got, port)
	}
	before := proxyHits.Load()
	resp, err := r.httpClient.Get(public + LoadTestPeerPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if proxyHits.Load() != before+1 {
		t.Fatal("a request to the public name after the switch did not reach the proxy at its local address")
	}
}

// A proxy outside (a rented server, a tunnel) is on real viewers' path: the
// public address is kept.
func TestSkipRouterLoop_ProxyOutside(t *testing.T) {
	var routerHits atomic.Int64
	router := peerServer(t, "203.0.113.5", &routerHits)
	r, redirect := routerRunner(strings.TrimPrefix(router.URL, "http://"))
	if got := r.skipRouterLoop(context.Background(), "http://public.test:8080", "tok"); got != "" {
		t.Fatalf("proxy address = %q, want the public path kept", got)
	}
	if redirect.from != "" {
		t.Fatal("a redirect was left set for a proxy outside")
	}
}

// A local peer that does not answer at its local address keeps the public
// path, and the redirect is cleared.
func TestSkipRouterLoop_LocalAddressSilent(t *testing.T) {
	var routerHits atomic.Int64
	router := peerServer(t, "127.0.0.1", &routerHits)
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	_, port, _ := net.SplitHostPort(closed.Addr().String())
	_ = closed.Close()
	r, redirect := routerRunner(strings.TrimPrefix(router.URL, "http://"))
	if got := r.skipRouterLoop(context.Background(), fmt.Sprintf("http://public.test:%s", port), "tok"); got != "" {
		t.Fatalf("proxy address = %q, want the public path kept", got)
	}
	if redirect.from != "" {
		t.Fatal("the redirect was left set after the local address stayed silent")
	}
	resp, err := r.httpClient.Get("http://public.test:" + port + LoadTestPeerPath)
	if err != nil {
		t.Fatalf("the public path no longer works after the fallback: %v", err)
	}
	_ = resp.Body.Close()
}
