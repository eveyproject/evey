package loadtest

import (
	"context"
	"net/http"
)

// viewer_client.go - one connection per simulated viewer, as a real audience.
//
// The run's client keeps one pool of connections per host. Against a server
// that speaks HTTP/2 (the public proxy, Caddy) that pool multiplexes every
// viewer onto a single TCP/TLS connection once it is open (measured on the
// client wired in cmd/evey: 49 concurrent requests, no new connection). The
// 2026-10-17 run through the proxy then plateaued near 250-290 Mb/s at every
// audience size with nothing on the server busy: it measured one connection,
// not a proxy serving one connection per device. Each viewer now dials its
// own.

type viewerClientKey struct{}

// withViewerClient gives the viewer running under ctx a client of its own: the
// run's client with a transport cloned from the run's, so its own connection
// pool. release closes that pool's connections when the viewer leaves.
func (r *Runner) withViewerClient(ctx context.Context) (out context.Context, release func()) {
	base, ok := r.httpClient.Transport.(*http.Transport)
	if !ok || base == nil {
		if r.httpClient.Transport != nil {
			// A test's own RoundTripper: nothing to clone, share it.
			return ctx, func() {}
		}
		base = http.DefaultTransport.(*http.Transport)
	}
	tr := base.Clone()
	c := *r.httpClient
	c.Transport = tr
	return context.WithValue(ctx, viewerClientKey{}, &c), tr.CloseIdleConnections
}

// clientFor is the client of the viewer running under ctx, or the run's own
// for a request no viewer makes (the eligibility check, the stats read).
func (r *Runner) clientFor(ctx context.Context) *http.Client {
	if c, ok := ctx.Value(viewerClientKey{}).(*http.Client); ok {
		return c
	}
	return r.httpClient
}
