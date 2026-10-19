package loadtest

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
)

// localproxy.go - reaching the reverse proxy the way an outside viewer's
// bytes do.
//
// The run reaches the server through its public address, the way viewers
// come. From inside the network that address is the router's: every byte goes
// up to the router, which turns it round to the proxy and back (prod
// 2026-10-19: 611 Mb/s through the router against 2603 Mb/s to the same proxy
// at its local address, the same file on 16 streams). No outside viewer takes
// that turn, so the run measured the router's loopback and called it the
// proxy. When the proxy is on the local network, the run now dials it at its
// local address, under the public name: the same proxy, the same TLS, the
// same certificate, without the turn. A proxy outside (a rented server, a
// tunnel) is on real viewers' path and is kept.

// LoadTestPeerPath is the admin route that answers with the address the
// request came from, as the server saw it: the proxy's, when it came through
// one.
const LoadTestPeerPath = "/api/v1/admin/load-test/peer"

// PeerHandler answers LoadTestPeerPath: the immediate peer of the request,
// never a forwarded header - the proxy, not the client behind it.
func PeerHandler(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"peer": host})
}

// DialRedirect sends the connections the run opens to one address (the
// public host:port) to another (the proxy's local address). Its zero value
// redirects nothing. The run sets it once the local address has answered, and
// clears it at the end.
type DialRedirect struct {
	mu       sync.RWMutex
	from, to string
}

// Wrap returns dial with the redirect applied.
func (d *DialRedirect) Wrap(dial func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		d.mu.RLock()
		if d.from != "" && addr == d.from {
			addr = d.to
		}
		d.mu.RUnlock()
		return dial(ctx, network, addr)
	}
}

func (d *DialRedirect) set(from, to string) {
	d.mu.Lock()
	d.from, d.to = from, to
	d.mu.Unlock()
}

// hostPort is the public URL's host:port and its port alone, the port its
// scheme's default when the URL names none.
func hostPort(publicURL string) (hostport, port string, ok bool) {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return "", "", false
	}
	port = u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port), port, true
}

// peerOf asks the server, through the public address, who handed it the
// request. ok is false when the server did not answer the question: nothing
// listening, or something else than this server.
func (r *Runner) peerOf(ctx context.Context, publicURL, token string) (netip.Addr, bool) {
	probeCtx, cancel := context.WithTimeout(ctx, endpointProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(publicURL, "/")+LoadTestPeerPath, nil)
	if err != nil {
		return netip.Addr{}, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return netip.Addr{}, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return netip.Addr{}, false
	}
	var body struct {
		Peer string `json:"peer"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<10)).Decode(&body) != nil {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(body.Peer)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// onLocalNetwork reports whether addr is a private, loopback or link-local
// address: a proxy there sits on the same network as the server.
func onLocalNetwork(addr netip.Addr) bool {
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast()
}

// skipRouterLoop points the run's connections to the public address at the
// proxy's local address when the proxy is on the local network and answers
// there, and returns that address ("" when the public path is kept).
func (r *Runner) skipRouterLoop(ctx context.Context, publicURL, token string) string {
	if r.dialRedirect == nil {
		return ""
	}
	from, port, ok := hostPort(publicURL)
	if !ok {
		return ""
	}
	peer, ok := r.peerOf(ctx, publicURL, token)
	if !ok || !onLocalNetwork(peer) {
		return ""
	}
	to := net.JoinHostPort(peer.String(), port)
	r.dialRedirect.set(from, to)
	r.httpClient.CloseIdleConnections()
	// Something answering at that address is not enough: it must be this
	// server behind it, still seeing the proxy hand it the request.
	if again, ok := r.peerOf(ctx, publicURL, token); !ok || again != peer {
		r.dialRedirect.set("", "")
		r.httpClient.CloseIdleConnections()
		r.logger.Info("loadtest: the proxy does not answer at its local address; the viewers go through the public address", "proxy", to)
		return ""
	}
	r.logger.Info("loadtest: the proxy is on the local network; the viewers reach it at its local address, without the router's turn", "proxy", to)
	return to
}
